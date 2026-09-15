package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeParentStudentRepoForStudy / fakeUserRepoForStudy — fake toi thieu cho 2 interface ma
// GetChildOverview dung toi. Nhung interface (xem comment cua fakeEnrollmentRepoWatched) de viec
// them method moi vao interface khong lam vo file test nay.
type fakeParentStudentRepoForStudy struct {
	repository.ParentStudentRepositoryInterface
	relation *model.ParentStudentRelation
	err      error
}

func (f *fakeParentStudentRepoForStudy) FindByParentAndStudent(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentStudentRelation, error) {
	return f.relation, f.err
}

type fakeUserRepoForStudy struct {
	repository.UserRepositoryInterface
	user *model.User
	err  error
}

func (f *fakeUserRepoForStudy) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return f.user, f.err
}

// activeRelation tra ve mot lien ket da kich hoat — dieu kien toi thieu de GetChildOverview di
// tiep thay vi dung o buoc verify.
func activeRelation(parentID, childID uuid.UUID) *model.ParentStudentRelation {
	return &model.ParentStudentRelation{
		ParentUserID:  parentID,
		StudentUserID: childID,
		Status:        model.ParentStudentStatusActive,
	}
}

func newParentDashboardServiceForStudy(relation *model.ParentStudentRelation, enrollRepo *fakeEnrollmentRepoWatched) *ParentDashboardService {
	return NewParentDashboardService(
		&fakeParentStudentRepoForStudy{relation: relation},
		&fakeUserRepoForStudy{user: &model.User{BaseModel: model.BaseModel{ID: relation.StudentUserID}}},
		enrollRepo,
		nil, // gradeRepo
		nil, // scheduleRepo
		nil, // submissionRepo
		nil, // userStatsRepo — GetChildOverview chiu duoc nil (co guard)
	)
}

// TestGetChildOverview_TotalStudyMinutes_RealValue — loi ma fix nay sua: truong nay truoc day
// hard-code 0 nen MOI phu huynh deu thay con hoc 0 phut, bat ke hoc that bao nhieu. Test pin
// gia tri THAT de mot lan hard-code lai (hoac doi don vi) se do ngay.
func TestGetChildOverview_TotalStudyMinutes_RealValue(t *testing.T) {
	parentID, childID := uuid.New(), uuid.New()
	e1, e2 := uuid.New(), uuid.New()

	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{{BaseModel: model.BaseModel{ID: e1}}, {BaseModel: model.BaseModel{ID: e2}}},
		total:       2,
		// 300 + 240 = 540 giay = 9 phut.
		watched: map[uuid.UUID]int{e1: 300, e2: 240},
	}

	svc := newParentDashboardServiceForStudy(activeRelation(parentID, childID), repo)

	got, err := svc.GetChildOverview(context.Background(), parentID, childID)
	if err != nil {
		t.Fatalf("GetChildOverview loi: %v", err)
	}
	if got.TotalStudyMinutes != 9 {
		t.Errorf("TotalStudyMinutes = %d, mong doi 9 (540 giay / 60)", got.TotalStudyMinutes)
	}
}

// TestGetChildOverview_TotalStudyMinutes_TruncatesPartialMinute — 119 giay la 1 phut chu khong
// phai 2: phep chia phai lay phan nguyen. Neu ai doi sang lam tron len, test nay do.
func TestGetChildOverview_TotalStudyMinutes_TruncatesPartialMinute(t *testing.T) {
	parentID, childID := uuid.New(), uuid.New()
	e1 := uuid.New()

	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{{BaseModel: model.BaseModel{ID: e1}}},
		total:       1,
		watched:     map[uuid.UUID]int{e1: 119},
	}

	svc := newParentDashboardServiceForStudy(activeRelation(parentID, childID), repo)

	got, err := svc.GetChildOverview(context.Background(), parentID, childID)
	if err != nil {
		t.Fatalf("GetChildOverview loi: %v", err)
	}
	if got.TotalStudyMinutes != 1 {
		t.Errorf("TotalStudyMinutes = %d, mong doi 1 (119 giay, lay phan nguyen)", got.TotalStudyMinutes)
	}
}

// TestGetChildOverview_TotalStudyMinutes_SumsAllEnrollments — diem de sai nhat khi tu viet SUM:
// chi lay enrollment dau tien, hoac tra trung binh thay vi tong. Test pin ca hai: nhieu enrollment
// phai CONG DON, va phai hoi repo dung MOT lan voi DU cac id.
func TestGetChildOverview_TotalStudyMinutes_SumsAllEnrollments(t *testing.T) {
	parentID, childID := uuid.New(), uuid.New()
	e1, e2, e3 := uuid.New(), uuid.New(), uuid.New()

	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{
			{BaseModel: model.BaseModel{ID: e1}},
			{BaseModel: model.BaseModel{ID: e2}},
			{BaseModel: model.BaseModel{ID: e3}},
		},
		total:   3,
		watched: map[uuid.UUID]int{e1: 600, e2: 600, e3: 600}, // 1800 giay = 30 phut
	}

	svc := newParentDashboardServiceForStudy(activeRelation(parentID, childID), repo)

	got, err := svc.GetChildOverview(context.Background(), parentID, childID)
	if err != nil {
		t.Fatalf("GetChildOverview loi: %v", err)
	}
	if got.TotalStudyMinutes != 30 {
		t.Errorf("TotalStudyMinutes = %d, mong doi 30 (1800 giay / 60)", got.TotalStudyMinutes)
	}

	// Mot loi goi duy nhat, mang du 3 id (khong phai N+1).
	if len(repo.watchedCalls) != 1 {
		t.Fatalf("SumWatchedSecondsByEnrollmentIDs duoc goi %d lan, mong doi 1", len(repo.watchedCalls))
	}
	if len(repo.watchedCalls[0]) != 3 {
		t.Errorf("loi goi mang %d id, mong doi 3 — thieu enrollment thi thoi gian hoc bi hut",
			len(repo.watchedCalls[0]))
	}
}

// TestGetChildOverview_TotalStudyMinutes_NoEnrollment — child chua ghi danh gi: 0 phut la DUNG,
// va khong duoc goi repo (id rong thi cau GROUP BY la vo nghia).
func TestGetChildOverview_TotalStudyMinutes_NoEnrollment(t *testing.T) {
	parentID, childID := uuid.New(), uuid.New()

	repo := &fakeEnrollmentRepoWatched{enrollments: nil, total: 0}

	svc := newParentDashboardServiceForStudy(activeRelation(parentID, childID), repo)

	got, err := svc.GetChildOverview(context.Background(), parentID, childID)
	if err != nil {
		t.Fatalf("GetChildOverview loi: %v", err)
	}
	if got.TotalStudyMinutes != 0 {
		t.Errorf("TotalStudyMinutes = %d, mong doi 0", got.TotalStudyMinutes)
	}
	if len(repo.watchedCalls) != 0 {
		t.Errorf("goi repo %d lan khi khong co enrollment nao, mong doi 0", len(repo.watchedCalls))
	}
}

// TestGetChildOverview_TotalStudyMinutes_RepoErrorSurfaces — query hong PHAI noi len, khong duoc
// am tham tra 0. Tra 0 khi loi se lam phu huynh tuong con minh khong hoc — dung lop loi ma fix
// nay dang sua, chi kho phat hien hon.
func TestGetChildOverview_TotalStudyMinutes_RepoErrorSurfaces(t *testing.T) {
	parentID, childID := uuid.New(), uuid.New()

	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{{BaseModel: model.BaseModel{ID: uuid.New()}}},
		total:       1,
		watchedErr:  errors.New("db down"),
	}

	svc := newParentDashboardServiceForStudy(activeRelation(parentID, childID), repo)

	got, err := svc.GetChildOverview(context.Background(), parentID, childID)
	if err == nil {
		t.Fatalf("mong doi loi khi repo hong, nhung nhan duoc ket qua: %+v", got)
	}
}

// TestGetChildOverview_RejectsInactiveRelation — khang dinh fix nay KHONG noi long quyen: quan he
// parent-child chua kich hoat van phai bi chan TRUOC khi cham toi du lieu hoc tap.
func TestGetChildOverview_RejectsInactiveRelation(t *testing.T) {
	parentID, childID := uuid.New(), uuid.New()
	e1 := uuid.New()

	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{{BaseModel: model.BaseModel{ID: e1}}},
		total:       1,
		watched:     map[uuid.UUID]int{e1: 600},
	}

	relation := activeRelation(parentID, childID)
	relation.Status = "pending"
	svc := newParentDashboardServiceForStudy(relation, repo)

	_, err := svc.GetChildOverview(context.Background(), parentID, childID)
	if err == nil {
		t.Fatal("mong doi loi khi lien ket chua kich hoat, nhung van tra duoc du lieu")
	}
	if len(repo.watchedCalls) != 0 {
		t.Errorf("cham vao du lieu hoc tap %d lan truoc khi kiem tra quyen", len(repo.watchedCalls))
	}
}
