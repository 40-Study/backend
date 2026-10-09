package handler

// HTTP-level pin cho contract C1 (plan 261008 phase 1): body article/quiz đi qua validate DTO tới service, và mỗi
// lỗi nghiệp vụ ra đúng {status, code, message} của envelope. Service giả chỉ trả lỗi cho trước — luật nghiệp vụ
// thật được test ở service (lesson_content_article_quiz_test.go); ở đây chỉ pin phần ánh xạ HTTP.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubContentService struct {
	service.LessonContentServiceInterface
	err       error
	gotCreate *dto.CreateLessonContentDTO
	gotUpdate *dto.UpdateLessonContentDTO
}

func (s *stubContentService) CreateContent(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ bool, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	s.gotCreate = &req
	return &dto.LessonContentResponseDTO{Type: req.Type}, s.err
}

func (s *stubContentService) UpdateContent(_ context.Context, _, _ uuid.UUID, _ bool, req dto.UpdateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	s.gotUpdate = &req
	return &dto.LessonContentResponseDTO{}, s.err
}

func contentTestApp(svc service.LessonContentServiceInterface) *fiber.App {
	app := fiber.New()
	h := NewLessonContentHandler(svc, nil)
	asUser := func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}
	app.Post("/lessons/:lesson_id/contents", asUser, h.CreateContent)
	app.Put("/lessons/:lesson_id/contents/:id", asUser, h.UpdateContent)
	return app
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("response không phải JSON: %q", raw)
	}
	return resp.StatusCode, out
}

func TestLessonContentArticleQuizHandler_RuleErrorsUseContractEnvelope(t *testing.T) {
	lesson, content := uuid.NewString(), uuid.NewString()
	cases := []struct {
		err    *service.LessonContentRuleError
		status int
		code   string
	}{
		{service.ErrArticleBodyRequired, 400, "ARTICLE_BODY_REQUIRED"},
		{service.ErrArticleBodyTooLong, 400, "ARTICLE_BODY_TOO_LONG"},
		{service.ErrQuizIDRequired, 400, "QUIZ_ID_REQUIRED"},
		{service.ErrContentQuizNotFound, 404, "QUIZ_NOT_FOUND"},
		{service.ErrQuizLessonMismatch, 409, "QUIZ_LESSON_MISMATCH"},
		{service.ErrQuizAlreadyLinked, 409, "QUIZ_ALREADY_LINKED"},
		{service.ErrContentTypeImmutable, 400, "CONTENT_TYPE_IMMUTABLE"},
	}
	for _, c := range cases {
		for _, call := range []struct{ method, path, body string }{
			{http.MethodPost, "/lessons/" + lesson + "/contents", `{"type":"article","article_body":"<p>x</p>"}`},
			{http.MethodPut, "/lessons/" + lesson + "/contents/" + content, `{"title":"t"}`},
		} {
			status, body := doJSON(t, contentTestApp(&stubContentService{err: c.err}), call.method, call.path, call.body)
			if status != c.status || body["code"] != c.code {
				t.Errorf("%s %s: %d %v, muốn %d %s", call.method, c.code, status, body, c.status, c.code)
			}
			if msg, _ := body["message"].(string); msg != c.err.Message || msg == "" {
				t.Errorf("%s: message = %q, muốn %q (tiếng Việt có dấu, web hiện nguyên văn)", c.code, msg, c.err.Message)
			}
		}
	}
}

func TestLessonContentArticleQuizHandler_WrappedRuleErrorStillMapped(t *testing.T) {
	wrapped := errors.Join(errors.New("ctx"), service.ErrQuizAlreadyLinked)
	status, body := doJSON(t, contentTestApp(&stubContentService{err: wrapped}), http.MethodPost,
		"/lessons/"+uuid.NewString()+"/contents", `{"type":"quiz","quiz_id":"`+uuid.NewString()+`"}`)
	if status != 409 || body["code"] != "QUIZ_ALREADY_LINKED" {
		t.Fatalf("lỗi bọc vẫn phải ra 409 QUIZ_ALREADY_LINKED, nhận %d %v", status, body)
	}
}

func TestLessonContentArticleQuizHandler_NonOwnerIs403AndOtherErrorsKeepLegacyShape(t *testing.T) {
	path := "/lessons/" + uuid.NewString() + "/contents"
	status, body := doJSON(t, contentTestApp(&stubContentService{err: service.ErrNotLessonCourseOwner}), http.MethodPost, path, `{"type":"article","article_body":"<p>x</p>"}`)
	if status != 403 || body["message"] != "Forbidden" {
		t.Errorf("không phải chủ khoá: %d %v, muốn 403 Forbidden", status, body)
	}
	status, body = doJSON(t, contentTestApp(&stubContentService{err: errors.New("lesson not found")}), http.MethodPost, path, `{"type":"article","article_body":"<p>x</p>"}`)
	if status != 400 || body["message"] != "Failed to create content" || body["code"] != nil {
		t.Errorf("lỗi thường giữ hình dạng cũ: %d %v", status, body)
	}
}

func TestLessonContentArticleQuizHandler_BodiesReachServiceAndTypeSetIsValidated(t *testing.T) {
	path := "/lessons/" + uuid.NewString() + "/contents"
	quizID := uuid.New()

	svc := &stubContentService{}
	status, _ := doJSON(t, contentTestApp(svc), http.MethodPost, path, `{"type":"article","title":"Bài đọc","article_body":"<p>html</p>","is_mandatory":true,"display_order":0}`)
	if status != 201 || svc.gotCreate == nil || svc.gotCreate.ArticleBody == nil || *svc.gotCreate.ArticleBody != "<p>html</p>" {
		t.Fatalf("POST article (ví dụ C1): %d, req=%+v", status, svc.gotCreate)
	}

	svc = &stubContentService{}
	status, _ = doJSON(t, contentTestApp(svc), http.MethodPost, path, `{"type":"quiz","title":"Kiểm tra","quiz_id":"`+quizID.String()+`","is_mandatory":true}`)
	if status != 201 || svc.gotCreate == nil || svc.gotCreate.QuizID == nil || *svc.gotCreate.QuizID != quizID {
		t.Fatalf("POST quiz (ví dụ C1): %d, req=%+v", status, svc.gotCreate)
	}

	for _, typ := range []string{"video", "livestream", "exercise"} {
		svc = &stubContentService{}
		if status, _ = doJSON(t, contentTestApp(svc), http.MethodPost, path, `{"type":"`+typ+`"}`); status != 201 {
			t.Errorf("loại cũ %q không được bị validate từ chối: %d", typ, status)
		}
	}
	svc = &stubContentService{}
	if status, _ = doJSON(t, contentTestApp(svc), http.MethodPost, path, `{"type":"podcast"}`); status != 400 || svc.gotCreate != nil {
		t.Errorf("loại ngoài tập C1 phải bị validate chặn trước service: %d", status)
	}
	svc = &stubContentService{}
	status, _ = doJSON(t, contentTestApp(svc), http.MethodPut, path+"/"+uuid.NewString(), `{"type":"article","article_body":"<p>mới</p>"}`)
	if status != 200 || svc.gotUpdate == nil || svc.gotUpdate.Type == nil || *svc.gotUpdate.Type != "article" || svc.gotUpdate.ArticleBody == nil {
		t.Errorf("PUT article: %d, req=%+v", status, svc.gotUpdate)
	}
}
