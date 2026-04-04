package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Course DTOs

type CreateCourseDTO struct {
	InstructorID      uuid.UUID  `json:"instructor_id" validate:"required"`
	CategoryID        *uuid.UUID `json:"category_id"`
	Title             string     `json:"title" validate:"required,min=2,max=255"`
	ShortDescription  *string    `json:"short_description"`
	Description       *string    `json:"description"`
	ThumbnailURL      *string    `json:"thumbnail_url"`
	PreviewVideoURL   *string    `json:"preview_video_url"`
	Level             string     `json:"level" validate:"omitempty,oneof=beginner intermediate advanced all_levels"`
	Language          string     `json:"language" validate:"omitempty,max=10"`
	Price             decimal.Decimal `json:"price"`
	DiscountPrice     *decimal.Decimal `json:"discount_price"`
	DiscountExpiresAt *time.Time `json:"discount_expires_at"`
	Requirements      []string   `json:"requirements"`
	Objectives        []string   `json:"objectives"`
	TargetAudience    []string   `json:"target_audience"`
	IsFree            *bool      `json:"is_free"`
	TagIDs            []uuid.UUID `json:"tag_ids"`
}

type UpdateCourseDTO struct {
	CategoryID        *uuid.UUID       `json:"category_id"`
	Title             *string          `json:"title" validate:"omitempty,min=2,max=255"`
	ShortDescription  *string          `json:"short_description"`
	Description       *string          `json:"description"`
	ThumbnailURL      *string          `json:"thumbnail_url"`
	PreviewVideoURL   *string          `json:"preview_video_url"`
	Level             *string          `json:"level" validate:"omitempty,oneof=beginner intermediate advanced all_levels"`
	Language          *string          `json:"language" validate:"omitempty,max=10"`
	Price             *decimal.Decimal `json:"price"`
	DiscountPrice     *decimal.Decimal `json:"discount_price"`
	DiscountExpiresAt *time.Time       `json:"discount_expires_at"`
	Requirements      []string        `json:"requirements"`
	Objectives        []string        `json:"objectives"`
	TargetAudience    []string        `json:"target_audience"`
	IsFree            *bool           `json:"is_free"`
	IsFeatured        *bool           `json:"is_featured"`
	Status            *string         `json:"status" validate:"omitempty,oneof=draft pending_review published archived"`
	TagIDs            []uuid.UUID      `json:"tag_ids"`
}

type CourseResponseDTO struct {
	ID                uuid.UUID        `json:"id"`
	InstructorID      uuid.UUID        `json:"instructor_id"`
	CategoryID        *uuid.UUID       `json:"category_id,omitempty"`
	Title             string           `json:"title"`
	Slug              string           `json:"slug"`
	ShortDescription  *string          `json:"short_description,omitempty"`
	Description       *string          `json:"description,omitempty"`
	ThumbnailURL      *string          `json:"thumbnail_url,omitempty"`
	PreviewVideoURL   *string          `json:"preview_video_url,omitempty"`
	Level             string           `json:"level"`
	Language          string           `json:"language"`
	Price             decimal.Decimal  `json:"price"`
	DiscountPrice     *decimal.Decimal `json:"discount_price,omitempty"`
	DiscountExpiresAt *time.Time       `json:"discount_expires_at,omitempty"`
	TotalDurationMins int              `json:"total_duration_minutes"`
	TotalLessons      int              `json:"total_lessons"`
	TotalStudents     int              `json:"total_students"`
	AverageRating     decimal.Decimal  `json:"average_rating"`
	TotalReviews      int              `json:"total_reviews"`
	Requirements      []string         `json:"requirements,omitempty"`
	Objectives        []string         `json:"objectives,omitempty"`
	TargetAudience    []string         `json:"target_audience,omitempty"`
	Status            string           `json:"status"`
	PublishedAt       *time.Time       `json:"published_at,omitempty"`
	IsFeatured        bool             `json:"is_featured"`
	IsFree            bool             `json:"is_free"`
	Instructor        *CourseInstructorDTO `json:"instructor,omitempty"`
	Category          *CategoryResponseDTO `json:"category,omitempty"`
	Tags              []TagResponseDTO   `json:"tags,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type CourseInstructorDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AvatarURL *string   `json:"avatar,omitempty"`
	Bio       *string   `json:"bio,omitempty"`
}

type CourseDetailDTO struct {
	CourseResponseDTO
	Sections []SectionResponseDTO `json:"sections,omitempty"`
}

type CourseListResponseDTO struct {
	Courses  []CourseResponseDTO `json:"courses"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type CourseFilterParams struct {
	CategoryID *uuid.UUID `query:"category_id"`
	InstructorID *uuid.UUID `query:"instructor_id"`
	Level      string     `query:"level"`
	Status     string     `query:"status"`
	Keyword    string     `query:"keyword"`
	IsFree     *bool      `query:"is_free"`
	IsFeatured *bool      `query:"is_featured"`
	MinPrice   *float64   `query:"min_price"`
	MaxPrice   *float64   `query:"max_price"`
	TagIDs     []uuid.UUID `query:"tag_ids"`
	Page       int        `query:"page"`
	PageSize   int        `query:"page_size"`
}

// Convert Course model to CourseResponseDTO
func ToCourseResponse(c interface{}) *CourseResponseDTO {
	return nil // Implemented in service layer with proper relations
}
