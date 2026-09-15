package service

// Test cho V3-6 (issue #58), tang service cua ChatService: gui/doc chat phai la THANH VIEN cua
// phien (EnsureSessionMember); xoa cho phep TAC GIA hoac nguoi QUAN TRI phien
// (EnsureSessionManage); ghim/bo ghim chi nguoi QUAN TRI. Truoc day khong module nao kiem quyen
// gi ca — bat ky user dang nhap deu gui/doc/xoa/ghim duoc chat cua bat ky phien nao, va
// SendMessage con nhan `user_id` tu body (mao danh).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeLivestreamSvcForChat: chi EnsureSessionMember/EnsureSessionManage duoc dung trong pham vi
// test nay — cac method khac cua LivestreamServiceInterface se panic neu bi goi (embed interface
// that, khong tu khai gia tri zero), giup test to ra ngay neu code san xuat goi nham method khac.
type fakeLivestreamSvcForChat struct {
	LivestreamServiceInterface
	memberErr        error
	manageErr        error
	memberCalls      int
	manageCalls      int
	gotMemberSession uuid.UUID
	gotMemberUser    uuid.UUID
	gotManageSession uuid.UUID
	gotManageUser    uuid.UUID
}

func (f *fakeLivestreamSvcForChat) EnsureSessionMember(ctx context.Context, sessionID, userID uuid.UUID) error {
	f.memberCalls++
	f.gotMemberSession, f.gotMemberUser = sessionID, userID
	return f.memberErr
}

func (f *fakeLivestreamSvcForChat) EnsureSessionManage(ctx context.Context, sessionID, userID uuid.UUID) error {
	f.manageCalls++
	f.gotManageSession, f.gotManageUser = sessionID, userID
	return f.manageErr
}

// fakeChatRepo: chi cac method duoc tung test can moi duoc override.
type fakeChatRepo struct {
	repository.ChatMessageRepositoryInterface
	createCalls     int
	lastCreated     *model.ChatMessage
	getByIDMsg      *model.ChatMessage
	getByIDErr      error
	softDeleteCalls int
	softDeleteBy    uuid.UUID
	pinCalls        int
	unpinCalls      int
}

func (f *fakeChatRepo) Create(ctx context.Context, message *model.ChatMessage) error {
	f.createCalls++
	f.lastCreated = message
	return nil
}

func (f *fakeChatRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.ChatMessage, error) {
	return f.getByIDMsg, f.getByIDErr
}

func (f *fakeChatRepo) GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.ChatMessage, int64, error) {
	return nil, 0, nil
}

func (f *fakeChatRepo) SoftDelete(ctx context.Context, id, deletedBy uuid.UUID) error {
	f.softDeleteCalls++
	f.softDeleteBy = deletedBy
	return nil
}

func (f *fakeChatRepo) Pin(ctx context.Context, id uuid.UUID) error {
	f.pinCalls++
	return nil
}

func (f *fakeChatRepo) UnPin(ctx context.Context, id uuid.UUID) error {
	f.unpinCalls++
	return nil
}

// fakeAnalyticsRepoForChat/fakeLivestreamRepoForChat: SendMessage goi cac dependency nay trong
// goroutine nen (SafeGo, tu phuc hoi panic) — nil-embed la du, khong can fake that.
type fakeAnalyticsRepoForChat struct{ repository.AnalyticsRepositoryInterface }
type fakeLivestreamRepoForChat struct{ repository.LivestreamRepositoryInterface }
type fakeLivekitSvcForChat struct{ LivekitServiceInterface }

func newChatServiceForTest(chatRepo *fakeChatRepo, lsSvc *fakeLivestreamSvcForChat) *ChatService {
	return NewChatService(
		chatRepo,
		&fakeAnalyticsRepoForChat{},
		&fakeLivestreamRepoForChat{},
		&fakeLivekitSvcForChat{},
		lsSvc,
	)
}

// --- SendMessage -----------------------------------------------------------------------------

func TestChatSendMessage_KhongPhaiThanhVien_Bi403(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForChat{memberErr: ErrNotSessionMember}
	repo := &fakeChatRepo{}
	s := newChatServiceForTest(repo, lsSvc)

	sessionID := uuid.New()
	callerID := uuid.New()

	_, err := s.SendMessage(context.Background(), callerID, dto.SendChatMessageDTO{
		SessionID: sessionID.String(),
		Message:   "xin chao",
	})

	if !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen (ErrNotSessionMember)", err)
	}
	if repo.createCalls != 0 {
		t.Error("repo.Create bi goi du bi tu choi quyen")
	}
	if lsSvc.gotMemberUser != callerID || lsSvc.gotMemberSession != sessionID {
		t.Errorf("EnsureSessionMember nhan (session=%s, user=%s), muon (session=%s, user=%s)",
			lsSvc.gotMemberSession, lsSvc.gotMemberUser, sessionID, callerID)
	}
}

// TestChatSendMessage_ThanhVien_DanhTinhTuThamSo (kich ban chinh cua V3-6): message.UserID phai
// bang chinh tham so userID truyen vao (nguoi goi that su) — DTO khong con truong UserID nao de
// mao danh nua (da bi xoa khoi SendChatMessageDTO).
func TestChatSendMessage_ThanhVien_DanhTinhTuThamSo(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForChat{}
	repo := &fakeChatRepo{}
	s := newChatServiceForTest(repo, lsSvc)

	sessionID := uuid.New()
	realCaller := uuid.New()

	msg, err := s.SendMessage(context.Background(), realCaller, dto.SendChatMessageDTO{
		SessionID: sessionID.String(),
		Message:   "xin chao",
	})

	if err != nil {
		t.Fatalf("khong muon loi: %v", err)
	}
	if repo.createCalls != 1 {
		t.Fatalf("repo.Create duoc goi %d lan, muon 1", repo.createCalls)
	}
	if msg.UserID != realCaller {
		t.Errorf("message.UserID = %s, muon nguoi goi that su = %s", msg.UserID, realCaller)
	}
}

// --- GetMessages -------------------------------------------------------------------------------

func TestChatGetMessages_KhongPhaiThanhVien_Bi403(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForChat{memberErr: ErrNotSessionMember}
	repo := &fakeChatRepo{}
	s := newChatServiceForTest(repo, lsSvc)

	_, err := s.GetMessages(context.Background(), uuid.New(), uuid.New(), 1, 20)

	if !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
}

// --- DeleteMessage -----------------------------------------------------------------------------

func TestChatDeleteMessage_TacGia_KhongCanQuyenQuanTri(t *testing.T) {
	messageID := uuid.New()
	author := uuid.New()
	lsSvc := &fakeLivestreamSvcForChat{manageErr: ErrNotClassTeacher} // se loi neu bi goi
	repo := &fakeChatRepo{getByIDMsg: &model.ChatMessage{
		BaseModel: model.BaseModel{ID: messageID},
		UserID:    author,
		SessionID: uuid.New(),
	}}
	s := newChatServiceForTest(repo, lsSvc)

	if err := s.DeleteMessage(context.Background(), author, messageID); err != nil {
		t.Fatalf("tac gia xoa tin cua chinh minh phai duoc phep: %v", err)
	}
	if lsSvc.manageCalls != 0 {
		t.Error("EnsureSessionManage bi goi du nguoi xoa la tac gia — khong can kiem quyen quan tri")
	}
	if repo.softDeleteCalls != 1 || repo.softDeleteBy != author {
		t.Errorf("SoftDelete(deletedBy=%s) sai, muon deletedBy=%s, calls=%d", repo.softDeleteBy, author, repo.softDeleteCalls)
	}
}

func TestChatDeleteMessage_NguoiLa_Bi403(t *testing.T) {
	messageID := uuid.New()
	sessionID := uuid.New()
	lsSvc := &fakeLivestreamSvcForChat{manageErr: ErrNotClassTeacher}
	repo := &fakeChatRepo{getByIDMsg: &model.ChatMessage{
		BaseModel: model.BaseModel{ID: messageID},
		UserID:    uuid.New(), // tac gia khac nguoi goi
		SessionID: sessionID,
	}}
	s := newChatServiceForTest(repo, lsSvc)

	stranger := uuid.New()
	err := s.DeleteMessage(context.Background(), stranger, messageID)

	if !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
	if repo.softDeleteCalls != 0 {
		t.Error("SoftDelete bi goi du bi tu choi quyen")
	}
	if lsSvc.gotManageSession != sessionID {
		t.Errorf("EnsureSessionManage kiem sai session: %s, muon %s", lsSvc.gotManageSession, sessionID)
	}
}

func TestChatDeleteMessage_NguoiQuanTriPhien_XoaTinNguoiKhac_ChoPhep(t *testing.T) {
	messageID := uuid.New()
	lsSvc := &fakeLivestreamSvcForChat{} // manageErr = nil -> la nguoi quan tri
	repo := &fakeChatRepo{getByIDMsg: &model.ChatMessage{
		BaseModel: model.BaseModel{ID: messageID},
		UserID:    uuid.New(),
		SessionID: uuid.New(),
	}}
	s := newChatServiceForTest(repo, lsSvc)

	host := uuid.New()
	if err := s.DeleteMessage(context.Background(), host, messageID); err != nil {
		t.Fatalf("nguoi quan tri phien phai xoa duoc tin cua nguoi khac: %v", err)
	}
	if repo.softDeleteBy != host {
		t.Errorf("SoftDelete(deletedBy=%s), muon %s", repo.softDeleteBy, host)
	}
}

// --- Pin/UnPin -----------------------------------------------------------------------------

func TestChatPinMessage_NguoiLa_Bi403(t *testing.T) {
	messageID := uuid.New()
	lsSvc := &fakeLivestreamSvcForChat{manageErr: ErrNotClassTeacher}
	repo := &fakeChatRepo{getByIDMsg: &model.ChatMessage{
		BaseModel: model.BaseModel{ID: messageID},
		SessionID: uuid.New(),
	}}
	s := newChatServiceForTest(repo, lsSvc)

	if err := s.PinMessage(context.Background(), uuid.New(), messageID); !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
	if repo.pinCalls != 0 {
		t.Error("repo.Pin bi goi du bi tu choi quyen")
	}
}

func TestChatPinMessage_NguoiQuanTriPhien_ChoPhep(t *testing.T) {
	messageID := uuid.New()
	lsSvc := &fakeLivestreamSvcForChat{}
	repo := &fakeChatRepo{getByIDMsg: &model.ChatMessage{
		BaseModel: model.BaseModel{ID: messageID},
		SessionID: uuid.New(),
	}}
	s := newChatServiceForTest(repo, lsSvc)

	if err := s.PinMessage(context.Background(), uuid.New(), messageID); err != nil {
		t.Fatalf("khong muon loi: %v", err)
	}
	if repo.pinCalls != 1 {
		t.Errorf("repo.Pin duoc goi %d lan, muon 1", repo.pinCalls)
	}
}

func TestChatUnPinMessage_NguoiLa_Bi403(t *testing.T) {
	messageID := uuid.New()
	lsSvc := &fakeLivestreamSvcForChat{manageErr: ErrNotClassTeacher}
	repo := &fakeChatRepo{getByIDMsg: &model.ChatMessage{
		BaseModel: model.BaseModel{ID: messageID},
		SessionID: uuid.New(),
	}}
	s := newChatServiceForTest(repo, lsSvc)

	if err := s.UnPinMessage(context.Background(), uuid.New(), messageID); !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
	if repo.unpinCalls != 0 {
		t.Error("repo.UnPin bi goi du bi tu choi quyen")
	}
}

func TestChatDeleteMessage_KhongTimThayTinNhan_LoiKhongPhaiUyQuyen(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForChat{}
	repo := &fakeChatRepo{getByIDErr: errors.New("db error")}
	s := newChatServiceForTest(repo, lsSvc)

	err := s.DeleteMessage(context.Background(), uuid.New(), uuid.New())
	if err == nil || IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi ha tang (khong phai uy quyen)", err)
	}
	if lsSvc.manageCalls != 0 {
		t.Error("EnsureSessionManage bi goi du chua load duoc tin nhan")
	}
}
