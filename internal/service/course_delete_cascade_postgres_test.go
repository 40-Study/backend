package service

// QA2 Q2-01 — xoá khoá học phải dọn cả cây con trong MỘT transaction: sections và quizzes (kể cả quiz
// gắn vào bài) bị xoá mềm theo, dòng nội dung loại quiz trỏ vào chúng bị gỡ, và các danh sách của học
// viên / giảng viên không còn hiện chúng. lessons và lesson_contents KHÔNG có cột deleted_at (xoá bài
// là xoá thật, xem LessonRepository.Delete) nên được ẩn bằng cách xoá mềm chương chứa chúng — đúng quy
// ước của EnrollmentRepository.GetCourseIDByLessonID / CourseRepository.RecalculateLessonStats.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type courseTree struct {
	course, section, lesson, content, quiz, courseQuiz, quizContent uuid.UUID
}

type courseCascadeFixture struct {
	t       *testing.T
	f       *contestFixture
	teacher uuid.UUID
	svc     *CourseService
}

func newCourseCascadeFixture(t *testing.T) *courseCascadeFixture {
	f := newContestFixture(t)
	svc := NewCourseService(repository.NewCourseRepository(f.db), nil, nil, repository.NewEnrollmentRepository(f.db))
	return &courseCascadeFixture{t: t, f: f, teacher: f.user("teacher"), svc: svc}
}

// tree dựng 1 khoá: 1 chương, 1 bài có video + nội dung quiz, 1 quiz gắn bài, 1 quiz cấp khoá.
func (c *courseCascadeFixture) tree(title string) courseTree {
	c.t.Helper()
	db := c.f.db
	must := func(err error) {
		c.t.Helper()
		if err != nil {
			c.t.Fatalf("dựng cây khoá học: %v", err)
		}
	}
	course := model.Course{InstructorID: c.teacher, Title: title, Slug: "q201-" + uuid.NewString(), Price: decimal.NewFromInt(100000), Status: model.CourseStatusDraft}
	must(db.Create(&course).Error)
	section := model.Section{CourseID: course.ID, Title: "Chương 1", DisplayOrder: 1}
	must(db.Create(&section).Error)
	lesson := model.Lesson{SectionID: section.ID, Title: "Bài 1", DisplayOrder: 1}
	must(db.Create(&lesson).Error)
	video := "https://example.test/v.mp4"
	content := model.LessonContent{LessonID: lesson.ID, Type: model.LessonContentTypeVideo, VideoURL: &video}
	must(db.Create(&content).Error)
	lessonQuiz := model.Quiz{LessonID: &lesson.ID, CourseID: &course.ID, CreatedBy: &c.teacher, Title: "Quiz bài", TriggerType: "manual"}
	must(db.Create(&lessonQuiz).Error)
	must(db.Create(&model.Question{QuizID: lessonQuiz.ID, QuestionText: "Câu 1", QuestionType: "single_choice", Points: decimal.NewFromInt(1), DisplayOrder: 1}).Error)
	quizContent := model.LessonContent{LessonID: lesson.ID, Type: model.LessonContentTypeQuiz, QuizID: &lessonQuiz.ID, DisplayOrder: 2}
	must(db.Create(&quizContent).Error)
	courseQuiz := model.Quiz{CourseID: &course.ID, CreatedBy: &c.teacher, Title: "Quiz khoá", TriggerType: "manual"}
	must(db.Create(&courseQuiz).Error)
	return courseTree{course.ID, section.ID, lesson.ID, content.ID, lessonQuiz.ID, courseQuiz.ID, quizContent.ID}
}

func (c *courseCascadeFixture) live(model interface{}, id uuid.UUID) bool {
	c.t.Helper()
	var n int64
	if err := c.f.db.Model(model).Where("id = ?", id).Count(&n).Error; err != nil {
		c.t.Fatalf("đếm: %v", err)
	}
	return n > 0
}

func TestDeleteCourse_CascadesSoftDeleteToSectionsAndQuizzes(t *testing.T) {
	c := newCourseCascadeFixture(t)
	doomed, kept := c.tree("Khoá bị xoá"), c.tree("Khoá giữ lại")
	student := c.f.user("student")
	enr := model.Enrollment{UserID: student, CourseID: doomed.course}
	if err := c.f.db.Create(&enr).Error; err != nil {
		t.Fatal(err)
	}
	keptEnr := model.Enrollment{UserID: student, CourseID: kept.course}
	if err := c.f.db.Create(&keptEnr).Error; err != nil {
		t.Fatal(err)
	}

	if err := c.svc.DeleteCourse(context.Background(), doomed.course, c.teacher, false); err != nil {
		t.Fatalf("DeleteCourse: %v", err)
	}

	for name, alive := range map[string]bool{
		"course":       c.live(&model.Course{}, doomed.course),
		"section":      c.live(&model.Section{}, doomed.section),
		"lesson quiz":  c.live(&model.Quiz{}, doomed.quiz),
		"course quiz":  c.live(&model.Quiz{}, doomed.courseQuiz),
		"quiz content": c.live(&model.LessonContent{}, doomed.quizContent),
	} {
		if alive {
			t.Errorf("%s của khoá đã xoá vẫn còn sống (deleted_at IS NULL)", name)
		}
	}
	for name, alive := range map[string]bool{
		"course":      c.live(&model.Course{}, kept.course),
		"section":     c.live(&model.Section{}, kept.section),
		"lesson":      c.live(&model.Lesson{}, kept.lesson),
		"lesson quiz": c.live(&model.Quiz{}, kept.quiz),
		"course quiz": c.live(&model.Quiz{}, kept.courseQuiz),
		"quiz ctnt":   c.live(&model.LessonContent{}, kept.quizContent),
	} {
		if !alive {
			t.Errorf("%s của khoá KHÁC bị xoá lây", name)
		}
	}

	// Dữ liệu học (bài/nội dung video, tiến độ) không bị xoá cứng: xoá mềm là có thể đối soát.
	if !c.live(&model.Lesson{}, doomed.lesson) || !c.live(&model.LessonContent{}, doomed.content) {
		t.Error("bài học / nội dung video bị xoá cứng, muốn chỉ bị ẩn qua chương đã xoá mềm")
	}

	ctx := context.Background()
	quizzes, total, err := repository.NewQuizRepository(c.f.db).GetQuizzesByCreator(ctx, c.teacher, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(quizzes) != 2 {
		t.Errorf("danh sách quiz của giảng viên: total=%d len=%d, muốn 2 (chỉ khoá còn lại)", total, len(quizzes))
	}
	for _, q := range quizzes {
		if q.ID == doomed.quiz || q.ID == doomed.courseQuiz {
			t.Errorf("danh sách quiz của giảng viên vẫn hiện quiz %s của khoá đã xoá", q.ID)
		}
	}

	mine, _, err := repository.NewEnrollmentRepository(c.f.db).GetByUserID(ctx, student, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].CourseID != kept.course {
		t.Errorf("khoá học của học viên: %d ghi danh, muốn 1 (khoá còn lại) — khoá đã xoá không được hiện", len(mine))
	}
	if _, err := repository.NewEnrollmentRepository(c.f.db).GetCourseIDByLessonID(ctx, doomed.lesson); err == nil {
		t.Error("GetCourseIDByLessonID vẫn tìm ra khoá cho bài của khoá đã xoá")
	}
}

// Tất cả hoặc không gì cả: bước dọn quiz lỗi giữa chừng thì khoá và chương KHÔNG bị xoá.
func TestDeleteCourse_CascadeIsAtomic(t *testing.T) {
	c := newCourseCascadeFixture(t)
	tree := c.tree("Khoá lỗi giữa chừng")
	db := c.f.db
	if err := db.Exec(`CREATE OR REPLACE FUNCTION q201_fail_quiz_update() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'q201 injected failure'; END; $$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER q201_fail BEFORE UPDATE ON quizzes FOR EACH ROW EXECUTE FUNCTION q201_fail_quiz_update()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DROP TRIGGER IF EXISTS q201_fail ON quizzes`)
		db.Exec(`DROP FUNCTION IF EXISTS q201_fail_quiz_update()`)
	})

	if err := c.svc.DeleteCourse(context.Background(), tree.course, c.teacher, false); err == nil {
		t.Fatal("DeleteCourse trả nil dù bước xoá quiz lỗi")
	}
	if !c.live(&model.Course{}, tree.course) || !c.live(&model.Section{}, tree.section) || !c.live(&model.Quiz{}, tree.quiz) {
		t.Fatal("cascade không atomic: khoá/chương/quiz đã bị xoá dù giao dịch lỗi")
	}
}
