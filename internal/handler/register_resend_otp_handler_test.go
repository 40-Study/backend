package handler

// QA4-N07: POST /auth/register/resend-otp chỉ nhận email; không còn đăng ký đang chờ -> 410 ERR_REGISTRATION_EXPIRED.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

type resendOTPStub struct {
	service.AuthServiceInterface
	err   error
	email string
}

func (s *resendOTPStub) ResendRegisterOTP(_ context.Context, email string) error {
	s.email = email
	return s.err
}

func TestResendRegisterOTPHandler(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		status   int
		wantCode string
	}{{"gửi được", nil, fiber.StatusOK, ""}, {"hết hạn", service.ErrRegistrationNotPending, fiber.StatusGone, "ERR_REGISTRATION_EXPIRED"}} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &resendOTPStub{err: tc.err}
			app := fiber.New()
			app.Post("/auth/register/resend-otp", NewAuthHandler(stub).ResendRegisterOTP)
			req := httptest.NewRequest("POST", "/auth/register/resend-otp", strings.NewReader(`{"email":"a@40study.test"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if resp.StatusCode != tc.status || stub.email != "a@40study.test" || (tc.wantCode != "" && body["code"] != tc.wantCode) {
				t.Fatalf("status=%d body=%v email=%q", resp.StatusCode, body, stub.email)
			}
		})
	}
}
