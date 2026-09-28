package handler

// Lane B2 "Cuộc thi" — contract §9 B2 (a), (b) ở tầng HTTP: quiz gắn cuộc thi trả 403
// {code:"QUIZ_LOCKED_BY_CONTEST"} cho học viên trên mọi route quiz, 200/201 cho người tạo cuộc thi,
// và 409 {code:"CONTEST_QUIZ_LOCKED"} khi sửa lúc cuộc thi đã công bố. QuizService thật trên Postgres
// thật (schema tạm); ContestService (lane B1) được thay bằng gate giả. Nhánh admin kiểm ở test
// service (isAdminActor cần PermissionChecker thật).

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
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

type lockedQuizGate struct {
	quizID, creator uuid.UUID
}

func (g lockedQuizGate) CheckQuizAccess(_ context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if quizID == g.quizID && !isAdmin && userID != g.creator {
		return service.ErrQuizLockedByContest
	}
	return nil
}

func (g lockedQuizGate) CheckQuizEditable(_ context.Context, quizID uuid.UUID) error {
	if quizID == g.quizID {
		return service.ErrQuizEditLockedByContest
	}
	return nil
}

// mountQuizRoutesAs dựng đúng các đường dẫn của router/quiz_router.go, với user_id lấy từ header
// X-Test-User (thay cho AuthMiddleware).
func mountQuizRoutesAs(h *QuizHandler) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if id, err := uuid.Parse(c.Get("X-Test-User")); err == nil {
			c.Locals("user_id", id)
		}
		return c.Next()
	})
	app.Get("/quizzes", h.GetAllQuizzes)
	app.Get("/quizzes/:id", h.GetQuizByID)
	app.Put("/quizzes/:id", h.UpdateQuiz)
	app.Delete("/quizzes/:id", h.DeleteQuiz)
	app.Post("/quizzes/:id/duplicate", h.DuplicateQuiz)
	app.Get("/quizzes/:quizId/questions", h.GetQuestionsByQuiz)
	app.Post("/quizzes/:quizId/questions", h.CreateQuestion)
	app.Put("/quizzes/:quizId/questions/:id", h.UpdateQuestion)
	app.Post("/quizzes/:id/start", h.StartQuiz)
	app.Post("/quizzes/:id/submit", h.SubmitQuiz)
	app.Get("/quizzes/:id/attempts", h.GetMyAttempts)
	app.Get("/quizzes/:id/attempts/:attemptId", h.GetAttemptByID)
	app.Get("/quizzes/:id/results", h.GetQuizResults)
	app.Get("/quizzes/:id/statistics", h.GetQuizStatistics)
	app.Post("/attempts/:attemptId/save-answer", h.SaveAnswer)
	app.Get("/attempts/:attemptId/progress", h.GetAttemptProgress)
	return app
}

func callAs(t *testing.T, app *fiber.App, user uuid.UUID, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user.String())
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestQuizRoutes_QuizGanCuocThi_HocVien403_Chu200_Sua409(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mkUser := func(kind string) uuid.UUID {
		s := uuid.NewString()
		u := model.User{Email: "qa-contest-h-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-" + kind + s[:8]}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user: %v", err)
		}
		return u.ID
	}
	owner, student := mkUser("owner"), mkUser("student")
	quiz := model.Quiz{Title: "Quiz cuộc thi", TriggerType: "manual", CreatedBy: &owner}
	if err := db.Create(&quiz).Error; err != nil {
		t.Fatalf("tạo quiz: %v", err)
	}
	question := model.Question{QuizID: quiz.ID, QuestionText: "1+1?", QuestionType: "single_choice", Points: decimal.NewFromInt(1), DisplayOrder: 1}
	if err := db.Create(&question).Error; err != nil {
		t.Fatalf("tạo câu hỏi: %v", err)
	}
	if err := db.Create(&[]model.QuestionAnswer{{QuestionID: question.ID, AnswerText: "2", IsCorrect: true, DisplayOrder: 1}}).Error; err != nil {
		t.Fatalf("tạo đáp án: %v", err)
	}

	svc := service.NewQuizService(repository.NewQuizRepository(db), nil, repository.NewCourseRepository(db),
		repository.NewSectionRepository(db), repository.NewLessonRepository(db), nil, repository.NewEnrollmentRepository(db))
	svc.SetContestGate(lockedQuizGate{quizID: quiz.ID, creator: owner})
	app := mountQuizRoutesAs(NewQuizHandler(svc, nil))

	// Người tạo cuộc thi mở một attempt luyện tập để có attempt_id cho các route theo attempt.
	code, body := callAs(t, app, owner, "POST", "/quizzes/"+quiz.ID.String()+"/start", `{"mode":"practice"}`)
	if code != 201 {
		t.Fatalf("người tạo start: %d %v", code, body)
	}
	attemptID := body["data"].(map[string]interface{})["attempt_id"].(string)

	q := "/quizzes/" + quiz.ID.String()
	reads := []struct{ method, path, body string }{
		{"GET", q, ""},
		{"GET", q + "/questions", ""},
		{"POST", q + "/start", `{"mode":"practice"}`},
		{"POST", q + "/submit", `{"answers":[]}`},
		{"GET", q + "/attempts", ""},
		{"GET", q + "/attempts/" + attemptID, ""},
		{"GET", q + "/results", ""},
		{"GET", q + "/statistics", ""},
		{"POST", q + "/duplicate", ""},
		{"POST", "/attempts/" + attemptID + "/save-answer", `{"question_id":"` + question.ID.String() + `"}`},
		{"GET", "/attempts/" + attemptID + "/progress", ""},
	}
	for _, r := range reads {
		code, body := callAs(t, app, student, r.method, r.path, r.body)
		if code != 403 || body["code"] != "QUIZ_LOCKED_BY_CONTEST" {
			t.Errorf("học viên %s %s: muốn 403 QUIZ_LOCKED_BY_CONTEST, nhận %d %v", r.method, r.path, code, body)
		}
	}
	for _, r := range reads {
		if r.method == "POST" && strings.HasSuffix(r.path, "/submit") {
			continue // kết quả nộp của người tạo phụ thuộc attempt dang dở, không phải điều test này kiểm
		}
		code, body := callAs(t, app, owner, r.method, r.path, r.body)
		if code != 200 && code != 201 {
			t.Errorf("người tạo %s %s: muốn 200/201, nhận %d %v", r.method, r.path, code, body)
		}
	}

	listedFor := func(user uuid.UUID) bool {
		code, body := callAs(t, app, user, "GET", "/quizzes?page_size=50", "")
		if code != 200 {
			t.Fatalf("GET /quizzes: %d %v", code, body)
		}
		for _, item := range body["data"].(map[string]interface{})["data"].([]interface{}) {
			if item.(map[string]interface{})["id"] == quiz.ID.String() {
				return true
			}
		}
		return false
	}
	if listedFor(student) {
		t.Errorf("GET /quizzes của học viên vẫn liệt kê quiz gắn cuộc thi")
	}
	if !listedFor(owner) {
		t.Errorf("GET /quizzes của người tạo phải có quiz gắn cuộc thi")
	}

	edits := []struct{ method, path, body string }{
		{"PUT", q, `{"title":"Tên mới"}`},
		{"POST", q + "/questions", `{"question_text":"Mới","question_type":"essay"}`},
		{"PUT", q + "/questions/" + question.ID.String(), `{"question_text":"Sửa"}`},
		{"DELETE", q, ""},
	}
	for _, r := range edits {
		code, body := callAs(t, app, owner, r.method, r.path, r.body)
		if code != 409 || body["code"] != "CONTEST_QUIZ_LOCKED" {
			t.Errorf("người tạo %s %s khi đã công bố: muốn 409 CONTEST_QUIZ_LOCKED, nhận %d %v", r.method, r.path, code, body)
		}
		code, body = callAs(t, app, student, r.method, r.path, r.body)
		if code != 403 || body["code"] != "QUIZ_LOCKED_BY_CONTEST" {
			t.Errorf("học viên %s %s: muốn 403, nhận %d %v", r.method, r.path, code, body)
		}
	}
}
