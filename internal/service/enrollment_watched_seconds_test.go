package service

import (
	"context"
	"errors"
	"testing"
	"time"

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

	// Duong UPDATE nguyen tu (review 260912, finding #2): ghi lai DUNG nhung gi service truyen
	// xuong, de khang dinh duoc HOP DONG giua service va repository.
	updateCalls   int
	updateUpdates map[string]interface{}
	updateWatched *int
	updateErr     error
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

// UpdateLessonProgressFields: mo phong dung ngu nghia cua cau UPDATE that (chi set cac cot co
// trong map; video_watched_seconds chi tang) de service co mot ban ghi "sau khi ghi" de doc lai.
//
// LUU Y VE GIA TRI CUA TEST NAY: phan mo phong nay KHONG chung minh duoc tinh nguyen tu — no chi
// la mot struct trong bo nho. Tinh nguyen tu cua phep max duoc pin rieng o tang repository, bang
// test DryRun khang dinh cau SQL co GREATEST(video_watched_seconds, ?)
// (TestUpdateLessonProgressFields_DungGreatestTrongSQL). Cac assertion o day khang dinh HOP DONG
// service -> repository: service KHONG tu so sanh o Go, KHONG ghi de cot status.
func (f *fakeEnrollmentRepoWatched) UpdateLessonProgressFields(ctx context.Context, userID, lessonID uuid.UUID, updates map[string]interface{}, watchedSeconds *int) (*model.LessonProgress, error) {
	f.updateCalls++
	f.updateUpdates = updates
	f.updateWatched = watchedSeconds
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.lessonProgress == nil {
		return nil, nil
	}
	for k, v := range updates {
		switch k {
		case "status":
			if s, ok := v.(string); ok {
				f.lessonProgress.Status = s
			}
		case "progress_percentage":
			if d, ok := v.(decimal.Decimal); ok {
				f.lessonProgress.ProgressPercent = d
			}
		case "completed_at":
			if t, ok := v.(time.Time); ok {
				f.lessonProgress.CompletedAt = &t
			}
		}
	}
	if watchedSeconds != nil && *watchedSeconds > f.lessonProgress.VideoWatchedSecs {
		f.lessonProgress.VideoWatchedSecs = *watchedSeconds
	}
	return f.lessonProgress, nil
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
//
// Review 260912 finding #2: phep max KHONG con duoc quyet dinh o Go (doc row -> so sanh -> ghi de
// toan bo struct). No duoc day xuong SQL duoi dang GREATEST(video_watched_seconds, ?) va ghi bang
// map chi chua dung cac cot thay doi. Test nay khang dinh hop dong do:
//   - gia tri service truyen xuong la gia tri CLIENT GUI, khong phai gia tri da max hoa o Go
//     (neu ai do quay lai so sanh o Go, `*repo.updateWatched` se la 432 thay vi 10 => do ngay);
//   - DTO tra ve lay tu ban ghi doc lai SAU khi ghi, khong phai gia tri trong bo nho da stale;
//   - cot `status` khong nam trong map UPDATE khi request khong gui status.
func TestUpdateLessonProgress_WatchedSecondsChiTangKhongGiam(t *testing.T) {
	cases := []struct {
		ten             string
		hienTai         int
		guiLen          int
		mongDoi         int
		mongTruyenXuong int
	}{
		{"gui nho hon thi giu nguyen", 432, 10, 432, 10},
		{"gui lon hon thi tang", 432, 500, 500, 500},
		{"gui bang thi giu nguyen", 432, 432, 432, 432},
		{"gui 0 khi da co so lon", 500, 0, 500, 0},
		{"tu 0 len lan dau", 0, 87, 87, 87},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			enrollmentID := uuid.New()
			enrollment := newEnrollmentWithID(enrollmentID)
			existing := &model.LessonProgress{
				VideoWatchedSecs: tc.hienTai,
				EnrollmentID:     enrollmentID,
				Status:           "in_progress",
			}
			repo := &fakeEnrollmentRepoWatched{
				lessonProgress: existing,
				enrollment:     &enrollment,
				courseID:       uuid.New(),
			}
			svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

			secs := tc.guiLen
			res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
				dto.UpdateLessonProgressDTO{VideoWatchedSecs: &secs})
			if err != nil {
				t.Fatalf("khong mong doi loi: %v", err)
			}

			// Duong UPDATE nguyen tu phai duoc dung, KHONG phai UpsertLessonProgress/Save.
			if repo.updateCalls != 1 {
				t.Fatalf("goi UpdateLessonProgressFields %d lan, mong doi dung 1", repo.updateCalls)
			}
			if repo.upserted != nil {
				t.Error("UpsertLessonProgress (Save) van duoc goi tren duong cap nhat — quay lai ghi de toan bo struct")
			}
			if repo.updateWatched == nil {
				t.Fatal("khong truyen watchedSeconds xuong repository")
			}
			if *repo.updateWatched != tc.mongTruyenXuong {
				t.Errorf("gia tri truyen xuong = %d, mong doi %d (gia tri CLIENT gui, chua max hoa o Go)",
					*repo.updateWatched, tc.mongTruyenXuong)
			}

			// status/completed_at khong duoc nam trong map khi request khong gui status — day chinh la
			// cai Save() cu ghi de lam mat trang thai `completed` cua request song song.
			if _, ok := repo.updateUpdates["status"]; ok {
				t.Error("map UPDATE chua cot status du request khong gui status")
			}
			if _, ok := repo.updateUpdates["completed_at"]; ok {
				t.Error("map UPDATE chua cot completed_at du request khong gui status")
			}
			if _, ok := repo.updateUpdates["video_watched_seconds"]; ok {
				t.Error("video_watched_seconds nam trong map — no phai di qua GREATEST() trong SQL, khong set tho")
			}
			if existing.Status != "in_progress" {
				t.Errorf("status bi doi thanh %q du request khong gui status", existing.Status)
			}

			// DTO phai phan anh ban ghi SAU khi ghi (doc lai tu repo), khong phai gia tri stale trong
			// bo nho: gui 10 len mot row 432 thi phai tra ve 432.
			if res.VideoWatchedSecs != tc.mongDoi {
				t.Errorf("VideoWatchedSecs tra ve = %d, mong doi %d (hien tai %d, gui len %d)",
					res.VideoWatchedSecs, tc.mongDoi, tc.hienTai, tc.guiLen)
			}
			if existing.VideoWatchedSecs != tc.mongDoi {
				t.Errorf("VideoWatchedSecs trong DB (gia lap) = %d, mong doi %d (hien tai %d, gui len %d)",
					existing.VideoWatchedSecs, tc.mongDoi, tc.hienTai, tc.guiLen)
			}
		})
	}
}

// status duoc gui len thi PHAI duoc ghi, nhung video_watched_seconds van khong nam trong map.
func TestUpdateLessonProgress_GuiStatusThiCapNhatStatus(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	existing := &model.LessonProgress{
		VideoWatchedSecs: 432,
		EnrollmentID:     enrollmentID,
		Status:           "in_progress",
	}
	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: existing,
		enrollment:     &enrollment,
		courseID:       uuid.New(),
	}
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

	status := "completed"
	secs := 10
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{Status: &status, VideoWatchedSecs: &secs})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if got, _ := repo.updateUpdates["status"].(string); got != "completed" {
		t.Errorf("cot status trong map UPDATE = %q, mong doi \"completed\"", got)
	}
	if _, ok := repo.updateUpdates["completed_at"]; !ok {
		t.Error("map UPDATE thieu completed_at khi status chuyen sang completed")
	}
	if res.Status != "completed" {
		t.Errorf("status tra ve = %q, mong doi \"completed\"", res.Status)
	}
	// status di qua map, khong duoc set tho trong cung cau lenh voi watched seconds.
	if _, ok := repo.updateUpdates["video_watched_seconds"]; ok {
		t.Error("video_watched_seconds nam trong map — phai di qua GREATEST() trong SQL")
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

// Chan TREN (review 260912, finding #1): min=0 chan so am nhung khong chan gia tri rac. Cot nay
// chi TANG, nen mot lan gui 2000000000 se khong bao gio bi ghi de boi cac gia tri nho hon nua —
// "thoi gian hoc" tren web hong VINH VIEN, chi sua duoc bang UPDATE tay trong DB.
func TestUpdateLessonProgressDTO_ChanGiaTriVuotNguong(t *testing.T) {
	// 86400 = 24 gio: bien tren, van hop le.
	bien := 86400
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{VideoWatchedSecs: &bien}); len(errs) != 0 {
		t.Errorf("86400 (dung bang nguong) phai hop le nhung bi tu choi: %v", errs)
	}

	vuaQua := 86401
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{VideoWatchedSecs: &vuaQua}); len(errs) == 0 {
		t.Error("86401 (vuot nguong) phai bi tu choi, nhung validate lai cho qua")
	}

	// Gia tri rac da tai hien duoc tren server that truoc khi sua: luu ~63 nam vao DB.
	rac := 2000000000
	if errs := utils.ValidateStruct(dto.UpdateLessonProgressDTO{VideoWatchedSecs: &rac}); len(errs) == 0 {
		t.Error("2000000000 phai bi tu choi, nhung validate lai cho qua")
	}
}

// Ban ghi tien do CHUA ton tai thi phai tao moi va luu DUNG gia tri client gui (khong co ban ghi
// cu de so sanh, khong co GREATEST() nao). Nhanh nay duoc tach ra khoi duong UPDATE o review
// 260912 finding #2 — test khoa lai de viec tach nhanh khong lam mat du lieu lan dau.
func TestUpdateLessonProgress_BanGhiMoiThiTaoVoiGiaTriClientGui(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: nil, // chua co ban ghi nao
		enrollment:     &enrollment,
		courseID:       uuid.New(),
	}
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{})

	status := "in_progress"
	secs := 87
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{Status: &status, VideoWatchedSecs: &secs})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if repo.upserted == nil {
		t.Fatal("ban ghi moi khong duoc tao")
	}
	if repo.upserted.VideoWatchedSecs != 87 {
		t.Errorf("VideoWatchedSecs = %d, mong doi 87", repo.upserted.VideoWatchedSecs)
	}
	if repo.upserted.Status != "in_progress" {
		t.Errorf("status = %q, mong doi \"in_progress\"", repo.upserted.Status)
	}
	if repo.upserted.EnrollmentID != enrollmentID {
		t.Errorf("enrollment_id = %s, mong doi %s", repo.upserted.EnrollmentID, enrollmentID)
	}
	if repo.updateCalls != 0 {
		t.Errorf("ban ghi moi khong duoc di qua duong UPDATE, nhung UpdateLessonProgressFields duoc goi %d lan", repo.updateCalls)
	}
	if res.VideoWatchedSecs != 87 {
		t.Errorf("DTO tra ve VideoWatchedSecs = %d, mong doi 87", res.VideoWatchedSecs)
	}
}
