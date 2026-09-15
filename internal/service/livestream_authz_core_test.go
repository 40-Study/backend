package service

// F-3 (issue #58 review vong 2, 260915): test co RUOT cho 5 diem uy quyen loi cua
// LivestreamService, dung fake repository (khong phai stub handler service nhu
// livestream_handler_test.go) de bai kiem thuc su cham vao logic can bao ve:
//
//   - resolveJoinRole: host/GV lop/instructor -> teacher; hoc sinh lop -> student;
//     nguoi la -> ErrNotSessionMember; D1 (F-6): hoc sinh lop KHAC cung khoa -> tu choi.
//   - canManageSession / getManageableSession: host/GV lop/admin -> cho phep, nguoi la -> tu choi.
//   - KickParticipant: chan kick host (ErrCannotKickHost).
//   - Join: nguoi da bi kick (IsKicked=true) khong duoc vao lai (ErrParticipantKicked).
//
// Hai kich ban dot bien (M-1, M-2) ma nguoi review chi dinh duoc chay THU CONG bang tay (sua
// code that, chay lai bo test nay, xac nhan RED, roi revert) — ket qua ghi trong bao cao "Vong 2",
// khong sinh test rieng trong file nay (mutation testing khong phai mot test case ton tai lau dai).

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	livekit "github.com/livekit/protocol/livekit"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// --- fakes dung rieng cho file nay --------------------------------------------------------

// fakeClassRepoJoin: cho phep cau hinh doc lap ca 3 nhanh quan he (GV lop / hoc sinh lop /
// quan he gop IsUserRelatedToClass) ma khong dung chung voi fakeClassRepoAuthz (file
// livestream_service_authz_test.go) vi file do chi phuc vu Create, khong co StudentClassExists
// hay IsUserRelatedToClass.
type fakeClassRepoJoin struct {
	repository.ClassRepositoryInterface
	class       *model.Class
	isTeacher   bool
	isStudent   bool
	relatedFlag bool
}

func (f *fakeClassRepoJoin) GetByID(ctx context.Context, id uuid.UUID) (*model.Class, error) {
	return f.class, nil
}

func (f *fakeClassRepoJoin) TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error) {
	return f.isTeacher, nil
}

func (f *fakeClassRepoJoin) StudentClassExists(ctx context.Context, classID, studentID uuid.UUID) (bool, error) {
	return f.isStudent, nil
}

func (f *fakeClassRepoJoin) IsUserRelatedToClass(ctx context.Context, classID, userID uuid.UUID) (bool, error) {
	return f.relatedFlag, nil
}

type fakeCourseRepoJoin struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseRepoJoin) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

// fakeLivestreamRepoJoin: GetByID tra ve session cau hinh san — dung cho getManageableSession
// va Join (khong can cac method khac cua LivestreamRepositoryInterface trong pham vi test nay).
type fakeLivestreamRepoJoin struct {
	repository.LivestreamRepositoryInterface
	session *model.LivestreamSession
}

func (f *fakeLivestreamRepoJoin) GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	return f.session, nil
}

// fakeParticipantRepoJoin: GetBySessionAndUser tra ve participant cau hinh san (co the nil =
// chua tung tham gia); MarkKicked/ ghi lai loi goi de test kiem duoc F-5 co thuc su chay.
type fakeParticipantRepoJoin struct {
	repository.ParticipantRepositoryInterface
	existing        *model.Participant
	markKickedCalls []uuid.UUID
}

func (f *fakeParticipantRepoJoin) GetBySessionAndUser(ctx context.Context, sessionID, userID uuid.UUID) (*model.Participant, error) {
	return f.existing, nil
}

func (f *fakeParticipantRepoJoin) MarkKicked(ctx context.Context, id uuid.UUID) error {
	f.markKickedCalls = append(f.markKickedCalls, id)
	return nil
}

// fakeLivekitSvcJoin: ghi lai identity ma RemoveParticipant duoc goi voi — dung de chung minh
// KickParticipant that su goi LiveKit ngat ket noi (khong chi doi DB). gotUpdateReq (R3-2, review
// vong 3) ghi lai DTO ma UpdateParticipant nhan duoc lan goi GAN NHAT — dung cho
// TestStartScreenShare_ChiCapNguonManHinh.
type fakeLivekitSvcJoin struct {
	LivekitServiceInterface
	removeParticipantCalls []string
	gotUpdateReq           dto.UpdateParticipantDTO
	updateCalls            int
}

func (f *fakeLivekitSvcJoin) RemoveParticipant(ctx context.Context, roomName, identity string) error {
	f.removeParticipantCalls = append(f.removeParticipantCalls, identity)
	return nil
}

func (f *fakeLivekitSvcJoin) UpdateParticipant(ctx context.Context, roomName, identity string, req dto.UpdateParticipantDTO) (*livekit.ParticipantInfo, error) {
	f.updateCalls++
	f.gotUpdateReq = req
	return nil, nil
}

func newLivestreamServiceForJoin(
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	repo repository.LivestreamRepositoryInterface,
	participantRepo repository.ParticipantRepositoryInterface,
	livekitSvc LivekitServiceInterface,
) *LivestreamService {
	return NewLivestreamService(repo, participantRepo, &fakeAnalyticsRepoHostTest{}, classRepo, courseRepo, nil, nil, livekitSvc, nil, nil)
}

// --- resolveJoinRole ------------------------------------------------------------------------

func TestResolveJoinRole_Host_LaTeacher(t *testing.T) {
	hostID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: hostID, ClassID: uuid.New()}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, nil, nil, nil)

	role, err := svc.resolveJoinRole(context.Background(), hostID, session)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if role != model.ParticipantRoleTeacher {
		t.Errorf("role = %q, mong doi teacher", role)
	}
}

func TestResolveJoinRole_GiaoVienLop_LaTeacher(t *testing.T) {
	classID := uuid.New()
	teacherID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isTeacher: true}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	role, err := svc.resolveJoinRole(context.Background(), teacherID, session)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if role != model.ParticipantRoleTeacher {
		t.Errorf("role = %q, mong doi teacher", role)
	}
}

func TestResolveJoinRole_InstructorKhoaHoc_LaTeacher(t *testing.T) {
	classID := uuid.New()
	courseID := uuid.New()
	instructorID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}, isTeacher: false}
	courseRepo := &fakeCourseRepoJoin{course: &model.Course{BaseModel: model.BaseModel{ID: courseID}, InstructorID: instructorID}}
	svc := newLivestreamServiceForJoin(classRepo, courseRepo, nil, nil, nil)

	role, err := svc.resolveJoinRole(context.Background(), instructorID, session)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if role != model.ParticipantRoleTeacher {
		t.Errorf("role = %q, mong doi teacher", role)
	}
}

func TestResolveJoinRole_HocSinhCuaLop_LaStudent(t *testing.T) {
	classID := uuid.New()
	studentID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isTeacher: false, isStudent: true}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	role, err := svc.resolveJoinRole(context.Background(), studentID, session)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if role != model.ParticipantRoleStudent {
		t.Errorf("role = %q, mong doi student", role)
	}
}

func TestResolveJoinRole_NguoiLa_ErrNotSessionMember(t *testing.T) {
	classID := uuid.New()
	strangerID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isTeacher: false, isStudent: false}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	_, err := svc.resolveJoinRole(context.Background(), strangerID, session)
	if err != ErrNotSessionMember {
		t.Errorf("loi = %v, mong doi ErrNotSessionMember", err)
	}
}

// TestResolveJoinRole_D1_HocSinhLopKhacCungKhoa_BiTuChoi (D1/F-6, tam diem cua vong review nay):
// truoc fix, khi khong phai hoc sinh CUA LOP nay thi co mot nhanh du phong kiem enrollment theo
// KHOA HOC (session.CourseID) — hoc sinh cua lop B (khac lop A cua phien, nhung cung mot khoa hoc)
// van vao duoc phien cua lop A. Sau D1, nhanh du phong CHI con dieu kien khi session.ClassID ==
// uuid.Nil (khong xay ra voi schema hien tai vi ClassID NOT NULL) — nen mot hoc sinh khong thuoc
// LOP cua phien phai bi tu choi VO DIEU KIEN, duho co the la hoc sinh hop le cua mot lop khac
// trong CUNG khoa hoc do (mo phong bang isStudent=false, class co CourseID nhung enrollmentRepo
// duoc truyen nil de chung minh nhanh do KHONG con duoc goi toi khi ClassID != Nil).
func TestResolveJoinRole_D1_HocSinhLopKhacCungKhoa_BiTuChoi(t *testing.T) {
	classIDCuaPhien := uuid.New()
	courseID := uuid.New()
	hocSinhLopKhac := uuid.New()

	session := &model.LivestreamSession{
		BaseModel: model.BaseModel{ID: uuid.New()},
		HostID:    uuid.New(),
		ClassID:   classIDCuaPhien,
		CourseID:  &courseID, // phien co gan khoa hoc — truoc D1 day la dieu kien kich hoat fallback sai
	}
	classRepo := &fakeClassRepoJoin{
		class:     &model.Class{BaseModel: model.BaseModel{ID: classIDCuaPhien}, CourseID: &courseID},
		isTeacher: false,
		isStudent: false, // KHONG phai hoc sinh cua LOP nay (chi la hoc sinh mot lop khac cung khoa)
	}
	// enrollmentRepo = nil: neu resolveJoinRole con goi toi (loi hoi quy cua D1) se panic ngay,
	// lam lo test thay vi im lang cho qua.
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	_, err := svc.resolveJoinRole(context.Background(), hocSinhLopKhac, session)
	if err != ErrNotSessionMember {
		t.Errorf("loi = %v, mong doi ErrNotSessionMember (D1: hoc sinh lop khac KHONG duoc vao qua fallback theo khoa hoc)", err)
	}
}

// --- canManageSession / getManageableSession -------------------------------------------------

func TestCanManageSession_Host_ChoPhep(t *testing.T) {
	hostID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: hostID, ClassID: uuid.New()}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, nil, nil, nil)

	if err := svc.canManageSession(context.Background(), hostID, false, session); err != nil {
		t.Errorf("khong mong doi loi cho host: %v", err)
	}
}

func TestCanManageSession_GiaoVienLop_ChoPhep(t *testing.T) {
	classID := uuid.New()
	teacherID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isTeacher: true}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	if err := svc.canManageSession(context.Background(), teacherID, false, session); err != nil {
		t.Errorf("khong mong doi loi cho GV lop: %v", err)
	}
}

// TestCanManageSession_Admin_ChoPhepDuKhongPhaiHost (D2): admin he thong quan tri duoc phien du
// khong phai host/GV lop — dung classRepo KHONG cho phep (isTeacher=false) de chung minh nhanh
// admin thuc su bypass, khong phai tinh co canManageClass cho qua.
func TestCanManageSession_Admin_ChoPhepDuKhongPhaiHost(t *testing.T) {
	classID := uuid.New()
	adminID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isTeacher: false}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	if err := svc.canManageSession(context.Background(), adminID, true, session); err != nil {
		t.Errorf("khong mong doi loi cho admin (D2 bypass): %v", err)
	}
}

func TestCanManageSession_NguoiLa_TuChoi(t *testing.T) {
	classID := uuid.New()
	strangerID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: uuid.New()}, HostID: uuid.New(), ClassID: classID}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isTeacher: false}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, nil, nil, nil)

	err := svc.canManageSession(context.Background(), strangerID, false, session)
	if err != ErrNotClassTeacher {
		t.Errorf("loi = %v, mong doi ErrNotClassTeacher", err)
	}
}

func TestGetManageableSession_TaiPhienVaKiemQuyenTrongMotBuoc(t *testing.T) {
	classID := uuid.New()
	sessionID := uuid.New()
	hostID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: hostID, ClassID: classID}
	repo := &fakeLivestreamRepoJoin{session: session}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, repo, nil, nil)

	got, err := svc.getManageableSession(context.Background(), hostID, false, sessionID)
	if err != nil {
		t.Fatalf("khong mong doi loi cho host: %v", err)
	}
	if got != session {
		t.Error("session tra ve khong phai session da nap tu repo")
	}

	// Nguoi la voi cung phien -> phai bi tu choi (khong chi kiem hostID ma con phai qua canManageSession that su).
	if _, err := svc.getManageableSession(context.Background(), uuid.New(), false, sessionID); err == nil {
		t.Error("mong doi loi khi nguoi la goi getManageableSession")
	}
}

// --- KickParticipant: chan kick host ----------------------------------------------------------

func TestKickParticipant_ChanDaHostRaKhoiPhong(t *testing.T) {
	hostID := uuid.New()
	sessionID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: hostID, ClassID: uuid.New(), RoomName: "room-1"}
	repo := &fakeLivestreamRepoJoin{session: session}
	livekitSvc := &fakeLivekitSvcJoin{}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, repo, &fakeParticipantRepoJoin{}, livekitSvc)

	// actorID = hostID: host tu goi kick nham chinh minh (hoac mot GV khac co quyen quan tri
	// nhung target = host) — V3-7 phai chan boi VI TARGET la host, khong phai vi actor thieu quyen.
	err := svc.KickParticipant(context.Background(), hostID, false, sessionID, hostID)
	if err != ErrCannotKickHost {
		t.Errorf("loi = %v, mong doi ErrCannotKickHost", err)
	}
	if len(livekitSvc.removeParticipantCalls) != 0 {
		t.Error("LiveKit RemoveParticipant KHONG duoc goi khi target la host — nhung da bi goi")
	}
}

func TestKickParticipant_HostKickHocSinh_ThanhCong_VaGhiNhoIsKicked(t *testing.T) {
	hostID := uuid.New()
	sessionID := uuid.New()
	targetID := uuid.New()
	targetParticipantID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: hostID, ClassID: uuid.New(), RoomName: "room-1"}
	repo := &fakeLivestreamRepoJoin{session: session}
	livekitSvc := &fakeLivekitSvcJoin{}
	participantRepo := &fakeParticipantRepoJoin{existing: &model.Participant{BaseModel: model.BaseModel{ID: targetParticipantID}, SessionID: sessionID, UserID: targetID}}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, repo, participantRepo, livekitSvc)

	if err := svc.KickParticipant(context.Background(), hostID, false, sessionID, targetID); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if len(livekitSvc.removeParticipantCalls) != 1 || livekitSvc.removeParticipantCalls[0] != targetID.String() {
		t.Errorf("RemoveParticipant calls = %v, mong doi mot lan voi identity %s", livekitSvc.removeParticipantCalls, targetID)
	}
	if len(participantRepo.markKickedCalls) != 1 || participantRepo.markKickedCalls[0] != targetParticipantID {
		t.Errorf("MarkKicked calls = %v, mong doi mot lan voi id %s (F-5: kick phai ben trong DB)", participantRepo.markKickedCalls, targetParticipantID)
	}
}

// --- Join: nguoi da bi kick khong duoc vao lai (F-5) -------------------------------------------

func TestJoin_NguoiDaBiKick_KhongDuocVaoLai(t *testing.T) {
	hostID := uuid.New()
	sessionID := uuid.New()
	kickedUserID := uuid.New()
	session := &model.LivestreamSession{
		BaseModel:  model.BaseModel{ID: sessionID},
		HostID:     hostID,
		ClassID:    uuid.New(),
		RoomName:   "room-1",
		Status:     model.LivestreamStatusLive, // qua duoc kiem trang thai/som-tre truoc khi cham toi kiem kick
		MaxViewers: 0,                          // = 0 de khong cham toi CountActiveBySession (chua fake)
	}
	repo := &fakeLivestreamRepoJoin{session: session}
	participantRepo := &fakeParticipantRepoJoin{
		existing: &model.Participant{
			BaseModel: model.BaseModel{ID: uuid.New()},
			SessionID: sessionID,
			UserID:    kickedUserID,
			IsKicked:  true,
		},
	}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, repo, participantRepo, nil)

	_, err := svc.Join(context.Background(), sessionID, kickedUserID, false, dto.JoinLivestreamDTO{Name: "Nguoi da bi kick"})
	if err != ErrParticipantKicked {
		t.Errorf("loi = %v, mong doi ErrParticipantKicked", err)
	}
}

// --- R2-2 (review vong 3): EnsureSessionMember phai chan nguoi da bi kick ---------------------
//
// F-5 (vong 2) chi chan duong Join — EnsureSessionMember la cong gac cua chat gui/doc VA bang
// trang doc/ghi/broadcast, khong kiem IsKicked, nen nguoi bi kick van gui chat/ghi bang binh
// thuong qua REST du da bi ngat khoi LiveKit.

func TestEnsureSessionMember_NguoiDaBiKick_Bi403Kicked(t *testing.T) {
	sessionID := uuid.New()
	kickedUserID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: uuid.New(), ClassID: uuid.New()}
	repo := &fakeLivestreamRepoJoin{session: session}
	// relatedFlag=true: VAN la thanh vien lop hop le — chung minh chinh IsKicked la ly do tu choi,
	// khong phai vi thieu quan he lop.
	classRepo := &fakeClassRepoJoin{relatedFlag: true}
	participantRepo := &fakeParticipantRepoJoin{
		existing: &model.Participant{
			BaseModel: model.BaseModel{ID: uuid.New()},
			SessionID: sessionID,
			UserID:    kickedUserID,
			IsKicked:  true,
		},
	}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, repo, participantRepo, nil)

	err := svc.EnsureSessionMember(context.Background(), sessionID, kickedUserID)
	if err != ErrParticipantKicked {
		t.Errorf("loi = %v, mong doi ErrParticipantKicked (van la thanh vien lop nhung da bi kick khoi PHIEN NAY)", err)
	}
}

// TestEnsureSessionMember_ChuaTungThamGia_KhongBiChanOanBoiKiemKick: hoi quy — them kiem IsKicked
// khong duoc lam chan oan nguoi CHUA TUNG join phien (participant == nil trong DB).
func TestEnsureSessionMember_ChuaTungThamGia_KhongBiChanOanBoiKiemKick(t *testing.T) {
	sessionID := uuid.New()
	userID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: uuid.New(), ClassID: uuid.New()}
	repo := &fakeLivestreamRepoJoin{session: session}
	classRepo := &fakeClassRepoJoin{relatedFlag: true}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, repo, &fakeParticipantRepoJoin{}, nil)

	if err := svc.EnsureSessionMember(context.Background(), sessionID, userID); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
}

// --- R2-3 (review vong 3): participantGrant khong co test nao — M-5 huy toan bo D3 ma suite van
// xanh. Bon ca bang thuan, khang dinh ca bo ba (canPublish, canSubscribe, canPublishData).

func TestParticipantGrant_BangTheoVaiTroVaKhoaBang(t *testing.T) {
	cases := []struct {
		name                                        string
		role                                        model.ParticipantRole
		locked                                      bool
		wantPublish, wantSubscribe, wantPublishData bool
	}{
		{"teacher_bang_mo", model.ParticipantRoleTeacher, false, true, true, true},
		{"teacher_bang_khoa", model.ParticipantRoleTeacher, true, true, true, true},
		{"student_bang_mo", model.ParticipantRoleStudent, false, false, true, true},
		{"student_bang_khoa", model.ParticipantRoleStudent, true, false, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPublish, gotSubscribe, gotPublishData := participantGrant(c.role, c.locked)
			if gotPublish != c.wantPublish {
				t.Errorf("canPublish = %v, muon %v", gotPublish, c.wantPublish)
			}
			if gotSubscribe != c.wantSubscribe {
				t.Errorf("canSubscribe = %v, muon %v", gotSubscribe, c.wantSubscribe)
			}
			if gotPublishData != c.wantPublishData {
				t.Errorf("canPublishData = %v, muon %v", gotPublishData, c.wantPublishData)
			}
		})
	}
}

// --- R3-2 (review vong 3): D5 khong co test — M-7 xoa CanPublishSources khoi StartScreenShare
// (duyet chia se man hinh mo lai ca camera/microphone) ma suite van xanh.

func TestStartScreenShare_ChiCapNguonManHinh_KhongCamMic(t *testing.T) {
	hostID := uuid.New()
	sessionID := uuid.New()
	targetID := uuid.New()
	session := &model.LivestreamSession{
		BaseModel: model.BaseModel{ID: sessionID},
		HostID:    hostID,
		ClassID:   uuid.New(),
		RoomName:  "room-1",
	}
	repo := &fakeLivestreamRepoJoin{session: session}
	livekitSvc := &fakeLivekitSvcJoin{}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, repo, &fakeParticipantRepoJoin{}, livekitSvc)

	if err := svc.StartScreenShare(context.Background(), hostID, false, sessionID, targetID); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if livekitSvc.updateCalls != 1 {
		t.Fatalf("UpdateParticipant duoc goi %d lan, muon 1", livekitSvc.updateCalls)
	}
	if livekitSvc.gotUpdateReq.CanPublish == nil || !*livekitSvc.gotUpdateReq.CanPublish {
		t.Error("CanPublish phai la true khi duyet chia se man hinh")
	}
	wantSources := []string{"screen_share", "screen_share_audio"}
	if !reflect.DeepEqual(livekitSvc.gotUpdateReq.CanPublishSources, wantSources) {
		t.Errorf("CanPublishSources = %v, muon %v (D5: khong duoc mo kem camera/microphone)",
			livekitSvc.gotUpdateReq.CanPublishSources, wantSources)
	}
}

// TestStopScreenShare_HaCanPublish_KhongConGioiHanNguon: StopScreenShare thu lai quyen bang cach
// dat CanPublish=false — kiem tra khong con truyen CanPublishSources (khong can thiet khi da tat
// publish hoan toan), tranh nham lan neu ai do sau nay vo tinh copy sai tu StartScreenShare.
func TestStopScreenShare_HaCanPublish(t *testing.T) {
	hostID := uuid.New()
	sessionID := uuid.New()
	targetID := uuid.New()
	session := &model.LivestreamSession{
		BaseModel: model.BaseModel{ID: sessionID},
		HostID:    hostID,
		ClassID:   uuid.New(),
		RoomName:  "room-1",
	}
	repo := &fakeLivestreamRepoJoin{session: session}
	livekitSvc := &fakeLivekitSvcJoin{}
	svc := newLivestreamServiceForJoin(&fakeClassRepoJoin{}, &fakeCourseRepoJoin{}, repo, &fakeParticipantRepoJoin{}, livekitSvc)

	if err := svc.StopScreenShare(context.Background(), hostID, false, sessionID, targetID); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if livekitSvc.gotUpdateReq.CanPublish == nil || *livekitSvc.gotUpdateReq.CanPublish {
		t.Error("CanPublish phai la false sau khi thu hoi chia se man hinh")
	}
}

// --- R3-5 (review vong 3): ensureMemberOfSession (duong ma GetByID/GetParticipants dung) khong
// kiem IsKicked nhu EnsureSessionMember (duong chat/bang trang) — hai dinh nghia "thanh vien
// phien" lech nhau trong cung file (dung R2-9 da canh bao). Nguoi bi kick van doc duoc chi tiet
// phien va roster.

func TestGetByID_NguoiDaBiKick_Bi403Kicked(t *testing.T) {
	sessionID := uuid.New()
	classID := uuid.New()
	kickedUserID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: uuid.New(), ClassID: classID}
	repo := &fakeLivestreamRepoJoin{session: session}
	// isStudent=true: van la thanh vien lop hop le, chung minh chinh IsKicked la ly do tu choi.
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isStudent: true}
	participantRepo := &fakeParticipantRepoJoin{
		existing: &model.Participant{
			BaseModel: model.BaseModel{ID: uuid.New()},
			SessionID: sessionID,
			UserID:    kickedUserID,
			IsKicked:  true,
		},
	}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, repo, participantRepo, nil)

	_, err := svc.GetByID(context.Background(), kickedUserID, false, sessionID)
	if err != ErrParticipantKicked {
		t.Errorf("loi = %v, mong doi ErrParticipantKicked (nguoi da bi kick van doc duoc chi tiet phien)", err)
	}
}

func TestGetParticipants_NguoiDaBiKick_Bi403Kicked(t *testing.T) {
	sessionID := uuid.New()
	classID := uuid.New()
	kickedUserID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: uuid.New(), ClassID: classID}
	repo := &fakeLivestreamRepoJoin{session: session}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isStudent: true}
	participantRepo := &fakeParticipantRepoJoin{
		existing: &model.Participant{
			BaseModel: model.BaseModel{ID: uuid.New()},
			SessionID: sessionID,
			UserID:    kickedUserID,
			IsKicked:  true,
		},
	}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, repo, participantRepo, nil)

	_, _, err := svc.GetParticipants(context.Background(), kickedUserID, false, sessionID, 1, 20)
	if err != ErrParticipantKicked {
		t.Errorf("loi = %v, mong doi ErrParticipantKicked (nguoi da bi kick van doc duoc danh sach nguoi tham gia)", err)
	}
}

// TestGetByID_ThanhVienBinhThuong_ChoPhep: hoi quy — them kiem IsKicked khong duoc chan oan
// nguoi CHUA TUNG join phien (participant == nil trong DB).
func TestGetByID_ThanhVienBinhThuong_ChoPhep(t *testing.T) {
	sessionID := uuid.New()
	classID := uuid.New()
	userID := uuid.New()
	session := &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}, HostID: uuid.New(), ClassID: classID}
	repo := &fakeLivestreamRepoJoin{session: session}
	classRepo := &fakeClassRepoJoin{class: &model.Class{BaseModel: model.BaseModel{ID: classID}}, isStudent: true}
	svc := newLivestreamServiceForJoin(classRepo, &fakeCourseRepoJoin{}, repo, &fakeParticipantRepoJoin{}, nil)

	if _, err := svc.GetByID(context.Background(), userID, false, sessionID); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
}
