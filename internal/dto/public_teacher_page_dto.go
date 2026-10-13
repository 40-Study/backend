package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PG-M2 trang giảng viên công khai - contract: plans/261013-qa2-role-findings/contract-be3.md §3.
// Cố ý KHÔNG có email, số điện thoại, ngày sinh, thông tin ngân hàng, trạng thái duyệt hay chứng chỉ:
// route này không cần đăng nhập (xem TestPublicTeacherProfile_JSONKhongCoTruongNhayCam).

type PublicTeacherStatsDTO struct {
	CourseCount   int64           `json:"course_count"`
	StudentCount  int64           `json:"student_count"`
	ReviewCount   int64           `json:"review_count"`
	AverageRating decimal.Decimal `json:"average_rating"`
}

type PublicTeacherCourseDTO struct {
	ID                uuid.UUID        `json:"id"`
	Title             string           `json:"title"`
	Slug              string           `json:"slug"`
	ShortDescription  *string          `json:"short_description"`
	ThumbnailURL      *string          `json:"thumbnail_url"`
	Level             string           `json:"level"`
	Price             decimal.Decimal  `json:"price"`
	DiscountPrice     *decimal.Decimal `json:"discount_price"`
	IsFree            bool             `json:"is_free"`
	AverageRating     decimal.Decimal  `json:"average_rating"`
	TotalReviews      int              `json:"total_reviews"`
	TotalStudents     int              `json:"total_students"`
	TotalLessons      int              `json:"total_lessons"`
	TotalDurationMins int              `json:"total_duration_minutes"`
	PublishedAt       *time.Time       `json:"published_at"`
}

type PublicTeacherPageDTO struct {
	ID         uuid.UUID `json:"id"`
	UserName   string    `json:"user_name"`
	FullName   *string   `json:"full_name"`
	AvatarURL  *string   `json:"avatar_url"`
	Bio        *string   `json:"bio"`
	IsVerified bool      `json:"is_verified"`
	JoinedAt   time.Time `json:"joined_at"`

	// Chỉ có giá trị khi hồ sơ giảng viên đã được duyệt (approved); ngược lại là null.
	Specialization  *string `json:"specialization"`
	Education       *string `json:"education"`
	ExperienceYears *int    `json:"experience_years"`
	Department      *string `json:"department"`

	Stats   PublicTeacherStatsDTO    `json:"stats"`
	Courses []PublicTeacherCourseDTO `json:"courses"`
}
