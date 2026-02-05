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

type UserSystemRoleServiceInterface interface {
	// User APIs
	GetMySystemRoles(ctx context.Context, userID uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error)

	// Admin APIs
	AssignSystemRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignSystemRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error)
	RevokeSystemRoleFromUser(ctx context.Context, userID, systemRoleID, revokedBy uuid.UUID) error
	GetUserSystemRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserSystemRoleResponseDTO, error)
	GetUsersBySystemRole(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) (*dto.UserSystemRoleListResponseDTO, error)
}

type UserSystemRoleService struct {
	userRepo           repository.UserRepositoryInterface
	systemRoleRepo     repository.SystemRoleRepositoryInterface
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface
}

func NewUserSystemRoleService(
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
) *UserSystemRoleService {
	return &UserSystemRoleService{
		userRepo:           userRepo,
		systemRoleRepo:     systemRoleRepo,
		userSystemRoleRepo: userSystemRoleRepo,
	}
}

// ============ User APIs ============

// GetMySystemRoles - Lấy danh sách system roles của chính mình
func (s *UserSystemRoleService) GetMySystemRoles(ctx context.Context, userID uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error) {
	// 1. Verify user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// 2. Get user system roles with details (only active)
	userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, model.UserSystemRoleStatusActive)
	if err != nil {
		return nil, err
	}

	// 3. Map to response DTOs
	return s.mapToResponseDTOs(userSystemRoles), nil
}

// ============ Admin APIs ============

// AssignSystemRolesToUser - Gán system roles cho user
func (s *UserSystemRoleService) AssignSystemRolesToUser(
	ctx context.Context,
	userID uuid.UUID,
	req dto.AssignSystemRolesToUserDTO,
	grantedBy uuid.UUID,
) ([]dto.UserSystemRoleResponseDTO, error) {
	// 1. Verify target user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// 2. Validate all system roles exist and are active (single query)
	validRoles, err := s.systemRoleRepo.GetSystemRoleByIDs(ctx, req.SystemRoleIDs)
	if err != nil {
		return nil, err
	}
	if len(validRoles) != len(req.SystemRoleIDs) {
		return nil, errors.New("one or more system roles not found")
	}
	// Check all roles are active
	for _, role := range validRoles {
		if role.Status != "active" {
			return nil, errors.New("system role is not active: " + role.Name)
		}
	}

	// 3. Load existing mappings in one query (includes soft-deleted)
	existingMappings, err := s.userSystemRoleRepo.FindByUserAndSystemRoleIDs(ctx, userID, req.SystemRoleIDs)
	if err != nil {
		return nil, err
	}

	// 4. Build map[systemRoleID]*UserSystemRole for O(1) lookup
	existingMap := make(map[uuid.UUID]*model.UserSystemRole, len(existingMappings))
	for i := range existingMappings {
		existingMap[existingMappings[i].SystemRoleID] = &existingMappings[i]
	}

	// 5. Process roles: reactivate inactive, error if active, collect new
	var rolesToAssign []model.UserSystemRole
	var rolesToReactivate []*model.UserSystemRole
	var alreadyActiveRoles []string
	now := time.Now()

	for _, role := range validRoles {
		existing, exists := existingMap[role.ID]

		if exists {
			if existing.Status == model.UserSystemRoleStatusInactive {
				// Prepare for reactivation
				existing.Status = model.UserSystemRoleStatusActive
				existing.GrantedAt = now
				existing.GrantedBy = &grantedBy
				existing.Notes = req.Notes
				existing.RevokedAt = nil
				existing.RevokedBy = nil
				rolesToReactivate = append(rolesToReactivate, existing)
			} else {
				// Already active - collect for error message
				alreadyActiveRoles = append(alreadyActiveRoles, role.Name)
			}
			continue
		}

		// New assignment
		rolesToAssign = append(rolesToAssign, model.UserSystemRole{
			UserID:       userID,
			SystemRoleID: role.ID,
			GrantedAt:    now,
			GrantedBy:    &grantedBy,
			Notes:        req.Notes,
			Status:       model.UserSystemRoleStatusActive,
		})
	}

	// 6. Return error if any role already active
	if len(alreadyActiveRoles) > 0 {
		return nil, fmt.Errorf("roles already assigned to user: %v", alreadyActiveRoles)
	}

	// 7. Execute reactivations and inserts in single transaction
	if len(rolesToReactivate) > 0 || len(rolesToAssign) > 0 {
		if err := s.userSystemRoleRepo.AssignRolesWithTx(ctx, rolesToReactivate, rolesToAssign); err != nil {
			return nil, err
		}
	}

	// 8. Return updated list
	return s.GetUserSystemRoles(ctx, userID, model.UserSystemRoleStatusActive)
}

// RevokeSystemRoleFromUser - Gỡ system role khỏi user (soft delete/revoke)
func (s *UserSystemRoleService) RevokeSystemRoleFromUser(
	ctx context.Context,
	userID, systemRoleID, revokedBy uuid.UUID,
) error {
	// 1. Verify user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	// 2. Verify system role exists
	role, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("system role not found")
	}

	// 3. Find the assignment
	assignment, err := s.userSystemRoleRepo.FindByUserAndSystemRole(ctx, userID, systemRoleID)
	if err != nil {
		return err
	}
	if assignment == nil {
		return errors.New("user does not have this system role")
	}

	// 4. Check if already inactive/revoked
	if assignment.Status == model.UserSystemRoleStatusInactive {
		return errors.New("system role already inactive for this user")
	}

	// 5. Revoke (soft delete via status change)
	return s.userSystemRoleRepo.UpdateStatus(ctx, assignment.ID, model.UserSystemRoleStatusInactive, &revokedBy)
}

// GetUserSystemRoles - Lấy system roles của một user (Admin view)
func (s *UserSystemRoleService) GetUserSystemRoles(
	ctx context.Context,
	userID uuid.UUID,
	status string,
) ([]dto.UserSystemRoleResponseDTO, error) {
	// 1. Verify user exists
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// 2. Get user system roles with details
	userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, status)
	if err != nil {
		return nil, err
	}

	// 3. Map to response DTOs
	return s.mapToResponseDTOs(userSystemRoles), nil
}

// GetUsersBySystemRole - Lấy danh sách users theo system role
func (s *UserSystemRoleService) GetUsersBySystemRole(
	ctx context.Context,
	systemRoleID uuid.UUID,
	page, pageSize int,
	status string,
) (*dto.UserSystemRoleListResponseDTO, error) {
	// 1. Verify system role exists
	role, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("system role not found")
	}

	// 2. Set defaults
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 3. Get users with this system role
	userSystemRoles, total, err := s.userSystemRoleRepo.FindBySystemRoleID(ctx, systemRoleID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	// 4. Map to response
	return &dto.UserSystemRoleListResponseDTO{
		UserSystemRoles: s.mapToResponseDTOs(userSystemRoles),
		Total:           total,
		Page:            page,
		PageSize:        pageSize,
	}, nil
}

// ============ Helper Methods ============

func (s *UserSystemRoleService) mapToResponseDTOs(userSystemRoles []model.UserSystemRole) []dto.UserSystemRoleResponseDTO {
	result := make([]dto.UserSystemRoleResponseDTO, len(userSystemRoles))

	for i, usr := range userSystemRoles {
		result[i] = s.mapToResponseDTO(usr)
	}

	return result
}

func (s *UserSystemRoleService) mapToResponseDTO(usr model.UserSystemRole) dto.UserSystemRoleResponseDTO {
	response := dto.UserSystemRoleResponseDTO{
		ID:           usr.ID,
		UserID:       usr.UserID,
		SystemRoleID: usr.SystemRoleID,
		GrantedAt:    usr.GrantedAt.Format(time.RFC3339),
		GrantedBy:    usr.GrantedBy,
		Notes:        usr.Notes,
		Status:       usr.Status,
		RevokedBy:    usr.RevokedBy,
		CreatedAt:    usr.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    usr.UpdatedAt.Format(time.RFC3339),
	}

	// Map RevokedAt
	if usr.RevokedAt != nil {
		revokedAt := usr.RevokedAt.Format(time.RFC3339)
		response.RevokedAt = &revokedAt
	}

	// Map SystemRole if loaded
	if usr.SystemRole != nil {
		var desc *string
		if usr.SystemRole.Description.Valid {
			desc = &usr.SystemRole.Description.String
		}
		response.SystemRole = &dto.SystemRoleResponseDTO{
			ID:          usr.SystemRole.ID,
			Name:        usr.SystemRole.Name,
			Description: desc,
			Status:      usr.SystemRole.Status,
			CreatedAt:   usr.SystemRole.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   usr.SystemRole.UpdatedAt.Format(time.RFC3339),
		}
	}

	return response
}
