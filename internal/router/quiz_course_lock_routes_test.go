package router

// Review PR #79 (Q5 cho quiz): mọi route GHI quiz/câu hỏi phải đi qua lớp khoá khoá-học-chờ-duyệt
// (CourseEditLock + handler = 2 handler; auth gắn ở cấp group nên không đếm trong route), route đọc
// thì không (chỉ handler = 1). Gỡ
// middleware khỏi 1 route ghi ở quiz_router.go -> test đỏ.

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
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
