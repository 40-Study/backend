package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeLivestreamRepoHostTest ghi lai session ma Create truyen xuong — chi can Create, cac method
// khac cua LivestreamRepositoryInterface khong duoc goi trong pham vi test nay.
type fakeLivestreamRepoHostTest struct {
	repository.LivestreamRepositoryInterface
	created *model.LivestreamSession
}

func (f *fakeLivestreamRepoHostTest) Create(ctx context.Context, session *model.LivestreamSession) error {
	f.created = session
	return nil
}

// fakeAnalyticsRepoHostTest: Create() cua LivestreamService.Create goi analyticsRepo.Create vo
// dieu kien sau khi luu session — can mot fake that de khong panic tren interface nil.
type fakeAnalyticsRepoHostTest struct {
	repository.AnalyticsRepositoryInterface
}

func (f *fakeAnalyticsRepoHostTest) Create(ctx context.Context, analytics *model.LivestreamAnalytics) error {
	return nil
}

// TestLivestreamCreate_HostIDLuonLaNguoiGoiThat (review 260915, tu PR web #16): host cua phien
// PHAI la nguoi goi Create — truoc day handler nhan host_id THANG TU BODY client va dung nguyen,
// nen bat ky user dang nhap nao cung tao duoc livestream mang ten mot user KHAC bang cach tu khai
// UUID cua ho trong body. CreateLivestreamDTO gio KHONG CON truong host_id — hostID la tham so
// RIENG cua Create, nen khong con duong nao de client anh huong toi gia tri nay nua (chan o tang
// kieu du lieu, khong chi o logic runtime).
func TestLivestreamCreate_HostIDLuonLaNguoiGoiThat(t *testing.T) {
	repo := &fakeLivestreamRepoHostTest{}
	analyticsRepo := &fakeAnalyticsRepoHostTest{}
	svc := NewLivestreamService(repo, nil, analyticsRepo, nil, nil, nil, nil)

	callerID := uuid.New()
	classID := uuid.New()

	req := dto.CreateLivestreamDTO{
		Title:   "Buoi hoc thu 1",
		ClassID: classID.String(),
	}

	session, err := svc.Create(context.Background(), callerID, req)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if session.HostID != callerID {
		t.Errorf("HostID tra ve = %s, mong doi callerID = %s", session.HostID, callerID)
	}
	if repo.created == nil {
		t.Fatal("repo.Create khong duoc goi")
	}
	if repo.created.HostID != callerID {
		t.Errorf("ban ghi truyen xuong repo co HostID = %s, mong doi %s", repo.created.HostID, callerID)
	}
}

// TestLivestreamCreate_TrenNguoiGoiKhacNhauChoRaHostKhacNhau: hai lan goi voi hai callerID khac
// nhau, cung mot req (ClassID giong het) phai cho ra hai session voi HostID khac nhau tuong ung —
// khang dinh gia tri KHONG bi hard-code/nham lan sang mot bien khac trong scope.
func TestLivestreamCreate_TrenNguoiGoiKhacNhauChoRaHostKhacNhau(t *testing.T) {
	classID := uuid.New()
	req := dto.CreateLivestreamDTO{Title: "Buoi hoc", ClassID: classID.String()}

	caller1, caller2 := uuid.New(), uuid.New()

	repo1 := &fakeLivestreamRepoHostTest{}
	svc1 := NewLivestreamService(repo1, nil, &fakeAnalyticsRepoHostTest{}, nil, nil, nil, nil)
	session1, err := svc1.Create(context.Background(), caller1, req)
	if err != nil {
		t.Fatalf("khong mong doi loi (caller1): %v", err)
	}

	repo2 := &fakeLivestreamRepoHostTest{}
	svc2 := NewLivestreamService(repo2, nil, &fakeAnalyticsRepoHostTest{}, nil, nil, nil, nil)
	session2, err := svc2.Create(context.Background(), caller2, req)
	if err != nil {
		t.Fatalf("khong mong doi loi (caller2): %v", err)
	}

	if session1.HostID != caller1 {
		t.Errorf("session1.HostID = %s, mong doi caller1 = %s", session1.HostID, caller1)
	}
	if session2.HostID != caller2 {
		t.Errorf("session2.HostID = %s, mong doi caller2 = %s", session2.HostID, caller2)
	}
	if session1.HostID == session2.HostID {
		t.Error("hai callerID khac nhau nhung ra cung mot HostID — gia tri co the bi hard-code")
	}
}
