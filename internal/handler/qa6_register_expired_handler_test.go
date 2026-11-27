package handler

// QA6-PG-02 (C7): POST /auth/register khi phiên chờ OTP hết hạn -> 410 ERR_REGISTRATION_EXPIRED (cùng resend-otp);
// các lỗi khác (sai OTP) vẫn 400. Bỏ nhánh errors.Is trong Register thì ca "hết hạn" ĐỎ (400).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type registerStub struct {
	service.AuthServiceInterface
	err error
}

func (s *registerStub) Register(context.Context, dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error) {
	return nil, s.err
}

func TestQA6_RegisterVerify_HetHan410(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		status   int
		wantCode string
	}{
		{"hết hạn", service.ErrRegistrationNotPending, fiber.StatusGone, "ERR_REGISTRATION_EXPIRED"},
		{"sai OTP", fmt.Errorf("%w: invalid OTP, 4 attempts remaining", service.ErrRegisterOTPRejected), fiber.StatusBadRequest, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/auth/register", NewAuthHandler(&registerStub{err: tc.err}).Register)
			req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"email":"a@40study.test","otp":"123456"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if resp.StatusCode != tc.status || (tc.wantCode != "" && body["code"] != tc.wantCode) {
				t.Fatalf("status=%d body=%v", resp.StatusCode, body)
			}
		})
	}
}
