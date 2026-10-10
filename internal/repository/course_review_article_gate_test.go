package repository

// QA T6 + plan 261008 phase 1: cổng "mọi bài phải có nội dung" của nộp duyệt khoá học phải coi dòng article / quiz
// là nội dung — KHÔNG cần thêm logic, nhưng cổng đọc thẳng lesson_contents nên cần một test ghim hành vi này.
// Chạy trong schema tạm đã migrate như lúc API khởi động (cột/CHK mới), không đụng DB dev dùng chung:
// bỏ mệnh đề NOT EXISTS lesson_contents của cổng thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestCourseReviewGate_ArticleOnlyAndQuizOnlyLessonsCountAsHavingContent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()

	teacher := model.User{Email: "qa-gate-" + uuid.NewString() + "@40study.test", PasswordHash: "x", UserName: "QA-gate-" + uuid.NewString()[:8]}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	course := model.Course{InstructorID: teacher.ID, Title: "QA-gate", Slug: "qa-gate-" + uuid.NewString(), Status: model.CourseStatusDraft}
	if err := db.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	section := apvSection(t, db, course.ID)
	articleOnly := apvEmptyLesson(t, db, section.ID, "Chỉ có bài đọc", 1)
	quizOnly := apvEmptyLesson(t, db, section.ID, "Chỉ có dòng quiz", 2)
	empty := apvEmptyLesson(t, db, section.ID, "Bài rỗng", 3)

	body := "<p>Nội dung</p>"
	if err := db.Create(&model.LessonContent{LessonID: articleOnly.ID, Type: model.LessonContentTypeArticle, ArticleBody: &body}).Error; err != nil {
		t.Fatalf("tạo dòng article: %v", err)
	}
	// Quiz KHÔNG gắn bài (lesson_id NULL) nên mệnh đề "quiz gắn bài" của cổng không cứu được: chỉ dòng
	// lesson_contents type='quiz' làm bài này có nội dung.
	standalone := model.Quiz{Title: "QA quiz", TriggerType: "manual"}
	if err := db.Create(&standalone).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.LessonContent{LessonID: quizOnly.ID, Type: model.LessonContentTypeQuiz, QuizID: &standalone.ID}).Error; err != nil {
		t.Fatalf("tạo dòng quiz: %v", err)
	}

	repo := NewCourseReviewRepository(db)
	_, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &teacher.ID, nil, nil)
	var missing *CourseLessonsWithoutContentError
	if !errors.As(err, &missing) || !errors.Is(err, ErrCourseLessonsWithoutContent) {
		t.Fatalf("bài rỗng vẫn phải bị báo: err=%v", err)
	}
	if len(missing.Lessons) != 1 || missing.Lessons[0].ID != empty.ID {
		t.Fatalf("chỉ bài rỗng được báo, bài đọc/quiz phải qua cổng; nhận %+v", missing.Lessons)
	}

	apvVideoContent(t, db, empty.ID)
	if _, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &teacher.ID, nil, nil); err != nil {
		t.Fatalf("khi mọi bài có nội dung (bài đọc, quiz, video) phải nộp được: %v", err)
	}
}
