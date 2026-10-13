package dto

import "github.com/google/uuid"

// F-10 wishlist - contract: plans/261013-qa2-role-findings/contract-be3.md §4.
// Thông tin khoá học dùng chung CartCourseInfoDTO để web tái dùng một kiểu cho giỏ hàng và wishlist.

type AddToWishlistDTO struct {
	CourseID uuid.UUID `json:"course_id" validate:"required"`
}

type WishlistItemResponseDTO struct {
	ID        uuid.UUID          `json:"id"`
	CourseID  uuid.UUID          `json:"course_id"`
	UserID    uuid.UUID          `json:"user_id"`
	CreatedAt string             `json:"created_at"`
	Course    *CartCourseInfoDTO `json:"course,omitempty"`
}

type WishlistListResponseDTO struct {
	Items     []WishlistItemResponseDTO `json:"items"`
	TotalItem int                       `json:"total_item"`
}
