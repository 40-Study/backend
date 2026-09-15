package service

// Test cho N1 (review vong 2, 260915): POST /api/livestream truoc day chi co AuthMiddleware,
// khong kiem hostID co day/so huu class_id hay khong — bat ky user dang nhap nao (ke ca hoc
// sinh) tao duoc phien live gan vao mot lop bat ky, va khi co scheduled_at con enqueue reminder
// ban thong bao toi ca lop do. Kiem quyen: hostID phai la giao vien cua class_id
// (TeacherClassExists) HOAC la instructor cua khoa hoc chua lop do.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeClassRepoAuthz: GetByID tra ve class co san (voi CourseID tuy chon), TeacherClassExists tra
// ve dung gia tri configure — dung de dung lap ca 2 nhanh cua dieu kien uy quyen.
type fakeClassRepoAuthz struct {
	repository.ClassRepositoryInterface
	class          *model.Class
	isTeacher      bool
	teacherCalls   int
	getByIDErr     error
	teacherErr     error
}

func (f *fakeClassRepoAuthz) GetByID(ctx context.Context, id uuid.UUID) (*model.Class, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	return f.class, nil
}

func (f *fakeClassRepoAuthz) TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error) {
	f.teacherCalls++
	if f.teacherErr != nil {
		return false, f.teacherErr
	}
	return f.isTeacher, nil
}

// fakeCourseRepoAuthz: GetByID tra ve course co InstructorID configure san.
type fakeCourseRepoAuthz struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseRepoAuthz) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

func newLivestreamServiceForAuthz(classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, repo repository.LivestreamRepositoryInterface) *LivestreamService {
	return NewLivestreamService(repo, nil, &fakeAnalyticsRepoHostTest{}, classRepo, courseRepo, nil, nil, nil, nil, nil)
}

// TestLivestreamCreate_HocSinhTaoPhienChoLopKhongDay_BiTuChoi (kich ban chinh cua N1): user
// khong phai giao vien lop, khong phai instructor khoa hoc -> 403 (ErrNotClassTeacher), VA khong
// co ban ghi nao duoc tao (repo.Create khong duoc goi) -> chung minh khong co enqueue reminder
// nao xay ra sau do (enqueue nam SAU buoc repo.Create trong code, xem livestream_service.go).
func TestLivestreamCreate_HocSinhTaoPhienChoLopKhongDay_BiTuChoi(t *testing.T) {
	classID := uuid.New()
	studentID := uuid.New()

	classRepo := &fakeClassRepoAuthz{
		class:     &model.Class{BaseModel: model.BaseModel{ID: classID}},
		isTeacher: false,
	}
	repo := &fakeLivestreamRepoHostTest{}
	svc := newLivestreamServiceForAuthz(classRepo, &fakeCourseRepoAuthz{}, repo)

	req := dto.CreateLivestreamDTO{
		Title:       "Buoi hoc gia mao",
		ClassID:     classID.String(),
		ScheduledAt: "2026-10-01T10:00:00Z", // co lich -> neu tao duoc se enqueue reminder
	}

	session, err := svc.Create(context.Background(), studentID, req)
	if err == nil {
		t.Fatal("mong doi loi, nhung Create thanh cong")
	}
	if session != nil {
		t.Error("session phai la nil khi bi tu choi")
	}
	if !errorIsNotClassTeacher(err) {
		t.Errorf("loi = %v, mong doi ErrNotClassTeacher", err)
	}
	if repo.created != nil {
		t.Error("repo.Create BI GOI du khong co quyen — nghia la enqueue reminder cung se chay, ban thong bao toi ca lop nguoi khac")
	}
	if classRepo.teacherCalls != 1 {
		t.Errorf("TeacherClassExists duoc goi %d lan, mong doi 1", classRepo.teacherCalls)
	}
}

// TestLivestreamCreate_GiaoVienCuaLop_ChoPhep: giao vien that su duoc gan vao lop (teacher_classes)
// phai tao duoc phien binh thuong.
func TestLivestreamCreate_GiaoVienCuaLop_ChoPhep(t *testing.T) {
	classID := uuid.New()
	teacherID := uuid.New()

	classRepo := &fakeClassRepoAuthz{
		class:     &model.Class{BaseModel: model.BaseModel{ID: classID}},
		isTeacher: true,
	}
	repo := &fakeLivestreamRepoHostTest{}
	svc := newLivestreamServiceForAuthz(classRepo, &fakeCourseRepoAuthz{}, repo)

	req := dto.CreateLivestreamDTO{Title: "Buoi hoc that", ClassID: classID.String()}

	session, err := svc.Create(context.Background(), teacherID, req)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if session.HostID != teacherID {
		t.Errorf("HostID = %s, mong doi %s", session.HostID, teacherID)
	}
	if repo.created == nil {
		t.Fatal("repo.Create khong duoc goi du giao vien co quyen hop le")
	}
}

// TestLivestreamCreate_InstructorCuaKhoaHoc_ChoPhep: user khong nam trong teacher_classes cua
// lop nay, nhung la instructor cua khoa hoc chua lop do — vong review de xuat nhanh nay nhu mot
// "hoac", khong chi rieng teacher_classes.
func TestLivestreamCreate_InstructorCuaKhoaHoc_ChoPhep(t *testing.T) {
	classID := uuid.New()
	courseID := uuid.New()
	instructorID := uuid.New()

	classRepo := &fakeClassRepoAuthz{
		class:     &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID},
		isTeacher: false, // khong co trong teacher_classes cua LOP nay
	}
	courseRepo := &fakeCourseRepoAuthz{
		course: &model.Course{BaseModel: model.BaseModel{ID: courseID}, InstructorID: instructorID},
	}
	repo := &fakeLivestreamRepoHostTest{}
	svc := newLivestreamServiceForAuthz(classRepo, courseRepo, repo)

	req := dto.CreateLivestreamDTO{Title: "Buoi hoc cua khoa", ClassID: classID.String()}

	session, err := svc.Create(context.Background(), instructorID, req)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if session.HostID != instructorID {
		t.Errorf("HostID = %s, mong doi %s", session.HostID, instructorID)
	}
}

// TestLivestreamCreate_KhongPhaiInstructorKhoaHoc_TuChoi: mot instructor cua MOT khoa hoc KHAC
// (khong phai khoa hoc chua lop nay) van bi tu choi — chan hard-code "instructorID != uuid.Nil
// thi cho qua" thay vi so sanh dung ID.
func TestLivestreamCreate_KhongPhaiInstructorKhoaHoc_TuChoi(t *testing.T) {
	classID := uuid.New()
	courseID := uuid.New()
	realInstructor := uuid.New()
	someOtherInstructor := uuid.New()

	classRepo := &fakeClassRepoAuthz{
		class:     &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID},
		isTeacher: false,
	}
	courseRepo := &fakeCourseRepoAuthz{
		course: &model.Course{BaseModel: model.BaseModel{ID: courseID}, InstructorID: realInstructor},
	}
	repo := &fakeLivestreamRepoHostTest{}
	svc := newLivestreamServiceForAuthz(classRepo, courseRepo, repo)

	req := dto.CreateLivestreamDTO{Title: "Buoi hoc gia mao", ClassID: classID.String()}

	_, err := svc.Create(context.Background(), someOtherInstructor, req)
	if !errorIsNotClassTeacher(err) {
		t.Errorf("loi = %v, mong doi ErrNotClassTeacher", err)
	}
	if repo.created != nil {
		t.Error("repo.Create khong duoc phep goi")
	}
}

func errorIsNotClassTeacher(err error) bool {
	return err == ErrNotClassTeacher
}
