package dto

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type AddToCartDTO struct {
	CourseID uuid.UUID `json:"course_id" validate:"required"`
}

type CartItemResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	CourseID  uuid.UUID `json:"course_id"`
	UserID    uuid.UUID `json:"user_id"`
	CreatedAt string    `json:"created_at"`

	// Course info
	Course *CartCourseInfoDTO `json:"course,omitempty"`
}

type CartCourseInfoDTO struct {
	ID               uuid.UUID       `json:"id"`
	Title            string          `json:"title"`
	Slug             string          `json:"slug"`
	ThumbnailURL     string          `json:"thumbnail_url,omitempty"`
	Price            decimal.Decimal `json:"price"`
	DiscountPrice    *decimal.Decimal `json:"discount_price,omitempty"`
	Level            string          `json:"level"`
	InstructorName   string          `json:"instructor_name"`
	TotalDurationMins int            `json:"total_duration_mins"`
	TotalLessons     int             `json:"total_lessons"`
	AverageRating    decimal.Decimal `json:"average_rating"`
	TotalStudents    int             `json:"total_students"`
}

// W1: Total là TỔNG TIỀN giỏ hàng (VND), không phải số lượng — trước đây khai
// kiểu int và không bao giờ được gán giá trị nên luôn hiển thị 0đ trên FE.
type CartListResponseDTO struct {
	Items     []CartItemResponseDTO `json:"items"`
	Total     float64               `json:"total"`
	TotalItem int                   `json:"total_item"`
}

type RemoveFromCartDTO struct {
	CourseIDs []uuid.UUID `json:"course_ids" validate:"required,min=1"`
}
