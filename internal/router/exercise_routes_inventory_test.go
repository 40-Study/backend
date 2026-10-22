package router

// Lane BE-2 (code-editor gaps #2, #4): bảng route của bài tập. Route cũ POST /exercises/:id/submit
// đẩy bài nộp vào hàng RabbitMQ exercise.submit không có consumer (bài kẹt 'pending' mãi) và đã bị
// gỡ; PUT test case là route mới giảng viên dùng để sửa test.

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
)

func TestExerciseRoutes_LegacyQueueSubmitRemovedAndTestCaseUpdateRegistered(t *testing.T) {
	app := fiber.New()
	SetupExerciseRoutes(app.Group("/api"), &config.Config{}, handler.NewExerciseHandler(nil, nil), nil)

	has := map[string]bool{}
	for _, r := range app.GetRoutes(true) {
		has[r.Method+" "+r.Path] = true
	}
	for _, gone := range []string{"POST /api/exercises/:id/submit"} {
		if has[gone] {
			t.Errorf("%s phải bị gỡ (hàng exercise.submit không có consumer)", gone)
		}
	}
	for _, want := range []string{
		"PUT /api/exercises/:id/testcases/:testCaseId",
		"POST /api/exercises/:id/testcases",
		"DELETE /api/exercises/:id/testcases/:testCaseId",
		"PUT /api/exercises/:id",
		"GET /api/exercises/:id/submissions",
		"POST /api/lessons/:lessonId/exercises/:id/submit",
	} {
		if !has[want] {
			t.Errorf("thiếu route %s", want)
		}
	}
}
