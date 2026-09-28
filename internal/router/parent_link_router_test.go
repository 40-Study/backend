package router

// Review PR #81, MAJOR-1: POST /family/link-requests có rate-limit THEO IP (gộp mọi tài khoản),
// để không dùng nhiều tài khoản phụ huynh từ cùng máy để dò email học sinh. Route THẬT +
// AuthMiddleware THẬT (miniredis) + handler THẬT; chỉ service được fake.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type plFakeService struct {
	service.ParentLinkServiceInterface
	created int
}

func (f *plFakeService) CreateRequest(ctx context.Context, parentID uuid.UUID, req dto.CreateParentLinkRequestDto) (*dto.ParentLinkRequestDto, error) {
	f.created++
	return &dto.ParentLinkRequestDto{ID: uuid.NewString(), Status: "pending", StudentEmail: req.StudentEmail}, nil
}

func (f *plFakeService) ListSent(ctx context.Context, parentID uuid.UUID) ([]dto.ParentLinkRequestDto, error) {
	return []dto.ParentLinkRequestDto{}, nil
}

func TestParentLinkRoutes_CreateRequestRateLimitedPerIPAcrossAccounts(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "parent-link-route-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}
	token := func() string {
		id := uuid.New()
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), int64(1), 0).Err(); err != nil {
			t.Fatalf("seed user_version: %v", err)
		}
		s, _, err := utils.GenerateTokens(cfg, id, uuid.New(), "PARENT", nil, 1)
		if err != nil {
			t.Fatalf("GenerateTokens: %v", err)
		}
		return s
	}

	svc := &plFakeService{}
	app := fiber.New()
	SetupParentLinkRoutes(app.Group("/api"), cfg, handler.NewParentLinkHandler(svc), rdb)
	do := func(method, path, tok, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp.StatusCode
	}

	// Mỗi lượt dùng một tài khoản MỚI: hạn mức theo tài khoản không bao giờ chạm, chỉ IP chặn.
	body := `{"student_email":"hs@40study.test","relationship":"parent"}`
	for i := 0; i < parentLinkIPMaxPerHour; i++ {
		if code := do("POST", "/api/family/link-requests", token(), body); code != fiber.StatusCreated {
			t.Fatalf("lượt %d = %d, muốn 201", i+1, code)
		}
	}
	if code := do("POST", "/api/family/link-requests", token(), body); code != fiber.StatusTooManyRequests {
		t.Fatalf("vượt hạn mức IP bằng tài khoản mới = %d, muốn 429", code)
	}
	if svc.created != parentLinkIPMaxPerHour {
		t.Fatalf("service được gọi %d lần, muốn %d", svc.created, parentLinkIPMaxPerHour)
	}
	// Chỉ route gửi bị giới hạn; xem danh sách vẫn được.
	if code := do("GET", "/api/family/link-requests/sent", token(), ""); code != fiber.StatusOK {
		t.Fatalf("GET sent sau khi bị chặn gửi = %d, muốn 200", code)
	}
}
