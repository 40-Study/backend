package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// assignmentOrgAccess (R4): chủ/quản trị tổ chức của lớp được XEM bài tập và bài nộp của lớp thuộc tổ chức mình
// để chấm điểm. Quyền chấm (ensureClassGrade) đã có nhánh tổ chức từ trước nhưng hai bước đứng trước nó trong
// màn chấm là đọc bài tập (CanView) và liệt kê bài nộp (canManageAssignment) thì không, nên chủ tổ chức chấm
// được qua API mà không mở nổi màn chấm. CHỈ cho đọc: sửa/publish/test case vẫn theo CanManage (giảng viên).
type assignmentOrgAccess interface {
	OrgManagesClass(ctx context.Context, classID, userID uuid.UUID) (bool, error)
	ClassAudience(ctx context.Context, classID, userID uuid.UUID, isAdmin bool) (ClassAudience, error)
}

// ClassAudience: người gọi là ai đối với một lớp, để quyết định xem được bài tập nào.
type ClassAudience int

const (
	// ClassAudienceNone: không xem được lớp (lớp không tồn tại cũng vậy) -> 404.
	ClassAudienceNone ClassAudience = iota
	// ClassAudienceStudent: học viên đang học lớp, chỉ thấy bài đã công bố.
	ClassAudienceStudent
	// ClassAudienceManager: admin, giảng viên/chủ khoá/người tạo lớp, hoặc chủ/quản trị tổ chức của lớp: thấy cả bản nháp.
	ClassAudienceManager
)

type classOrgAccess struct {
	classRepo  repository.ClassRepositoryInterface
	courseRepo repository.CourseRepositoryInterface
	authz      ClassAuthorizer
}

// NewClassOrgAccess dùng lại orgManagesClass, classTeacherOrInstructor (class_access.go) để mỗi khái niệm
// "quản lý lớp" chỉ có MỘT định nghĩa; không thêm luật tổ chức mới.
func NewClassOrgAccess(classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, authz ClassAuthorizer) *classOrgAccess {
	return &classOrgAccess{classRepo: classRepo, courseRepo: courseRepo, authz: authz}
}

func (a *classOrgAccess) OrgManagesClass(ctx context.Context, classID, userID uuid.UUID) (bool, error) {
	class, err := a.classRepo.GetByID(ctx, classID)
	if err != nil || class == nil {
		// Lớp không đọc được thì không có căn cứ cấp quyền: từ chối (fail-closed), không báo lỗi hạ tầng giả.
		return false, nil
	}
	return orgManagesClass(ctx, a.classRepo, a.authz, userID, class)
}

// EnsureClassWritable (W3-BE): lớp đã lưu trữ -> ErrClassArchived; dùng chung ensureClassWritable với các service khác.
func (a *classOrgAccess) EnsureClassWritable(ctx context.Context, classID uuid.UUID) error {
	return ensureClassWritable(ctx, a.classRepo, classID)
}

// ClassAudience: admin -> quản lý; giảng viên lớp/chủ khoá/người tạo -> quản lý; chủ/quản trị tổ chức của lớp
// -> quản lý; học viên đang học -> học viên; còn lại (kể cả lớp không tồn tại) -> không.
func (a *classOrgAccess) ClassAudience(ctx context.Context, classID, userID uuid.UUID, isAdmin bool) (ClassAudience, error) {
	class, err := a.classRepo.GetByID(ctx, classID)
	if err != nil {
		return ClassAudienceNone, fmt.Errorf("failed to load class: %w", err)
	}
	if class == nil {
		return ClassAudienceNone, nil
	}
	if isAdmin {
		return ClassAudienceManager, nil
	}
	teacher, err := classTeacherOrInstructor(ctx, a.classRepo, a.courseRepo, userID, class)
	if err != nil {
		return ClassAudienceNone, err
	}
	if teacher {
		return ClassAudienceManager, nil
	}
	org, err := orgManagesClass(ctx, a.classRepo, a.authz, userID, class)
	if err != nil {
		return ClassAudienceNone, err
	}
	if org {
		return ClassAudienceManager, nil
	}
	student, err := a.classRepo.StudentClassExists(ctx, class.ID, userID)
	if err != nil {
		return ClassAudienceNone, fmt.Errorf("failed to verify class membership: %w", err)
	}
	if student {
		return ClassAudienceStudent, nil
	}
	return ClassAudienceNone, nil
}

// orgManagesAssignmentClass: assignment thuộc một lớp mà userID quản trị qua tổ chức. Không có checker/lớp thì false.
func orgManagesAssignmentClass(ctx context.Context, access assignmentOrgAccess, assignment *model.Assignment, userID uuid.UUID) (bool, error) {
	if access == nil || assignment == nil || assignment.ClassID == nil {
		return false, nil
	}
	return access.OrgManagesClass(ctx, *assignment.ClassID, userID)
}
