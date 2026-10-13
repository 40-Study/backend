package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// MODERATION REPORT
// ============================================================================

type CreateReportDTO struct {
	ReportedType string `json:"reported_type" validate:"required,oneof=course review discussion user"`
	ReportedID   string `json:"reported_id" validate:"required,uuid"`
	Reason       string `json:"reason" validate:"required,oneof=spam inappropriate copyright harassment other"`
	Description  string `json:"description,omitempty"`
}

type UpdateReportStatusDTO struct {
	Status     string `json:"status" validate:"required,oneof=reviewing resolved dismissed"`
	AdminNotes string `json:"admin_notes,omitempty"`
}

type ReportResponseDTO struct {
	ID           uuid.UUID  `json:"id"`
	ReporterID   uuid.UUID  `json:"reporter_id"`
	ReporterName string     `json:"reporter_name"`
	ReportedType string     `json:"reported_type"`
	ReportedID   uuid.UUID  `json:"reported_id"`
	Reason       string     `json:"reason"`
	Description  *string    `json:"description,omitempty"`
	Status       string     `json:"status"`
	AdminNotes   *string    `json:"admin_notes,omitempty"`
	ResolvedBy   *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`

	// Target (ADM-N4): tóm tắt đối tượng bị báo cáo, CHỈ có khi người gọi là người kiểm duyệt
	// (GET /reports, GET /reports/:id). Contract: plans/261013-qa2-role-findings/contract-be3.md §1a.
	Target *ReportTargetDTO `json:"target,omitempty"`
}

// ReportTargetDTO — mọi khoá luôn có mặt khi Target được trả; trường không áp dụng là null (không bỏ khoá).
type ReportTargetDTO struct {
	Type             string     `json:"type"`
	ID               uuid.UUID  `json:"id"`
	Exists           bool       `json:"exists"`
	Name             *string    `json:"name"`
	Status           *string    `json:"status"`
	Slug             *string    `json:"slug"`
	CourseID         *uuid.UUID `json:"course_id"`
	CourseSlug       *string    `json:"course_slug"`
	OwnerID          *uuid.UUID `json:"owner_id"`
	AvailableActions []string   `json:"available_actions"`
}

// ReportActionRequestDTO — POST /reports/:id/actions (ADM-N4). Reason bắt buộc với unpublish_course và
// hide_discussion (kiểm ở service vì điều kiện phụ thuộc action). Resolve mặc định true khi bỏ trống.
type ReportActionRequestDTO struct {
	Action  string `json:"action" validate:"required,oneof=unpublish_course hide_discussion unhide_discussion"`
	Reason  string `json:"reason" validate:"omitempty,max=500"`
	Resolve *bool  `json:"resolve"`
}

type ReportActionResponseDTO struct {
	Action string             `json:"action"`
	Report *ReportResponseDTO `json:"report"`
}

type ReportListDTO struct {
	Data     []ReportResponseDTO `json:"data"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}
