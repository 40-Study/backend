package dto

import (
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// Course DTOs

type CreateCourseDTO struct {
	InstructorID     uuid.UUID        `json:"instructor_id" validate:"required"`
	CategoryID       *uuid.UUID       `json:"category_id"`
	Title            string           `json:"title" validate:"required,min=2,max=255"`
	ShortDescription *string          `json:"short_description"`
	Description      *string          `json:"description"`
	ThumbnailURL     *string          `json:"thumbnail_url"`
	PreviewVideoURL  *string          `json:"preview_video_url"`
	Level            string           `json:"level"`
	Language         string           `json:"language"`
	Price            *decimal.Decimal `json:"price"`
	Requirements     pq.StringArray   `json:"requirements"`
	Objectives       pq.StringArray   `json:"objectives"`
	TargetAudience   pq.StringArray   `json:"target_audience"`
	IsFree           *bool            `json:"is_free"`
	TagIDs           []uuid.UUID      `json:"tag_ids"`
}

type UpdateCourseDTO struct {
	CategoryID       *uuid.UUID       `json:"category_id"`
	Title            *string          `json:"title" validate:"omitempty,min=2,max=255"`
	ShortDescription *string          `json:"short_description"`
	Description      *string          `json:"description"`
	ThumbnailURL     *string          `json:"thumbnail_url"`
	PreviewVideoURL  *string          `json:"preview_video_url"`
	Level            *string          `json:"level"`
	Language         *string          `json:"language"`
	Price            *decimal.Decimal `json:"price"`
	Status           *string          `json:"status"`
	Requirements     pq.StringArray   `json:"requirements"`
	Objectives       pq.StringArray   `json:"objectives"`
	TargetAudience   pq.StringArray   `json:"target_audience"`
	IsFree           *bool            `json:"is_free"`
	IsFeatured       *bool            `json:"is_featured"`
	TagIDs           []uuid.UUID      `json:"tag_ids"`
}

type CourseResponseDTO struct {
	ID               uuid.UUID          `json:"id"`
	InstructorID     uuid.UUID          `json:"instructor_id"`
	CategoryID       *uuid.UUID         `json:"category_id,omitempty"`
	Title            string             `json:"title"`
	Slug             string             `json:"slug"`
	ShortDescription *string            `json:"short_description,omitempty"`
	Description      *string            `json:"description,omitempty"`
	ThumbnailURL     *string            `json:"thumbnail_url,omitempty"`
	PreviewVideoURL  *string            `json:"preview_video_url,omitempty"`
	Level            string             `json:"level"`
	Language         string             `json:"language"`
	Price            decimal.Decimal    `json:"price"`
	DiscountPrice    *decimal.Decimal   `json:"discount_price,omitempty"`
	TotalDurationMins int              `json:"total_duration_minutes"`
	TotalLessons     int                `json:"total_lessons"`
	TotalStudents    int                `json:"total_students"`
	AverageRating    decimal.Decimal    `json:"average_rating"`
	TotalReviews     int                `json:"total_reviews"`
	Requirements     pq.StringArray     `json:"requirements,omitempty"`
	Objectives       pq.StringArray     `json:"objectives,omitempty"`
	TargetAudience   pq.StringArray     `json:"target_audience,omitempty"`
	Status           string             `json:"status"`
	IsFeatured       bool               `json:"is_featured"`
	IsFree           bool               `json:"is_free"`
	Category         *CategoryResponseDTO `json:"category,omitempty"`
	Tags             []TagResponseDTO   `json:"tags,omitempty"`
	CreatedAt        string             `json:"created_at"`
	UpdatedAt        string             `json:"updated_at"`
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
	Level      string     `query:"level"`
	Status     string     `query:"status"`
	Keyword    string     `query:"keyword"`
	IsFree     *bool      `query:"is_free"`
	MinPrice   *float64   `query:"min_price"`
	MaxPrice   *float64   `query:"max_price"`
	Page       int        `query:"page"`
	PageSize   int        `query:"page_size"`
}
