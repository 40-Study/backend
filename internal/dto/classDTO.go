package dto

import (
	"time"

	"github.com/google/uuid"
)

// Class DTOs

type CreateClassDTO struct {
	Name        string     `json:"name" binding:"required,min=2,max=255"`
	Description *string    `json:"description"`
	CourseID    *uuid.UUID `json:"course_id"`
	MaxStudents *int       `json:"max_students" binding:"omitempty,min=1"`
	StartDate   *string    `json:"start_date"`
	EndDate     *string    `json:"end_date"`
}

type UpdateClassDTO struct {
	Name        *string    `json:"name" binding:"omitempty,min=2,max=255"`
	Description *string    `json:"description"`
	CourseID    *uuid.UUID `json:"course_id"`
	Status      *string    `json:"status" binding:"omitempty,oneof=draft active archived"`
	MaxStudents *int       `json:"max_students" binding:"omitempty,min=1"`
	StartDate   *string    `json:"start_date"`
	EndDate     *string    `json:"end_date"`
}

type ClassResponseDTO struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Description  *string    `json:"description,omitempty"`
	CourseID     *uuid.UUID `json:"course_id,omitempty"`
	Status       string     `json:"status"`
	MaxStudents  *int       `json:"max_students,omitempty"`
	StartDate    *time.Time `json:"start_date,omitempty"`
	EndDate      *time.Time `json:"end_date,omitempty"`
	TeacherCount int64      `json:"teacher_count"`
	StudentCount int64      `json:"student_count"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

type ClassListResponseDTO struct {
	Classes  []ClassResponseDTO `json:"classes"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

// TeacherClass DTOs

type AssignTeacherDTO struct {
	TeacherID uuid.UUID `json:"teacher_id" binding:"required"`
	Role      string    `json:"role" binding:"omitempty,oneof=primary assistant"`
}

type TeacherClassResponseDTO struct {
	ID         uuid.UUID           `json:"id"`
	TeacherID  uuid.UUID           `json:"teacher_id"`
	ClassID    uuid.UUID           `json:"class_id"`
	Role       string              `json:"role"`
	AssignedAt string              `json:"assigned_at"`
	Teacher    *TeacherResponseDTO `json:"teacher,omitempty"`
}

// StudentClass DTOs

type EnrollStudentDTO struct {
	StudentID uuid.UUID `json:"student_id" binding:"required"`
}

type StudentClassResponseDTO struct {
	ID         uuid.UUID `json:"id"`
	StudentID  uuid.UUID `json:"student_id"`
	ClassID    uuid.UUID `json:"class_id"`
	EnrolledAt string    `json:"enrolled_at"`
	Status     string    `json:"status"`
	UserName   string    `json:"user_name"`
	Email      string    `json:"email"`
	FullName   *string   `json:"full_name,omitempty"`
	AvatarURL  *string   `json:"avatar_url,omitempty"`
}
