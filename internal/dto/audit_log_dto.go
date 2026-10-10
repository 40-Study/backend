package dto

import (
	"time"

	"github.com/google/uuid"
)

// Contract C2 (plans/261008-qa-followup-features/contract.md): admin audit log.
// Chỉ là kiểu dữ liệu — đổi json tag = đổi contract, phải sửa contract.md trước.

// AuditActorDTO — người thực hiện hành động; null trong AuditLogItemDTO khi hành động không có actor.
type AuditActorDTO struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

// AuditLogItemDTO — một dòng nhật ký. actor/target_id/ip/metadata là null (KHÔNG omitempty) khi vắng.
type AuditLogItemDTO struct {
	ID         uuid.UUID      `json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	Actor      *AuditActorDTO `json:"actor"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   *string        `json:"target_id"`
	StatusCode int            `json:"status_code"`
	IP         *string        `json:"ip"`
	Metadata   map[string]any `json:"metadata"`
}

// AuditLogListDTO — GET /api/admin/audit-logs. items luôn là mảng (xem NewAuditLogListDTO).
type AuditLogListDTO struct {
	Items    []AuditLogItemDTO `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// NewAuditLogListDTO bảo đảm items mã hoá thành [] (không phải null) khi rỗng.
func NewAuditLogListDTO(items []AuditLogItemDTO, total int64, page, pageSize int) AuditLogListDTO {
	if items == nil {
		items = []AuditLogItemDTO{}
	}
	return AuditLogListDTO{Items: items, Total: total, Page: page, PageSize: pageSize}
}

// AuditLogFilterDTO — query string của GET /api/admin/audit-logs. page_size tối đa 100; from/to là
// RFC3339 hoặc YYYY-MM-DD (parse + lỗi 400 INVALID_FILTER thuộc service, không phải DTO).
type AuditLogFilterDTO struct {
	Page       int    `query:"page"`
	PageSize   int    `query:"page_size"`
	Action     string `query:"action"`
	ActorID    string `query:"actor_id"`
	TargetType string `query:"target_type"`
	TargetID   string `query:"target_id"`
	From       string `query:"from"`
	To         string `query:"to"`
}
