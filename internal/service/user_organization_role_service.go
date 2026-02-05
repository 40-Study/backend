package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type UserOrganizationRoleServiceInterface interface {
	// User APIs
	GetMyOrgRoles(ctx context.Context, userID uuid.UUID, orgID *uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error)

	// Admin APIs
	GetUserOrgRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error)
	AssignOrgRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignOrgRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error)
	RevokeOrgRoleFromUser(ctx context.Context, userID, orgRoleID, revokedBy uuid.UUID) error

	// Role Management
	GetUsersWithOrgRoleByRoleID(ctx context.Context, roleID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error)

	// Organization Members
	GetOrganizationMembers(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error)
	GetUsersWithOrgRole(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithOrgRoleResponseDTO, error)
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

// ============ User APIs ============

// GetMyOrgRoles - GET /me/org-roles
func (s *UserOrganizationRoleService) GetMyOrgRoles(ctx context.Context, userID uuid.UUID, orgID *uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error) {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	var userOrgRoles []model.UserOrganizationRole

	if orgID != nil {
		userOrgRoles, err = s.repo.FindByUserAndOrganization(ctx, userID, *orgID, model.UserOrgRoleStatusActive)
	} else {
		userOrgRoles, err = s.repo.FindByUserIDWithDetails(ctx, userID, model.UserOrgRoleStatusActive)
	}

	if err != nil {
		return nil, err
	}

	result := make([]dto.UserOrgRoleResponseDTO, len(userOrgRoles))
	for i, uor := range userOrgRoles {
		result[i] = *toUserOrgRoleResponseDTO(&uor)
	}

	return result, nil
}

// ============ Admin APIs ============

// GetUserOrgRoles - GET /users/:user_id/org-roles
func (s *UserOrganizationRoleService) GetUserOrgRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error) {
	// Verify user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

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

// AssignOrgRolesToUser - POST /users/:user_id/org-roles
func (s *UserOrganizationRoleService) AssignOrgRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignOrgRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error) {
	// 1. Verify user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// 2. Verify organization exists
	org, err := s.orgRepo.GetOrganizationByID(ctx, req.OrganizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	// 3. Validate all roles exist, are active, and belong to the organization
	validRoles, err := s.roleRepo.GetRoleByIDs(ctx, req.RoleIDs)
	if err != nil {
		return nil, err
	}
	if len(validRoles) != len(req.RoleIDs) {
		return nil, errors.New("one or more roles not found")
	}

	for _, role := range validRoles {
		if role.Status != "active" {
			return nil, errors.New("role is not active: " + role.Name)
		}
		if role.OrganizationID != req.OrganizationID {
			return nil, errors.New("role does not belong to this organization: " + role.Name)
		}
	}

	// 4. Load existing mappings in one query (N+1 optimization)
	existingMappings, err := s.repo.FindByUserAndRoleIDsInOrg(ctx, userID, req.OrganizationID, req.RoleIDs)
	if err != nil {
		return nil, err
	}

	// 5. Build map for O(1) lookup
	existingMap := make(map[uuid.UUID]*model.UserOrganizationRole, len(existingMappings))
	for i := range existingMappings {
		existingMap[existingMappings[i].RoleID] = &existingMappings[i]
	}

	// 6. Process roles: reactivate inactive, error if active, collect new
	var rolesToAssign []model.UserOrganizationRole
	var rolesToReactivate []*model.UserOrganizationRole
	var alreadyActiveRoles []string
	now := time.Now()

	for _, role := range validRoles {
		existing, exists := existingMap[role.ID]

		if exists {
			if existing.Status == model.UserOrgRoleStatusInactive {
				existing.Status = model.UserOrgRoleStatusActive
				existing.GrantedAt = now
				existing.GrantedBy = &grantedBy
				existing.Notes = req.Notes
				existing.RevokedAt = nil
				existing.RevokedBy = nil
				rolesToReactivate = append(rolesToReactivate, existing)
			} else {
				alreadyActiveRoles = append(alreadyActiveRoles, role.Name)
			}
			continue
		}

		rolesToAssign = append(rolesToAssign, model.UserOrganizationRole{
			UserID:         userID,
			RoleID:         role.ID,
			OrganizationID: req.OrganizationID,
			GrantedAt:      now,
			GrantedBy:      &grantedBy,
			Notes:          req.Notes,
			Status:         model.UserOrgRoleStatusActive,
		})
	}

	// 7. Return error if any role already active
	if len(alreadyActiveRoles) > 0 {
		return nil, fmt.Errorf("roles already assigned to user: %v", alreadyActiveRoles)
	}

	// 8. Execute in transaction
	if len(rolesToReactivate) > 0 || len(rolesToAssign) > 0 {
		if err := s.repo.AssignRolesWithTx(ctx, rolesToReactivate, rolesToAssign); err != nil {
			return nil, err
		}
	}

	// 9. Return updated list
	created, err := s.repo.FindByUserAndOrganization(ctx, userID, req.OrganizationID, model.UserOrgRoleStatusActive)
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserOrgRoleResponseDTO, len(created))
	for i, uor := range created {
		result[i] = *toUserOrgRoleResponseDTO(&uor)
	}

	return result, nil
}

// RevokeOrgRoleFromUser - DELETE /users/:user_id/org-roles/:org_role_id
func (s *UserOrganizationRoleService) RevokeOrgRoleFromUser(ctx context.Context, userID, orgRoleID, revokedBy uuid.UUID) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	userOrgRole, err := s.repo.FindByID(ctx, orgRoleID)
	if err != nil {
		return err
	}
	if userOrgRole == nil {
		return errors.New("organization role assignment not found")
	}

	if userOrgRole.UserID != userID {
		return errors.New("role assignment does not belong to this user")
	}

	if userOrgRole.Status == model.UserOrgRoleStatusInactive {
		return errors.New("organization role already inactive for this user")
	}

	return s.repo.UpdateStatus(ctx, orgRoleID, model.UserOrgRoleStatusInactive, &revokedBy)
}

// ============ Role Management ============

// GetUsersWithOrgRoleByRoleID - GET /org-roles/:role_id/users
func (s *UserOrganizationRoleService) GetUsersWithOrgRoleByRoleID(ctx context.Context, roleID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("organization role not found")
	}

	userOrgRoles, total, err := s.repo.FindByRoleID(ctx, roleID, page, pageSize, status)
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

// ============ Organization Members ============

// GetOrganizationMembers - GET /organizations/:organization_id/members
func (s *UserOrganizationRoleService) GetOrganizationMembers(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Verify organization exists
	org, err := s.orgRepo.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
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

// GetUsersWithOrgRole - GET /organizations/:organization_id/roles/:role_id/users
func (s *UserOrganizationRoleService) GetUsersWithOrgRole(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithOrgRoleResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Verify organization exists
	org, err := s.orgRepo.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	// Verify role exists
	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}

	// Query with both role_id and organization_id filter at DB level
	userOrgRoles, total, err := s.repo.FindByRoleIDAndOrgID(ctx, roleID, organizationID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	users := make([]dto.UserWithOrgRolesResponseDTO, len(userOrgRoles))
	for i, uor := range userOrgRoles {
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

// ============ Helper ============

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
