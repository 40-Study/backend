package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ============================================================================
// Stub repository.
//
// Nhung ScheduleRepositoryInterface (nil) de tu dong thoa man interface 25
// method, roi chi override dung nhung method test can. Goi method chua override
// se panic — dieu do la co y: test nao lo cham vao se bao ngay.
// ============================================================================

type stubScheduleRepo struct {
	repository.ScheduleRepositoryInterface

	// state: attendance da ton tai, key = sessionID|studentID
	existing map[string]*model.SessionAttendance

	createdCount int
	updatedCount int
}

func newStubRepo() *stubScheduleRepo {
	return &stubScheduleRepo{existing: map[string]*model.SessionAttendance{}}
}

func key(sessionID, studentID uuid.UUID) string {
	return sessionID.String() + "|" + studentID.String()
}

func (r *stubScheduleRepo) GetAttendanceBySessionAndStudent(ctx context.Context, sessionID, studentID uuid.UUID) (*model.SessionAttendance, error) {
	if att, ok := r.existing[key(sessionID, studentID)]; ok {
		return att, nil
	}
	return nil, nil
}

func (r *stubScheduleRepo) CreateAttendance(ctx context.Context, att *model.SessionAttendance) error {
	att.ID = uuid.New()
	r.existing[key(att.SessionID, att.StudentID)] = att
	r.createdCount++
	return nil
}

func (r *stubScheduleRepo) UpdateAttendance(ctx context.Context, att *model.SessionAttendance) error {
	r.existing[key(att.SessionID, att.StudentID)] = att
	r.updatedCount++
	return nil
}

func (r *stubScheduleRepo) GetAttendanceByID(ctx context.Context, id uuid.UUID) (*model.SessionAttendance, error) {
	for _, att := range r.existing {
		if att.ID == id {
			return att, nil
		}
	}
	return nil, nil
}

func newTestService(repo repository.ScheduleRepositoryInterface) *ScheduleService {
	// redis/queue de nil: cac ham attendance duoc test khong dung toi
	return &ScheduleService{repo: repo}
}

// ============================================================================
// MarkAttendance
// ============================================================================

func TestMarkAttendance_CreatesNewRecord(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)
	sessionID, studentID, teacherID := uuid.New(), uuid.New(), uuid.New()

	got, err := svc.MarkAttendance(context.Background(), sessionID, dto.MarkAttendanceDTO{
		StudentID: studentID.String(),
		Status:    "present",
	}, teacherID)

	if err != nil {
		t.Fatalf("loi khong mong doi: %v", err)
	}
	if got.Status != "present" {
		t.Errorf("status = %q, muon \"present\"", got.Status)
	}
	if repo.createdCount != 1 {
		t.Errorf("createdCount = %d, muon 1", repo.createdCount)
	}
}

func TestMarkAttendance_RejectsDuplicate(t *testing.T) {
	repo := newStubRepo()
	sessionID, studentID, teacherID := uuid.New(), uuid.New(), uuid.New()
	repo.existing[key(sessionID, studentID)] = &model.SessionAttendance{
		ID: uuid.New(), SessionID: sessionID, StudentID: studentID, Status: model.AttendanceAbsent,
	}

	svc := newTestService(repo)
	_, err := svc.MarkAttendance(context.Background(), sessionID, dto.MarkAttendanceDTO{
		StudentID: studentID.String(),
		Status:    "present",
	}, teacherID)

	if err == nil {
		t.Fatal("muon loi khi hoc sinh da co ban ghi, nhung err = nil")
	}
	if repo.createdCount != 0 {
		t.Errorf("khong duoc tao ban ghi moi, createdCount = %d", repo.createdCount)
	}
}

func TestMarkAttendance_InvalidStudentID(t *testing.T) {
	svc := newTestService(newStubRepo())

	_, err := svc.MarkAttendance(context.Background(), uuid.New(), dto.MarkAttendanceDTO{
		StudentID: "khong-phai-uuid",
		Status:    "present",
	}, uuid.New())

	if err == nil {
		t.Fatal("muon loi khi student_id khong phai UUID")
	}
}

// ============================================================================
// BulkMarkAttendance — HANH VI NGUY HIEM, ghi lai bang test
// ============================================================================

// Bulk goi MarkAttendance cho tung hoc sinh; hoc sinh da co ban ghi se bi
// MarkAttendance tra loi, va bulk NUOT loi do bang `continue` (chi log phia
// server) roi van tra ve nil error.
//
// => Giao vien diem danh lan 2 de SUA se khong luu duoc gi, ma client van
// nhan 200 OK. Frontend da phai ne bang cach tach create/update
// (web/src/components/attendance/use-attendance-draft.ts).
//
// Test nay CO Y khoa lai hanh vi hien tai de mo ta dung su that. Khi backend
// duoc sua de bao cao danh sach bi bo qua, test nay se do va can cap nhat —
// do la tin hieu dung, khong phai loi test.
func TestBulkMarkAttendance_SilentlySkipsExistingRecords(t *testing.T) {
	repo := newStubRepo()
	sessionID, teacherID := uuid.New(), uuid.New()
	studentA, studentB := uuid.New(), uuid.New()

	// A da duoc diem danh tu truoc, B thi chua
	repo.existing[key(sessionID, studentA)] = &model.SessionAttendance{
		ID: uuid.New(), SessionID: sessionID, StudentID: studentA, Status: model.AttendanceAbsent,
	}

	svc := newTestService(repo)
	results, err := svc.BulkMarkAttendance(context.Background(), sessionID, dto.BulkMarkAttendanceDTO{
		Attendances: []dto.MarkAttendanceDTO{
			{StudentID: studentA.String(), Status: "present"}, // se bi bo qua
			{StudentID: studentB.String(), Status: "present"}, // se duoc tao
		},
	}, teacherID)

	if err != nil {
		t.Fatalf("bulk tra loi: %v — dang le nuot loi va tra nil", err)
	}

	// Chi 1 ban ghi duoc tao (cua B); A bi bo qua trong im lang
	if len(results) != 1 {
		t.Errorf("len(results) = %d, muon 1 (chi B duoc tao)", len(results))
	}
	if repo.createdCount != 1 {
		t.Errorf("createdCount = %d, muon 1", repo.createdCount)
	}

	// Trang thai cua A KHONG doi du client gui "present"
	if got := repo.existing[key(sessionID, studentA)].Status; got != model.AttendanceAbsent {
		t.Errorf("trang thai cua A = %q, muon van la \"absent\" (bi bo qua)", got)
	}

	// Day la mau chot: caller khong co cach nao biet A bi bo qua.
	// err == nil VA results chi thieu phan tu — khong co danh sach loi tra ve.
	if err == nil && len(results) < 2 {
		t.Log("XAC NHAN: bulk bo qua ban ghi da ton tai ma khong bao loi cho client")
	}
}

func TestBulkMarkAttendance_AllNewRecords(t *testing.T) {
	repo := newStubRepo()
	sessionID, teacherID := uuid.New(), uuid.New()

	svc := newTestService(repo)
	results, err := svc.BulkMarkAttendance(context.Background(), sessionID, dto.BulkMarkAttendanceDTO{
		Attendances: []dto.MarkAttendanceDTO{
			{StudentID: uuid.New().String(), Status: "present"},
			{StudentID: uuid.New().String(), Status: "late"},
			{StudentID: uuid.New().String(), Status: "absent"},
		},
	}, teacherID)

	if err != nil {
		t.Fatalf("loi khong mong doi: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, muon 3", len(results))
	}
}

// ============================================================================
// StudentCheckIn / CheckOut
// ============================================================================

func TestStudentCheckIn_CreatesRecordWithPresentStatus(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)
	sessionID, studentID := uuid.New(), uuid.New()

	got, err := svc.StudentCheckIn(context.Background(), sessionID, studentID)
	if err != nil {
		t.Fatalf("loi khong mong doi: %v", err)
	}

	// Check-in tu dong danh dau co mat
	if got.Status != string(model.AttendancePresent) {
		t.Errorf("status = %q, muon \"present\"", got.Status)
	}
	if got.CheckInTime == nil {
		t.Error("check_in_time phai duoc set")
	}
}

func TestStudentCheckIn_UpsertsWhenRecordExists(t *testing.T) {
	repo := newStubRepo()
	sessionID, studentID := uuid.New(), uuid.New()
	repo.existing[key(sessionID, studentID)] = &model.SessionAttendance{
		ID: uuid.New(), SessionID: sessionID, StudentID: studentID, Status: model.AttendanceAbsent,
	}

	svc := newTestService(repo)
	got, err := svc.StudentCheckIn(context.Background(), sessionID, studentID)

	if err != nil {
		t.Fatalf("loi khong mong doi: %v", err)
	}
	if repo.createdCount != 0 {
		t.Errorf("khong duoc tao ban ghi moi khi da ton tai, createdCount = %d", repo.createdCount)
	}
	if got.CheckInTime == nil {
		t.Error("check_in_time phai duoc cap nhat")
	}
}

func TestStudentCheckOut_FailsWithoutCheckIn(t *testing.T) {
	svc := newTestService(newStubRepo())

	_, err := svc.StudentCheckOut(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("muon loi khi chua tung check-in")
	}
}
