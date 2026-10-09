package dto

// Contract C3 (plans/261008-qa-followup-features/contract.md): admin broadcast notifications.
// Vai trò không hợp lệ -> 400 UNKNOWN_ROLE ở service (tập vai trò lấy từ SSOT, không hardcode ở đây).

// BroadcastRequestDTO — POST /api/admin/notifications/broadcast. notification_type bỏ trống = "system".
type BroadcastRequestDTO struct {
	Title            string   `json:"title" validate:"required,max=255"`
	Content          string   `json:"content" validate:"required,max=2000"`
	Audience         string   `json:"audience" validate:"required,oneof=all roles"`
	Roles            []string `json:"roles,omitempty"`
	NotificationType string   `json:"notification_type,omitempty" validate:"omitempty,oneof=system promotion"`
}

// BroadcastPreviewRequestDTO — POST /api/admin/notifications/broadcast/preview.
type BroadcastPreviewRequestDTO struct {
	Audience string   `json:"audience" validate:"required,oneof=all roles"`
	Roles    []string `json:"roles,omitempty"`
}

// BroadcastPreviewDTO — data của preview.
type BroadcastPreviewDTO struct {
	RecipientCount int64 `json:"recipient_count"`
}

// BroadcastResultDTO — data của 201. roles luôn có mặt (truyền []string{} cho audience=all để ra []).
type BroadcastResultDTO struct {
	RecipientCount   int64    `json:"recipient_count"`
	Audience         string   `json:"audience"`
	Roles            []string `json:"roles"`
	NotificationType string   `json:"notification_type"`
}
