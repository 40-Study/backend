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

// classTeacherOrInstructor tra ve true khi userID la giao vien cua lop HOAC instructor cua khoa
// chua lop. `class` duoc truyen vao (da load san) vi moi caller deu can no cho muc dich khac nua.
func classTeacherOrInstructor(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID uuid.UUID, class *model.Class) (bool, error) {
	if class == nil {
		return false, errors.New("class not found")
	}
	isTeacher, err := classRepo.TeacherClassExists(ctx, class.ID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to verify class teacher: %w", err)
	}
	if isTeacher {
		return true, nil
	}
	if class.CourseID != nil {
		course, err := courseRepo.GetByID(ctx, *class.CourseID)
		if err != nil {
			return false, fmt.Errorf("failed to load course: %w", err)
		}
		if course != nil && course.InstructorID == userID {
			return true, nil
		}
	}
	return false, nil
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
