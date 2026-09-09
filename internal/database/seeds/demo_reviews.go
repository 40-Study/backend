package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// demoReviewSpec mô tả một đánh giá khoá học kèm danh sách người bấm "hữu ích".
type demoReviewSpec struct {
	AuthorEmail string
	CourseSlug  string
	Rating      int
	Comment     string
	// HelpfulBy là email của những người phản ứng "helpful" với đánh giá này.
	HelpfulBy []string
}

// demoReviews chứa TỐI ĐA MỘT đánh giá cho mỗi khoá học.
//
// Giới hạn này KHÔNG phải lựa chọn thiết kế mà do ràng buộc DB hiện tại:
// index `idx_user_course_review` được khai báo trong model.Review là composite
// (user_id, course_id) nhưng tag `uniqueIndex` chỉ nằm trên trường CourseID
// (internal/model/review.go:12), nên GORM sinh ra UNIQUE btree (course_id).
// Hệ quả: mỗi khoá chỉ nhận được đúng một đánh giá. Sau khi sửa model và chạy
// migration đổi index thành (user_id, course_id), có thể thêm nhiều đánh giá
// cho cùng một khoá vào danh sách dưới đây.
//
// Chỉ đánh giá khoá mà học viên đã ghi danh (xem demoEnrollments) để dữ liệu
// demo không mâu thuẫn với luồng thật.
var demoReviews = []demoReviewSpec{
	{
		AuthorEmail: "student1@demo.com",
		CourseSlug:  "react-nextjs-tu-co-ban-den-nang-cao",
		Rating:      5,
		Comment:     "Khoá học bám sát thực tế. Phần Server Components giải thích rất dễ hiểu, mình áp dụng ngay được vào đồ án.",
		HelpfulBy:   []string{"student2@demo.com", "parent1@demo.com"},
	},
	{
		AuthorEmail: "student1@demo.com",
		CourseSlug:  "git-github-cho-nguoi-moi-bat-dau",
		Rating:      5,
		Comment:     "Miễn phí mà chất lượng hơn nhiều khoá trả phí. Giải thích rebase và conflict cực kỳ rõ ràng.",
		HelpfulBy:   []string{"student2@demo.com"},
	},
	{
		AuthorEmail: "student1@demo.com",
		CourseSlug:  "flutter-mobile-development",
		Rating:      4,
		Comment:     "Giảng viên nhiệt tình, trả lời thảo luận nhanh. Mong có thêm phần state management nâng cao.",
		HelpfulBy:   []string{"student2@demo.com"},
	},
	{
		AuthorEmail: "student2@demo.com",
		CourseSlug:  "python-cho-khoa-hoc-du-lieu",
		Rating:      4,
		Comment:     "Mới học được vài chương nhưng thấy lộ trình hợp lý, ví dụ dùng dataset thật nên dễ hình dung.",
		HelpfulBy:   []string{"student1@demo.com"},
	},
}

// SeedDemoReviews tạo đánh giá khoá học và phản ứng "hữu ích" tương ứng.
// Idempotent: khoá theo unique index (user_id, course_id) của bảng reviews.
func (s *Seeder) SeedDemoReviews(
	users map[string]model.User,
	courses map[string]model.Course,
) error {
	log.Println("Seeding demo reviews...")

	created := 0
	for _, spec := range demoReviews {
		author, ok := users[spec.AuthorEmail]
		if !ok {
			return fmt.Errorf("review author %s not found", spec.AuthorEmail)
		}
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return fmt.Errorf("review course %s not found", spec.CourseSlug)
		}

		review, err := s.upsertReview(author.ID, course.ID, spec)
		if err != nil {
			return err
		}
		created++

		if err := s.seedReviewReactions(review.ID, spec.HelpfulBy, users); err != nil {
			return err
		}
	}

	log.Printf("Seeded %d reviews\n", created)
	return nil
}

func (s *Seeder) upsertReview(userID, courseID uuid.UUID, spec demoReviewSpec) (model.Review, error) {
	review := model.Review{
		UserID:   userID,
		CourseID: courseID,
		Rating:   spec.Rating,
		Comment:  ptr(spec.Comment),
	}

	if err := s.db.Where("user_id = ? AND course_id = ?", userID, courseID).
		Attrs(review).
		FirstOrCreate(&review).Error; err != nil {
		return model.Review{}, fmt.Errorf("failed to seed review for course %s: %w", spec.CourseSlug, err)
	}
	return review, nil
}

// seedReviewReactions gắn phản ứng "helpful" của các user khác lên đánh giá.
// Bỏ qua email không tồn tại trong tập demo thay vì lỗi — phản ứng là dữ liệu phụ.
func (s *Seeder) seedReviewReactions(
	reviewID uuid.UUID,
	voterEmails []string,
	users map[string]model.User,
) error {
	for _, email := range voterEmails {
		voter, ok := users[email]
		if !ok {
			continue
		}

		reaction := model.ReviewReaction{
			ReviewID:     reviewID,
			UserID:       voter.ID,
			ReactionType: "helpful",
		}
		if err := s.db.Where("review_id = ? AND user_id = ?", reviewID, voter.ID).
			Attrs(reaction).
			FirstOrCreate(&reaction).Error; err != nil {
			return fmt.Errorf("failed to seed review reaction: %w", err)
		}
	}
	return nil
}
