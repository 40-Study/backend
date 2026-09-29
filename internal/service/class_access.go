package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// class_access.go (V3-6/V3-7, issue #58) — NGUON SU THAT DUY NHAT cho cau hoi "user nay co quan
// he gi voi lop nay khong". Truoc day cau hoi nay duoc tra loi bang mot phep kiem copy trong
// LivestreamService.Create (fix N1); khi mo rong kiem quyen sang nhom handler
// /lesson-contents/:id/classes (finding V3-7) thi copy thu hai se xuat hien — nen lop quyen duoc
// tach ra day va ca hai service cung goi.
//
// Cac ham o day la package-level (khong phai method) vi ca LivestreamService lan
// ClassLessonContentService deu can chung, trong khi hai service khong chia se mot base type.
// Moi ham deu fail-closed: loi doc du lieu tra ve loi, khong bao gio thanh "cho phep".

// ErrNotClassMember: nguoi goi khong phai giao vien lop/instructor khoa, cung khong phai hoc
// sinh cua lop — dung cho cac handler DOC (hoi vien lop/giao vien). La loi UY QUYEN (403).
var ErrNotClassMember = errors.New("forbidden: not a member of this class")

// ErrClassNotFound (S4): lop khong ton tai HOAC nguoi goi khong xem duoc lop do — handler tra 404,
// khong phan biet hai truong hop de khong do duoc id lop.
var ErrClassNotFound = errors.New("class not found")

// ErrNotClassOwner (S4): nguoi goi thay duoc lop nhung khong phai CHU lop (nguoi tao lop hoac giang
// vien chu khoa) khi gan/go giang vien hoac doi khoa cua lop. Handler tra 403.
var ErrNotClassOwner = errors.New("forbidden: only the class owner or an admin can assign or remove teachers or change the course")

// ErrNotCourseInstructor (S4): tao lop vao khoa, hoac doi lop sang khoa, ma nguoi goi khong phai
// giang vien chu khoa do (va khong phai admin). Handler tra 403.
var ErrNotCourseInstructor = errors.New("forbidden: only the course instructor or an admin can put a class in this course")

// classOwner (S4): CHU lop = nguoi tao lop (classes.created_by) HOAC giang vien chu khoa chua lop.
// Hep hon classTeacherOrInstructor: giang vien duoc gan vao lop (teacher_classes) quan tri lop nhung
// KHONG phai chu, nen khong tu gan them giang vien khac — neu khong, moi giang vien deu tu gan minh
// vao lop cua nguoi khac roi co quyen CanManage tren assignment cua lop do.
func classOwner(ctx context.Context, courseRepo repository.CourseRepositoryInterface, userID uuid.UUID, class *model.Class) (bool, error) {
	if class == nil {
		return false, ErrClassNotFound
	}
	if class.CreatedBy != nil && *class.CreatedBy == userID {
		return true, nil
	}
	return courseInstructorOf(ctx, courseRepo, userID, class.CourseID)
}

// courseInstructorOf: userID co phai instructor cua khoa (nil courseID = khong co khoa = false).
func courseInstructorOf(ctx context.Context, courseRepo repository.CourseRepositoryInterface, userID uuid.UUID, courseID *uuid.UUID) (bool, error) {
	if courseID == nil {
		return false, nil
	}
	course, err := courseRepo.GetByID(ctx, *courseID)
	if err != nil {
		return false, fmt.Errorf("failed to load course: %w", err)
	}
	return course != nil && course.InstructorID == userID, nil
}

// ensureClassOwner (S4): admin, hoac chu lop. Nguoi khong phai chu: da thay duoc lop (giang vien lop,
// hoc sinh dang hoc, instructor) -> ErrNotClassOwner (403); khong thay duoc -> ErrClassNotFound (404).
func ensureClassOwner(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, isAdmin bool) error {
	class, err := classRepo.GetByID(ctx, classID)
	if err != nil {
		return fmt.Errorf("failed to load class: %w", err)
	}
	if class == nil {
		return ErrClassNotFound
	}
	if isAdmin {
		return nil
	}
	owner, err := classOwner(ctx, courseRepo, userID, class)
	if err != nil {
		return err
	}
	if owner {
		return nil
	}
	related, err := classRepo.IsUserRelatedToClass(ctx, class.ID, userID)
	if err != nil {
		return fmt.Errorf("failed to verify class membership: %w", err)
	}
	if related {
		return ErrNotClassOwner
	}
	return ErrClassNotFound
}

// classTeacherOrInstructor tra ve true khi userID la giao vien cua lop, instructor cua khoa chua
// lop, HOAC nguoi tao lop (S4: nguoi tao chua duoc gan giang vien van phai quan tri duoc lop minh
// vua tao). `class` duoc truyen vao (da load san) vi moi caller deu can no cho muc dich khac nua.
func classTeacherOrInstructor(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID uuid.UUID, class *model.Class) (bool, error) {
	if class == nil {
		return false, ErrClassNotFound
	}
	if class.CreatedBy != nil && *class.CreatedBy == userID {
		return true, nil
	}
	isTeacher, err := classRepo.TeacherClassExists(ctx, class.ID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to verify class teacher: %w", err)
	}
	if isTeacher {
		return true, nil
	}
	return courseInstructorOf(ctx, courseRepo, userID, class.CourseID)
}

// ensureClassManage = giao vien lop, HOAC instructor cua khoa chua lop, HOAC admin he thong.
// isAdmin duoc tinh san o tang handler (middleware.PermissionChecker) roi truyen xuong — cung
// pattern voi ClassService.requireClassTeacherOrAdmin.
func ensureClassManage(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	class, err := classRepo.GetByID(ctx, classID)
	if err != nil {
		return fmt.Errorf("failed to load class: %w", err)
	}
	ok, err := classTeacherOrInstructor(ctx, classRepo, courseRepo, userID, class)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotClassTeacher
	}
	return nil
}

// ensureClassView = nguoi quan tri duoc lop (ensureClassManage) HOAC hoc sinh dang hoc trong lop.
// Dung cho cac handler DOC chi tiet mot lop: danh sach hoc sinh, thoi khoa bieu cua lop — truoc
// day bat ky user dang nhap nao cung doc duoc (ro ri ten/email hoc sinh va lich hoc).
func ensureClassView(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	class, err := classRepo.GetByID(ctx, classID)
	if err != nil {
		return fmt.Errorf("failed to load class: %w", err)
	}
	ok, err := classTeacherOrInstructor(ctx, classRepo, courseRepo, userID, class)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	isStudent, err := classRepo.StudentClassExists(ctx, class.ID, userID)
	if err != nil {
		return fmt.Errorf("failed to verify class membership: %w", err)
	}
	if !isStudent {
		return ErrNotClassMember
	}
	return nil
}

// ensureAnyClassView dung cho cac handler doc TRA VE NHIEU LOP cung luc (vd: cac lop duoc gan vao
// mot lesson content). Quyen duoc xet tren dung tap lop sap tra ve, va dung ngay khi mot lop cho
// qua — nen so truy van bi chan tren so lop cua trang, khong phai toan bo khoa hoc.
func ensureAnyClassView(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID uuid.UUID, isAdmin bool, classIDs []uuid.UUID) error {
	if isAdmin {
		return nil
	}
	if len(classIDs) == 0 {
		// Khong co lop nao de kiem: khong co gi bi ro ri, va day thuong la trang rong.
		return nil
	}
	for _, classID := range classIDs {
		class, err := classRepo.GetByID(ctx, classID)
		if err != nil {
			return fmt.Errorf("failed to load class: %w", err)
		}
		ok, err := classTeacherOrInstructor(ctx, classRepo, courseRepo, userID, class)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		isStudent, err := classRepo.StudentClassExists(ctx, classID, userID)
		if err != nil {
			return fmt.Errorf("failed to verify class membership: %w", err)
		}
		if isStudent {
			return nil
		}
	}
	return ErrNotClassMember
}
