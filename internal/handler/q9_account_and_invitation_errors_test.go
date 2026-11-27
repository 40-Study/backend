package handler_test

// Q9 fix-up của #158 ở tầng HTTP (handler THẬT, service thay bằng stub):
//   - #142 M1: DELETE /auth/me sai mật khẩu -> 400 ERR_INVALID_PASSWORD (câu "invalid password" giữ nguyên); lỗi không xác định
//     -> 500 ERR_INTERNAL KHÔNG lộ err.Error(); lỗi đã biết khác (ERR_ACCOUNT_HAS_BALANCE) giữ nguyên. Đỏ khi handler quay về
//     trả 400 {message: err.Error()}.
//   - #158 L3: POST /invitations/invite với hạn mức 0 -> 403 ERR_FEATURE_DISABLED, không phải 400 "đã đạt giới hạn". Đỏ khi bỏ
//     respondIfKnownError ở InviteParent (lỗi rơi về 500 "Failed to send invitation").

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
)

func TestQ9_DeleteAccount_ErrorContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
		msg    string // "" = chỉ cần khác rỗng
		leaks  string // chuỗi KHÔNG được xuất hiện trong body
	}{
		{"sai mật khẩu", service.ErrInvalidDeletePassword, 400, "ERR_INVALID_PASSWORD", "invalid password", ""},
		{"còn số dư ví", service.ErrAccountHasBalance, 409, "ERR_ACCOUNT_HAS_BALANCE", "", ""},
		{"lỗi không xác định", errors.New("pq: connection refused to 10.0.0.5"), 500, "ERR_INTERNAL", "", "10.0.0.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewAuthHandler(&qa8AuthSvc{err: tc.err})
			app := fiber.New()
			app.Use(qa7AsUser(uuid.New()))
			app.Delete("/me", h.DeleteAccount)
			code, out, raw := qa7Do(t, app, "DELETE", "/me", `{"password":"Demo@12345"}`)
			msg, _ := out["message"].(string)
			if code != tc.status || out["code"] != tc.code || msg == "" || (tc.msg != "" && msg != tc.msg) {
				t.Fatalf("status=%d body=%s, muốn %d %s message=%q", code, raw, tc.status, tc.code, tc.msg)
			}
			if tc.leaks != "" && strings.Contains(raw, tc.leaks) {
				t.Fatalf("body lộ lỗi nội bộ: %s", raw)
			}
		})
	}
}

type q9InviteSvc struct {
	service.ParentInvitationServiceInterface
	err error
}

func (s *q9InviteSvc) InviteParent(context.Context, uuid.UUID, dto.InviteParentRequestDto) (*dto.InviteParentResponseDto, error) {
	return nil, s.err
}

func TestQ9_InviteParent_FeatureDisabled_403ERR_FEATURE_DISABLED(t *testing.T) {
	h := handler.NewParentInvitationHandler(&q9InviteSvc{err: service.ErrParentInvitationDisabled})
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Post("/invite", h.InviteParent)
	code, out, raw := qa7Do(t, app, "POST", "/invite", `{"email":"phuhuynh@40study.test","relationship":"parent"}`)
	if code != 403 || out["code"] != "ERR_FEATURE_DISABLED" || out["message"] != "Tính năng mời phụ huynh đang tạm tắt." {
		t.Fatalf("status=%d body=%s, muốn 403 ERR_FEATURE_DISABLED \"Tính năng mời phụ huynh đang tạm tắt.\"", code, raw)
	}
}
