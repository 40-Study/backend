package service

// Test cho V3-7 (issue #58), nhom handler /lesson-contents/:id/classes + /classes/:id/contents:
// truoc day ca 6 method cua ClassLessonContentService deu KHONG kiem quyen — bat ky user dang nhap
// nao (ke ca hoc sinh lop khac) gan/doi lich/xoa duoc lesson content khoi mot lop, va doc duoc
// thoi khoa bieu cua lop bat ky.
//
// Ma tran quyen duoc chot o day:
//   - GHI (assign / update / remove / bulk): chi giao vien lop, instructor khoa chua lop, hoac admin.
//   - DOC (get classes for content / content schedule for class): them hoc sinh cua lop.
//   - Nguoi la: 403 (ErrNotClassTeacher cho ghi, ErrNotClassMember cho doc) — va khong co thao tac
//     ghi nao cham toi repo.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// --- fakes -----------------------------------------------------------------------------------

// fakeClassRepoCLCAuthz: tra loi dung 2 cau hoi quan he lop — "co phai giao vien lop" va "co phai
// hoc sinh lop". Cac method khac cua interface de panic (embed interface nil) de bat ky phu thuoc
// ngoai y muon nao lo ra thi test do ngay.
type fakeClassRepoCLCAuthz struct {
	repository.ClassRepositoryInterface
	class        *model.Class
	isTeacher    bool
	isStudent    bool
	teacherCalls int
	studentCalls int
}

func (f *fakeClassRepoCLCAuthz) GetByID(ctx context.Context, id uuid.UUID) (*model.Class, error) {
	return f.class, nil
}

func (f *fakeClassRepoCLCAuthz) TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error) {
	f.teacherCalls++
	return f.isTeacher, nil
}

func (f *fakeClassRepoCLCAuthz) StudentClassExists(ctx context.Context, classID, studentID uuid.UUID) (bool, error) {
	f.studentCalls++
	return f.isStudent, nil
}

type fakeCourseRepoCLCAuthz struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseRepoCLCAuthz) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

type fakeLessonRepoCLCAuthz struct {
	repository.LessonRepositoryInterface
	content *model.LessonContent
}

func (f *fakeLessonRepoCLCAuthz) GetContentByID(ctx context.Context, id uuid.UUID) (*model.LessonContent, error) {
	return f.content, nil
}

type fakeEnrollmentRepoCLCAuthz struct {
	repository.EnrollmentRepositoryInterface
	courseID uuid.UUID
}

func (f *fakeEnrollmentRepoCLCAuthz) GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error) {
	return f.courseID, nil
}

// fakeCLCRepoAuthz: ghi lai moi thao tac GHI de chung minh nguoi khong co quyen khong cham duoc
// vao du lieu.
type fakeCLCRepoAuthz struct {
	repository.ClassLessonContentRepositoryInterface
	items           []model.ClassLessonContent
	exists          bool
	createCalls     int
	createBatchCall int
}

func (f *fakeCLCRepoAuthz) GetByContentID(ctx context.Context, contentID uuid.UUID, page, pageSize int) ([]model.ClassLessonContent, int64, error) {
	return f.items, int64(len(f.items)), nil
}

func (f *fakeCLCRepoAuthz) GetByClassID(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.ClassLessonContent, int64, error) {
	return f.items, int64(len(f.items)), nil
}

func (f *fakeCLCRepoAuthz) Exists(ctx context.Context, classID, contentID uuid.UUID) (bool, error) {
	return f.exists, nil
}

func (f *fakeCLCRepoAuthz) Create(ctx context.Context, clc *model.ClassLessonContent) error {
	f.createCalls++
	return nil
}

func (f *fakeCLCRepoAuthz) CreateBatch(ctx context.Context, clcs []*model.ClassLessonContent) error {
	f.createBatchCall++
	return nil
}

func (f *fakeCLCRepoAuthz) GetByClassAndContent(ctx context.Context, classID, contentID uuid.UUID) (*model.ClassLessonContent, error) {
	return &model.ClassLessonContent{
		ID:              uuid.New(),
		ClassID:         classID,
		LessonContentID: contentID,
	}, nil
}

// newCLCServiceForAuthz: dung service that voi day du fake can thiet, content type "video" (khong
// kich hoat nhanh tao phien livestream — nhanh do khong thuoc pham vi test uy quyen nay).
func newCLCServiceForAuthz(courseID uuid.UUID) (*ClassLessonContentService, *fakeClassRepoCLCAuthz, *fakeCLCRepoAuthz) {
	classRepo := &fakeClassRepoCLCAuthz{}
	clcRepo := &fakeCLCRepoAuthz{}
	svc := &ClassLessonContentService{
		clcRepo:        clcRepo,
		classRepo:      classRepo,
		courseRepo:     &fakeCourseRepoCLCAuthz{},
		lessonRepo:     &fakeLessonRepoCLCAuthz{content: &model.LessonContent{ID: uuid.New(), Type: "video"}},
		enrollmentRepo: &fakeEnrollmentRepoCLCAuthz{courseID: courseID},
	}
	return svc, classRepo, clcRepo
}

// --- GHI: assign / update / remove / bulk ----------------------------------------------------

// TestCLCAssignClassToContent_NguoiLa_Bi403 (kich ban chinh cua V3-7): nguoi khong phai giao vien
// lop/instructor khoa — gan lesson content vao lop phai bi tu choi, va khong ban ghi nao duoc tao.
func TestCLCAssignClassToContent_NguoiLa_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()
	stranger := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isTeacher = false
	classRepo.isStudent = false

	_, err := svc.AssignClassToContent(context.Background(), uuid.New(), stranger, false, dto.AssignClassToContentDTO{ClassID: classID})

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi loi uy quyen (ErrNotClassTeacher)", err)
	}
	if clcRepo.createCalls != 0 {
		t.Error("clcRepo.Create BI GOI du nguoi goi khong co quyen — da gan lich hoc vao lop nguoi khac")
	}
}

// TestCLCAssignClassToContent_HocSinhCuaLop_Bi403: hoc sinh CHINH lop do van khong duoc ghi —
// quyen doc khong keo theo quyen ghi.
func TestCLCAssignClassToContent_HocSinhCuaLop_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()
	student := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isStudent = true // la hoc sinh that cua lop

	_, err := svc.AssignClassToContent(context.Background(), uuid.New(), student, false, dto.AssignClassToContentDTO{ClassID: classID})

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi ErrNotClassTeacher (hoc sinh khong duoc gan lich)", err)
	}
	if clcRepo.createCalls != 0 {
		t.Error("clcRepo.Create BI GOI boi hoc sinh")
	}
}

// TestCLCAssignClassToContent_GiaoVienLop_ChoPhep: giao vien cua lop di het duoc luong.
func TestCLCAssignClassToContent_GiaoVienLop_ChoPhep(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()
	teacher := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isTeacher = true

	_, err := svc.AssignClassToContent(context.Background(), uuid.New(), teacher, false, dto.AssignClassToContentDTO{ClassID: classID})

	if err != nil {
		t.Fatalf("giao vien lop phai gan duoc, nhung loi: %v", err)
	}
	if clcRepo.createCalls != 1 {
		t.Errorf("clcRepo.Create duoc goi %d lan, mong doi 1", clcRepo.createCalls)
	}
}

// TestCLCAssignClassToContent_Admin_ChoPhep: admin he thong (SYSTEM_SETTINGS_MANAGE) xu ly duoc
// lop co van de — nhanh isAdmin phai duoc xet TRUOC moi truy van quan he lop.
func TestCLCAssignClassToContent_Admin_ChoPhep(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()
	admin := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isTeacher = false
	classRepo.isStudent = false

	_, err := svc.AssignClassToContent(context.Background(), uuid.New(), admin, true, dto.AssignClassToContentDTO{ClassID: classID})

	if err != nil {
		t.Fatalf("admin phai gan duoc, nhung loi: %v", err)
	}
	if clcRepo.createCalls != 1 {
		t.Errorf("clcRepo.Create duoc goi %d lan, mong doi 1", clcRepo.createCalls)
	}
	if classRepo.teacherCalls != 0 {
		t.Errorf("TeacherClassExists bi goi %d lan cho admin — admin khong phai tra cuu quan he lop", classRepo.teacherCalls)
	}
}

// TestCLCUpdateClassContentSchedule_NguoiLa_Bi403: doi lich hoc cua lop la thao tac ghi.
func TestCLCUpdateClassContentSchedule_NguoiLa_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, _ := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}

	_, err := svc.UpdateClassContentSchedule(context.Background(), uuid.New(), classID, uuid.New(), false, dto.UpdateClassContentScheduleDTO{})

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi loi uy quyen", err)
	}
}

// TestCLCRemoveClassFromContent_NguoiLa_Bi403: xoa lich hoc cua lop khoi lesson content.
func TestCLCRemoveClassFromContent_NguoiLa_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, _ := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}

	err := svc.RemoveClassFromContent(context.Background(), uuid.New(), classID, uuid.New(), false)

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi loi uy quyen", err)
	}
}

// TestCLCBulkAssignClassesToContent_NguoiLa_Bi403: duong gang hang loat. Day la duong DE BI BO
// QUA nhat khi va loi — req.ClassIDs rong nghia la "gan cho MOI lop cua khoa", va truoc day
// khong co cong kiem quyen nao tren nhanh do (AssignClassToContent khong duoc goi tu day).
func TestCLCBulkAssignClassesToContent_NguoiLa_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}

	_, err := svc.BulkAssignClassesToContent(context.Background(), uuid.New(), uuid.New(), false, dto.BulkAssignClassesToContentDTO{ClassIDs: []uuid.UUID{classID}})

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi loi uy quyen", err)
	}
	if clcRepo.createBatchCall != 0 {
		t.Error("CreateBatch BI GOI du nguoi goi khong co quyen tren bat ky lop nao")
	}
}

// --- DOC: get classes for content / content schedule for class --------------------------------

// TestCLCGetContentScheduleForClass_NguoiLa_Bi403: thoi khoa bieu cua lop la du lieu noi bo cua
// lop — nguoi ngoai khong duoc doc.
func TestCLCGetContentScheduleForClass_NguoiLa_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, _ := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}

	_, err := svc.GetContentScheduleForClass(context.Background(), classID, uuid.New(), false, 1, 20)

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi loi uy quyen (ErrNotClassMember)", err)
	}
}

// TestCLCGetContentScheduleForClass_HocSinhCuaLop_ChoPhep: hoc sinh cua lop PHAI doc duoc thoi
// khoa bieu lop minh — neu khong, ban va quyen se chan luon ca nguoi dung hop le.
func TestCLCGetContentScheduleForClass_HocSinhCuaLop_ChoPhep(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, _ := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isStudent = true

	resp, err := svc.GetContentScheduleForClass(context.Background(), classID, uuid.New(), false, 1, 20)

	if err != nil {
		t.Fatalf("hoc sinh cua lop phai doc duoc, nhung loi: %v", err)
	}
	if resp == nil {
		t.Fatal("response nil du khong co loi")
	}
}

// TestCLCGetContentScheduleForClass_GiaoVienLop_ChoPhep: giao vien cua lop doc duoc (khong can la
// hoc sinh).
func TestCLCGetContentScheduleForClass_GiaoVienLop_ChoPhep(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, _ := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isTeacher = true

	if _, err := svc.GetContentScheduleForClass(context.Background(), classID, uuid.New(), false, 1, 20); err != nil {
		t.Fatalf("giao vien lop phai doc duoc, nhung loi: %v", err)
	}
}

// TestCLCGetContentScheduleForClass_Admin_ChoPhep: admin doc duoc moi lop.
func TestCLCGetContentScheduleForClass_Admin_ChoPhep(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, _ := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}

	if _, err := svc.GetContentScheduleForClass(context.Background(), classID, uuid.New(), true, 1, 20); err != nil {
		t.Fatalf("admin phai doc duoc, nhung loi: %v", err)
	}
	if classRepo.teacherCalls != 0 || classRepo.studentCalls != 0 {
		t.Error("admin van phai tra cuu quan he lop — nhanh isAdmin phai duoc xet truoc")
	}
}

// TestCLCGetClassesForContent_NguoiLa_Bi403: danh sach lop duoc gan vao mot lesson content — nguoi
// khong thuoc lop nao trong so do khong duoc xem.
func TestCLCGetClassesForContent_NguoiLa_Bi403(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	clcRepo.items = []model.ClassLessonContent{{ID: uuid.New(), ClassID: classID}}

	_, err := svc.GetClassesForContent(context.Background(), uuid.New(), uuid.New(), false, 1, 20)

	if !IsForbiddenErr(err) {
		t.Errorf("loi = %v, mong doi loi uy quyen (ErrNotClassMember)", err)
	}
}

// TestCLCGetClassesForContent_HocSinhCuaMotTrongCacLop_ChoPhep: chi can la thanh vien cua IT NHAT
// MOT lop trong trang la doc duoc (ngu nghia "any-of" cua ensureAnyClassView).
func TestCLCGetClassesForContent_HocSinhCuaMotTrongCacLop_ChoPhep(t *testing.T) {
	courseID := uuid.New()
	classID := uuid.New()

	svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
	classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID}
	classRepo.isStudent = true
	clcRepo.items = []model.ClassLessonContent{{ID: uuid.New(), ClassID: classID}}

	if _, err := svc.GetClassesForContent(context.Background(), uuid.New(), uuid.New(), false, 1, 20); err != nil {
		t.Fatalf("hoc sinh cua lop phai doc duoc, nhung loi: %v", err)
	}
}
