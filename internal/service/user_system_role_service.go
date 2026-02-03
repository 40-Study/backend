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

type UserSystemRoleServiceInterface interface {
	// Assign operations
	AssignSystemRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignSystemRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error)

	// Query operations
	GetUserSystemRoleByID(ctx context.Context, id uuid.UUID) (*dto.UserSystemRoleResponseDTO, error)
	GetUserSystemRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserSystemRoleResponseDTO, error)
	GetUsersWithSystemRole(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithSystemRoleResponseDTO, error)
	CheckUserHasSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error)

	// Status operations
	UpdateUserSystemRoleStatus(ctx context.Context, id uuid.UUID, req dto.UpdateUserSystemRoleStatusDTO, updatedBy uuid.UUID) (*dto.UserSystemRoleResponseDTO, error)

	// Delete operations
	RemoveSystemRoleFromUser(ctx context.Context, id uuid.UUID) error
	RemoveAllSystemRolesFromUser(ctx context.Context, userID uuid.UUID) error
}

type UserSystemRoleService struct {
	repo           repository.UserSystemRoleRepositoryInterface
	userRepo       repository.UserRepositoryInterface
	systemRoleRepo repository.SystemRoleRepositoryInterface
}

func NewUserSystemRoleService(
	repo repository.UserSystemRoleRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
) *UserSystemRoleService {
	return &UserSystemRoleService{
		repo:           repo,
		userRepo:       userRepo,
		systemRoleRepo: systemRoleRepo,
	}
}

// ============ Assign Operations ============

func (s *UserSystemRoleService) AssignSystemRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignSystemRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error) {
	// Validate user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Validate all system roles exist
	for _, roleID := range req.SystemRoleIDs {
		systemRole, err := s.systemRoleRepo.GetSystemRoleByID(ctx, roleID)
		if err != nil {
			return nil, err
		}
		if systemRole == nil {
			return nil, errors.New("system role not found: " + roleID.String())
		}

		// Check if already assigned
		exists, err := s.repo.ExistsByUserAndSystemRole(ctx, userID, roleID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("user already has system role: " + roleID.String())
		}
	}

	// Create assignments
	now := time.Now()
	userSystemRoles := make([]model.UserSystemRole, len(req.SystemRoleIDs))
	for i, roleID := range req.SystemRoleIDs {
		userSystemRoles[i] = model.UserSystemRole{
			UserID:       userID,
			SystemRoleID: roleID,
			GrantedAt:    now,
			GrantedBy:    &grantedBy,
			Notes:        req.Notes,
			Status:       "active",
		}
	}

	if err := s.repo.CreateBatch(ctx, userSystemRoles); err != nil {
		return nil, err
	}

	// Fetch all with details
	created, err := s.repo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserSystemRoleResponseDTO, len(created))
	for i, usr := range created {
		result[i] = *toUserSystemRoleResponseDTO(&usr)
	}

	return result, nil
}

// ============ Query Operations ============

func (s *UserSystemRoleService) GetUserSystemRoleByID(ctx context.Context, id uuid.UUID) (*dto.UserSystemRoleResponseDTO, error) {
	userSystemRole, err := s.repo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	if userSystemRole == nil {
		return nil, errors.New("user system role not found")
	}

	return toUserSystemRoleResponseDTO(userSystemRole), nil
}

func (s *UserSystemRoleService) GetUserSystemRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserSystemRoleResponseDTO, error) {
	userSystemRoles, err := s.repo.FindByUserIDWithDetails(ctx, userID, status)
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserSystemRoleResponseDTO, len(userSystemRoles))
	for i, usr := range userSystemRoles {
		result[i] = *toUserSystemRoleResponseDTO(&usr)
	}

	return result, nil
}

func (s *UserSystemRoleService) GetUsersWithSystemRole(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithSystemRoleResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get system role info
	systemRole, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return nil, err
	}
	if systemRole == nil {
		return nil, errors.New("system role not found")
	}

	// Get users with this role
	userSystemRoles, total, err := s.repo.FindBySystemRoleID(ctx, systemRoleID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	// Build response
	users := make([]dto.UserWithSystemRolesResponseDTO, len(userSystemRoles))
	for i, usr := range userSystemRoles {
		users[i] = dto.UserWithSystemRolesResponseDTO{
			UserID:      usr.UserID,
			Username:    usr.User.UserName,
			Email:       usr.User.Email,
			SystemRoles: []dto.UserSystemRoleResponseDTO{*toUserSystemRoleResponseDTO(&usr)},
		}
	}

	return &dto.UsersWithSystemRoleResponseDTO{
		SystemRoleID:   systemRoleID,
		SystemRoleName: systemRole.Name,
		Users:          users,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func (s *UserSystemRoleService) CheckUserHasSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error) {
	userSystemRole, err := s.repo.FindByUserAndSystemRole(ctx, userID, systemRoleID)
	if err != nil {
		return false, err
	}
	return userSystemRole != nil && userSystemRole.Status == "active", nil
}

// ============ Status Operations ============

func (s *UserSystemRoleService) UpdateUserSystemRoleStatus(ctx context.Context, id uuid.UUID, req dto.UpdateUserSystemRoleStatusDTO, updatedBy uuid.UUID) (*dto.UserSystemRoleResponseDTO, error) {
	userSystemRole, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if userSystemRole == nil {
		return nil, errors.New("user system role not found")
	}

	var revokedBy *uuid.UUID
	if req.Status == "revoked" {
		revokedBy = &updatedBy
	}

	if err := s.repo.UpdateStatus(ctx, id, req.Status, revokedBy); err != nil {
		return nil, err
	}

	// Fetch updated record
	updated, err := s.repo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, err
	}

	return toUserSystemRoleResponseDTO(updated), nil
}

// ============ Delete Operations ============

func (s *UserSystemRoleService) RemoveSystemRoleFromUser(ctx context.Context, id uuid.UUID) error {
	userSystemRole, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if userSystemRole == nil {
		return errors.New("user system role not found")
	}

	return s.repo.Delete(ctx, id)
}

func (s *UserSystemRoleService) RemoveAllSystemRolesFromUser(ctx context.Context, userID uuid.UUID) error {
	return s.repo.DeleteByUserID(ctx, userID)
}

// ============ Helper Functions ============

func toUserSystemRoleResponseDTO(usr *model.UserSystemRole) *dto.UserSystemRoleResponseDTO {
	result := &dto.UserSystemRoleResponseDTO{
		ID:           usr.ID,
		UserID:       usr.UserID,
		SystemRoleID: usr.SystemRoleID,
		GrantedAt:    usr.GrantedAt.Format("2006-01-02T15:04:05Z"),
		GrantedBy:    usr.GrantedBy,
		Notes:        usr.Notes,
		Status:       usr.Status,
		RevokedBy:    usr.RevokedBy,
		CreatedAt:    usr.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    usr.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if usr.RevokedAt != nil {
		revokedAt := usr.RevokedAt.Format("2006-01-02T15:04:05Z")
		result.RevokedAt = &revokedAt
	}

	if usr.SystemRole != nil {
		result.SystemRole = toSystemRoleResponseDTO(usr.SystemRole)
	}

	return result
}
