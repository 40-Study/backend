package service

// Smoke QA2 (13/10): khoá `archived` ngừng bán nhưng người ĐÃ ghi danh phải học được tiếp. Trước đây trang /learn của web
// báo "Không tìm thấy khóa học" vì GET /courses/slug/:slug lọc cứng status=published, trong khi canViewCourse (đã đúng)
// chỉ được các đường khác dùng. Test này đi qua CÁC SERVICE THẬT trên Postgres cho mọi lời gọi mà trang học thực hiện,
// với cùng một khoá ở hai trạng thái: published (đối chứng) và archived.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type learnerWorld struct {
	course                   model.Course
	lesson                   model.Lesson
	quiz                     model.Quiz
	owner, student, outsider model.User
	admin                    model.User

	courses  *CourseService
	sections *SectionService
	lessons  *LessonService
	contents *LessonContentService
	quizzes  *QuizService
	enrolls  *EnrollmentService
}

func newLearnerWorld(t *testing.T, status string) *learnerWorld {
	t.Helper()
	f := newS2Fixture(t)
	w := &learnerWorld{owner: f.user("owner"), student: f.user("student"), outsider: f.user("outsider"), admin: f.user("admin")}
	w.course = f.course(w.owner)
	w.lesson = f.lessonOf(w.course)
	w.quiz = model.Quiz{LessonID: &w.lesson.ID, Title: "quiz bai hoc"}
	if err := f.db.Create(&w.quiz).Error; err != nil {
		t.Fatalf("tao quiz: %v", err)
	}
	if err := f.db.Create(&model.Enrollment{UserID: w.student.ID, CourseID: w.course.ID, TotalLessons: 1}).Error; err != nil {
		t.Fatalf("ghi danh: %v", err)
	}
	if err := f.db.Exec("UPDATE courses SET status = ? WHERE id = ?", status, w.course.ID).Error; err != nil {
		t.Fatalf("doi trang thai: %v", err)
	}

	courseRepo := repository.NewCourseRepository(f.db)
	sectionRepo := repository.NewSectionRepository(f.db)
	lessonRepo := repository.NewLessonRepository(f.db)
	enrollRepo := repository.NewEnrollmentRepository(f.db)
	w.courses = NewCourseService(courseRepo, repository.NewCategoryRepository(f.db), repository.NewTagRepository(f.db), enrollRepo)
	w.sections = NewSectionService(sectionRepo, courseRepo, enrollRepo)
	w.lessons = NewLessonService(lessonRepo, sectionRepo, courseRepo, enrollRepo)
	w.contents = NewLessonContentService(lessonRepo, sectionRepo, courseRepo, enrollRepo, nil)
	w.quizzes = NewQuizService(repository.NewQuizRepository(f.db), nil, courseRepo, sectionRepo, lessonRepo, repository.NewLivestreamRepository(f.db), enrollRepo)
	w.enrolls = NewEnrollmentService(enrollRepo, courseRepo, lessonRepo, repository.NewVideoUploadRepository(f.db))
	return w
}

// calls: mọi lời gọi của trang /learn, theo người xem.
func (w *learnerWorld) calls(viewer uuid.UUID, isAdmin bool) map[string]error {
	ctx := context.Background()
	out := map[string]error{}
	_, out["course-by-slug"] = w.courses.GetCourseBySlug(ctx, w.course.Slug, viewer, isAdmin)
	_, out["course-by-id"] = w.courses.GetCourseByID(ctx, w.course.ID, viewer, isAdmin)
	_, out["sections"] = w.sections.GetAllSections(ctx, w.course.ID, viewer, isAdmin)
	_, out["lesson"] = w.lessons.GetLessonByID(ctx, w.lesson.ID, viewer, isAdmin)
	_, out["lesson-contents"] = w.contents.GetContentsByLessonID(ctx, w.lesson.ID, viewer, isAdmin)
	_, out["quiz-by-id"] = w.quizzes.GetQuizByID(ctx, w.quiz.ID, viewer, isAdmin)
	_, out["quiz-start"] = w.quizzes.StartQuiz(ctx, w.quiz.ID, viewer, isAdmin, dto.StartQuizDTO{})
	// Danh sách quiz lọc từng quiz ẩn thay vì báo lỗi: danh sách rỗng = bị ẩn (người trong cuộc phải thấy đúng 1 quiz).
	list, err := w.quizzes.GetAllQuizzes(ctx, &w.lesson.ID, nil, nil, viewer, isAdmin, 1, 20)
	if err == nil && (list == nil || len(list.Data) != 1) {
		err = ErrCourseHidden
	}
	out["quizzes-by-lesson"] = err
	return out
}

func TestLearnerPaths_KhoaPublishedVaArchived_NguoiTrongCuocDuocHoc(t *testing.T) {
	for _, status := range []string{model.CourseStatusPublished, model.CourseStatusArchived} {
		t.Run(status, func(t *testing.T) {
			w := newLearnerWorld(t, status)
			viewers := map[string]struct {
				id      uuid.UUID
				isAdmin bool
			}{
				"hoc vien da ghi danh": {w.student.ID, false},
				"chu khoa":             {w.owner.ID, false},
				"admin":                {w.admin.ID, true},
			}
			for who, v := range viewers {
				for call, err := range w.calls(v.id, v.isAdmin) {
					if err != nil {
						t.Errorf("%s / %s: %v (nguoi trong cuoc phai doc duoc khoa %s)", who, call, err, status)
					}
				}
			}
			// Học viên ghi danh vẫn cập nhật được tiến độ (đường ghi của trang học).
			if _, err := w.enrolls.UpdateLessonProgress(context.Background(), w.student.ID, w.lesson.ID,
				dto.UpdateLessonProgressDTO{}, false); err != nil && !errors.Is(err, ErrLessonLocked) {
				t.Errorf("cap nhat tien do khoa %s: %v", status, err)
			}
			list, err := w.enrolls.GetMyEnrollments(context.Background(), w.student.ID, 1, 20)
			if err != nil || list == nil || len(list.Enrollments) != 1 {
				t.Errorf("GetMyEnrollments khoa %s: %v %+v (khoa da ghi danh phai con trong danh sach)", status, err, list)
			}
		})
	}
}

func TestLearnerPaths_KhoaArchived_NguoiLaVaKhachBi404(t *testing.T) {
	w := newLearnerWorld(t, model.CourseStatusArchived)
	for who, id := range map[string]uuid.UUID{"nguoi la": w.outsider.ID, "khach": uuid.Nil} {
		for call, err := range w.calls(id, false) {
			if !errors.Is(err, ErrCourseHidden) && !errors.Is(err, ErrQuizNotFound) {
				t.Errorf("%s / %s: err = %v, muon an khoa archived (ErrCourseHidden)", who, call, err)
			}
		}
	}
}
