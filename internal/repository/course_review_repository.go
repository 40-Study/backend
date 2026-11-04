package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// Phase 3 duyệt khoá học (2026-09-28) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-03-course-teacher-approval.md
var (
	ErrCourseReviewNotFound      = errors.New("course not found")
	ErrCourseReviewNotOwner      = errors.New("forbidden: not the owner")
	ErrCourseInvalidReviewStatus = errors.New("invalid course status for this review action")
	// ErrCourseEmptyContent (D2, QA vòng 2): nộp duyệt khoá chưa có bài học nào — trước đây gửi
	// được khoá 0 bài lên hàng chờ admin.
	ErrCourseEmptyContent = errors.New("course must have at least one lesson before submitting for review")
)

// Khoá nộp duyệt còn thiếu thông tin bắt buộc (Q3-09, QA3): ảnh bìa, mô tả, giá của khoá có phí. Key ổn định cho web.
const (
	CourseMissingThumbnail   = "thumbnail_url"
	CourseMissingDescription = "description"
	CourseMissingPrice       = "price"
)

// courseMissingLabelsVI: tên tiếng Việt của từng thông tin còn thiếu, dùng dựng câu báo lỗi.
var courseMissingLabelsVI = map[string]string{
	CourseMissingThumbnail:   "ảnh bìa",
	CourseMissingDescription: "mô tả khoá học",
	CourseMissingPrice:       "giá bán (khoá có phí phải có giá lớn hơn 0)",
}

// ErrCourseIncomplete là sentinel để errors.Is cho CourseIncompleteError.
var ErrCourseIncomplete = errors.New("course is missing required information before submitting for review")

// CourseIncompleteError: nộp duyệt khi khoá thiếu ảnh bìa / mô tả / giá. Mang danh sách key (CourseMissing*) để handler
// trả 422 kèm câu tiếng Việt nêu từng mục còn thiếu.
type CourseIncompleteError struct {
	Missing []string
}

func (e *CourseIncompleteError) Error() string {
	return ErrCourseIncomplete.Error() + ": " + strings.Join(e.Missing, ", ")
}

func (e *CourseIncompleteError) Is(target error) bool { return target == ErrCourseIncomplete }

// MessageVI là câu tiếng Việt hiển thị thẳng cho giảng viên.
func (e *CourseIncompleteError) MessageVI() string {
	labels := make([]string, len(e.Missing))
	for i, k := range e.Missing {
		labels[i] = courseMissingLabelsVI[k]
	}
	return "Khoá học chưa đủ thông tin để gửi duyệt. Còn thiếu: " + strings.Join(labels, "; ") + "."
}

// CourseMissingForReview là hàm THUẦN: các thông tin bắt buộc khoá còn thiếu khi nộp duyệt (rỗng = đủ). Ảnh bìa và mô tả
// chỉ gồm khoảng trắng coi như thiếu; khoá có phí (is_free = false) phải có giá > 0 sau khi làm tròn 2 chữ số như cột DB.
func CourseMissingForReview(c *model.Course) []string {
	var missing []string
	if c.ThumbnailURL == nil || strings.TrimSpace(*c.ThumbnailURL) == "" {
		missing = append(missing, CourseMissingThumbnail)
	}
	if c.Description == nil || strings.TrimSpace(*c.Description) == "" {
		missing = append(missing, CourseMissingDescription)
	}
	if !c.IsFree && !c.Price.Round(2).IsPositive() {
		missing = append(missing, CourseMissingPrice)
	}
	return missing
}

// LessonWithoutContent: một bài học chưa có nội dung nào (QA T6).
type LessonWithoutContent struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

// CourseLessonsWithoutContentError (QA T6): nộp duyệt khi còn bài học rỗng. Mang danh sách bài để
// handler nêu tên từng bài cho giảng viên. Dùng errors.As để lấy; errors.Is(err, ErrCourseLessonsWithoutContent) cũng đúng.
type CourseLessonsWithoutContentError struct {
	Lessons []LessonWithoutContent
}

// ErrCourseLessonsWithoutContent là sentinel để errors.Is.
var ErrCourseLessonsWithoutContent = errors.New("every lesson must have content before submitting for review")

func (e *CourseLessonsWithoutContentError) Error() string {
	titles := make([]string, len(e.Lessons))
	for i, l := range e.Lessons {
		titles[i] = l.Title
	}
	return ErrCourseLessonsWithoutContent.Error() + ": " + strings.Join(titles, ", ")
}

func (e *CourseLessonsWithoutContentError) Is(target error) bool {
	return target == ErrCourseLessonsWithoutContent
}

// CourseReviewAction — các hành động của luồng duyệt.
type CourseReviewAction string

const (
	CourseActionSubmit  CourseReviewAction = "submit"
	CourseActionApprove CourseReviewAction = "approve"
	CourseActionReject  CourseReviewAction = "reject"
	// CourseActionWithdraw (Q5, QA vòng 2): giảng viên rút yêu cầu duyệt để sửa tiếp — khoá đang
	// chờ duyệt bị khoá sửa (service.ErrCourseLockedForReview), đây là đường duy nhất để mở lại.
	CourseActionWithdraw CourseReviewAction = "withdraw"
)

// EvaluateCourseReviewTransition là hàm THUẦN (không DB) quyết định trạng thái mới của khoá học
// cho 1 hành động duyệt — tách riêng để unit test mọi cặp (trạng thái hiện tại, hành động):
//   - submit : draft|rejected -> pending_review (giáo viên nộp / nộp lại)
//   - approve: pending_review -> published   (chỉ admin)
//   - reject : pending_review -> rejected    (chỉ admin)
//   - withdraw: pending_review -> draft      (giáo viên chủ khoá rút yêu cầu để sửa)
//
// Mọi cặp khác trả ErrCourseInvalidReviewStatus — đặc biệt KHÔNG có đường nào tới published ngoài
// approve, nên giáo viên không tự xuất bản được.
func EvaluateCourseReviewTransition(current string, action CourseReviewAction) (string, error) {
	switch action {
	case CourseActionSubmit:
		if current == model.CourseStatusDraft || current == model.CourseStatusRejected {
			return model.CourseStatusPendingReview, nil
		}
	case CourseActionApprove:
		if current == model.CourseStatusPendingReview {
			return model.CourseStatusPublished, nil
		}
	case CourseActionReject:
		if current == model.CourseStatusPendingReview {
			return model.CourseStatusRejected, nil
		}
	case CourseActionWithdraw:
		if current == model.CourseStatusPendingReview {
			return model.CourseStatusDraft, nil
		}
	}
	return "", ErrCourseInvalidReviewStatus
}

// AdminCourseReviewFilter — tham số hàng chờ duyệt khoá học của admin.
type AdminCourseReviewFilter struct {
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

type CourseReviewRepositoryInterface interface {
	// ApplyReviewAction khoá dòng course (FOR UPDATE), kiểm transition bằng
	// EvaluateCourseReviewTransition rồi ghi — 2 admin bấm duyệt/từ chối cùng lúc sẽ xếp hàng,
	// người thứ 2 thấy trạng thái đã đổi và nhận ErrCourseInvalidReviewStatus.
	// ownerID != nil: bắt buộc course.instructor_id == *ownerID (luồng giáo viên nộp duyệt).
	ApplyReviewAction(ctx context.Context, courseID uuid.UUID, action CourseReviewAction,
		ownerID *uuid.UUID, reviewerID *uuid.UUID, reason *string) (*model.Course, error)
	ListForReview(ctx context.Context, filter AdminCourseReviewFilter) ([]model.Course, int64, error)
}

type CourseReviewRepository struct {
	db *gorm.DB
}

func NewCourseReviewRepository(db *gorm.DB) *CourseReviewRepository {
	return &CourseReviewRepository{db: db}
}

func (r *CourseReviewRepository) ApplyReviewAction(
	ctx context.Context,
	courseID uuid.UUID,
	action CourseReviewAction,
	ownerID *uuid.UUID,
	reviewerID *uuid.UUID,
	reason *string,
) (*model.Course, error) {
	var result model.Course
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var course model.Course
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", courseID).First(&course).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCourseReviewNotFound
			}
			return err
		}
		if ownerID != nil && course.InstructorID != *ownerID {
			return ErrCourseReviewNotOwner
		}
		next, err := EvaluateCourseReviewTransition(course.Status, action)
		if err != nil {
			return err
		}
		if action == CourseActionSubmit {
			// Đếm TRONG transaction đang giữ khoá dòng course: nộp và xoá bài chạy song song thì
			// xoá bài đã phải qua ensureCourseEditable (khoá còn draft) — trường hợp xoá commit
			// sau lần đếm này vẫn có thể lọt, chấp nhận vì admin còn duyệt tay.
			// sections xoá mềm (deleted_at), lessons xoá cứng: bài trong chương đã xoá không tính.
			var lessonCount int64
			if err := tx.Model(&model.Lesson{}).
				Joins("JOIN sections ON sections.id = lessons.section_id AND sections.deleted_at IS NULL").
				Where("sections.course_id = ?", course.ID).
				Count(&lessonCount).Error; err != nil {
				return err
			}
			if lessonCount == 0 {
				return ErrCourseEmptyContent
			}
			// QA T6: mọi bài phải có nội dung — dòng lesson_contents HOẶC quiz gắn bài (chưa xoá mềm).
			// Cùng khoá dòng course nên cùng mức bảo đảm như phép đếm bài ở trên.
			var empty []LessonWithoutContent
			if err := tx.Table("lessons").
				Select("lessons.id AS id, lessons.title AS title").
				Joins("JOIN sections ON sections.id = lessons.section_id AND sections.deleted_at IS NULL").
				Where("sections.course_id = ?", course.ID).
				Where("NOT EXISTS (SELECT 1 FROM lesson_contents lc WHERE lc.lesson_id = lessons.id)").
				Where("NOT EXISTS (SELECT 1 FROM quizzes q WHERE q.lesson_id = lessons.id AND q.deleted_at IS NULL)").
				Order("sections.display_order, lessons.display_order, lessons.title").
				Scan(&empty).Error; err != nil {
				return err
			}
			if len(empty) > 0 {
				return &CourseLessonsWithoutContentError{Lessons: empty}
			}
			// Q3-09 (QA3): ảnh bìa, mô tả và giá (khoá có phí) cũng bắt buộc — trước đây chỉ kiểm nội dung bài học.
			if missing := CourseMissingForReview(&course); len(missing) > 0 {
				return &CourseIncompleteError{Missing: missing}
			}
		}

		now := time.Now()
		updates := map[string]interface{}{"status": next}
		switch action {
		case CourseActionSubmit:
			updates["submitted_at"] = now
		case CourseActionApprove:
			updates["published_at"] = now
			updates["reviewed_by"] = reviewerID
			updates["reviewed_at"] = now
			updates["rejection_reason"] = nil
		case CourseActionReject:
			updates["reviewed_by"] = reviewerID
			updates["reviewed_at"] = now
			updates["rejection_reason"] = reason
		case CourseActionWithdraw:
			// Về nháp như chưa từng nộp: hàng chờ admin xếp theo submitted_at, lần nộp sau ghi lại.
			updates["submitted_at"] = nil
		}
		if err := tx.Model(&model.Course{}).Where("id = ?", course.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", course.ID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *CourseReviewRepository) ListForReview(ctx context.Context, filter AdminCourseReviewFilter) ([]model.Course, int64, error) {
	var courses []model.Course
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Course{}).
		Joins("JOIN users ON users.id = courses.instructor_id").
		Where("courses.status = ?", filter.Status)
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		query = utils.ApplyKeywordSearch(query, kw, "courses.title", "users.email", "users.full_name")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Hàng chờ: khoá nộp gần nhất lên đầu; khoá chưa từng nộp (NULL) xuống cuối.
	if err := utils.ApplyPagination(query, filter.Page, filter.PageSize).
		Select("courses.*").
		Preload("Instructor").
		Order("courses.submitted_at DESC NULLS LAST").
		Order("courses.created_at DESC").
		Find(&courses).Error; err != nil {
		return nil, 0, err
	}
	return courses, total, nil
}
