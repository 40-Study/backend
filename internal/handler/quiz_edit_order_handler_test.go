package handler

// Merge PR #80 với #79: route sửa quiz có hai lớp kiểm, CourseEditLock (#79, khoá học đang chờ
// duyệt -> 409) và quyền sửa của PR #80 (người tạo / chủ khoá / admin -> 403 QUIZ_FORBIDDEN). Người
// lạ phải nhận 403 bất kể khoá học đang ở trạng thái nào; chỉ người có quyền sửa mới thấy 409.

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
)

func TestQuizEditRoutes_NguoiLa403TruocKhoaChoDuyet409(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mkUser := func(kind string) uuid.UUID {
		s := uuid.NewString()
		u := model.User{Email: "qa-quiz-order-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-" + kind + s[:8]}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user: %v", err)
		}
		return u.ID
	}
	teacher, stranger := mkUser("teacher"), mkUser("stranger")
	mkCourseQuiz := func(status string) (quizID, questionID uuid.UUID) {
		c := model.Course{InstructorID: teacher, Title: "Khoá " + status, Slug: "qa-order-" + uuid.NewString(), Price: decimal.NewFromInt(1)}
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("tạo khoá: %v", err)
		}
		if err := db.Model(&model.Course{}).Where("id = ?", c.ID).Update("status", status).Error; err != nil {
			t.Fatalf("đặt trạng thái khoá: %v", err)
		}
		q := model.Quiz{Title: "Quiz " + status, TriggerType: "manual", CourseID: &c.ID, CreatedBy: &teacher}
		if err := db.Create(&q).Error; err != nil {
			t.Fatalf("tạo quiz: %v", err)
		}
		question := model.Question{QuizID: q.ID, QuestionText: "1+1?", QuestionType: "essay", Points: decimal.NewFromInt(1), DisplayOrder: 1}
		if err := db.Create(&question).Error; err != nil {
			t.Fatalf("tạo câu hỏi: %v", err)
		}
		return q.ID, question.ID
	}
	pending, pendingQ := mkCourseQuiz(model.CourseStatusPendingReview)
	published, publishedQ := mkCourseQuiz(model.CourseStatusPublished)

	svc := service.NewQuizService(repository.NewQuizRepository(db), nil, repository.NewCourseRepository(db),
		repository.NewSectionRepository(db), repository.NewLessonRepository(db), nil, repository.NewEnrollmentRepository(db))
	h := NewQuizHandler(svc, nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if id, err := uuid.Parse(c.Get("X-Test-User")); err == nil {
			c.Locals("user_id", id)
		}
		return c.Next()
	})
	app.Put("/quizzes/:id", h.CourseEditLock("id"), h.UpdateQuiz)
	app.Delete("/quizzes/:id", h.CourseEditLock("id"), h.DeleteQuiz)
	app.Put("/quizzes/:quizId/questions/:id", h.CourseEditLock("quizId"), h.UpdateQuestion)

	routes := func(quizID, questionID uuid.UUID) [][3]string {
		q := "/quizzes/" + quizID.String()
		return [][3]string{
			{"PUT", q, `{"title":"Sửa"}`},
			{"PUT", q + "/questions/" + questionID.String(), `{"question_text":"Sửa"}`},
			{"DELETE", q, ""},
		}
	}
	for name, ids := range map[string][2]uuid.UUID{"khoá chờ duyệt": {pending, pendingQ}, "khoá đã xuất bản": {published, publishedQ}} {
		for _, r := range routes(ids[0], ids[1]) {
			code, body := callAs(t, app, stranger, r[0], r[1], r[2])
			if code != 403 || body["code"] != "QUIZ_FORBIDDEN" {
				t.Errorf("%s: người lạ %s %s muốn 403 QUIZ_FORBIDDEN, nhận %d %v", name, r[0], r[1], code, body)
			}
		}
	}
	for _, r := range routes(pending, pendingQ) {
		code, body := callAs(t, app, teacher, r[0], r[1], r[2])
		if code != 409 || body["code"] != CourseLockedCode {
			t.Errorf("chủ khoá %s %s khi khoá chờ duyệt: muốn 409 %s, nhận %d %v", r[0], r[1], CourseLockedCode, code, body)
		}
	}
	if code, body := callAs(t, app, teacher, "PUT", "/quizzes/"+published.String(), `{"title":"Chủ sửa"}`); code != 200 {
		t.Errorf("chủ khoá sửa quiz khoá đã xuất bản: muốn 200, nhận %d %v", code, body)
	}
}
