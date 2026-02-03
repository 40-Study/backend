package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type UserOrganizationRoleServiceInterface interface {
	// Các thao tác gán quyền
	AssignOrgRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignOrgRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error)

	// Các thao tác truy vấn
	GetUserOrgRoleByID(ctx context.Context, id uuid.UUID) (*dto.UserOrgRoleResponseDTO, error)
	GetUserOrgRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error)
	GetUserOrgRolesInOrganization(ctx context.Context, userID, organizationID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error)
	GetUsersWithOrgRole(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithOrgRoleResponseDTO, error)
	GetOrganizationMembers(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error)
	CheckUserHasOrgRole(ctx context.Context, userID, roleID, organizationID uuid.UUID) (bool, error)

	// Các thao tác cập nhật trạng thái
	UpdateUserOrgRoleStatus(ctx context.Context, id uuid.UUID, req dto.UpdateUserOrgRoleStatusDTO, updatedBy uuid.UUID) (*dto.UserOrgRoleResponseDTO, error)

	// Các thao tác xóa
	RemoveOrgRoleFromUser(ctx context.Context, id uuid.UUID) error
	RemoveAllOrgRolesFromUser(ctx context.Context, userID uuid.UUID) error
	RemoveUserFromOrganization(ctx context.Context, userID, organizationID uuid.UUID) error
}

type UserOrganizationRoleService struct {
	repo     repository.UserOrganizationRoleRepositoryInterface
	userRepo repository.UserRepositoryInterface
	roleRepo repository.RoleRepositoryInterface
	orgRepo  repository.OrganizationRepositoryInterface
}

func NewUserOrganizationRoleService(
	repo repository.UserOrganizationRoleRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	roleRepo repository.RoleRepositoryInterface,
	orgRepo repository.OrganizationRepositoryInterface,
) *UserOrganizationRoleService {
	return &UserOrganizationRoleService{
		repo:     repo,
		userRepo: userRepo,
		roleRepo: roleRepo,
		orgRepo:  orgRepo,
	}
}

// ============ Các Thao Tác Gán Quyền ============

func (s *UserOrganizationRoleService) AssignOrgRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignOrgRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error) {
	// Kiểm tra người dùng tồn tại
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Kiểm tra tổ chức tồn tại
	org, err := s.orgRepo.GetOrganizationByID(ctx, req.OrganizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	// Kiểm tra tất cả các vai trò tồn tại và chưa được gán
	for _, roleID := range req.RoleIDs {
		role, err := s.roleRepo.GetRoleByID(ctx, roleID)
		if err != nil {
			return nil, err
		}
		if role == nil {
			return nil, errors.New("role not found: " + roleID.String())
		}

		exists, err := s.repo.ExistsByUserRoleAndOrg(ctx, userID, roleID, req.OrganizationID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("user already has role in organization: " + roleID.String())
		}
	}

	// Tạo các gán quyền
	now := time.Now()
	userOrgRoles := make([]model.UserOrganizationRole, len(req.RoleIDs))
	for i, roleID := range req.RoleIDs {
		userOrgRoles[i] = model.UserOrganizationRole{
			UserID:         userID,
			RoleID:         roleID,
			OrganizationID: req.OrganizationID,
			GrantedAt:      now,
			GrantedBy:      &grantedBy,
			Notes:          req.Notes,
			Status:         "active",
		}
	}

	if err := s.repo.CreateBatch(ctx, userOrgRoles); err != nil {
		return nil, err
	}

	// Lấy tất cả vai trò người dùng trong tổ chức này
	created, err := s.repo.FindByUserAndOrganization(ctx, userID, req.OrganizationID, "active")
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserOrgRoleResponseDTO, len(created))
	for i, uor := range created {
		result[i] = *toUserOrgRoleResponseDTO(&uor)
	}

	return result, nil
}

// ============ Các Thao Tác Truy Vấn ============

func (s *UserOrganizationRoleService) GetUserOrgRoleByID(ctx context.Context, id uuid.UUID) (*dto.UserOrgRoleResponseDTO, error) {
	userOrgRole, err := s.repo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	if userOrgRole == nil {
		return nil, errors.New("user organization role not found")
	}

	return toUserOrgRoleResponseDTO(userOrgRole), nil
}

func (s *UserOrganizationRoleService) GetUserOrgRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error) {
	userOrgRoles, err := s.repo.FindByUserIDWithDetails(ctx, userID, status)
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserOrgRoleResponseDTO, len(userOrgRoles))
	for i, uor := range userOrgRoles {
		result[i] = *toUserOrgRoleResponseDTO(&uor)
	}

	return result, nil
}

func (s *UserOrganizationRoleService) GetUserOrgRolesInOrganization(ctx context.Context, userID, organizationID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error) {
	userOrgRoles, err := s.repo.FindByUserAndOrganization(ctx, userID, organizationID, status)
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserOrgRoleResponseDTO, len(userOrgRoles))
	for i, uor := range userOrgRoles {
		result[i] = *toUserOrgRoleResponseDTO(&uor)
	}

	return result, nil
}

func (s *UserOrganizationRoleService) GetUsersWithOrgRole(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithOrgRoleResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Lấy thông tin vai trò
	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}

	// Lấy danh sách người dùng có vai trò này trong tổ chức
	userOrgRoles, total, err := s.repo.FindByRoleID(ctx, roleID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	// Lọc theo tổ chức nếu được chỉ định
	filteredRoles := make([]model.UserOrganizationRole, 0)
	for _, uor := range userOrgRoles {
		if uor.OrganizationID == organizationID {
			filteredRoles = append(filteredRoles, uor)
		}
	}

	// Tạo phản hồi
	users := make([]dto.UserWithOrgRolesResponseDTO, len(filteredRoles))
	for i, uor := range filteredRoles {
		users[i] = dto.UserWithOrgRolesResponseDTO{
			UserID:   uor.UserID,
			Username: uor.User.UserName,
			Email:    uor.User.Email,
			OrgRoles: []dto.UserOrgRoleResponseDTO{*toUserOrgRoleResponseDTO(&uor)},
		}
	}

	return &dto.UsersWithOrgRoleResponseDTO{
		RoleID:         roleID,
		RoleName:       role.Name,
		OrganizationID: organizationID,
		Users:          users,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func (s *UserOrganizationRoleService) GetOrganizationMembers(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	userOrgRoles, total, err := s.repo.FindByOrganizationID(ctx, organizationID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserOrgRoleResponseDTO, len(userOrgRoles))
	for i, uor := range userOrgRoles {
		result[i] = *toUserOrgRoleResponseDTO(&uor)
	}

	return &dto.UserOrgRoleListResponseDTO{
		UserOrgRoles: result,
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
	}, nil
}

func (s *UserOrganizationRoleService) CheckUserHasOrgRole(ctx context.Context, userID, roleID, organizationID uuid.UUID) (bool, error) {
	userOrgRole, err := s.repo.FindByUserRoleAndOrg(ctx, userID, roleID, organizationID)
	if err != nil {
		return false, err
	}
	return userOrgRole != nil && userOrgRole.Status == "active", nil
}

// ============ Các Thao Tác Cập Nhật Trạng Thái ============

func (s *UserOrganizationRoleService) UpdateUserOrgRoleStatus(ctx context.Context, id uuid.UUID, req dto.UpdateUserOrgRoleStatusDTO, updatedBy uuid.UUID) (*dto.UserOrgRoleResponseDTO, error) {
	userOrgRole, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if userOrgRole == nil {
		return nil, errors.New("user organization role not found")
	}

	var revokedBy *uuid.UUID
	if req.Status == "revoked" {
		revokedBy = &updatedBy
	}

	if err := s.repo.UpdateStatus(ctx, id, req.Status, revokedBy); err != nil {
		return nil, err
	}

	// Lấy bản ghi đã cập nhật
	updated, err := s.repo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, err
	}

	return toUserOrgRoleResponseDTO(updated), nil
}

// ============ Các Thao Tác Xóa ============

func (s *UserOrganizationRoleService) RemoveOrgRoleFromUser(ctx context.Context, id uuid.UUID) error {
	userOrgRole, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if userOrgRole == nil {
		return errors.New("user organization role not found")
	}

	return s.repo.Delete(ctx, id)
}

func (s *UserOrganizationRoleService) RemoveAllOrgRolesFromUser(ctx context.Context, userID uuid.UUID) error {
	return s.repo.DeleteByUserID(ctx, userID)
}

func (s *UserOrganizationRoleService) RemoveUserFromOrganization(ctx context.Context, userID, organizationID uuid.UUID) error {
	// Lấy tất cả vai trò của người dùng trong tổ chức này
	userOrgRoles, err := s.repo.FindByUserAndOrganization(ctx, userID, organizationID, "")
	if err != nil {
		return err
	}

	// Xóa từng gán quyền
	for _, uor := range userOrgRoles {
		if err := s.repo.Delete(ctx, uor.ID); err != nil {
			return err
		}
	}

	return nil
}

// ============ Các Hàm Hỗ Trợ ============

func toUserOrgRoleResponseDTO(uor *model.UserOrganizationRole) *dto.UserOrgRoleResponseDTO {
	result := &dto.UserOrgRoleResponseDTO{
		ID:             uor.ID,
		UserID:         uor.UserID,
		RoleID:         uor.RoleID,
		OrganizationID: uor.OrganizationID,
		GrantedAt:      uor.GrantedAt.Format("2006-01-02T15:04:05Z"),
		GrantedBy:      uor.GrantedBy,
		Notes:          uor.Notes,
		Status:         uor.Status,
		RevokedBy:      uor.RevokedBy,
		CreatedAt:      uor.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      uor.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if uor.RevokedAt != nil {
		revokedAt := uor.RevokedAt.Format("2006-01-02T15:04:05Z")
		result.RevokedAt = &revokedAt
	}

	if uor.Role != nil {
		var desc *string
		if uor.Role.Description.Valid {
			desc = &uor.Role.Description.String
		}
		result.Role = &dto.OrgRoleResponseDTO{
			ID:          uor.Role.ID,
			Name:        uor.Role.Name,
			Description: desc,
			Status:      uor.Role.Status,
		}
	}

	if uor.Organization != nil {
		result.Organization = &dto.OrgInfoResponseDTO{
			ID:   uor.Organization.ID,
			Name: uor.Organization.Name,
		}
	}

	return result
}
