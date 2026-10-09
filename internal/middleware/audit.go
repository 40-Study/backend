package middleware

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// AuditRecorder là phần của AuditLogService mà middleware cần (service.AuditLogService thoả mãn).
type AuditRecorder interface {
	Record(ctx context.Context, entry model.AuditEntry) error
}

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

		if err := c.Next(); err != nil {
			return err
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
		entry := model.AuditEntry{
			ActorID:    actor,
			Action:     st.action,
			TargetType: targetType,
			TargetID:   targetID,
			StatusCode: status,
			IP:         c.IP(),
			Metadata:   st.meta,
		}
		recordAudit(c.UserContext(), rec, entry)
		return nil
	}
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
