package handler

// Review PR #79 (Q5 cho quiz): middleware CourseEditLock/NewQuizCourseEditLock chặn ghi quiz/câu hỏi
// của khoá đang chờ duyệt bằng 409 COURSE_PENDING_REVIEW TRƯỚC khi handler ghi chạy; quiz của khoá
// chưa xuất bản bị ẩn thì GET trả 404.

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type lockedQuizSvc struct {
	service.QuizServiceInterface
	lockErr  error
	writes   int
	gotUser  uuid.UUID
	gotAdmin bool
}

func (s *lockedQuizSvc) EnsureQuizCourseEditable(context.Context, uuid.UUID) error { return s.lockErr }
func (s *lockedQuizSvc) EnsureNewQuizCourseEditable(_ context.Context, userID uuid.UUID, isAdmin bool, _, _ *uuid.UUID) error {
	s.gotUser, s.gotAdmin = userID, isAdmin
	return s.lockErr
}
func (s *lockedQuizSvc) GetQuizByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.QuizDetailDTO, error) {
	return nil, service.ErrCourseHidden
}
func (s *lockedQuizSvc) GetQuestionsByQuiz(context.Context, uuid.UUID, uuid.UUID, bool) ([]dto.QuestionResponseDTO, error) {
	return nil, service.ErrCourseHidden
}

func quizLockApp(svc *lockedQuizSvc) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	h := NewQuizHandler(svc, nil)
	// Handler cuối giả thay cho CreateQuiz/UpdateQuiz/DeleteQuiz: test chỉ kiểm middleware có
	// chặn TRƯỚC khi tới handler ghi hay không, không phụ thuộc chữ ký hàm ghi (PR #80 đổi chúng).
	write := func(c *fiber.Ctx) error { svc.writes++; return c.SendStatus(fiber.StatusOK) }
	app.Post("/quizzes", h.NewQuizCourseEditLock(), write)
	app.Put("/quizzes/:id", h.CourseEditLock("id"), write)
	app.Delete("/quizzes/:id", h.CourseEditLock("id"), write)
	app.Get("/quizzes/:id", h.GetQuizByID)
	app.Get("/quizzes/:quizId/questions", h.GetQuestionsByQuiz)
	return app
}

func doQuiz(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

func TestQuizCourseEditLock_PendingCourseBlocksWrites(t *testing.T) {
	svc := &lockedQuizSvc{lockErr: service.ErrCourseLockedForReview}
	app := quizLockApp(svc)
	qid := uuid.NewString()
	create := `{"title":"QA-quiz","course_id":"` + uuid.NewString() + `"}`
	for _, tc := range [][3]string{{"POST", "/quizzes", create}, {"PUT", "/quizzes/" + qid, `{"title":"QA-quiz"}`}, {"DELETE", "/quizzes/" + qid, ""}} {
		code, body := doQuiz(t, app, tc[0], tc[1], tc[2])
		if code != 409 || !strings.Contains(body, CourseLockedCode) {
			t.Errorf("%s %s = %d %s, muốn 409 %s", tc[0], tc[1], code, body, CourseLockedCode)
		}
	}
	if svc.writes != 0 {
		t.Fatalf("khoá chờ duyệt nhưng handler ghi vẫn chạy %d lần", svc.writes)
	}

	// Khoá sửa được: middleware cho qua.
	ok := &lockedQuizSvc{}
	if code, body := doQuiz(t, quizLockApp(ok), "PUT", "/quizzes/"+qid, `{"title":"QA-quiz"}`); code != 200 || ok.writes != 1 {
		t.Fatalf("khoá sửa được: PUT = %d %s, writes=%d", code, body, ok.writes)
	}
}

// Re-review vòng 2: lỗi guard khi TẠO quiz map đúng mã và chặn TRƯỚC CreateQuiz.
func TestQuizCourseEditLock_CreateGuardStatusMapping(t *testing.T) {
	body := `{"title":"QA-quiz","lesson_id":"` + uuid.NewString() + `","course_id":"` + uuid.NewString() + `"}`
	for _, tc := range []struct {
		err  error
		want int
	}{
		{service.ErrQuizCourseMismatch, 400},
		{service.ErrQuizCourseNotOwner, 403},
		{service.ErrCourseHidden, 404},
	} {
		svc := &lockedQuizSvc{lockErr: tc.err}
		if code, raw := doQuiz(t, quizLockApp(svc), "POST", "/quizzes", body); code != tc.want || svc.writes != 0 {
			t.Errorf("%v: POST /quizzes = %d %s (writes=%d), muốn %d và không ghi", tc.err, code, raw, svc.writes, tc.want)
		}
	}
}

// Guard tạo quiz phải nhận ĐÚNG người gọi (kiểm chủ khoá) — không phải uuid.Nil hay admin=true.
func TestQuizCourseEditLock_CreatePassesCaller(t *testing.T) {
	caller := uuid.New()
	svc := &lockedQuizSvc{}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", caller); return c.Next() })
	app.Post("/quizzes", NewQuizHandler(svc, nil).NewQuizCourseEditLock(), func(c *fiber.Ctx) error { return c.SendStatus(200) })
	if code, body := doQuiz(t, app, "POST", "/quizzes", `{"title":"QA-quiz","course_id":"`+uuid.NewString()+`"}`); code != 200 {
		t.Fatalf("POST = %d %s", code, body)
	}
	if svc.gotUser != caller || svc.gotAdmin {
		t.Fatalf("guard nhận user=%s admin=%v, muốn user=%s admin=false (không có permChecker)", svc.gotUser, svc.gotAdmin, caller)
	}
}

func TestQuizCourseVisibility_HiddenQuizIs404(t *testing.T) {
	app := quizLockApp(&lockedQuizSvc{})
	for _, path := range []string{"/quizzes/" + uuid.NewString(), "/quizzes/" + uuid.NewString() + "/questions"} {
		if code, body := doQuiz(t, app, "GET", path, ""); code != 404 {
			t.Errorf("quiz của khoá bị ẩn: GET %s = %d %s, muốn 404", path, code, body)
		}
	}
}
