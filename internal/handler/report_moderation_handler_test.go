package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// ADM-N4: pin bảng mã lỗi của POST /reports/:id/actions (contract-be3.md §1b) - web render theo `code`, không theo message.

type fakeModerationService struct {
	res *dto.ReportActionResponseDTO
	err error
	got dto.ReportActionRequestDTO
}

func (f *fakeModerationService) Apply(_ context.Context, _, _ uuid.UUID, req dto.ReportActionRequestDTO) (*dto.ReportActionResponseDTO, error) {
	f.got = req
	return f.res, f.err
}

func moderationApp(f *fakeModerationService, user *uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user_id", *user)
		}
		return c.Next()
	})
	app.Post("/reports/:id/actions", NewReportModerationHandler(f).ApplyAction)
	return app
}

func TestReportModerationHandler_MaLoi(t *testing.T) {
	admin, rid := uuid.New(), uuid.New()
	path := "/reports/" + rid.String() + "/actions"
	ok := `{"action":"unpublish_course","reason":"vi pham"}`
	cases := []struct {
		name     string
		path     string
		body     string
		err      error
		wantCode int
		wantTag  string
	}{
		{"report khong ton tai", path, ok, service.ErrReportNotFound, 404, "REPORT_NOT_FOUND"},
		{"doi tuong da mat", path, ok, service.ErrReportTargetNotFound, 404, "TARGET_NOT_FOUND"},
		{"action sai loai", path, ok, service.ErrReportActionNotApplicable, 400, "ACTION_NOT_APPLICABLE"},
		{"thieu ly do", path, ok, service.ErrReportActionReasonRequired, 400, "VALIDATION_FAILED"},
		{"trang thai khong doi duoc", path, ok, service.ErrReportTargetUnchanged, 409, "TARGET_STATE_UNCHANGED"},
		{"action la", path, `{"action":"nuke_site"}`, nil, 400, "VALIDATION_FAILED"},
		{"thieu action", path, `{}`, nil, 400, "VALIDATION_FAILED"},
		{"ly do qua dai", path, `{"action":"hide_discussion","reason":"` + longReason() + `"}`, nil, 400, "VALIDATION_FAILED"},
		{"body hong", path, `not json`, nil, 400, "VALIDATION_FAILED"},
		{"id hong", "/reports/xyz/actions", ok, nil, 400, "VALIDATION_FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeModerationService{err: tc.err}
			code, out := doWishlist(t, moderationApp(f, &admin), "POST", tc.path, tc.body)
			if code != tc.wantCode || out["code"] != tc.wantTag {
				t.Fatalf("got %d %v, muon %d %s", code, out, tc.wantCode, tc.wantTag)
			}
		})
	}
}

func longReason() string {
	b := make([]byte, 501)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func TestReportModerationHandler_ThanhCongVaMacDinhResolve(t *testing.T) {
	admin, rid := uuid.New(), uuid.New()
	f := &fakeModerationService{res: &dto.ReportActionResponseDTO{Action: "hide_discussion", Report: &dto.ReportResponseDTO{ID: rid, ReportedType: "discussion", ReportedID: uuid.New()}}}
	code, out := doWishlist(t, moderationApp(f, &admin), "POST", "/reports/"+rid.String()+"/actions", `{"action":"hide_discussion","reason":"ngon tu"}`)
	if code != http.StatusOK || out["message"] != "Report action applied" {
		t.Fatalf("got %d %v", code, out)
	}
	if f.got.Action != "hide_discussion" || f.got.Resolve != nil {
		t.Errorf("handler khong duoc tu dien resolve - mac dinh la viec cua service: %+v", f.got)
	}
	if data, _ := out["data"].(map[string]any); data["action"] != "hide_discussion" || data["report"] == nil {
		t.Errorf("data: %v", out["data"])
	}
}

func TestReportModerationHandler_LoiHaTangKhongLoChiTiet(t *testing.T) {
	admin, rid := uuid.New(), uuid.New()
	f := &fakeModerationService{err: errors.New("pq: relation \"reports\" does not exist")}
	code, out := doWishlist(t, moderationApp(f, &admin), "POST", "/reports/"+rid.String()+"/actions", `{"action":"unpublish_course","reason":"vi pham"}`)
	if code != 500 || out["message"] != "Failed to apply report action" {
		t.Fatalf("got %d %v", code, out)
	}
}

func TestReportModerationHandler_KhongCoUserTra401(t *testing.T) {
	code, _ := doWishlist(t, moderationApp(&fakeModerationService{}, nil), "POST", "/reports/"+uuid.NewString()+"/actions", `{"action":"hide_discussion","reason":"abc"}`)
	if code != 401 {
		t.Fatalf("got %d", code)
	}
}
