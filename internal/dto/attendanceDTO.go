package dto

import "github.com/google/uuid"

type CreateAttendanceDTO struct {
	StudentID uuid.UUID `json:"student_id" binding:"required"`
	Date      string    `json:"date" binding:"required"`
	Status    string    `json:"status" binding:"required,oneof=present absent late excused"`
	Note      *string   `json:"note"`
}

type BulkCreateAttendanceDTO struct {
	Date        string               `json:"date" binding:"required"`
	Attendances []AttendanceEntryDTO `json:"attendances" binding:"required,min=1"`
}

type AttendanceEntryDTO struct {
	StudentID uuid.UUID `json:"student_id" binding:"required"`
	Status    string    `json:"status" binding:"required,oneof=present absent late excused"`
	Note      *string   `json:"note"`
}

type UpdateAttendanceDTO struct {
	Status *string `json:"status" binding:"omitempty,oneof=present absent late excused"`
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
