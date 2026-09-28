package router

// Review PR #79 (Q5 cho quiz): mọi route GHI quiz/câu hỏi phải đi qua lớp khoá khoá-học-chờ-duyệt
// (CourseEditLock + handler = 2 handler; auth gắn ở cấp group nên không đếm trong route), route đọc
// thì không (chỉ handler = 1). Gỡ middleware khỏi 1 route ghi ở quiz_router.go -> test đỏ.
//
// Re-review vòng 2: đếm handler không bắt được route câu hỏi truyền SAI tên tham số
// (CourseEditLock("id") = id câu hỏi thay vì "quizId") — guard tra quiz theo id câu hỏi, không
// thấy, cho qua. Test thứ hai gửi request THẬT (auth thật qua miniredis) với quiz bị khoá ở
// :quizId và id câu hỏi KHÁC ở :id, nên tham số sai thì route không trả 409 -> đỏ.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

func TestQuizWriteRoutes_HaveCourseEditLock(t *testing.T) {
	app := fiber.New()
	SetupQuizRoutes(app.Group("/api"), nil, handler.NewQuizHandler(&fakeQuizService{}, nil), nil)

	writes := map[string]bool{
		"POST /api/quizzes/":                         true,
		"PUT /api/quizzes/:id":                       true,
		"DELETE /api/quizzes/:id":                    true,
		"POST /api/quizzes/:id/duplicate":            true,
		"POST /api/quizzes/:quizId/questions/":       true,
		"PUT /api/quizzes/:quizId/questions/reorder": true,
		"PUT /api/quizzes/:quizId/questions/:id":     true,
		"DELETE /api/quizzes/:quizId/questions/:id":  true,
		"POST /api/quizzes/:quizId/questions/bulk":   true,
	}
	seen := map[string]int{}
	for _, r := range app.GetRoutes(true) {
		seen[r.Method+" "+r.Path] = len(r.Handlers)
	}
	for route := range writes {
		n, ok := seen[route]
		if !ok {
			t.Errorf("thiếu route %s", route)
			continue
		}
		if n != 2 {
			t.Errorf("%s có %d handler, muốn 2 (CourseEditLock + handler)", route, n)
		}
	}
	if n := seen["GET /api/quizzes/:id"]; n != 1 {
		t.Errorf("GET /api/quizzes/:id có %d handler, muốn 1 — route đọc không được bị khoá", n)
	}
}

// lockingQuizSvc: chỉ quiz `locked` thuộc khoá đang chờ duyệt. Handler ghi thật KHÔNG được chạy
// tới (service nhúng nil -> panic -> recover 500), nên route nào lọt guard đều != 409.
type lockingQuizSvc struct {
	service.QuizServiceInterface
	locked uuid.UUID
}

func (s *lockingQuizSvc) EnsureQuizCourseEditable(_ context.Context, quizID uuid.UUID) error {
	if quizID == s.locked {
		return service.ErrCourseLockedForReview
	}
	return nil
}

func (s *lockingQuizSvc) EnsureNewQuizCourseEditable(context.Context, uuid.UUID, bool, *uuid.UUID, *uuid.UUID) error {
	return service.ErrCourseLockedForReview
}

func TestQuizWriteRoutes_PendingCourseQuizIs409(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "quiz-lock-route-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}
	userID := uuid.New()
	if err := rdb.Set(context.Background(), constants.KeyUserVersion(userID.String()), int64(1), 0).Err(); err != nil {
		t.Fatal(err)
	}
	token, _, err := utils.GenerateTokens(cfg, userID, uuid.New(), "TEACHER", nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	locked := uuid.New()
	app := fiber.New()
	app.Use(recover.New())
	SetupQuizRoutes(app.Group("/api"), cfg, handler.NewQuizHandler(&lockingQuizSvc{locked: locked}, nil), rdb)

	q := "/api/quizzes/" + locked.String()
	question := uuid.NewString() // id câu hỏi, KHÁC id quiz
	do := func(method, path string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"title":"QA-quiz"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return res.StatusCode
	}
	for _, r := range [][2]string{
		{"POST", "/api/quizzes/"},
		{"PUT", q},
		{"DELETE", q},
		{"POST", q + "/duplicate"},
		{"POST", q + "/questions/"},
		{"PUT", q + "/questions/reorder"},
		{"PUT", q + "/questions/" + question},
		{"DELETE", q + "/questions/" + question},
		{"POST", q + "/questions/bulk"},
	} {
		if code := do(r[0], r[1]); code != fiber.StatusConflict {
			t.Errorf("%s %s = %d, muốn 409 (quiz thuộc khoá chờ duyệt)", r[0], r[1], code)
		}
	}
	if code := do("GET", q); code == fiber.StatusConflict {
		t.Errorf("GET %s = 409 — route đọc không được bị khoá", q)
	}
}
