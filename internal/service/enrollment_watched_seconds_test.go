package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// fakeEnrollmentRepoWatched la fake toi thieu cho EnrollmentRepositoryInterface, chi override
// dung cac method ma GetMyEnrollments va UpdateLessonProgress dung toi. Nhung interface de
// viec them method moi vao interface khong lam vo file test nay.
type fakeEnrollmentRepoWatched struct {
	repository.EnrollmentRepositoryInterface

	enrollments []model.Enrollment
	total       int64

	watched    map[uuid.UUID]int
	watchedErr error
	// watchedCalls ghi lai danh sach id ma service truyen xuong - dung de khang dinh service
	// KHONG goi N lan (N+1) ma gop thanh mot loi goi duy nhat.
	watchedCalls [][]uuid.UUID

	lessonProgress *model.LessonProgress
	upserted       *model.LessonProgress
	courseID       uuid.UUID
	enrollment     *model.Enrollment
}

func (f *fakeEnrollmentRepoWatched) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error) {
	return f.enrollments, f.total, nil
}

func (f *fakeEnrollmentRepoWatched) SumWatchedSecondsByEnrollmentIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	f.watchedCalls = append(f.watchedCalls, ids)
	if f.watchedErr != nil {
		return nil, f.watchedErr
	}
	return f.watched, nil
}

func (f *fakeEnrollmentRepoWatched) GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error) {
	return f.courseID, nil
}

func (f *fakeEnrollmentRepoWatched) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	return f.enrollment, nil
}

func (f *fakeEnrollmentRepoWatched) GetLessonProgress(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error) {
	return f.lessonProgress, nil
}

func (f *fakeEnrollmentRepoWatched) UpsertLessonProgress(ctx context.Context, p *model.LessonProgress) error {
	f.upserted = p
	return nil
}

func (f *fakeEnrollmentRepoWatched) CountCompletedMandatory(ctx context.Context, enrollmentID uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeEnrollmentRepoWatched) CountTotalMandatory(ctx context.Context, courseID uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeEnrollmentRepoWatched) UpdateEnrollmentProgress(ctx context.Context, enrollmentID uuid.UUID, progress decimal.Decimal) error {
	return nil
}

type fakeLessonRepoWatched struct {
	repository.LessonRepositoryInterface
}

func (f *fakeLessonRepoWatched) GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error) {
	l := &model.Lesson{}
	l.ID = id
	return l, nil
}

func newEnrollmentWithID(id uuid.UUID) model.Enrollment {
	e := model.Enrollment{}
	e.ID = id
	return e
}

// GetMyEnrollments phai gan tong thoi gian xem THAT cho tung ghi danh. Truoc day web hien thi
// chuoi cung "1h 45m" vi phan hoi khong co truong nao mang so nay.
func TestGetMyEnrollments_GanWatchedSecondsChoTungGhiDanh(t *testing.T) {
	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{
			newEnrollmentWithID(id1),
			newEnrollmentWithID(id2),
			newEnrollmentWithID(id3),
		},
		total: 3,
		// id3 CO TINH vang mat: ghi danh chua xem bai nao thi khong co dong lesson_progress.
		watched: map[uuid.UUID]int{id1: 5640, id2: 720},
	}
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

	res, err := svc.GetMyEnrollments(context.Background(), uuid.New(), 1, 20)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	got := map[uuid.UUID]int{}
	for _, e := range res.Enrollments {
		got[e.ID] = e.WatchedSeconds
	}
	for id, want := range map[uuid.UUID]int{id1: 5640, id2: 720, id3: 0} {
		if got[id] != want {
			t.Errorf("ghi danh %s: WatchedSeconds = %d, mong doi %d", id, got[id], want)
		}
	}

	// Chan N+1: du co 3 ghi danh, service chi duoc hoi repository DUNG MOT lan.
	if len(repo.watchedCalls) != 1 {
		t.Fatalf("goi SumWatchedSeconds %d lan, mong doi dung 1 (N+1 regression)", len(repo.watchedCalls))
	}
	if len(repo.watchedCalls[0]) != 3 {
		t.Errorf("loi goi mang %d id, mong doi 3", len(repo.watchedCalls[0]))
	}
}

// Loi cong don KHONG duoc nuot thanh 0 - nguoi hoc se thay "0m" va tuong minh chua hoc gi.
func TestGetMyEnrollments_LoiTongHopPhaiDuocTraVe(t *testing.T) {
	boom := errors.New("db down")
	repo := &fakeEnrollmentRepoWatched{
		enrollments: []model.Enrollment{newEnrollmentWithID(uuid.New())},
		total:       1,
		watchedErr:  boom,
	}
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

	res, err := svc.GetMyEnrollments(context.Background(), uuid.New(), 1, 20)
	if err == nil {
		t.Fatalf("mong doi loi duoc tra ve, nhung nhan res=%v err=nil (dang nuot loi)", res)
	}
	if !errors.Is(err, boom) {
		t.Errorf("loi tra ve = %v, mong doi boc %v", err, boom)
	}
}

// Danh sach rong khong duoc gay truy van voi menh de IN rong.
func TestGetMyEnrollments_KhongCoGhiDanhThiTraVeRong(t *testing.T) {
	repo := &fakeEnrollmentRepoWatched{enrollments: nil, total: 0, watched: map[uuid.UUID]int{}}
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

	res, err := svc.GetMyEnrollments(context.Background(), uuid.New(), 1, 20)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if len(res.Enrollments) != 0 {
		t.Errorf("mong doi 0 ghi danh, nhan %d", len(res.Enrollments))
	}
}

// Trinh phat gui VI TRI phat hien tai. Xem lai bai tu dau gui so nho - neu ghi de thi chi so
// "thoi gian hoc" tren web tu dung giam. Gia tri chi duoc phep tang.
func TestUpdateLessonProgress_WatchedSecondsChiTangKhongGiam(t *testing.T) {
	cases := []struct {
		ten     string
		hienTai int
		guiLen  int
		mongDoi int
	}{
		{"gui nho hon thi giu nguyen", 432, 10, 432},
		{"gui lon hon thi tang", 432, 500, 500},
		{"gui bang thi giu nguyen", 432, 432, 432},
		{"gui 0 khi da co so lon", 500, 0, 500},
		{"tu 0 len lan dau", 0, 87, 87},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			enrollmentID := uuid.New()
			enrollment := newEnrollmentWithID(enrollmentID)
			existing := &model.LessonProgress{VideoWatchedSecs: tc.hienTai, EnrollmentID: enrollmentID}
			repo := &fakeEnrollmentRepoWatched{
				lessonProgress: existing,
				enrollment:     &enrollment,
				courseID:       uuid.New(),
			}
			svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

			secs := tc.guiLen
			_, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
				dto.UpdateLessonProgressDTO{VideoWatchedSecs: &secs})
			if err != nil {
				t.Fatalf("khong mong doi loi: %v", err)
			}
			if repo.upserted == nil {
				t.Fatal("khong co ban ghi nao duoc luu")
			}
			if repo.upserted.VideoWatchedSecs != tc.mongDoi {
				t.Errorf("VideoWatchedSecs = %d, mong doi %d (hien tai %d, gui len %d)",
					repo.upserted.VideoWatchedSecs, tc.mongDoi, tc.hienTai, tc.guiLen)
			}
		})
	}
}

// Gia tri am phai bi chan ngay o tang validate. Da tai hien duoc tren server that truoc khi sua:
// gui -999999 duoc luu thang vao DB va lam tong thoi gian hoc am.
func TestUpdateLessonProgressDTO_ChanGiaTriAm(t *testing.T) {
	am := -999999
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{VideoWatchedSecs: &am}); len(errs) == 0 {
		t.Error("gia tri am phai bi tu choi, nhung validate lai cho qua")
	}

	khong := 0
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{VideoWatchedSecs: &khong}); len(errs) != 0 {
		t.Errorf("0 la hop le nhung bi tu choi: %v", errs)
	}

	duong := 500
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{VideoWatchedSecs: &duong}); len(errs) != 0 {
		t.Errorf("500 la hop le nhung bi tu choi: %v", errs)
	}

	// Khong gui truong nay (nil) van phai hop le - client chi cap nhat status.
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{}); len(errs) != 0 {
		t.Errorf("bo trong phai hop le nhung bi tu choi: %v", errs)
	}
}
