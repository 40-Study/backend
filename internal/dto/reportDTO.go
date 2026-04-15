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
}

type ReportListDTO struct {
	Data     []ReportResponseDTO `json:"data"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}
