package seeds

import (
	"fmt"

	"study.com/v1/internal/model"
)

type reviewSpec struct {
	Email   string
	Slug    string
	Rating  int
	Comment string
	DaysAgo int
}

// demoReviews: chỉ đánh giá khoá mà học viên ĐÃ ghi danh (demoEnrollments) — cùng luật nghiệp vụ
// với scripts/seed-local-demo-260915.sql, nếu không dữ liệu tự mâu thuẫn.
var demoReviews = []reviewSpec{
	{Email: "student1@demo.com", Slug: "react-nextjs-tu-co-ban-den-nang-cao", Rating: 5, DaysAgo: 5,
		Comment: "Giảng viên đi từ dễ đến khó rất mạch lạc. Phần Server Component giải thích rõ hơn hẳn tài liệu chính thức."},
	{Email: "student1@demo.com", Slug: "flutter-mobile-development", Rating: 4, DaysAgo: 4,
		Comment: "Nội dung dễ theo, ví dụ gần với dự án thật. Mong có thêm bài về kiểm thử widget."},
	{Email: "student1@demo.com", Slug: "git-github-cho-nguoi-moi-bat-dau", Rating: 5, DaysAgo: 3,
		Comment: "Khoá ngắn mà đủ ý, học xong tự tin làm pull request cho nhóm đồ án."},
	{Email: "student2@demo.com", Slug: "python-cho-khoa-hoc-du-lieu", Rating: 4, DaysAgo: 2,
		Comment: "Phần NumPy hơi nhanh với người mới nhưng bài tập kèm lời giải rất hữu ích."},
}

type wishlistSpec struct {
	Email string
	Slug  string
}

// demoWishlists: chỉ khoá học viên CHƯA ghi danh (danh sách mong muốn mua sau).
var demoWishlists = []wishlistSpec{
	{Email: "student1@demo.com", Slug: "docker-kubernetes-thuc-chien"},
	{Email: "student1@demo.com", Slug: "python-cho-khoa-hoc-du-lieu"},
	{Email: "student2@demo.com", Slug: "react-nextjs-tu-co-ban-den-nang-cao"},
	{Email: "student2@demo.com", Slug: "flutter-mobile-development"},
}

type noteSpec struct {
	Email       string
	Slug        string
	LessonIndex int // vị trí bài trong khoá theo thứ tự hiển thị
	Seconds     int
	Content     string
	Bookmarked  bool
}

var demoNotes = []noteSpec{
	{Email: "student1@demo.com", Slug: "react-nextjs-tu-co-ban-den-nang-cao", LessonIndex: 1, Seconds: 185,
		Content: "Nhớ cài Node 20 LTS và bật ESLint trước khi tạo dự án.", Bookmarked: true},
	{Email: "student1@demo.com", Slug: "react-nextjs-tu-co-ban-den-nang-cao", LessonIndex: 4, Seconds: 412,
		Content: "useEffect chạy sau render; mảng phụ thuộc rỗng = chỉ chạy một lần khi mount."},
	{Email: "student1@demo.com", Slug: "git-github-cho-nguoi-moi-bat-dau", LessonIndex: 1, Seconds: 95,
		Content: "git reset --soft HEAD~1 để gỡ commit cuối nhưng giữ thay đổi trong staging."},
	{Email: "student2@demo.com", Slug: "python-cho-khoa-hoc-du-lieu", LessonIndex: 0, Seconds: 240,
		Content: "Kiểu int trong Python không giới hạn độ lớn, khác với C/Java.", Bookmarked: true},
}

// SeedDemoReviews tạo đánh giá khoá học của học viên đã ghi danh (trang chi tiết khoá, danh sách
// khoá) rồi tính lại average_rating/total_reviews của MỌI khoá demo từ bảng reviews bằng
// syncCourseRatingStats (cùng công thức ReviewService). Khoá tự nhiên: (user_id, course_id).
func (s *Seeder) SeedDemoReviews(users map[string]model.User, courses map[string]model.Course) error {
	for _, spec := range demoReviews {
		user, err := demoUser(users, spec.Email)
		if err != nil {
			return err
		}
		course, err := demoCourse(courses, spec.Slug)
		if err != nil {
			return err
		}
		review := model.Review{UserID: user.ID, CourseID: course.ID, Rating: spec.Rating, Comment: ptr(spec.Comment)}
		review.CreatedAt = daysAgo(spec.DaysAgo)
		review.UpdatedAt = review.CreatedAt
		if err := s.db.Where("user_id = ? AND course_id = ?", user.ID, course.ID).
			Attrs(review).FirstOrCreate(&review).Error; err != nil {
			return fmt.Errorf("failed to seed review %s/%s: %w", spec.Email, spec.Slug, err)
		}
	}

	for slug := range courses {
		course := courses[slug]
		if err := s.syncCourseRatingStats(&course); err != nil {
			return err
		}
		courses[slug] = course
	}
	return nil
}

// SeedDemoWishlistsAndNotes tạo wishlist (khoá chưa ghi danh) và ghi chú bài học có mốc thời gian
// video (panel ghi chú trong trình phát bài học). Khoá tự nhiên: wishlist (user_id, course_id);
// note (user_id, lesson_id, content).
func (s *Seeder) SeedDemoWishlistsAndNotes(users map[string]model.User, courses map[string]model.Course) error {
	for _, spec := range demoWishlists {
		user, err := demoUser(users, spec.Email)
		if err != nil {
			return err
		}
		course, err := demoCourse(courses, spec.Slug)
		if err != nil {
			return err
		}
		w := model.Wishlist{UserID: user.ID, CourseID: course.ID, CreatedAt: daysAgo(7)}
		if err := s.db.Where("user_id = ? AND course_id = ?", user.ID, course.ID).
			Attrs(w).FirstOrCreate(&w).Error; err != nil {
			return fmt.Errorf("failed to seed wishlist %s/%s: %w", spec.Email, spec.Slug, err)
		}
	}

	for _, spec := range demoNotes {
		user, err := demoUser(users, spec.Email)
		if err != nil {
			return err
		}
		course, err := demoCourse(courses, spec.Slug)
		if err != nil {
			return err
		}
		lessons, err := s.courseLessonsInOrder(course.ID)
		if err != nil {
			return err
		}
		if spec.LessonIndex >= len(lessons) {
			return fmt.Errorf("course %s has only %d lessons, note wants index %d", spec.Slug, len(lessons), spec.LessonIndex)
		}
		lesson := lessons[spec.LessonIndex]
		note := model.UserNote{UserID: user.ID, LessonID: lesson.ID, CourseID: ptr(course.ID),
			TimestampSecs: spec.Seconds, Content: spec.Content, IsBookmarked: spec.Bookmarked}
		if err := s.db.Where("user_id = ? AND lesson_id = ? AND content = ?", user.ID, lesson.ID, spec.Content).
			Attrs(note).FirstOrCreate(&note).Error; err != nil {
			return fmt.Errorf("failed to seed note for %s: %w", spec.Email, err)
		}
	}
	return nil
}
