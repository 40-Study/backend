package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// Review QA2 H2: mọi đường bán/ghi danh của khoá archived trả 409 + code COURSE_NOT_FOR_SALE + câu tiếng Việt.

type enrollNotForSaleStub struct {
	service.EnrollmentServiceInterface
}

func (enrollNotForSaleStub) Enroll(context.Context, uuid.UUID, uuid.UUID) (*dto.EnrollmentResponseDTO, error) {
	return nil, fmt.Errorf("wrapped: %w", service.ErrCourseNotForSale)
}

func TestEnroll_KhoaNgungBan_409CoCode(t *testing.T) {
	user := uuid.New()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", user); return c.Next() })
	app.Post("/courses/:courseId/enroll", NewEnrollmentHandler(enrollNotForSaleStub{}, nil).Enroll)

	code, out := doWishlist(t, app, "POST", "/courses/"+uuid.NewString()+"/enroll", "")
	if code != 409 || out["code"] != "COURSE_NOT_FOR_SALE" || out["message"] != service.ErrCourseNotForSale.Message {
		t.Fatalf("got %d %v", code, out)
	}
}

func TestPaymentStateConflict_KhoaNgungBan(t *testing.T) {
	for _, err := range []error{service.ErrCourseNotForSale, fmt.Errorf("tao don: %w", service.ErrCourseNotForSale)} {
		body, ok := paymentStateConflict(err)
		if !ok || body["code"] != "COURSE_NOT_FOR_SALE" || body["message"] != service.ErrCourseNotForSale.Message {
			t.Fatalf("paymentStateConflict(%v) = %v, %v", err, body, ok)
		}
	}
	if _, ok := paymentStateConflict(errors.New("khac")); ok {
		t.Error("loi la khong duoc map thanh COURSE_NOT_FOR_SALE")
	}
}

func TestRespondServiceError_MangCodeKhiCo(t *testing.T) {
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error { return RespondServiceError(c, service.ErrCourseNotForSale, "x") })
	code, out := doWishlist(t, app, "GET", "/x", "")
	if code != 409 || out["code"] != "COURSE_NOT_FOR_SALE" || out["message"] != service.ErrCourseNotForSale.Message {
		t.Fatalf("got %d %v", code, out)
	}
}
