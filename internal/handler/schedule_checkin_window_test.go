package handler

// Lane P: check-in ngoài ngày của buổi hoặc buổi đã đóng trả 409 (đúng người, sai thời điểm), không phải 400
// chung chung. Dùng lại service giả của S5 (s5ScheduleSvc). Bỏ ánh xạ ở scheduleFail thì test ĐỎ.

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

func TestStudentCheckIn_OutsideWindowIsConflict(t *testing.T) {
	for name, err := range map[string]error{
		"ngoài ngày của buổi": service.ErrCheckInOutsideSessionDay,
		"buổi đã đóng":        service.ErrSessionClosedForCheckIn,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &s5ScheduleSvc{err: err}
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
			app.Post("/sessions/:sessionId/check-in", NewScheduleHandler(svc, nil).StudentCheckIn)

			res, e := app.Test(httptest.NewRequest("POST", "/sessions/"+uuid.NewString()+"/check-in", nil), -1)
			if e != nil {
				t.Fatal(e)
			}
			if res.StatusCode != fiber.StatusConflict {
				t.Fatalf("status=%d, muốn 409", res.StatusCode)
			}
		})
	}
}
