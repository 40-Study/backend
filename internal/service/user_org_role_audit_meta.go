package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// OrgMemberAuditMeta (QA4-A02, contract 6): metadata cho dòng nhật ký gán / thu hồi vai trò tổ chức, để
// trang nhật ký hiện được "ai, tổ chức nào, vai trò nào" thay vì `user: <uuid>`.
// Khoá: organization_id, organization_name, role_code, role_name, target_user_id, target_user_name.
// Vai trò tổ chức không có cột mã riêng nên role_code là id vai trò (định danh ổn định), role_name là tên
// hiển thị; gán nhiều vai trò một lần thì các giá trị nối bằng ", " theo thứ tự yêu cầu.
func (s *UserOrganizationRoleService) OrgMemberAuditMeta(ctx context.Context, targetUserID, organizationID uuid.UUID, roleIDs []uuid.UUID) (map[string]any, error) {
	user, err := s.userRepo.FindUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}
	org, err := s.orgRepo.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}
	roles, err := s.roleRepo.GetRoleByIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	nameByID := make(map[uuid.UUID]string, len(roles))
	for _, r := range roles {
		nameByID[r.ID] = r.Name
	}
	codes, names := make([]string, 0, len(roleIDs)), make([]string, 0, len(roleIDs))
	for _, id := range roleIDs {
		codes = append(codes, id.String())
		names = append(names, nameByID[id])
	}
	userName := user.UserName
	if user.FullName != nil && *user.FullName != "" {
		userName = *user.FullName
	}
	return map[string]any{
		"organization_id":   org.ID.String(),
		"organization_name": org.Name,
		"role_code":         strings.Join(codes, ", "),
		"role_name":         strings.Join(names, ", "),
		"target_user_id":    user.ID.String(),
		"target_user_name":  userName,
	}, nil
}

// OrgAssignmentAuditMeta — như OrgMemberAuditMeta nhưng từ một dòng gán vai trò (DELETE theo id dòng gán).
func (s *UserOrganizationRoleService) OrgAssignmentAuditMeta(ctx context.Context, assignmentID uuid.UUID) (map[string]any, error) {
	a, err := s.repo.FindByID(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("organization role assignment not found")
	}
	return s.OrgMemberAuditMeta(ctx, a.UserID, a.OrganizationID, []uuid.UUID{a.RoleID})
}
