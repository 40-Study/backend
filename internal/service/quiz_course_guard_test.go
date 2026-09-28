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
		if err := svc.EnsureNewQuizCourseEditable(ctx, q.LessonID, q.CourseID); !errors.Is(err, ErrCourseLockedForReview) {
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
