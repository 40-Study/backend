package dto

import "github.com/google/uuid"

type CreateAttendanceDTO struct {
	StudentID uuid.UUID `json:"student_id" validate:"required"`
	Date      string    `json:"date" validate:"required"`
	Status    string    `json:"status" validate:"required,oneof=present absent late excused"`
	Note      *string   `json:"note"`
}

type BulkCreateAttendanceDTO struct {
	Date        string               `json:"date" validate:"required"`
	Attendances []AttendanceEntryDTO `json:"attendances" validate:"required,min=1"`
}

type AttendanceEntryDTO struct {
	StudentID uuid.UUID `json:"student_id" validate:"required"`
	Status    string    `json:"status" validate:"required,oneof=present absent late excused"`
	Note      *string   `json:"note"`
}

type UpdateAttendanceDTO struct {
	Status *string `json:"status" validate:"omitempty,oneof=present absent late excused"`
	Note   *string `json:"note"`
}

type AttendanceResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	ClassID   uuid.UUID `json:"class_id"`
	StudentID uuid.UUID `json:"student_id"`
	Date      string    `json:"date"`
	Status    string    `json:"status"`
	Note      *string   `json:"note,omitempty"`
	CreatedAt string    `json:"created_at"`
}

type AttendanceListResponseDTO struct {
	Attendances []AttendanceResponseDTO `json:"attendances"`
	Total       int64                   `json:"total"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"page_size"`
}
