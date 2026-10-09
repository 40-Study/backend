package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/database"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
)

// QA 261009 H1: wireQuizLessonCompletion PHẢI nối QuizService với EnrollmentService. Bỏ lời gọi (hoặc bỏ nội dung
// hàm) thì đỗ quiz chính thức không chốt bài học. Kiểm bằng HÀNH VI thật vì lessonCompleter là field private.
func TestWireQuizLessonCompletion_PassingOfficialQuizCompletesLesson(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	repos := InitRepositories(db)
	s := &Services{
		Quiz:       service.NewQuizService(repos.Quiz, nil, repos.Course, repos.Section, repos.Lesson, repos.Livestream, repos.Enrollment),
		Enrollment: service.NewEnrollmentService(repos.Enrollment, repos.Course, repos.Lesson, repos.VideoUpload),
	}
	wireQuizLessonCompletion(s)

	teacher := model.User{Email: "qa-wire-teacher@wire.test", UserName: "wire-teacher", PasswordHash: "x"}
	student := model.User{Email: "qa-wire-student@wire.test", UserName: "wire-student", PasswordHash: "x"}
	for _, u := range []*model.User{&teacher, &student} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	course := model.Course{InstructorID: teacher.ID, Title: "QA wire", Slug: "qa-wire-" + uuid.NewString()[:8], Status: "published"}
	if err := db.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	sec := model.Section{CourseID: course.ID, Title: "s", DisplayOrder: 1}
	if err := db.Create(&sec).Error; err != nil {
		t.Fatal(err)
	}
	lesson := model.Lesson{SectionID: sec.ID, Title: "l", DisplayOrder: 1}
	if err := db.Create(&lesson).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Enrollment{UserID: student.ID, CourseID: course.ID}).Error; err != nil {
		t.Fatal(err)
	}
	quiz := model.Quiz{LessonID: &lesson.ID, Title: "QA wire quiz", TriggerType: "manual", CreatedBy: &teacher.ID, PassPercentage: decimal.NewFromInt(70)}
	if err := db.Create(&quiz).Error; err != nil {
		t.Fatal(err)
	}
	q := model.Question{QuizID: quiz.ID, QuestionText: "1+1?", QuestionType: "single_choice", Points: decimal.NewFromInt(1), DisplayOrder: 1}
	if err := db.Create(&q).Error; err != nil {
		t.Fatal(err)
	}
	right := model.QuestionAnswer{QuestionID: q.ID, AnswerText: "2", IsCorrect: true, DisplayOrder: 1}
	if err := db.Create(&right).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	started, err := s.Quiz.StartQuiz(ctx, quiz.ID, student.ID, false, dto.StartQuizDTO{})
	if err != nil {
		t.Fatalf("StartQuiz: %v", err)
	}
	attemptID := started.AttemptID.String()
	res, err := s.Quiz.SubmitQuiz(ctx, quiz.ID, student.ID, false, dto.SubmitQuizDTO{
		AttemptID: &attemptID,
		Answers:   []dto.SubmitAnswerDTO{{QuestionID: q.ID.String(), SelectedAnswerIDs: []string{right.ID.String()}}},
	})
	if err != nil || res.IsPassed == nil || !*res.IsPassed {
		t.Fatalf("SubmitQuiz: err=%v res=%+v, muon da do", err, res)
	}

	var p model.LessonProgress
	if err := db.First(&p, "user_id = ? AND lesson_id = ?", student.ID, lesson.ID).Error; err != nil || p.Status != "completed" {
		t.Fatalf("bai hoc phai completed sau khi do quiz (wireQuizLessonCompletion chua noi?): err=%v status=%q", err, p.Status)
	}
}
