package service

import (
	"context"

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
}

type classOrgAccess struct {
	classRepo repository.ClassRepositoryInterface
	authz     ClassAuthorizer
}

// NewClassOrgAccess dùng lại orgManagesClass (class_access.go) để một định nghĩa "chủ tổ chức của lớp" duy nhất.
func NewClassOrgAccess(classRepo repository.ClassRepositoryInterface, authz ClassAuthorizer) *classOrgAccess {
	return &classOrgAccess{classRepo: classRepo, authz: authz}
}

func (a *classOrgAccess) OrgManagesClass(ctx context.Context, classID, userID uuid.UUID) (bool, error) {
	class, err := a.classRepo.GetByID(ctx, classID)
	if err != nil || class == nil {
		// Lớp không đọc được thì không có căn cứ cấp quyền: từ chối (fail-closed), không báo lỗi hạ tầng giả.
		return false, nil
	}
	return orgManagesClass(ctx, a.classRepo, a.authz, userID, class)
}

// orgManagesAssignmentClass: assignment thuộc một lớp mà userID quản trị qua tổ chức. Không có checker/lớp thì false.
func orgManagesAssignmentClass(ctx context.Context, access assignmentOrgAccess, assignment *model.Assignment, userID uuid.UUID) (bool, error) {
	if access == nil || assignment == nil || assignment.ClassID == nil {
		return false, nil
	}
	return access.OrgManagesClass(ctx, *assignment.ClassID, userID)
}
