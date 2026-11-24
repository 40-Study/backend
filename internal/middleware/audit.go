package middleware

import (
	"context"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// AuditRecorder là phần của AuditLogService mà middleware cần (service.AuditLogService thoả mãn).
type AuditRecorder interface {
	Record(ctx context.Context, entry model.AuditEntry) error
}

// AuditTargetNamer là phần TUỲ CHỌN của recorder (service.AuditLogService thoả mãn): tra tên người đọc được của đối tượng
// đích để ghi metadata `target_name` (QA5-A02, contract C4). Recorder không có thì dòng nhật ký không có target_name.
type AuditTargetNamer interface {
	AuditTargetName(ctx context.Context, targetType, targetID string) (string, error)
}

// AuditTargetNameKey: khoá metadata chứa tên đối tượng đích.
const AuditTargetNameKey = "target_name"

type auditLocalsKey struct{}

// auditState sống trong c.Locals suốt một request: handler dùng SetAudit* để chỉnh nội dung dòng
// nhật ký mà middleware sẽ ghi SAU khi handler xong.
type auditState struct {
	action   string
	targetID string
	meta     map[string]any
	// recordOnFailure: handler đã chủ động đánh dấu "ghi cả khi response không phải 2xx" (xem
	// SetAuditRecordOnFailure). Mặc định false: D6 chỉ ghi 2xx.
	recordOnFailure bool
	// skip: handler xác định request này không phải hành động quản trị (xem SkipAudit).
	skip bool
}

// Audit ghi một dòng nhật ký quản trị khi handler phía sau THÀNH CÔNG (plan D5/D6, contract C2).
//
// Đặt SAU AuthMiddleware và RequirePermissions trong chuỗi route (cần actor `user_id`, và request
// bị 401/403 không bao giờ chạm tới đây). targetParam là tên URL param chứa id đối tượng ("" =
// route không có đối tượng đích, dùng SetAuditTarget nếu cần).
//
// Quy tắc ghi: chỉ khi c.Next() trả nil VÀ status 2xx. App không có ErrorHandler tuỳ biến, nên
// khi handler trả Go error thì status trên response CHƯA phải status cuối cùng (Fiber xử lý error
// sau khi cả chuỗi middleware thoát ra) — đọc status lúc đó sẽ cho kết quả sai; vì vậy lỗi thì
// trả nguyên và KHÔNG ghi. Không bao giờ lưu body của request.
//
// Ghi nhật ký lỗi (DB hỏng, entry sai, thậm chí panic của recorder) chỉ log lớn tiếng — hành
// động quản trị đã commit rồi, không được biến thành lỗi cho người dùng (D5).
func Audit(rec AuditRecorder, action, targetType, targetParam string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		st := &auditState{action: action}
		c.Locals(auditLocalsKey{}, st)

		// Tra tên TRƯỚC khi handler chạy: đối tượng bị xoá cứng bởi handler thì sau đó không tra được nữa.
		// Sau handler tra lại (tên mới sau khi sửa); nếu không ra thì dùng tên đã tra trước.
		var preName string
		if targetParam != "" {
			preName = lookupAuditTargetName(c.UserContext(), rec, targetType, c.Params(targetParam))
		}

		if err := c.Next(); err != nil {
			return err
		}
		if st.skip {
			return nil
		}
		status := c.Response().StatusCode()
		if (status < fiber.StatusOK || status >= fiber.StatusMultipleChoices) && !st.recordOnFailure {
			return nil
		}

		actor, ok := c.Locals("user_id").(uuid.UUID)
		if !ok || actor == uuid.Nil {
			log.Printf("[Audit] KHÔNG ghi được %q %s %s: request không có actor user_id (middleware Audit đặt trước AuthMiddleware?)",
				st.action, c.Method(), c.Path())
			return nil
		}
		targetID := st.targetID
		if targetID == "" && targetParam != "" {
			targetID = c.Params(targetParam)
		}
		meta := st.meta
		if name, _ := meta[AuditTargetNameKey].(string); name == "" {
			lookupID := targetID
			if lookupID == "" {
				// Route tạo mới không có id trên URL: lấy id từ response ({"id"} hoặc {"data":{"id"}}) chỉ để tra tên.
				lookupID = responseEntityID(c.Response().Body())
			}
			name = lookupAuditTargetName(c.UserContext(), rec, targetType, lookupID)
			if name == "" {
				name = preName
			}
			if name != "" {
				meta = copyMeta(meta)
				meta[AuditTargetNameKey] = name
			}
		}
		entry := model.AuditEntry{
			ActorID:    actor,
			Action:     st.action,
			TargetType: targetType,
			TargetID:   targetID,
			StatusCode: status,
			IP:         c.IP(),
			Metadata:   meta,
		}
		recordAudit(c.UserContext(), rec, entry)
		return nil
	}
}

func copyMeta(src map[string]any) map[string]any {
	out := make(map[string]any, len(src)+1)
	for k, v := range src {
		out[k] = v
	}
	return out
}

// lookupAuditTargetName tra tên đối tượng qua recorder (nếu hỗ trợ). Mọi lỗi/panic chỉ log: thiếu tên thì web rơi về id,
// không bao giờ làm hỏng response của hành động quản trị đã commit (D5).
func lookupAuditTargetName(ctx context.Context, rec AuditRecorder, targetType, id string) (name string) {
	namer, ok := rec.(AuditTargetNamer)
	if !ok || targetType == "" || id == "" {
		return ""
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Audit] PANIC khi tra tên đối tượng %s/%s: %v", targetType, id, r)
			name = ""
		}
	}()
	name, err := namer.AuditTargetName(ctx, targetType, id)
	if err != nil {
		log.Printf("[Audit] KHÔNG tra được tên đối tượng %s/%s: %v", targetType, id, err)
		return ""
	}
	return name
}

// responseEntityID đọc id đối tượng từ body JSON của response tạo mới: {"id":...} hoặc {"data":{"id":...}}.
func responseEntityID(body []byte) string {
	var env struct {
		ID   string `json:"id"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if len(body) == 0 || json.Unmarshal(body, &env) != nil {
		return ""
	}
	if env.Data.ID != "" {
		return env.Data.ID
	}
	return env.ID
}

// recordAudit gọi recorder và nuốt (có log) mọi lỗi/panic — xem D5.
func recordAudit(ctx context.Context, rec AuditRecorder, entry model.AuditEntry) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Audit] PANIC khi ghi %q actor=%s target=%s/%s: %v", entry.Action, entry.ActorID, entry.TargetType, entry.TargetID, r)
		}
	}()
	if err := rec.Record(ctx, entry); err != nil {
		log.Printf("[Audit] KHÔNG ghi được %q actor=%s target=%s/%s: %v", entry.Action, entry.ActorID, entry.TargetType, entry.TargetID, err)
	}
}

func auditStateOf(c *fiber.Ctx) *auditState {
	st, _ := c.Locals(auditLocalsKey{}).(*auditState)
	return st
}

// SetAuditAction đổi mã hành động của request hiện tại (vd user.lock / user.unlock trên cùng một
// route). No-op khi route không gắn middleware Audit.
func SetAuditAction(c *fiber.Ctx, action string) {
	if st := auditStateOf(c); st != nil {
		st.action = action
	}
}

// SetAuditRecordOnFailure cho phép dòng nhật ký được ghi dù response KHÔNG phải 2xx (ngoại lệ hẹp so với
// D6, chỉ handler đã đánh dấu mới được hưởng). Dùng cho hành động có tác dụng phụ dở dang không hoàn
// tác được, vd broadcast gửi tới một phần người nhận rồi lỗi (500 BROADCAST_PARTIAL): thiếu dòng nhật
// ký thì quản trị viên không biết đã có bao nhiêu người nhận. Handler trả Go error vẫn KHÔNG được ghi.
// No-op khi route không gắn middleware Audit.
func SetAuditRecordOnFailure(c *fiber.Ctx) {
	if st := auditStateOf(c); st != nil {
		st.recordOnFailure = true
	}
}

// SkipAudit bỏ ghi nhật ký cho request hiện tại: dùng khi một route dùng chung cho cả quản trị viên và
// chủ sở hữu (vd DELETE /courses/:id), và người gọi lần này KHÔNG hành động với tư cách quản trị.
// No-op khi route không gắn middleware Audit.
func SkipAudit(c *fiber.Ctx) {
	if st := auditStateOf(c); st != nil {
		st.skip = true
	}
}

// SetAuditMeta gộp thêm khoá vào metadata của dòng nhật ký (khoá nhạy cảm bị service loại).
func SetAuditMeta(c *fiber.Ctx, meta map[string]any) {
	st := auditStateOf(c)
	if st == nil {
		return
	}
	if st.meta == nil {
		st.meta = make(map[string]any, len(meta))
	}
	for k, v := range meta {
		st.meta[k] = v
	}
}

// SetAuditTarget đặt id đối tượng đích khi nó không phải URL param (vd id nằm trong response).
func SetAuditTarget(c *fiber.Ctx, id string) {
	if st := auditStateOf(c); st != nil {
		st.targetID = id
	}
}
