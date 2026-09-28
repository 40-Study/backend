package service

// Review đối kháng PR #79: quiz gắn khoá học (course_id/lesson_id) theo đúng D4 (ẩn với người
// ngoài khi khoá chưa xuất bản) và Q5 (khoá đang chờ duyệt không tạo/sửa quiz được).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type guardQuizRepo struct {
	repository.QuizRepositoryInterface
	quiz *model.Quiz
}

func (r *guardQuizRepo) GetQuizByID(context.Context, uuid.UUID) (*model.Quiz, error) {
	return r.quiz, nil
}
func (r *guardQuizRepo) GetQuizWithQuestions(context.Context, uuid.UUID) (*model.Quiz, error) {
	return r.quiz, nil
}
func (r *guardQuizRepo) ListQuizzes(context.Context, *uuid.UUID, *uuid.UUID, *uuid.UUID, int, int) ([]model.Quiz, int64, error) {
	return []model.Quiz{*r.quiz}, 1, nil
}

// quizFixtures: 1 quiz gắn thẳng course_id, 1 quiz gắn lesson_id (lần course qua section).
func quizFixtures(f *editLockFixture) map[string]*model.Quiz {
	cid, lid := f.courseID(), f.lessonID()
	byCourse := &model.Quiz{Title: "QA-quiz-khoa", CourseID: &cid}
	byCourse.ID = uuid.New()
	byLesson := &model.Quiz{Title: "QA-quiz-bai", LessonID: &lid}
	byLesson.ID = uuid.New()
	return map[string]*model.Quiz{"course_id": byCourse, "lesson_id": byLesson}
}

func quizSvc(f *editLockFixture, q *model.Quiz) *QuizService {
	return NewQuizService(&guardQuizRepo{quiz: q}, nil, f.course, f.sections, f.lessons, nil, f.enrollment)
}

func TestQuizCourseVisibility_DraftQuizHiddenFromOtherTeacher(t *testing.T) {
	ctx := context.Background()
	for _, status := range []string{model.CourseStatusDraft, model.CourseStatusPendingReview, model.CourseStatusRejected} {
		f := newEditLockFixture(status)
		f.lessons.lesson.IsPreview = true // bài preview không được là cửa sau
		student := uuid.New()
		f.enrollment.enrolledUser = student
		for attach, q := range quizFixtures(f) {
			svc := quizSvc(f, q)
			stranger := uuid.New()
			if _, err := svc.GetQuizByID(ctx, q.ID, stranger, false); !errors.Is(err, ErrCourseHidden) {
				t.Errorf("%s/%s: GV khác GetQuizByID err=%v, muốn ErrCourseHidden", status, attach, err)
			}
			list, err := svc.GetAllQuizzes(ctx, nil, nil, nil, stranger, false, 1, 10)
			if err != nil || len(list.Data) != 0 {
				t.Errorf("%s/%s: GV khác GetAllQuizzes = %v (err=%v), muốn danh sách rỗng", status, attach, list, err)
			}
			for who, v := range map[string]struct {
				id    uuid.UUID
				admin bool
			}{"chủ khoá": {f.owner, false}, "admin": {uuid.New(), true}, "học viên đã ghi danh": {student, false}} {
				if _, err := svc.GetQuizByID(ctx, q.ID, v.id, v.admin); errors.Is(err, ErrCourseHidden) {
					t.Errorf("%s/%s: %s bị ẩn quiz", status, attach, who)
				}
			}
		}
	}
	// Khoá đã xuất bản: quiz không bị ẩn bởi luật khoá riêng tư.
	f := newEditLockFixture(model.CourseStatusPublished)
	f.lessons.lesson.IsPreview = true
	for attach, q := range quizFixtures(f) {
		if err := quizSvc(f, q).ensureQuizCourseVisible(context.Background(), q, uuid.New()); err != nil {
			t.Errorf("published/%s: %v", attach, err)
		}
	}
}

func TestQuizCourseEditLock_PendingReviewBlocksQuizWrites(t *testing.T) {
	ctx := context.Background()
	f := newEditLockFixture(model.CourseStatusPendingReview)
	for attach, q := range quizFixtures(f) {
		svc := quizSvc(f, q)
		if err := svc.EnsureQuizCourseEditable(ctx, q.ID); !errors.Is(err, ErrCourseLockedForReview) {
			t.Errorf("%s: sửa quiz khoá chờ duyệt err=%v, muốn ErrCourseLockedForReview", attach, err)
		}
		if err := svc.EnsureNewQuizCourseEditable(ctx, f.owner, false, q.LessonID, q.CourseID); !errors.Is(err, ErrCourseLockedForReview) {
			t.Errorf("%s: tạo quiz khoá chờ duyệt err=%v, muốn ErrCourseLockedForReview", attach, err)
		}
	}
	for _, status := range []string{model.CourseStatusDraft, model.CourseStatusRejected, model.CourseStatusPublished} {
		f := newEditLockFixture(status)
		for attach, q := range quizFixtures(f) {
			if err := quizSvc(f, q).EnsureQuizCourseEditable(ctx, q.ID); err != nil {
				t.Errorf("%s/%s: quiz phải sửa được, err=%v", status, attach, err)
			}
		}
	}
	// Quiz buổi live (session_id) và quiz không tồn tại: không thuộc phạm vi khoá.
	sid := uuid.New()
	live := &model.Quiz{SessionID: &sid}
	if err := quizSvc(f, live).EnsureQuizCourseEditable(ctx, uuid.New()); err != nil {
		t.Errorf("quiz live: %v", err)
	}
	if err := quizSvc(f, nil).EnsureQuizCourseEditable(ctx, uuid.New()); err != nil {
		t.Errorf("quiz không tồn tại: %v", err)
	}
}

// otherCourse thêm khoá B (cùng chủ khoá A) vào repo của fixture.
func otherCourse(f *editLockFixture, status string) *model.Course {
	b := &model.Course{InstructorID: f.owner, Status: status}
	b.ID = uuid.New()
	f.course.others = map[uuid.UUID]*model.Course{b.ID: b}
	return b
}

// Re-review vòng 2: {lesson_id: bài khoá A đang chờ duyệt, course_id: khoá nháp B} từng lách Q5
// (guard chỉ xét course_id). Tạo quiz: 2 khoá lệch -> 400; quiz ĐÃ CÓ gắn lệch: xét MỌI khoá.
func TestQuizCourseGuard_LessonAndCourseMismatch(t *testing.T) {
	ctx := context.Background()
	f := newEditLockFixture(model.CourseStatusPendingReview)
	b := otherCourse(f, model.CourseStatusDraft)
	lid, aID := f.lessonID(), f.courseID()
	svc := quizSvc(f, nil)
	for who, admin := range map[string]bool{"chủ khoá": false, "admin": true} {
		if err := svc.EnsureNewQuizCourseEditable(ctx, f.owner, admin, &lid, &b.ID); !errors.Is(err, ErrQuizCourseMismatch) {
			t.Errorf("%s: lesson khoá A + course_id khoá B err=%v, muốn ErrQuizCourseMismatch", who, err)
		}
	}
	if err := svc.EnsureNewQuizCourseEditable(ctx, f.owner, false, &lid, &aID); !errors.Is(err, ErrCourseLockedForReview) {
		t.Errorf("lesson + course_id cùng khoá A chờ duyệt: err=%v, muốn ErrCourseLockedForReview", err)
	}

	// Quiz đã có (dữ liệu tạo trước bản vá): gắn bài A (chờ duyệt) + course_id B (nháp).
	mixed := &model.Quiz{Title: "QA-quiz-lech", LessonID: &lid, CourseID: &b.ID}
	mixed.ID = uuid.New()
	if err := quizSvc(f, mixed).EnsureQuizCourseEditable(ctx, mixed.ID); !errors.Is(err, ErrCourseLockedForReview) {
		t.Errorf("quiz gắn lệch: sửa err=%v, muốn ErrCourseLockedForReview (khoá A đang chờ duyệt)", err)
	}

	// Đọc: bài thuộc khoá nháp, course_id là khoá đã xuất bản -> người ngoài vẫn bị ẩn.
	g := newEditLockFixture(model.CourseStatusDraft)
	pub := otherCourse(g, model.CourseStatusPublished)
	glid := g.lessonID()
	hidden := &model.Quiz{Title: "QA-quiz-an", LessonID: &glid, CourseID: &pub.ID}
	hidden.ID = uuid.New()
	if err := quizSvc(g, hidden).ensureQuizCourseVisible(ctx, hidden, uuid.New()); !errors.Is(err, ErrCourseHidden) {
		t.Errorf("quiz gắn bài khoá nháp + course_id khoá published: err=%v, muốn ErrCourseHidden", err)
	}
}

// Re-review vòng 2: IDOR — giảng viên khác tạo quiz trong khoá của mình (có từ trước #79).
func TestQuizCourseGuard_CreateRequiresCourseOwner(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		status string
		want   error
	}{
		{model.CourseStatusPublished, ErrQuizCourseNotOwner},
		{model.CourseStatusDraft, ErrCourseHidden}, // khoá chưa xuất bản: không lộ sự tồn tại
	} {
		f := newEditLockFixture(tc.status)
		for attach, q := range quizFixtures(f) {
			svc := quizSvc(f, q)
			if err := svc.EnsureNewQuizCourseEditable(ctx, uuid.New(), false, q.LessonID, q.CourseID); !errors.Is(err, tc.want) {
				t.Errorf("%s/%s: GV khác tạo quiz err=%v, muốn %v", tc.status, attach, err, tc.want)
			}
			if err := svc.EnsureNewQuizCourseEditable(ctx, f.owner, false, q.LessonID, q.CourseID); err != nil {
				t.Errorf("%s/%s: chủ khoá tạo quiz err=%v", tc.status, attach, err)
			}
			if err := svc.EnsureNewQuizCourseEditable(ctx, uuid.New(), true, q.LessonID, q.CourseID); err != nil {
				t.Errorf("%s/%s: admin tạo quiz err=%v", tc.status, attach, err)
			}
		}
	}
}