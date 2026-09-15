package handler

// Test cho V3-6 (issue #58), tang handler cua nhom /chat: danh tinh nguoi goi (Send) va nguoi xoa
// (DeleteMessage) phai lay tu access token — `user_id`/`deleted_by` cu trong body phai bi BO QUA
// lang le du client van gui (mao danh); thieu dang nhap phai la 401 TRUOC khi cham service; loi
// uy quyen tu service phai thanh 403.
//
// mountWithCaller/doJSON dung lai tu livestream_authz_handler_test.go (cung package).

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

// stubChatService: ghi lai userID/actorID ma handler truyen xuong, va tra ve loi uy quyen khi
// duoc yeu cau.
type stubChatService struct {
	err error

	sendCalls   int
	gotSendUser uuid.UUID

	getCalls   int
	gotGetUser uuid.UUID

	deleteCalls int
	gotDeleteBy uuid.UUID

	pinCalls    int
	gotPinActor uuid.UUID

	unpinCalls   int
	gotUnpinUser uuid.UUID
}

func (s *stubChatService) SendMessage(ctx context.Context, userID uuid.UUID, req dto.SendChatMessageDTO) (*model.ChatMessage, error) {
	s.sendCalls++
	s.gotSendUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return &model.ChatMessage{BaseModel: model.BaseModel{ID: uuid.New()}, UserID: userID}, nil
}

func (s *stubChatService) GetMessages(ctx context.Context, userID, sessionID uuid.UUID, page, pageSize int) (*dto.ChatMessageListDTO, error) {
	s.getCalls++
	s.gotGetUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.ChatMessageListDTO{Page: page, PageSize: pageSize}, nil
}

func (s *stubChatService) DeleteMessage(ctx context.Context, actorID, messageID uuid.UUID) error {
	s.deleteCalls++
	s.gotDeleteBy = actorID
	return s.err
}

func (s *stubChatService) PinMessage(ctx context.Context, actorID, messageID uuid.UUID) error {
	s.pinCalls++
	s.gotPinActor = actorID
	return s.err
}

func (s *stubChatService) UnPinMessage(ctx context.Context, actorID, messageID uuid.UUID) error {
	s.unpinCalls++
	s.gotUnpinUser = actorID
	return s.err
}

var _ service.ChatServiceInterface = (*stubChatService)(nil)

// --- Send: mao danh bang user_id trong body (truong da bi xoa khoi DTO) ------------------------

func TestChatSend_MaoDanhBangUserIDTrongBody_BiBoQua(t *testing.T) {
	svc := &stubChatService{}
	h := NewChatHandler(svc)

	realCaller := uuid.New()
	fakeUserID := uuid.New()
	app := mountWithCaller("POST", "/chat/send", realCaller, h.Send)

	sessionID := uuid.New()
	body := `{"session_id":"` + sessionID.String() + `","user_id":"` + fakeUserID.String() + `","message":"hi"}`
	status := doJSON(t, app, "POST", "/chat/send", body)

	if status != fiber.StatusCreated {
		t.Fatalf("status = %d, muon 201", status)
	}
	if svc.gotSendUser != realCaller {
		t.Errorf("userID truyen xuong service = %s, muon nguoi dang dang nhap = %s (khong phai user_id gia mao %s)",
			svc.gotSendUser, realCaller, fakeUserID)
	}
}

func TestChatSend_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubChatService{}
	h := NewChatHandler(svc)
	app := fiber.New()
	app.Post("/chat/send", h.Send)

	status := doJSON(t, app, "POST", "/chat/send", `{"session_id":"`+uuid.New().String()+`","message":"hi"}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", status)
	}
	if svc.sendCalls != 0 {
		t.Error("service bi goi du chua xac thuc")
	}
}

func TestChatSend_LoiUyQuyenTuService_Bi403(t *testing.T) {
	svc := &stubChatService{err: service.ErrNotSessionMember}
	h := NewChatHandler(svc)
	app := mountWithCaller("POST", "/chat/send", uuid.New(), h.Send)

	status := doJSON(t, app, "POST", "/chat/send", `{"session_id":"`+uuid.New().String()+`","message":"hi"}`)
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}

// --- GetMessages ---------------------------------------------------------------------------

func TestChatGetMessages_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubChatService{}
	h := NewChatHandler(svc)
	app := fiber.New()
	app.Get("/chat/:sessionId/messages", h.GetMessages)

	req := httptest.NewRequest("GET", "/chat/"+uuid.New().String()+"/messages", nil)
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

func TestChatGetMessages_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubChatService{err: service.ErrNotSessionMember}
	h := NewChatHandler(svc)
	caller := uuid.New()
	app := mountWithCaller("GET", "/chat/:sessionId/messages", caller, h.GetMessages)

	status := doJSON(t, app, "GET", "/chat/"+uuid.New().String()+"/messages", "")
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}

// --- DeleteMessage: mao danh bang deleted_by trong body (DTO cu da bi xoa het) -------------------

func TestChatDeleteMessage_MaoDanhBangDeletedByTrongBody_BiBoQua(t *testing.T) {
	svc := &stubChatService{}
	h := NewChatHandler(svc)

	realCaller := uuid.New()
	fakeDeletedBy := uuid.New()
	messageID := uuid.New()
	app := mountWithCaller("DELETE", "/chat/:id", realCaller, h.DeleteMessage)

	// Body cu (`message_id`, `deleted_by`) gui kem — phai bi bo qua hoan toan: message id lay tu
	// URL, actor lay tu token.
	body := `{"message_id":"` + uuid.New().String() + `","deleted_by":"` + fakeDeletedBy.String() + `"}`
	status := doJSON(t, app, "DELETE", "/chat/"+messageID.String(), body)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotDeleteBy != realCaller {
		t.Errorf("actorID truyen xuong service = %s, muon nguoi dang dang nhap = %s (khong phai deleted_by gia mao %s)",
			svc.gotDeleteBy, realCaller, fakeDeletedBy)
	}
}

func TestChatDeleteMessage_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubChatService{}
	h := NewChatHandler(svc)
	app := fiber.New()
	app.Delete("/chat/:id", h.DeleteMessage)

	req := httptest.NewRequest("DELETE", "/chat/"+uuid.New().String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", resp.StatusCode)
	}
	if svc.deleteCalls != 0 {
		t.Error("service bi goi du chua xac thuc")
	}
}

func TestChatDeleteMessage_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubChatService{err: service.ErrNotSessionMember}
	h := NewChatHandler(svc)
	app := mountWithCaller("DELETE", "/chat/:id", uuid.New(), h.DeleteMessage)

	status := doJSON(t, app, "DELETE", "/chat/"+uuid.New().String(), "")
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}

// --- Pin/UnPin -------------------------------------------------------------------------------

func TestChatPinMessage_NguoiGoiTuToken(t *testing.T) {
	svc := &stubChatService{}
	h := NewChatHandler(svc)
	caller := uuid.New()
	app := mountWithCaller("POST", "/chat/:id/pin", caller, h.PinMessage)

	status := doJSON(t, app, "POST", "/chat/"+uuid.New().String()+"/pin", "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotPinActor != caller {
		t.Errorf("actorID = %s, muon %s", svc.gotPinActor, caller)
	}
}

func TestChatPinMessage_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubChatService{err: service.ErrNotClassTeacher}
	h := NewChatHandler(svc)
	app := mountWithCaller("POST", "/chat/:id/pin", uuid.New(), h.PinMessage)

	status := doJSON(t, app, "POST", "/chat/"+uuid.New().String()+"/pin", "")
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}

func TestChatUnPinMessage_LoiUyQuyen_Bi403(t *testing.T) {
	svc := &stubChatService{err: service.ErrNotClassTeacher}
	h := NewChatHandler(svc)
	app := mountWithCaller("POST", "/chat/:id/unpin", uuid.New(), h.UnPinMessage)

	status := doJSON(t, app, "POST", "/chat/"+uuid.New().String()+"/unpin", "")
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", status)
	}
}
