package service

// Test cho V3-6 (issue #58), tang service cua WhiteboardService: doc/ghi/phat song bang trang
// deu doi hoi la THANH VIEN cua phien (EnsureSessionMember). Truoc day khong kiem gi ca — bat ky
// user dang nhap nao cung doc/ghi/phat song duoc bang trang cua bat ky phien nao.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeLivestreamSvcForWhiteboard: chi EnsureSessionMember duoc dung.
type fakeLivestreamSvcForWhiteboard struct {
	LivestreamServiceInterface
	memberErr        error
	memberCalls      int
	gotMemberSession uuid.UUID
	gotMemberUser    uuid.UUID
}

func (f *fakeLivestreamSvcForWhiteboard) EnsureSessionMember(ctx context.Context, sessionID, userID uuid.UUID) error {
	f.memberCalls++
	f.gotMemberSession, f.gotMemberUser = sessionID, userID
	return f.memberErr
}

type fakeWhiteboardRepo struct {
	repository.WhiteboardRepositoryInterface
	saveCalls int
	getCalls  int
}

func (f *fakeWhiteboardRepo) GetSnapshot(ctx context.Context, sessionID uuid.UUID) (*model.WhiteboardSnapshot, error) {
	f.getCalls++
	return nil, nil
}

func (f *fakeWhiteboardRepo) SaveSnapshot(ctx context.Context, snapshot *model.WhiteboardSnapshot) error {
	f.saveCalls++
	return nil
}

type fakeSessionRepoForWhiteboard struct {
	repository.LivestreamRepositoryInterface
	session *model.LivestreamSession
}

func (f *fakeSessionRepoForWhiteboard) GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	return f.session, nil
}

type fakeLivekitSvcForWhiteboard struct {
	LivekitServiceInterface
	sendDataCalls int
}

func (f *fakeLivekitSvcForWhiteboard) SendData(ctx context.Context, roomName string, req dto.SendDataDTO) error {
	f.sendDataCalls++
	return nil
}

func newWhiteboardServiceForTest(repo *fakeWhiteboardRepo, sessionRepo *fakeSessionRepoForWhiteboard, lsSvc *fakeLivestreamSvcForWhiteboard) *WhiteboardService {
	return NewWhiteboardService(repo, sessionRepo, nil, lsSvc)
}

func TestWhiteboardGetSnapshot_KhongPhaiThanhVien_Bi403(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForWhiteboard{memberErr: ErrNotSessionMember}
	repo := &fakeWhiteboardRepo{}
	s := newWhiteboardServiceForTest(repo, &fakeSessionRepoForWhiteboard{}, lsSvc)

	sessionID := uuid.New()
	caller := uuid.New()
	_, err := s.GetSnapshot(context.Background(), caller, sessionID)

	if !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
	if repo.getCalls != 0 {
		t.Error("repo.GetSnapshot bi goi du bi tu choi quyen")
	}
	if lsSvc.gotMemberSession != sessionID || lsSvc.gotMemberUser != caller {
		t.Errorf("EnsureSessionMember nhan sai tham so: session=%s user=%s", lsSvc.gotMemberSession, lsSvc.gotMemberUser)
	}
}

func TestWhiteboardGetSnapshot_ThanhVien_ChoPhep(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForWhiteboard{}
	repo := &fakeWhiteboardRepo{}
	s := newWhiteboardServiceForTest(repo, &fakeSessionRepoForWhiteboard{}, lsSvc)

	if _, err := s.GetSnapshot(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("khong muon loi: %v", err)
	}
	if repo.getCalls != 1 {
		t.Errorf("repo.GetSnapshot duoc goi %d lan, muon 1", repo.getCalls)
	}
}

func TestWhiteboardSaveSnapshot_KhongPhaiThanhVien_Bi403(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForWhiteboard{memberErr: ErrNotSessionMember}
	repo := &fakeWhiteboardRepo{}
	s := newWhiteboardServiceForTest(repo, &fakeSessionRepoForWhiteboard{}, lsSvc)

	sessionID := uuid.New()
	err := s.SaveSnapshot(context.Background(), uuid.New(), dto.WhiteboardSnapshotDTO{SessionID: sessionID.String()})

	if !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
	if repo.saveCalls != 0 {
		t.Error("repo.SaveSnapshot bi goi du bi tu choi quyen")
	}
}

func TestWhiteboardSaveSnapshot_ThanhVien_ChoPhep(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForWhiteboard{}
	repo := &fakeWhiteboardRepo{}
	s := newWhiteboardServiceForTest(repo, &fakeSessionRepoForWhiteboard{}, lsSvc)

	err := s.SaveSnapshot(context.Background(), uuid.New(), dto.WhiteboardSnapshotDTO{SessionID: uuid.New().String()})
	if err != nil {
		t.Fatalf("khong muon loi: %v", err)
	}
	if repo.saveCalls != 1 {
		t.Errorf("repo.SaveSnapshot duoc goi %d lan, muon 1", repo.saveCalls)
	}
}

func TestWhiteboardBroadcastEvent_KhongPhaiThanhVien_Bi403(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForWhiteboard{memberErr: ErrNotSessionMember}
	sessionRepo := &fakeSessionRepoForWhiteboard{session: &model.LivestreamSession{RoomName: "room-1"}}
	s := newWhiteboardServiceForTest(&fakeWhiteboardRepo{}, sessionRepo, lsSvc)
	livekit := &fakeLivekitSvcForWhiteboard{}

	err := s.BroadcastEvent(context.Background(), uuid.New(), uuid.New(), dto.WhiteboardEventDTO{Type: "draw", Action: "add"}, livekit)

	if !IsForbiddenErr(err) {
		t.Fatalf("err = %v, muon loi uy quyen", err)
	}
	if livekit.sendDataCalls != 0 {
		t.Error("livekitSvc.SendData bi goi du bi tu choi quyen")
	}
}

func TestWhiteboardBroadcastEvent_ThanhVien_ChoPhep(t *testing.T) {
	lsSvc := &fakeLivestreamSvcForWhiteboard{}
	sessionRepo := &fakeSessionRepoForWhiteboard{session: &model.LivestreamSession{RoomName: "room-1"}}
	s := newWhiteboardServiceForTest(&fakeWhiteboardRepo{}, sessionRepo, lsSvc)
	livekit := &fakeLivekitSvcForWhiteboard{}

	err := s.BroadcastEvent(context.Background(), uuid.New(), uuid.New(), dto.WhiteboardEventDTO{Type: "draw", Action: "add"}, livekit)

	if err != nil {
		t.Fatalf("khong muon loi: %v", err)
	}
	if livekit.sendDataCalls != 1 {
		t.Errorf("livekitSvc.SendData duoc goi %d lan, muon 1", livekit.sendDataCalls)
	}
}
