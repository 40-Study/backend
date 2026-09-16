package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// Test cho NoteService (Phase 1 §3) — chi chu so huu doc/sua/xoa, tao ghi chu yeu cau da
// enroll, va validate content <= 2000 ky tu o tang DTO.

type fakeNoteRepo struct {
	repository.NoteRepositoryInterface

	notes map[uuid.UUID]*model.UserNote

	createCalls int
	created     *model.UserNote
	createErr   error

	updateCalls int
	updated     *model.UserNote
	updateErr   error

	deleteCalls int
	deletedID   uuid.UUID
	deleteErr   error

	listByLessonSort string
	listByLessonRes  []model.UserNote
	// listByLessonUserID/listByCourseUserID (T-1, review vòng 2): ghi lại userID THẬT SỰ nhận
	// được — trước bản vá, cả hai fake bỏ qua tham số này hoàn toàn, nên thay userID bằng
	// uuid.Nil ở NoteService.ListByLesson/ListByCourse (đường ĐỌC, không có kiểm chủ sở hữu
	// tường minh như Update/Delete, dựa hoàn toàn vào userID truyền đúng xuống WHERE) vẫn xanh.
	listByLessonUserID uuid.UUID

	listByCourseSort      string
	listByCourseSectionID *uuid.UUID
	listByCourseRes       []model.UserNote
	listByCourseUserID    uuid.UUID
}

func (f *fakeNoteRepo) Create(ctx context.Context, note *model.UserNote) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	if note.ID == uuid.Nil {
		note.ID = uuid.New()
	}
	f.created = note
	if f.notes == nil {
		f.notes = map[uuid.UUID]*model.UserNote{}
	}
	f.notes[note.ID] = note
	return nil
}

func (f *fakeNoteRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.UserNote, error) {
	if f.notes == nil {
		return nil, nil
	}
	return f.notes[id], nil
}

func (f *fakeNoteRepo) Update(ctx context.Context, note *model.UserNote) error {
	f.updateCalls++
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = note
	return nil
}

func (f *fakeNoteRepo) Delete(ctx context.Context, id uuid.UUID) error {
	f.deleteCalls++
	f.deletedID = id
	return f.deleteErr
}

func (f *fakeNoteRepo) ListByLesson(ctx context.Context, userID, lessonID uuid.UUID, sort string) ([]model.UserNote, error) {
	f.listByLessonUserID = userID
	f.listByLessonSort = sort
	return f.listByLessonRes, nil
}

func (f *fakeNoteRepo) ListByCourse(ctx context.Context, userID, courseID uuid.UUID, sectionID *uuid.UUID, sort string) ([]model.UserNote, error) {
	f.listByCourseUserID = userID
	f.listByCourseSort = sort
	f.listByCourseSectionID = sectionID
	return f.listByCourseRes, nil
}

// newTestNote xay mot UserNote da kem Lesson.Section — dung hop dong ma NoteRepository.GetByID
// that su thoa (Preload("Lesson.Section")), de toNoteResponseDTO doc duoc lesson_title/section_title.
func newTestNote(id, userID, lessonID, courseID uuid.UUID) *model.UserNote {
	n := &model.UserNote{
		UserID:        userID,
		LessonID:      lessonID,
		CourseID:      &courseID,
		TimestampSecs: 42,
		Content:       "ghi chu test",
	}
	n.ID = id
	n.Lesson = model.Lesson{
		ID:        lessonID,
		Title:     "Bai hoc test",
		SectionID: uuid.New(),
	}
	n.Lesson.Section = model.Section{Title: "Chuong test"}
	return n
}

// TestNoteService_CreateNote_YeuCauEnroll: chua enroll khoa chua bai hoc do thi tao ghi chu
// phai bi tu choi bang ErrNoteNotEnrolled, KHONG duoc goi Create.
func TestNoteService_CreateNote_YeuCauEnroll(t *testing.T) {
	lessonID, courseID := uuid.New(), uuid.New()
	enrollRepo := &fakeEnrollmentRepoWatched{courseID: courseID, enrollment: nil}
	noteRepo := &fakeNoteRepo{}
	svc := NewNoteService(noteRepo, enrollRepo)

	_, err := svc.CreateNote(context.Background(), uuid.New(), lessonID, dto.CreateNoteDTO{
		TimestampSecs: 10, Content: "abc",
	})

	if err != ErrNoteNotEnrolled {
		t.Fatalf("loi = %v, muon ErrNoteNotEnrolled", err)
	}
	if noteRepo.createCalls != 0 {
		t.Error("Create khong duoc goi khi chua enroll")
	}
}

// TestNoteService_CreateNote_DaEnroll_TaoThanhCong: da enroll thi tao ghi chu thanh cong, dung
// user_id/lesson_id/course_id/content/timestamp_seconds.
func TestNoteService_CreateNote_DaEnroll_TaoThanhCong(t *testing.T) {
	userID, lessonID, courseID := uuid.New(), uuid.New(), uuid.New()
	enrollment := &model.Enrollment{UserID: userID, CourseID: courseID}
	enrollRepo := &fakeEnrollmentRepoWatched{courseID: courseID, enrollment: enrollment}
	noteRepo := &fakeNoteRepo{}
	svc := NewNoteService(noteRepo, enrollRepo)

	res, err := svc.CreateNote(context.Background(), userID, lessonID, dto.CreateNoteDTO{
		TimestampSecs: 123, Content: "ghi chu",
	})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if noteRepo.createCalls != 1 {
		t.Fatalf("Create duoc goi %d lan, muon 1", noteRepo.createCalls)
	}
	if noteRepo.created.UserID != userID || noteRepo.created.LessonID != lessonID ||
		noteRepo.created.CourseID == nil || *noteRepo.created.CourseID != courseID {
		t.Errorf("note tao ra = %+v, thieu dung user_id/lesson_id/course_id", noteRepo.created)
	}
	if res.TimestampSecs != 123 || res.Content != "ghi chu" {
		t.Errorf("response = %+v, khong khop du lieu gui len", res)
	}
}

// TestNoteService_UpdateNote_KhongPhaiChuSoHuu_BiChan: user khac chu so huu sua ghi chu phai
// bi tu choi bang ErrNotNoteOwner, KHONG duoc ghi de noi dung.
func TestNoteService_UpdateNote_KhongPhaiChuSoHuu_BiChan(t *testing.T) {
	noteID, owner, ke := uuid.New(), uuid.New(), uuid.New()
	note := newTestNote(noteID, owner, uuid.New(), uuid.New())
	noteRepo := &fakeNoteRepo{notes: map[uuid.UUID]*model.UserNote{noteID: note}}
	svc := NewNoteService(noteRepo, &fakeEnrollmentRepoWatched{})

	newContent := "noi dung gia mao"
	_, err := svc.UpdateNote(context.Background(), ke, noteID, dto.UpdateNoteDTO{Content: &newContent})

	if err != ErrNotNoteOwner {
		t.Fatalf("loi = %v, muon ErrNotNoteOwner", err)
	}
	if noteRepo.updateCalls != 0 {
		t.Error("Update khong duoc goi khi khong phai chu so huu")
	}
	if note.Content == newContent {
		t.Error("noi dung ghi chu bi ghi de du khong phai chu so huu")
	}
}

// TestNoteService_UpdateNote_ChuSoHuu_SuaMotPhan: chu so huu sua CHI content (khong gui
// timestamp_seconds) thi timestamp_seconds phai GIU NGUYEN, chi content doi.
func TestNoteService_UpdateNote_ChuSoHuu_SuaMotPhan(t *testing.T) {
	noteID, owner := uuid.New(), uuid.New()
	note := newTestNote(noteID, owner, uuid.New(), uuid.New())
	note.TimestampSecs = 999
	noteRepo := &fakeNoteRepo{notes: map[uuid.UUID]*model.UserNote{noteID: note}}
	svc := NewNoteService(noteRepo, &fakeEnrollmentRepoWatched{})

	newContent := "noi dung moi"
	res, err := svc.UpdateNote(context.Background(), owner, noteID, dto.UpdateNoteDTO{Content: &newContent})
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if noteRepo.updateCalls != 1 {
		t.Fatalf("Update duoc goi %d lan, muon 1", noteRepo.updateCalls)
	}
	if res.Content != newContent {
		t.Errorf("content = %q, muon %q", res.Content, newContent)
	}
	if res.TimestampSecs != 999 {
		t.Errorf("timestamp_seconds = %d, muon giu nguyen 999 (khong gui truong nay)", res.TimestampSecs)
	}
}

// TestNoteService_DeleteNote_KhongPhaiChuSoHuu_BiChan: user khac xoa ghi chu cua nguoi khac
// phai bi tu choi, KHONG duoc goi Delete.
func TestNoteService_DeleteNote_KhongPhaiChuSoHuu_BiChan(t *testing.T) {
	noteID, owner, ke := uuid.New(), uuid.New(), uuid.New()
	note := newTestNote(noteID, owner, uuid.New(), uuid.New())
	noteRepo := &fakeNoteRepo{notes: map[uuid.UUID]*model.UserNote{noteID: note}}
	svc := NewNoteService(noteRepo, &fakeEnrollmentRepoWatched{})

	err := svc.DeleteNote(context.Background(), ke, noteID)

	if err != ErrNotNoteOwner {
		t.Fatalf("loi = %v, muon ErrNotNoteOwner", err)
	}
	if noteRepo.deleteCalls != 0 {
		t.Error("Delete khong duoc goi khi khong phai chu so huu")
	}
}

// TestNoteService_DeleteNote_ChuSoHuu_XoaThanhCong: chu so huu xoa duoc, dung ID truyen xuong repo.
func TestNoteService_DeleteNote_ChuSoHuu_XoaThanhCong(t *testing.T) {
	noteID, owner := uuid.New(), uuid.New()
	note := newTestNote(noteID, owner, uuid.New(), uuid.New())
	noteRepo := &fakeNoteRepo{notes: map[uuid.UUID]*model.UserNote{noteID: note}}
	svc := NewNoteService(noteRepo, &fakeEnrollmentRepoWatched{})

	if err := svc.DeleteNote(context.Background(), owner, noteID); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if noteRepo.deleteCalls != 1 || noteRepo.deletedID != noteID {
		t.Errorf("Delete goi %d lan voi id=%s, muon 1 lan voi id=%s", noteRepo.deleteCalls, noteRepo.deletedID, noteID)
	}
}

// TestNoteService_ListByLesson_TruyenDungThamSoSort: service phai truyen NGUYEN gia tri sort
// (newest/oldest) xuong repository — logic dich sang ORDER BY nam o repository
// (xem note_repository_sort_test.go), o day chi khang dinh hop dong service->repo.
func TestNoteService_ListByLesson_TruyenDungThamSoSort(t *testing.T) {
	for _, sort := range []string{"newest", "oldest", ""} {
		t.Run(sort, func(t *testing.T) {
			noteRepo := &fakeNoteRepo{}
			svc := NewNoteService(noteRepo, &fakeEnrollmentRepoWatched{})
			userID := uuid.New()

			if _, err := svc.ListByLesson(context.Background(), userID, uuid.New(), sort); err != nil {
				t.Fatalf("khong mong doi loi: %v", err)
			}
			if noteRepo.listByLessonSort != sort {
				t.Errorf("sort truyen xuong repo = %q, muon %q", noteRepo.listByLessonSort, sort)
			}
			// T-1 (review vòng 2): userID phải được truyền NGUYÊN xuống repo — đây là rào chắn
			// DUY NHẤT chống đọc ghi chú của người khác ở đường này (không có kiểm chủ sở hữu
			// tường minh như Update/Delete, dựa hoàn toàn vào WHERE user_id = ?).
			if noteRepo.listByLessonUserID != userID {
				t.Errorf("userID truyen xuong repo = %s, muon %s (dung userID cua chinh nguoi goi)", noteRepo.listByLessonUserID, userID)
			}
		})
	}
}

// TestNoteService_ListByCourse_TruyenDungSectionIDVaSort: section_id (khi co) va sort phai
// duoc truyen nguyen xuong repository.
func TestNoteService_ListByCourse_TruyenDungSectionIDVaSort(t *testing.T) {
	sectionID := uuid.New()
	noteRepo := &fakeNoteRepo{}
	svc := NewNoteService(noteRepo, &fakeEnrollmentRepoWatched{})
	userID := uuid.New()

	if _, err := svc.ListByCourse(context.Background(), userID, uuid.New(), &sectionID, "oldest"); err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if noteRepo.listByCourseSort != "oldest" {
		t.Errorf("sort = %q, muon \"oldest\"", noteRepo.listByCourseSort)
	}
	if noteRepo.listByCourseSectionID == nil || *noteRepo.listByCourseSectionID != sectionID {
		t.Errorf("section_id truyen xuong repo = %v, muon %s", noteRepo.listByCourseSectionID, sectionID)
	}
	// T-1 (review vòng 2): xem chú thích tại TestNoteService_ListByLesson_TruyenDungThamSoSort.
	if noteRepo.listByCourseUserID != userID {
		t.Errorf("userID truyen xuong repo = %s, muon %s (dung userID cua chinh nguoi goi)", noteRepo.listByCourseUserID, userID)
	}
}

// ---------------------------------------------------------------------------
// Validate DTO — content <= 2000 ky tu (contract §3)
// ---------------------------------------------------------------------------

// TestCreateNoteDTO_ContentVuotNguongBiTuChoi: content > 2000 ky tu phai bi validate chan o
// tang DTO (utils.ValidateStruct, dung nhu handler goi truoc khi vao service).
func TestCreateNoteDTO_ContentVuotNguongBiTuChoi(t *testing.T) {
	vuaDu := make([]byte, 2000)
	for i := range vuaDu {
		vuaDu[i] = 'a'
	}
	vuotNguong := make([]byte, 2001)
	for i := range vuotNguong {
		vuotNguong[i] = 'a'
	}

	if errs := utils.ValidateStruct(dto.CreateNoteDTO{TimestampSecs: 0, Content: string(vuaDu)}); len(errs) != 0 {
		t.Errorf("content dung 2000 ky tu phai hop le nhung bi tu choi: %v", errs)
	}
	if errs := utils.ValidateStruct(dto.CreateNoteDTO{TimestampSecs: 0, Content: string(vuotNguong)}); len(errs) == 0 {
		t.Error("content 2001 ky tu phai bi tu choi, nhung validate lai cho qua")
	}
	if errs := utils.ValidateStruct(dto.CreateNoteDTO{TimestampSecs: 0, Content: ""}); len(errs) == 0 {
		t.Error("content rong phai bi tu choi (required), nhung validate lai cho qua")
	}
}

// TestUpdateNoteDTO_ContentVuotNguongBiTuChoi: cung nguong ap dung cho UpdateNoteDTO — content
// la con tro (optional) nen rieng truong hop KHONG gui (nil) phai hop le.
func TestUpdateNoteDTO_ContentVuotNguongBiTuChoi(t *testing.T) {
	vuotNguong := make([]byte, 2001)
	for i := range vuotNguong {
		vuotNguong[i] = 'a'
	}
	content := string(vuotNguong)

	if errs := utils.ValidateStruct(dto.UpdateNoteDTO{Content: &content}); len(errs) == 0 {
		t.Error("content 2001 ky tu phai bi tu choi, nhung validate lai cho qua")
	}
	if errs := utils.ValidateStruct(dto.UpdateNoteDTO{}); len(errs) != 0 {
		t.Errorf("khong gui gi (chi sua timestamp o request khac) phai hop le: %v", errs)
	}
}
