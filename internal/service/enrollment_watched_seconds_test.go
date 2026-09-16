package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
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
	// gotUserID (LOW-8, review 260915): ghi lai userID ma GetByUserID nhan duoc — truoc day fake
	// nay bo qua tham so nay hoan toan, nen mot nham lan doi childID thanh parentID o tang goi
	// (GetChildOverview) van xanh du phu huynh se thay nham thoi gian hoc CUA CHINH MINH.
	gotUserID uuid.UUID

	watched    map[uuid.UUID]int
	watchedErr error
	// watchedCalls ghi lai danh sach id ma service truyen xuong - dung de khang dinh service
	// KHONG goi N lan (N+1) ma gop thanh mot loi goi duy nhat.
	watchedCalls [][]uuid.UUID

	lessonProgress *model.LessonProgress
	upserted       *model.LessonProgress
	courseID       uuid.UUID
	enrollment     *model.Enrollment
	// lessonOrder: thu tu bai hoc trong khoa ma GetLessonIDsByCourseID tra ve (Phase 1 §1,
	// next_lesson_unlocked). Rong = "khong co bai ke tiep".
	lessonOrder []uuid.UUID

	// Duong re-enroll (review 260912, finding N1): ban ghi ma GetByUserAndCourseUnscoped tra ve,
	// DA kem LessonProgress dung nhu hop dong cua repository that.
	unscopedEnrollment *model.Enrollment
	unscopedCalls      int
	reactivateUpdates  map[string]interface{}

	// Duong UPDATE nguyen tu (review 260912, finding #2): ghi lai DUNG nhung gi service truyen
	// xuong, de khang dinh duoc HOP DONG giua service va repository.
	updateCalls   int
	updateUpdates map[string]interface{}
	updateWatched *int
	updateErr     error
}

func (f *fakeEnrollmentRepoWatched) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error) {
	f.gotUserID = userID
	return f.enrollments, f.total, nil
}

// GetLessonIDsByCourseID (Phase 1 §1): thu tu bai hoc trong khoa. Mac dinh rong — nghia la
// "khong co bai ke tiep" — de cac test khong lien quan toi next_lesson_unlocked khong phai khai.
func (f *fakeEnrollmentRepoWatched) GetLessonIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error) {
	return f.lessonOrder, nil
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

// GetByUserAndCourseUnscoped mo phong DUNG hop dong cua repository that: ban ghi tra ve da kem
// LessonProgress (Preload) — xem comment tai EnrollmentRepository.GetByUserAndCourseUnscoped.
func (f *fakeEnrollmentRepoWatched) GetByUserAndCourseUnscoped(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	f.unscopedCalls++
	return f.unscopedEnrollment, nil
}

func (f *fakeEnrollmentRepoWatched) RestoreAndReactivate(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	f.reactivateUpdates = updates
	return nil
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
		case "watched_pct":
			// Phase 1 §1: cot nay di qua map (khong qua GREATEST), set thang nhu UPDATE that.
			if d, ok := v.(decimal.Decimal); ok {
				f.lessonProgress.WatchedPct = d
			}
		case "last_position_seconds":
			if n, ok := v.(int); ok {
				f.lessonProgress.LastPositionSeconds = n
			}
		case "played_ranges":
			// Phase 1 §1: played_ranges duoc GHI DE (khong phai GREATEST) — gia tri moi da la
			// hop nhat cua cu + moi.
			if r, ok := v.(model.PlayedRanges); ok {
				f.lessonProgress.PlayedRanges = r
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
	// contents/legacyDuration (B-1, review vòng 2): resolveServerVideoDuration goi CA HAI method
	// nay o MOI lan UpdateLessonProgress — de trong (mac dinh) nghia la server CHUA BIET duration
	// nao, giu nguyen hanh vi cu cua cac test da co (durationSeconds roi ve client tu khai) ma
	// khong phai sua tung test.
	contents       []model.LessonContent
	legacyDuration int
}

func (f *fakeLessonRepoWatched) GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonContent, error) {
	return f.contents, nil
}

func (f *fakeLessonRepoWatched) GetLegacyVideoDurationByLessonID(ctx context.Context, lessonID uuid.UUID) (int, error) {
	return f.legacyDuration, nil
}

// fakeCourseRepoWatched: chi override hai method ma Enroll dung toi. Nhung interface de method them
// sau nay khong lam vo file test; Enroll lai can courseRepo khac nil nen phai co mot stub that su.
// GetByID tra ve khoa MIEN PHI (Price = 0) de Enroll khong dung o buoc kiem tra thanh toan.
type fakeCourseRepoWatched struct {
	repository.CourseRepositoryInterface
	course         *model.Course
	incrementCalls int
}

func (f *fakeCourseRepoWatched) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	if f.course != nil {
		return f.course, nil
	}
	c := &model.Course{}
	c.ID = id
	// MinVideoPct = 0 => service dung defaultMinVideoPct (90). Cac test cu khong quan tam toi
	// nguong nay nen khong phai khai bao gi.
	return c, nil
}

func (f *fakeCourseRepoWatched) IncrementTotalStudents(ctx context.Context, courseID uuid.UUID, delta int) error {
	f.incrementCalls++
	return nil
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

// fakeVideoUploadRepoWatched (BLOCKER-1, review vòng 3): phuc vu healDurationFromVideoUpload.
// Nhung interface de method them sau nay khong lam vo file test. `uploads` khoa theo upload id
// nhung trong URL HLS; `updated` ghi lai content ma service da ghi nguoc duration xuong.
type fakeVideoUploadRepoWatched struct {
	repository.VideoUploadRepositoryInterface
	uploads map[uuid.UUID]*model.VideoUpload
	updated []model.LessonContent
}

func (f *fakeVideoUploadRepoWatched) GetUploadByID(ctx context.Context, uploadID uuid.UUID) (*model.VideoUpload, error) {
	if f.uploads == nil {
		return nil, nil
	}
	return f.uploads[uploadID], nil
}

// fakeLessonRepoHealWatched: nhu fakeLessonRepoWatched nhung ghi lai UpdateContentDuration, de
// khang dinh duration tim duoc CO duoc ghi nguoc xuong lesson_contents hay khong.
//
// C-5 (review vòng 3): fake nay CHI override UpdateContentDuration, khong override UpdateContent.
// Neu ai do doi nguoc duong chua ve `UpdateContent` (db.Save — ghi de moi cot), loi goi se roi
// vao interface nhung (nil) o fakeLessonRepoWatched va PANIC => test do ngay. Do la pin cho C-5.
type fakeLessonRepoHealWatched struct {
	fakeLessonRepoWatched
	updatedIDs       []uuid.UUID
	updatedDurations []int
}

func (f *fakeLessonRepoHealWatched) UpdateContentDuration(ctx context.Context, id uuid.UUID, duration int) error {
	f.updatedIDs = append(f.updatedIDs, id)
	f.updatedDurations = append(f.updatedDurations, duration)
	return nil
}

// TestUpdateLessonProgress_TuChuaDurationTuVideoUpload (BLOCKER-1, review vòng 3): bai hoc co
// lesson_contents.duration = 0 (web khong gui duration khi tao content video) nhung video goc da
// xu ly xong nen video_uploads.duration co gia tri. Truoc ban va nay, mau so roi ve
// duration_seconds CLIENT tu khai => C-2 chan completed => bai KHONG BAO GIO hoan thanh duoc, va
// voi sequential=true khoa hoc ket vinh vien o bai dau.
//
// Khang dinh: duration duoc lay tu video_uploads (server-truth) nen completed VAN cap duoc, va
// gia tri do duoc ghi nguoc xuong lesson_contents de nhip heartbeat sau khong phai tra lai.
func TestUpdateLessonProgress_TuChuaDurationTuVideoUpload(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	lessonID := uuid.New()
	uploadID := uuid.New()
	// URL HLS dung dinh dang backend tu dung: /api/hls/{uploadId}/master.m3u8
	hlsURL := "/api/hls/" + uploadID.String() + "/master.m3u8"
	duration := 120.0

	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: &model.LessonProgress{EnrollmentID: enrollmentID, Status: "in_progress"},
		enrollment:     &enrollment,
		courseID:       uuid.New(),
		lessonOrder:    []uuid.UUID{lessonID},
	}
	lessonRepo := &fakeLessonRepoHealWatched{}
	contentID := uuid.New()
	lessonRepo.contents = []model.LessonContent{{ID: contentID, Type: "video", Duration: 0, VideoURL: &hlsURL}}
	videoRepo := &fakeVideoUploadRepoWatched{
		uploads: map[uuid.UUID]*model.VideoUpload{uploadID: {Duration: &duration}},
	}
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, lessonRepo, videoRepo)

	khaiSai := 10
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), lessonID,
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &khaiSai,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 0, End: 120}},
		})
	if err != nil {
		t.Fatalf("UpdateLessonProgress loi: %v", err)
	}
	if res.Status != "completed" {
		t.Errorf("status = %q, muon \"completed\": duration lay tu video_uploads phai la server-truth "+
			"nen nguong tu chot completed phai co hieu luc", res.Status)
	}
	if len(lessonRepo.updatedDurations) != 1 {
		t.Fatalf("so lan ghi nguoc duration = %d, muon 1", len(lessonRepo.updatedDurations))
	}
	if lessonRepo.updatedDurations[0] != 120 {
		t.Errorf("duration ghi nguoc = %d, muon 120", lessonRepo.updatedDurations[0])
	}
	// C-5: phai ghi qua duong MOT COT, khong phai db.Save — neu khong se de im lang thay doi
	// cua request song song. Khang dinh content dung ID da duoc chua.
	if lessonRepo.updatedIDs[0] != contentID {
		t.Errorf("ghi nguoc vao content %s, muon %s", lessonRepo.updatedIDs[0], contentID)
	}
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
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{}, nil)

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

// Re-enroll (review 260912, finding N1) PHAI tra ve tong thoi gian xem THAT, khop voi
// GET /my-enrollments.
//
// Truoc day GetByUserAndCourseUnscoped khong Preload("LessonProgress") nen existing.LessonProgress
// luon nil => sumWatchedSeconds luon 0, trong khi comment ngay tren dong do khang dinh "cong don tu
// bo nho ... cho dung thuc te". Kich ban that: hoc vien xem 5640s -> unenroll -> enroll lai:
// POST /enroll tra watched_seconds = 0 (nghia la "chua xem gi" theo doc cua DTO), vai giay sau
// GET /my-enrollments tra 5640 cho CUNG ghi danh do.
//
// Test nay khoa ca hai dau cua hop dong:
//   - service phai doc so tu existing.LessonProgress (ban ghi repository da Preload);
//   - neu somebody bo Preload trong repository, fake nay van con nguyen du lieu => test nay xanh
//     gia. Vi vay test o tang repository (goi DryRun/GetByUserAndCourseUnscoped that) la thu
//     khang dinh Preload co that su duoc phat ra — xem
//     TestGetByUserAndCourseUnscoped_CoPreloadLessonProgress ben package repository.
func TestEnroll_ReEnrollTraWatchedSecondsThat(t *testing.T) {
	enrollmentID := uuid.New()
	courseID := uuid.New()

	// Got soft-delete: day la ban ghi cu duoc khoi phuc, khong phai enroll lan dau.
	deletedAt := gorm.DeletedAt{Time: time.Now(), Valid: true}
	existing := &model.Enrollment{
		BaseModel:  model.BaseModel{ID: enrollmentID, DeletedAt: deletedAt},
		CourseID:   courseID,
		EnrolledAt: time.Now().Add(-30 * 24 * time.Hour),
		// Cac dong lesson_progress KHONG bi xoa khi Unenroll — repo that Preload chung len day.
		LessonProgress: []model.LessonProgress{
			{VideoWatchedSecs: 4800, EnrollmentID: enrollmentID},
			{VideoWatchedSecs: 840, EnrollmentID: enrollmentID},
		},
	}
	repo := &fakeEnrollmentRepoWatched{unscopedEnrollment: existing}
	courseRepo := &fakeCourseRepoWatched{}
	svc := NewEnrollmentService(repo, courseRepo, &fakeLessonRepoWatched{}, nil)

	res, err := svc.Enroll(context.Background(), uuid.New(), courseID)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	if got, want := res.WatchedSeconds, 5640; got != want {
		t.Errorf("WatchedSeconds sau re-enroll = %d, mong doi %d (tong cua 4800 + 840 da Preload)", got, want)
	}
	// Nhanh re-enroll phai di qua RestoreAndReactivate chu khong tao ban ghi moi.
	if repo.reactivateUpdates == nil {
		t.Fatal("nhanh re-enroll khong goi RestoreAndReactivate")
	}
	if repo.unscopedCalls == 0 {
		t.Error("Enroll khong dung ban Unscoped de phat hien re-enroll")
	}
	// Reset tien do tren bang enrollments: neu khong reset, ghi danh vua khoi phuc se mang tien do
	// cu (khong nhat quan voi watched_seconds = 5640 vua tra ve).
	for _, col := range []string{"progress_percentage", "completed_at", "last_accessed_at", "enrolled_at"} {
		if _, ok := repo.reactivateUpdates[col]; !ok {
			t.Errorf("map RestoreAndReactivate thieu cot %q", col)
		}
	}
}

// TestGetByUserAndCourseUnscoped_PreloadLessonProgressChoNhanhReEnroll (review 260912, finding N1)
// — pin CHINH cai Preload, khong chi pin viec service doc du lieu.
//
// Test service o tren (TestEnroll_ReEnrollTraWatchedSecondsThat) chay bang fake, nen no chi chung
// minh duoc "NEU repository tra ve LessonProgress thi service cong dung". No VAN XANH neu ai do xoa
// Preload("LessonProgress") khoi repository that — dung kieu "green that proves nothing" ma finding
// N2 vua va o cho khac. Test nay goi THANG repository that (NewEnrollmentRepository + DryRun) va bat
// lay danh sach preload ma no phat ra, nen xoa Preload la do ngay o day.
//
// Vi sao dat o package service: trong repo nay moi file *_test.go cua internal/repository deu nam
// trong .git/info/exclude (local-only — xem comment "giu ngoai PR #54" trong file do), nen mot test
// dat tai internal/repository se KHONG BAO GIO chay tren CI. Day la hop dong ma chinh service phu
// thuoc de tra dung watched_seconds, nen dat canh no la cho duy nhat bao ve duoc tren CI.
func TestGetByUserAndCourseUnscoped_PreloadLessonProgressChoNhanhReEnroll(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("khong mo duoc gorm db: %v", err)
	}

	// Statement.Preloads cua cau truy van chinh; cac cau preload chay sau do qua callback
	// gorm:preload, nen phan tu [0] chinh la cau SELECT enrollments.
	var seen []map[string][]interface{}
	if err := db.Callback().Query().Before("gorm:query").Register("test:capture_preloads", func(tx *gorm.DB) {
		snapshot := make(map[string][]interface{}, len(tx.Statement.Preloads))
		for name, conds := range tx.Statement.Preloads {
			snapshot[name] = conds
		}
		seen = append(seen, snapshot)
	}); err != nil {
		t.Fatalf("khong dang ky duoc callback: %v", err)
	}

	repo := repository.NewEnrollmentRepository(db.Session(&gorm.Session{DryRun: true}))

	if _, err := repo.GetByUserAndCourseUnscoped(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	if len(seen) == 0 {
		t.Fatal("khong bat duoc cau truy van nao — callback khong chay, test nay vo nghia")
	}
	if _, ok := seen[0]["LessonProgress"]; !ok {
		t.Errorf("GetByUserAndCourseUnscoped KHONG Preload LessonProgress (preloads=%v).\n"+
			"Thieu Preload nay thi nhanh re-enroll cua Enroll luon tra watched_seconds = 0, "+
			"mau thuan voi GET /my-enrollments cho cung ghi danh do.", seen[0])
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
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{}, nil)

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
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{}, nil)

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
			svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

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
			// updated_at / last_accessed_at (review 260912, finding N2): hai cot nay PHAI co trong
			// map. Repository ghi bang UpdateColumns, ma UpdateColumns dat SkipHooks = true
			// (finisher_api.go:423-428, gorm v1.30.0) nen nhanh AutoUpdateTime bi bo qua
			// (callbacks/update.go:235-238) — dong "updated_at": now o service la thu DUY NHAT giu
			// cot do song. Xoa no di thi cot dung yen mai mai (hong sync incremental / cache
			// invalidation / audit) ma ca suite van xanh: dung kieu "green that proves nothing".
			if _, ok := repo.updateUpdates["updated_at"]; !ok {
				t.Error("map UPDATE thieu cot updated_at — UpdateColumns bo qua auto-update-time cua GORM, thieu cot nay thi updated_at dung yen vinh vien")
			}
			if _, ok := repo.updateUpdates["last_accessed_at"]; !ok {
				t.Error("map UPDATE thieu cot last_accessed_at — trinh phat vua cham vao bai hoc, cot nay phai duoc ghi")
			}
			if _, ok := repo.updateUpdates["video_watched_seconds"]; ok {
				t.Error("video_watched_seconds nam trong map — no phai di qua GREATEST() trong SQL, khong set tho")
			}
			if existing.Status != "in_progress" {
				t.Errorf("status bi doi thanh %q du request khong gui status", existing.Status)
			}

			// DTO phai phan anh ban ghi SAU khi ghi (doc lai tu repo), khong phai gia tri stale trong
			// bo nho: gui 10 len mot row 432 thi phai tra ve 432.
			if res.WatchedSeconds != tc.mongDoi {
				t.Errorf("VideoWatchedSecs tra ve = %d, mong doi %d (hien tai %d, gui len %d)",
					res.WatchedSeconds, tc.mongDoi, tc.hienTai, tc.guiLen)
			}
			if existing.VideoWatchedSecs != tc.mongDoi {
				t.Errorf("VideoWatchedSecs trong DB (gia lap) = %d, mong doi %d (hien tai %d, gui len %d)",
					existing.VideoWatchedSecs, tc.mongDoi, tc.hienTai, tc.guiLen)
			}
		})
	}
}

// TestUpdateLessonProgress_GuiStatusThiCapNhatStatus (cap nhat Phase 1 §1): tu sau played_ranges,
// client gui thang status="completed" KHONG con du de hoan thanh bai — server chi tu chot
// completed khi CHINH NO tinh ra watched_pct dat nguong (xem resolveLessonStatus). Test nay khang
// dinh ca hai ve: (1) status="completed" tu client, KHONG kem played_ranges/duration, bi bo qua;
// (2) played_ranges phu du thoi luong thi server tu chot completed va ghi completed_at, du client
// khong gui status nao ca.
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
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

	// (1) Client tu gui completed, khong co can cu (played_ranges/duration) => phai bi bo qua.
	status := "completed"
	secs := 10
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{Status: &status, VideoWatchedSecs: &secs})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if res.Status != "in_progress" {
		t.Errorf("status tra ve = %q, mong doi \"in_progress\" (client tu gui completed phai bi bo qua)", res.Status)
	}
	if _, ok := repo.updateUpdates["status"]; ok {
		t.Error("map UPDATE khong duoc chua status khi client tu gui completed ma chua co can cu")
	}
	if _, ok := repo.updateUpdates["completed_at"]; ok {
		t.Error("map UPDATE khong duoc chua completed_at khi client tu gui completed ma chua co can cu")
	}

	// (2) played_ranges phu 100% thoi luong => server TU chot completed (khong gui status).
	//
	// C-2 (review vòng 2, BLOCKER): ve nay CHI con dung khi duration la server-truth, nen bai nay
	// phai co duration THAT trong lesson_contents (100s). Truoc ban va C-2, test nay chay voi
	// fakeLessonRepoWatched RONG (server duration = 0) — dung cai mau so do client tu khai ma C-2
	// chan lai; xem TestUpdateLessonProgress_FallbackDuration_KhongTuChotCompleted cho chinh
	// truong hop do.
	svcWithDuration := NewEnrollmentService(repo, &fakeCourseRepoWatched{},
		&fakeLessonRepoWatched{contents: []model.LessonContent{{Type: "video", Duration: 100}}}, nil)
	duration := 100
	res2, err := svcWithDuration.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &duration,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 0, End: 100}},
		})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if got, _ := repo.updateUpdates["status"].(string); got != "completed" {
		t.Errorf("cot status trong map UPDATE = %q, mong doi \"completed\"", got)
	}
	if _, ok := repo.updateUpdates["completed_at"]; !ok {
		t.Error("map UPDATE thieu completed_at khi status chuyen sang completed")
	}
	if res2.Status != "completed" {
		t.Errorf("status tra ve = %q, mong doi \"completed\"", res2.Status)
	}
	// status di qua map, khong duoc set tho trong cung cau lenh voi watched seconds.
	if _, ok := repo.updateUpdates["video_watched_seconds"]; ok {
		t.Error("video_watched_seconds nam trong map — phai di qua GREATEST() trong SQL")
	}
}

// TestUpdateLessonProgress_FallbackDuration_KhongTuChotCompleted (C-2, review vòng 2, BLOCKER):
// bai hoc KHONG co duration nao phia server (lesson_contents/lesson_videos deu 0) — mau so cua
// watched_pct roi ve duration_seconds do CLIENT tu khai. Tren mot bai nhu vay, payload
// {"duration_seconds":10,"played_ranges":[[0,10]]} cho watched_pct = 100 va truoc ban va C-2 se
// TU CHOT completed — ma completed la sticky nen khong thu hoi duoc. Test nay khang dinh:
//   - watched_pct/played_ranges/fallback_duration_seconds VAN duoc ghi (khong mat du lieu);
//   - status KHONG duoc cap completed, khong co completed_at trong map UPDATE.
// Day la dang PIN cua lo hong: neu ai do bo dieu kien trustedDuration trong resolveLessonStatus,
// test nay DO ngay (con so 100% van con nguyen o cot watched_pct).
func TestUpdateLessonProgress_FallbackDuration_KhongTuChotCompleted(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	existing := &model.LessonProgress{
		EnrollmentID: enrollmentID,
		Status:       "in_progress",
	}
	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: existing,
		enrollment:     &enrollment,
		courseID:       uuid.New(),
	}
	// fakeLessonRepoWatched RONG => resolveServerVideoDuration tra 0 => usingFallbackDuration=true.
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

	khaiKhong := 10
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &khaiKhong,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 0, End: 10}},
		})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	// Du lieu tien do VAN duoc ghi — C-2 chi giu lai khoan "cap completed", khong chan ghi.
	if pct, ok := repo.updateUpdates["watched_pct"].(decimal.Decimal); !ok {
		t.Fatal("map UPDATE thieu watched_pct — C-2 khong duoc phep lam mat du lieu tien do")
	} else if got, _ := pct.Float64(); got < 99.9 || got > 100.1 {
		t.Fatalf("watched_pct trong map = %v, mong doi =100 (van phai ghi de khong mat du lieu)", got)
	}
	if _, ok := repo.updateUpdates["played_ranges"]; !ok {
		t.Error("map UPDATE thieu played_ranges — khoang da phat van phai duoc luu")
	}
	if _, ok := repo.updateUpdates["fallback_duration_seconds"]; !ok {
		t.Error("map UPDATE thieu fallback_duration_seconds — mau so client khai van phai duoc luu sticky")
	}

	// Khoan bi giu lai: completed.
	if got, ok := repo.updateUpdates["status"].(string); ok {
		t.Fatalf("cot status trong map UPDATE = %q — khong duoc cap completed khi mau so chua phai server-truth", got)
	}
	if _, ok := repo.updateUpdates["completed_at"]; ok {
		t.Error("map UPDATE co completed_at — bai khong duoc coi la hoan thanh khi mau so chua phai server-truth")
	}
	if res.Status != "in_progress" {
		t.Errorf("status tra ve = %q, mong doi \"in_progress\"", res.Status)
	}
	if existing.Status == "completed" {
		t.Error("ban ghi bi chot completed — day chinh la lo hong C-2 ma ban va nay dong lai")
	}
}

// TestUpdateLessonProgress_ServerDurationThangTheKhaiGiaCuaClient (B-1, review vòng 2, BLOCKER):
// bài học có duration THẬT ở server là 1200 giây (lesson_contents, Type="video"). Client khai
// khống duration_seconds=10 để watched_pct nhảy thẳng lên gần 100% chỉ với 10 giây xem thật —
// đây chính là lỗ hổng B-1. Server PHẢI dùng 1200 (của chính nó), không phải 10 (client khai),
// làm mẫu số — payload [[0,10]] trên bài 1200s cho pct ≈0.8 (10/1200*100 làm tròn 1 chữ số thập
// phân), KHÔNG completed. Bước 2 gửi tiếp một request khác với duration_seconds=5 (một giá trị
// khai khống KHÁC, nhỏ hơn) để chứng minh khoảng ĐÃ LƯU [[0,10]] KHÔNG bị mất — dưới code cũ,
// normalizePlayedRange sẽ REJECT (không phải clamp) khoảng cũ vì End=10 > duration=5 "giả" ở lần
// gửi sau, xoá sạch dữ liệu hợp lệ chỉ vì một request khác khai một duration nhỏ hơn.
func TestUpdateLessonProgress_ServerDurationThangTheKhaiGiaCuaClient(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: nil, // ban ghi dau tien — di duong INSERT
		enrollment:     &enrollment,
		courseID:       uuid.New(),
	}
	lessonRepo := &fakeLessonRepoWatched{
		contents: []model.LessonContent{{Type: "video", Duration: 1200}},
	}
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, lessonRepo, nil)

	// (1) Lan dau: payload [[0,10]] + duration_seconds=10 (khai khong) tren bai 1200s that.
	spoofedDuration := 10
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &spoofedDuration,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 0, End: 10}},
		})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if res.WatchedPct < 0.75 || res.WatchedPct > 0.85 {
		t.Fatalf("DTO tra ve watched_pct = %v, mong doi ≈0.8", res.WatchedPct)
	}
	if repo.upserted == nil {
		t.Fatal("ban ghi dau tien khong duoc tao")
	}
	gotPct, _ := repo.upserted.WatchedPct.Float64()
	if gotPct < 0.75 || gotPct > 0.85 {
		t.Fatalf("watched_pct = %v, mong doi ≈0.8 (10/1200*100, KHONG phai 10/10*100=100 — server phai dung duration THAT 1200, khong phai duration_seconds=10 client khai)", gotPct)
	}
	if repo.upserted.Status == "completed" {
		t.Fatal("status = completed — 0.8% khong the vuot nguong min_video_pct (mac dinh 90)")
	}
	if !sameRanges(repo.upserted.PlayedRanges, model.PlayedRanges{{Start: 0, End: 10}}) {
		t.Fatalf("played_ranges = %+v, mong doi [[0,10]]", repo.upserted.PlayedRanges)
	}

	// (2) Chuyen ban ghi vua tao thanh ban ghi DA CO de lan goi sau di duong UPDATE, dung mot
	// duration_seconds KHAI KHONG KHAC (5, nho hon ca lan truoc) — mo phong hai request khac nhau
	// tu CUNG mot client bi loi/bi tan cong voi hai gia tri khac nhau.
	repo.lessonProgress = repo.upserted
	repo.upserted = nil
	secondSpoof := 5
	res2, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &secondSpoof,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 20, End: 30}},
		})
	if err != nil {
		t.Fatalf("khong mong doi loi (lan 2): %v", err)
	}
	// Khong mat khoang cu [[0,10]]: merge voi khoang moi [[20,30]] phai la CA HAI, khong phai chi
	// khoang moi (neu code cu con reject khoang cu vi "vuot duration=5 gia").
	wantMerged := model.PlayedRanges{{Start: 0, End: 10}, {Start: 20, End: 30}}
	if !sameRanges(repo.lessonProgress.PlayedRanges, wantMerged) {
		t.Fatalf("played_ranges sau lan 2 = %+v, mong doi %+v — khoang cu [[0,10]] khong duoc mat du lan nay client khai duration=5",
			repo.lessonProgress.PlayedRanges, wantMerged)
	}
	if res2.WatchedSeconds != 20 {
		t.Fatalf("watched_seconds tra ve = %d, mong doi 20 (10 cu + 10 moi, ca hai khoang deu con)", res2.WatchedSeconds)
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
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

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
	if res.WatchedSeconds != 87 {
		t.Errorf("DTO tra ve VideoWatchedSecs = %d, mong doi 87", res.WatchedSeconds)
	}
}

// TestUpdateLessonProgress_StatusKhongDuocHaCap (HIGH-2, review 260915): beacon dong tab
// (POST /api/progress) luon gui cung "in_progress" du nguoi hoc dang xem lai mot bai da
// "completed". Ghi de vo dieu kien se ha cap ban ghi va lam CountCompletedMandatory tut so —
// % tien do khoa hoc phu huynh thay se giam moi khi con xem lai bai cu.
func TestUpdateLessonProgress_StatusKhongDuocHaCap(t *testing.T) {
	cases := []struct {
		ten           string
		hienTai       string
		gui           string
		mongStatusDoc string // status con lai trong "DB" gia lap sau khi goi
		mongCoTrongMap bool  // "status" co duoc dua vao map UPDATE khong
	}{
		{"completed -> in_progress: bi chan", "completed", "in_progress", "completed", false},
		{"completed -> not_started: bi chan", "completed", "not_started", "completed", false},
		// completed -> completed: gia tri khong doi nen KHONG can ghi lai (khac voi ban truoc
		// Phase 1, khi rank>=rank la ghi vo dieu kien du gia tri giong het nhau).
		{"completed -> completed: khong doi, khong ghi lai", "completed", "completed", "completed", false},
		// in_progress -> completed CHUA co can cu (khong kem played_ranges/duration dat nguong)
		// bi CHAN — day chinh la thay doi cot loi cua Phase 1 §1: client khong con tu chot
		// completed duoc nua, xem TestResolveLessonStatus_ClientGuiCompletedBiBoQua.
		{"in_progress -> completed: bi chan (chua dat nguong)", "in_progress", "completed", "in_progress", false},
		{"not_started -> in_progress: cho qua (tien len)", "not_started", "in_progress", "in_progress", true},
		{"in_progress -> not_started: bi chan", "in_progress", "not_started", "in_progress", false},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			enrollmentID := uuid.New()
			enrollment := newEnrollmentWithID(enrollmentID)
			existing := &model.LessonProgress{
				VideoWatchedSecs: 100,
				EnrollmentID:     enrollmentID,
				Status:           tc.hienTai,
			}
			repo := &fakeEnrollmentRepoWatched{
				lessonProgress: existing,
				enrollment:     &enrollment,
				courseID:       uuid.New(),
			}
			svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

			gui := tc.gui
			res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
				dto.UpdateLessonProgressDTO{Status: &gui})
			if err != nil {
				t.Fatalf("khong mong doi loi: %v", err)
			}

			_, coTrongMap := repo.updateUpdates["status"]
			if coTrongMap != tc.mongCoTrongMap {
				t.Errorf("status co trong map UPDATE = %v, mong doi %v (hien tai=%q, gui=%q)",
					coTrongMap, tc.mongCoTrongMap, tc.hienTai, tc.gui)
			}
			if existing.Status != tc.mongStatusDoc {
				t.Errorf("status sau cung trong DB gia lap = %q, mong doi %q (hien tai=%q, gui=%q)",
					existing.Status, tc.mongStatusDoc, tc.hienTai, tc.gui)
			}
			if res.Status != tc.mongStatusDoc {
				t.Errorf("status DTO tra ve = %q, mong doi %q", res.Status, tc.mongStatusDoc)
			}
		})
	}
}

// TestUpdateLessonProgress_HaCapKhongDongThoiXoaCompletedAt: mot request bi chan ha cap (vi du
// completed -> in_progress) khong duoc dua ca "completed_at" vao map UPDATE — guard nam TRUOC ca
// hai truong nen chan status thi chan luon completed_at cung mot cho, khong con duong nao khac
// de vo tinh xoa moc thoi gian hoan thanh.
func TestUpdateLessonProgress_HaCapKhongDongThoiXoaCompletedAt(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	existing := &model.LessonProgress{
		VideoWatchedSecs: 100,
		EnrollmentID:     enrollmentID,
		Status:           "completed",
	}
	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: existing,
		enrollment:     &enrollment,
		courseID:       uuid.New(),
	}
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

	status := "in_progress"
	if _, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{Status: &status}); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if _, ok := repo.updateUpdates["completed_at"]; ok {
		t.Error("map UPDATE chua completed_at du request bi chan ha cap status")
	}
}
