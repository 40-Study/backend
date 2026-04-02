package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type UserOrganizationRoleServiceInterface interface {
	GetMyOrgRoles(ctx context.Context, userID uuid.UUID, orgID *uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error)
	GetUserOrgRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error)
	AssignOrgRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignOrgRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error)
	RevokeOrgRoleFromUser(ctx context.Context, userID, orgRoleID, revokedBy uuid.UUID) error
	GetUsersWithOrgRoleByRoleID(ctx context.Context, roleID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error)
	GetOrganizationMembers(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error)
	GetUsersWithOrgRole(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithOrgRoleResponseDTO, error)
}

type UserOrganizationRoleService struct {
	repo        repository.UserOrganizationRoleRepositoryInterface
	userRepo    repository.UserRepositoryInterface
	roleRepo    repository.RoleRepositoryInterface
	orgRepo     repository.OrganizationRepositoryInterface
	redisClient *redis.Client
}

func NewUserOrganizationRoleService(
	repo repository.UserOrganizationRoleRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	roleRepo repository.RoleRepositoryInterface,
	orgRepo repository.OrganizationRepositoryInterface,
	redisClient *redis.Client,
) *UserOrganizationRoleService {
	return &UserOrganizationRoleService{
		repo:        repo,
		userRepo:    userRepo,
		roleRepo:    roleRepo,
		orgRepo:     orgRepo,
		redisClient: redisClient,
	}
}

// GetMyOrgRoles lay organization roles cua chinh minh
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

// GetUserOrgRoles lay organization roles cua mot user
func (s *UserOrganizationRoleService) GetUserOrgRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserOrgRoleResponseDTO, error) {
	// Kiem tra user ton tai
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

// AssignOrgRolesToUser gan organization roles cho user
func (s *UserOrganizationRoleService) AssignOrgRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignOrgRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserOrgRoleResponseDTO, error) {
	// Kiem tra user ton tai
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Kiem tra organization ton tai
	org, err := s.orgRepo.GetOrganizationByID(ctx, req.OrganizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	// Validate cac roles ton tai va active va thuoc organization
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
		if role.OrganizationID != nil && *role.OrganizationID != req.OrganizationID {
			return nil, errors.New("role does not belong to this organization: " + role.Name)
		}
	}

	// Load cac mapping hien co trong 1 query
	existingMappings, err := s.repo.FindByUserAndRoleIDsInOrg(ctx, userID, req.OrganizationID, req.RoleIDs)
	if err != nil {
		return nil, err
	}

	// Tao map de lookup O(1)
	existingMap := make(map[uuid.UUID]*model.UserOrganizationRole, len(existingMappings))
	for i := range existingMappings {
		existingMap[existingMappings[i].RoleID] = &existingMappings[i]
	}

	// Xu ly roles: reactivate inactive, error neu active, collect new
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

	// Tra loi neu co role da active
	if len(alreadyActiveRoles) > 0 {
		return nil, fmt.Errorf("roles already assigned to user: %v", alreadyActiveRoles)
	}

	// Thuc hien trong transaction
	if len(rolesToReactivate) > 0 || len(rolesToAssign) > 0 {
		if err := s.repo.AssignRolesWithTx(ctx, rolesToReactivate, rolesToAssign); err != nil {
			return nil, err
		}
	}

	// Tra ve danh sach moi
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

// RevokeOrgRoleFromUser thu hoi organization role tu user
// Security: Increments user_version to invalidate existing JWT tokens immediately
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

	// Update status in database
	if err := s.repo.UpdateStatus(ctx, orgRoleID, model.UserOrgRoleStatusInactive, &revokedBy); err != nil {
		return err
	}

	// Security: Increment user_version to invalidate all existing tokens
	// This forces user to re-login with updated roles
	if s.redisClient != nil {
		userVersionKey := constants.KeyUserVersion(userID.String())
		s.redisClient.Incr(ctx, userVersionKey)
	}

	return nil
}

// GetUsersWithOrgRoleByRoleID lay users theo organization role
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

// GetOrganizationMembers lay thanh vien cua organization
func (s *UserOrganizationRoleService) GetOrganizationMembers(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UserOrgRoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Kiem tra organization ton tai
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

// GetUsersWithOrgRole lay users theo role trong organization
func (s *UserOrganizationRoleService) GetUsersWithOrgRole(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithOrgRoleResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Kiem tra organization ton tai
	org, err := s.orgRepo.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	// Kiem tra role ton tai
	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}

	// Query voi ca role_id va organization_id
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

// Helper

// toUserOrgRoleResponseDTO — delegate to package-level mapper in user_role_service.go
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
