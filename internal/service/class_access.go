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

// ErrTeacherNotOrgMember (W2-A, quyết định 04/10): gán vào lớp của một tổ chức một giảng viên chưa là thành viên
// active của tổ chức đó. Admin hệ thống không bị giới hạn. Handler trả 400 kèm code TEACHER_NOT_ORG_MEMBER.
var ErrTeacherNotOrgMember = errors.New("teacher is not an active member of this class's organization")

// ErrClassDeleteAdminOnly (W2-A, quyết định 04/10): xoá lớp bị từ chối. Xoá vĩnh viễn (hard_delete) chỉ admin hệ
// thống; người quản lý lớp chỉ nhờ vai tổ chức (chủ/quản trị tổ chức) chỉ được lưu trữ lớp, không xoá. Handler
// trả 403 kèm code CLASS_DELETE_ADMIN_ONLY.
var ErrClassDeleteAdminOnly = errors.New("forbidden: only a system admin can delete a class; archive it instead")

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

// orgClassManagePermission: quyền org role phải có để được coi là chủ/quản trị tổ chức khi chấm điểm lớp
// của tổ chức. ORG_MEMBERS_MANAGE là quyền quản lý thành viên tổ chức, thuộc SSOT quyền phạm vi tổ chức
// (data.IsOrgPermission, kiểm trong test) nên bộ lọc S6 ở PermissionChecker không loại nó.
const orgClassManagePermission = "ORG_MEMBERS_MANAGE"

// ClassAuthorizer: phần PermissionChecker mà tầng service cần (interface ở đây để service không import
// middleware). Nil = không ai được nâng quyền (fail-closed, như isAdminActor khi permChecker nil).
type ClassAuthorizer interface {
	IsSystemAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
	HasOrgRolePermission(ctx context.Context, userID, orgID uuid.UUID, permission string) (bool, error)
}

// orgManagesClass: lớp thuộc một tổ chức (classes.organization_id) và userID có vai quản trị
// (orgClassManagePermission) TRONG CHÍNH tổ chức đó. Lớp cá nhân (organization_id NULL) không bao giờ thuộc
// quyền của chủ tổ chức nào: thêm một giảng viên vào tổ chức không kéo lớp cá nhân của họ vào phạm vi.
//
// Tổ chức đã bị xoá MỀM thì không còn ai là chủ của nó: lớp vẫn giữ organization_id (FK SET NULL chỉ chạy khi xoá
// cứng) và user_organization_roles không bị gỡ, nên phải kiểm deleted_at tường minh ở đây.
func orgManagesClass(ctx context.Context, classRepo repository.ClassRepositoryInterface, authz ClassAuthorizer, userID uuid.UUID, class *model.Class) (bool, error) {
	if authz == nil || class == nil || class.OrganizationID == nil {
		return false, nil
	}
	live, err := classRepo.OrganizationExists(ctx, *class.OrganizationID)
	if err != nil {
		return false, fmt.Errorf("failed to verify organization: %w", err)
	}
	if !live {
		return false, nil
	}
	ok, err := authz.HasOrgRolePermission(ctx, userID, *class.OrganizationID, orgClassManagePermission)
	if err != nil {
		return false, fmt.Errorf("failed to verify organization role: %w", err)
	}
	return ok, nil
}

// classAccessAsAdmin (B-05/B-12): chủ/quản trị tổ chức của lớp được đối xử như admin TRÊN LỚP ĐÓ cho các hàm
// ensureClass* (xem, điểm danh, buổi học, kích hoạt lớp, gán/gỡ giảng viên, ghi danh học viên). Trả
// `isAdmin || orgManagesClass`, tức dùng lại đúng orgManagesClass (một luật duy nhất như chấm điểm), không
// viết luật song song. Chỉ dùng kết quả này cho kiểm QUYỀN TRUY CẬP lớp; những chỗ "isAdmin" mang nghĩa admin hệ
// thống thật sự (đổi khoá của lớp, tạo lớp vào khoá người khác) phải giữ nguyên cờ gốc, nếu không chủ tổ chức
// sẽ kéo được lớp sang khoá bất kỳ. authz nil (test, môi trường không có PermissionChecker) = không nâng quyền.
func classAccessAsAdmin(ctx context.Context, classRepo repository.ClassRepositoryInterface, authz ClassAuthorizer, userID, classID uuid.UUID, isAdmin bool) (bool, error) {
	if isAdmin || authz == nil {
		return isAdmin, nil
	}
	class, err := classRepo.GetByID(ctx, classID)
	if err != nil {
		return false, fmt.Errorf("failed to load class: %w", err)
	}
	return orgManagesClass(ctx, classRepo, authz, userID, class)
}

// ensureClassGrade: quyền CHẤM ĐIỂM / quản bảng điểm của lớp = ensureClassManage (giảng viên lớp, người
// tạo lớp, instructor khoá, admin hệ thống) HOẶC chủ/quản trị của tổ chức mà lớp thuộc về. Lỗi theo
// "xem được hay không": người không xem được lớp -> ErrClassNotFound (404, không dò được id lớp); người
// xem được (học viên trong lớp) nhưng không được chấm -> ErrNotClassTeacher (403).
func ensureClassGrade(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, authz ClassAuthorizer, userID, classID uuid.UUID) error {
	isAdmin := false
	if authz != nil {
		var err error
		if isAdmin, err = authz.IsSystemAdmin(ctx, userID); err != nil {
			return fmt.Errorf("failed to verify system admin: %w", err)
		}
	}
	err := ensureClassManage(ctx, classRepo, courseRepo, userID, classID, isAdmin)
	if err == nil || !errors.Is(err, ErrNotClassTeacher) {
		return err
	}
	class, err := classRepo.GetByID(ctx, classID)
	if err != nil {
		return fmt.Errorf("failed to load class: %w", err)
	}
	managed, err := orgManagesClass(ctx, classRepo, authz, userID, class)
	if err != nil {
		return err
	}
	if managed {
		return nil
	}
	if err := ensureClassView(ctx, classRepo, courseRepo, userID, classID, false); err != nil {
		if errors.Is(err, ErrNotClassMember) {
			return ErrClassNotFound
		}
		return err
	}
	return ErrNotClassTeacher
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

// ensureClassVisible (S4): nhu ensureClassView nhung nguoi ngoai nhan ErrClassNotFound (404) thay vi
// ErrNotClassMember (403), de khong do duoc lop nao ton tai. Dung cho doc chi tiet lop va danh sach hoc vien:
// chi thanh vien lop (hoc vien, giang vien), nguoi quan tri lop (chu khoa, nguoi tao) va admin.
func ensureClassVisible(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, isAdmin bool) error {
	err := ensureClassView(ctx, classRepo, courseRepo, userID, classID, isAdmin)
	if errors.Is(err, ErrNotClassMember) {
		return ErrClassNotFound
	}
	return err
}

// ensureClassManageWrite: luật "không xem được thì 404, xem được mà không có quyền thì 403" cho route GHI vào lớp
// (sửa, kích hoạt, ghi danh, điểm danh...). ensureClassManage thuần trả ErrNotClassTeacher (403) cho cả người
// ngoài lớp, nên người lạ dò được lớp nào tồn tại qua route ghi; ở đây người không xem được nhận ErrClassNotFound,
// còn người xem được (học viên trong lớp) vẫn nhận ErrNotClassTeacher. isAdmin đã gồm nâng quyền chủ tổ chức; kiểm
// "xem được" cố ý KHÔNG nâng, vì chủ tổ chức đã qua ensureClassManage trước khi tới đây.
func ensureClassManageWrite(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, isAdmin bool) error {
	err := ensureClassManage(ctx, classRepo, courseRepo, userID, classID, isAdmin)
	if !errors.Is(err, ErrNotClassTeacher) {
		return err
	}
	return ensureClassVisibleOr(ctx, classRepo, courseRepo, userID, classID, ErrNotClassTeacher)
}

// ensureClassVisibleOr: nếu người gọi xem được lớp thì trả `denied` (lỗi 403 của route), không xem được thì
// ErrClassNotFound (404). Một chỗ duy nhất quyết định "xem được thì 403, không thì 404".
func ensureClassVisibleOr(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, denied error) error {
	if err := ensureClassVisible(ctx, classRepo, courseRepo, userID, classID, false); err != nil {
		return err
	}
	return denied
}

// ensureClassManageOrNotFound (S4): nhu ensureClassManage nhung nguoi khong quan tri duoc lop nhan
// ErrClassNotFound (404). Dung cho route DOC danh sach quan ly (diem danh): hoc vien khong duoc thay route
// nay ton tai, khac voi route GHI tra 403 (ensureClassManage).
func ensureClassManageOrNotFound(ctx context.Context, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, userID, classID uuid.UUID, isAdmin bool) error {
	err := ensureClassManage(ctx, classRepo, courseRepo, userID, classID, isAdmin)
	if errors.Is(err, ErrNotClassTeacher) {
		return ErrClassNotFound
	}
	return err
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
