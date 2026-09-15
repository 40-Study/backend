package handler

// Test cho V3-6 (issue #58), tang handler cua nhom /whiteboard: danh tinh nguoi goi phai lay tu
// access token cho ca 3 thao tac (GetSnapshot, SaveSnapshot, BroadcastEvent) — truoc day khong
// handler nao doi hoi dang nhap phai co quan he gi voi phien; thieu dang nhap phai la 401; loi uy
// quyen tu service phai thanh 403.

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubWhiteboardService struct {
	err            error
	getCalls       int
	gotGetUser     uuid.UUID
	saveCalls      int
	gotSaveUser    uuid.UUID
	broadcastCalls int
	gotBcastUser   uuid.UUID
}

func (s *stubWhiteboardService) GetSnapshot(ctx context.Context, userID, sessionID uuid.UUID) (*dto.WhiteboardSnapshotResponseDTO, error) {
	s.getCalls++
	s.gotGetUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.WhiteboardSnapshotResponseDTO{SessionID: sessionID}, nil
}

func (s *stubWhiteboardService) SaveSnapshot(ctx context.Context, userID uuid.UUID, req dto.WhiteboardSnapshotDTO) error {
	s.saveCalls++
	s.gotSaveUser = userID
	return s.err
}

func (s *stubWhiteboardService) BroadcastEvent(ctx context.Context, userID, sessionID uuid.UUID, event dto.WhiteboardEventDTO, livekitSvc service.LivekitServiceInterface) error {
	s.broadcastCalls++
	s.gotBcastUser = userID
	return s.err
}

var _ service.WhiteboardServiceInterface = (*stubWhiteboardService)(nil)

func TestWhiteboardGetSnapshot_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubWhiteboardService{}
	h := NewWhiteboardHandler(svc, nil)
	app := fiber.New()
	app.Get("/whiteboard/:sessionId/snapshot", h.GetSnapshot)

	req := httptest.NewRequest("GET", "/whiteboard/"+uuid.New().String()+"/snapshot", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", resp.StatusCode)
	}
	if svc.getCalls != 0 {
		t.Error("service bi goi du chua xac thuc")
	}
}

func TestWhiteboardGetSnapshot_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubWhiteboardService{err: service.ErrNotSessionMember}
	h := NewWhiteboardHandler(svc, nil)
	caller := uuid.New()
	app := mountWithCaller("GET", "/whiteboard/:sessionId/snapshot", caller, h.GetSnapshot)

	status := doJSON(t, app, "GET", "/whiteboard/"+uuid.New().String()+"/snapshot", "")
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}

func TestWhiteboardGetSnapshot_ThanhVien_NguoiGoiTuToken(t *testing.T) {
	svc := &stubWhiteboardService{}
	h := NewWhiteboardHandler(svc, nil)
	caller := uuid.New()
	app := mountWithCaller("GET", "/whiteboard/:sessionId/snapshot", caller, h.GetSnapshot)

	status := doJSON(t, app, "GET", "/whiteboard/"+uuid.New().String()+"/snapshot", "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotGetUser != caller {
		t.Errorf("userID = %s, muon %s", svc.gotGetUser, caller)
	}
}

func TestWhiteboardSaveSnapshot_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubWhiteboardService{}
	h := NewWhiteboardHandler(svc, nil)
	app := fiber.New()
	app.Post("/whiteboard/:sessionId/snapshot", h.SaveSnapshot)

	status := doJSON(t, app, "POST", "/whiteboard/"+uuid.New().String()+"/snapshot", `{}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", status)
	}
	if svc.saveCalls != 0 {
		t.Error("service bi goi du chua xac thuc")
	}
}

func TestWhiteboardSaveSnapshot_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubWhiteboardService{err: service.ErrNotSessionMember}
	h := NewWhiteboardHandler(svc, nil)
	app := mountWithCaller("POST", "/whiteboard/:sessionId/snapshot", uuid.New(), h.SaveSnapshot)

	status := doJSON(t, app, "POST", "/whiteboard/"+uuid.New().String()+"/snapshot", `{}`)
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}

func TestWhiteboardBroadcastEvent_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubWhiteboardService{}
	h := NewWhiteboardHandler(svc, nil)
	app := fiber.New()
	app.Post("/whiteboard/:sessionId/event", h.BroadcastEvent)

	status := doJSON(t, app, "POST", "/whiteboard/"+uuid.New().String()+"/event", `{"type":"draw","action":"add"}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", status)
	}
	if svc.broadcastCalls != 0 {
		t.Error("service bi goi du chua xac thuc")
	}
}

func TestWhiteboardBroadcastEvent_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubWhiteboardService{err: service.ErrNotSessionMember}
	h := NewWhiteboardHandler(svc, nil)
	app := mountWithCaller("POST", "/whiteboard/:sessionId/event", uuid.New(), h.BroadcastEvent)

	status := doJSON(t, app, "POST", "/whiteboard/"+uuid.New().String()+"/event", `{"type":"draw","action":"add"}`)
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}
