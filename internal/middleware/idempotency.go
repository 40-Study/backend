package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const (
	// IdempotencyKeyHeader: header client gửi kèm để request không hoàn tác được (vd broadcast) an toàn khi gửi lại.
	IdempotencyKeyHeader = "Idempotency-Key"
	// IdempotencyReplayHeader: có mặt (= "true") trên phản hồi được phát lại từ kết quả đã lưu, không phải lần chạy thật.
	IdempotencyReplayHeader = "Idempotent-Replay"
	idempotencyMaxKeyLen    = 128
	idempotencyStatePending = "pending"
	idempotencyStateDone    = "done"
)

// IdempotencyConfig cấu hình middleware Idempotency.
type IdempotencyConfig struct {
	// Header: tên header chứa key (mặc định IdempotencyKeyHeader).
	Header string
	// KeyPrefix: tiền tố khoá Redis; khoá đầy đủ = prefix:scope:sha256(key).
	KeyPrefix string
	// TTL: thời gian giữ kết quả đã hoàn tất (vd 24h). PendingTTL: giữ khoá "đang chạy" phòng khi tiến trình chết.
	TTL        time.Duration
	PendingTTL time.Duration
	// Scope: ai sở hữu key (vd id quản trị viên). Rỗng = không xác định được người gọi => 401, không chạy handler.
	Scope func(c *fiber.Ctx) string
	// Took: chạy SAU handler; true khi request ĐÃ CÓ TÁC DỤNG và phải được phát lại (không được chạy lại). false =
	// chưa làm gì (lỗi validate, không có người nhận, lỗi trước khi giao) nên khoá được nhả để thử lại cùng key.
	Took func(c *fiber.Ctx) bool
}

type idempotencyRecord struct {
	State       string `json:"state"`
	BodyHash    string `json:"body_hash"`
	Status      int    `json:"status,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

func idempotencyFail(c *fiber.Ctx, status int, code, msg string) error {
	return c.Status(status).JSON(fiber.Map{"message": msg, "code": code})
}

// Idempotency làm request an toàn khi gửi lại (QA 261009, M2): client sinh một key cho MỖI lần quyết định gửi và gửi
// kèm header Idempotency-Key. Trong TTL, cùng key của cùng Scope trả LẠI nguyên kết quả đã lưu của lần đầu (kèm
// Idempotent-Replay: true) mà không chạy lại handler; đang chạy dở thì 409; cùng key khác nội dung thì 422. Không có
// header = hành vi cũ, không đụng Redis. Đặt SAU Auth/permission và TRƯỚC rate limiter/audit: bản phát lại không tiêu
// hạn mức và không ghi nhật ký lần hai (lần gốc đã ghi).
//
// Redis lỗi => 503 (fail-closed, giống RateLimiter): bỏ qua lặng lẽ sẽ làm đúng điều key này sinh ra để ngăn.
// Handler trả Go error => nhả khoá (kết quả không rõ, an toàn cho handler trả lỗi qua JSON như broadcast).
func Idempotency(rdb *redis.Client, cfg IdempotencyConfig) fiber.Handler {
	if cfg.Header == "" {
		cfg.Header = IdempotencyKeyHeader
	}
	return func(c *fiber.Ctx) error {
		raw := strings.TrimSpace(c.Get(cfg.Header))
		if raw == "" {
			return c.Next()
		}
		if len(raw) > idempotencyMaxKeyLen {
			return idempotencyFail(c, fiber.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID",
				fmt.Sprintf("Idempotency-Key tối đa %d ký tự.", idempotencyMaxKeyLen))
		}
		scope := cfg.Scope(c)
		if scope == "" {
			return idempotencyFail(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Không xác định được người gửi yêu cầu.")
		}
		keyHash := sha256.Sum256([]byte(raw))
		redisKey := fmt.Sprintf("%s:%s:%s", cfg.KeyPrefix, scope, hex.EncodeToString(keyHash[:]))
		bodySum := sha256.Sum256(c.Body())
		bodyHash := hex.EncodeToString(bodySum[:])
		ctx := c.Context()

		pending, _ := json.Marshal(idempotencyRecord{State: idempotencyStatePending, BodyHash: bodyHash})
		acquired, err := rdb.SetNX(ctx, redisKey, pending, cfg.PendingTTL).Result()
		if err != nil {
			log.Printf("[Idempotency] SetNX %s: %v", cfg.KeyPrefix, err)
			return idempotencyFail(c, fiber.StatusServiceUnavailable, "IDEMPOTENCY_UNAVAILABLE", "Dịch vụ kiểm tra trùng lặp tạm thời không khả dụng, vui lòng thử lại sau.")
		}
		if !acquired {
			return replayIdempotent(ctx, c, rdb, redisKey, bodyHash)
		}

		if err := c.Next(); err != nil {
			releaseIdempotencyKey(ctx, rdb, redisKey)
			return err
		}
		if !cfg.Took(c) {
			releaseIdempotencyKey(ctx, rdb, redisKey)
			return nil
		}
		done, _ := json.Marshal(idempotencyRecord{
			State: idempotencyStateDone, BodyHash: bodyHash, Status: c.Response().StatusCode(),
			ContentType: string(c.Response().Header.ContentType()), Body: append([]byte(nil), c.Response().Body()...),
		})
		if err := rdb.Set(ctx, redisKey, done, cfg.TTL).Err(); err != nil {
			// Hành động ĐÃ xảy ra; chỉ mất khả năng phát lại. Khoá pending hết hạn sau PendingTTL.
			log.Printf("[Idempotency] không lưu được kết quả %s: %v", cfg.KeyPrefix, err)
		}
		return nil
	}
}

func replayIdempotent(ctx context.Context, c *fiber.Ctx, rdb *redis.Client, redisKey, bodyHash string) error {
	stored, err := rdb.Get(ctx, redisKey).Bytes()
	if errors.Is(err, redis.Nil) {
		// Khoá vừa hết hạn/được nhả giữa SetNX và Get: yêu cầu người gọi thử lại thay vì đoán.
		return idempotencyFail(c, fiber.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "Yêu cầu trước đó vừa kết thúc, vui lòng thử lại.")
	}
	var rec idempotencyRecord
	if err != nil || json.Unmarshal(stored, &rec) != nil {
		log.Printf("[Idempotency] đọc bản ghi %s: %v", redisKey, err)
		return idempotencyFail(c, fiber.StatusServiceUnavailable, "IDEMPOTENCY_UNAVAILABLE", "Dịch vụ kiểm tra trùng lặp tạm thời không khả dụng, vui lòng thử lại sau.")
	}
	if rec.BodyHash != bodyHash {
		return idempotencyFail(c, fiber.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "Idempotency-Key này đã được dùng cho một yêu cầu có nội dung khác.")
	}
	if rec.State != idempotencyStateDone {
		return idempotencyFail(c, fiber.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "Yêu cầu trước đó với cùng Idempotency-Key vẫn đang được xử lý.")
	}
	c.Set(IdempotencyReplayHeader, "true")
	c.Set(fiber.HeaderContentType, rec.ContentType)
	return c.Status(rec.Status).Send(rec.Body)
}

func releaseIdempotencyKey(ctx context.Context, rdb *redis.Client, redisKey string) {
	if err := rdb.Del(ctx, redisKey).Err(); err != nil {
		log.Printf("[Idempotency] không nhả được khoá %s: %v (hết hạn sau PendingTTL)", redisKey, err)
	}
}
