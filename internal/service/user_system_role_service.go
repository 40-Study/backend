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
	GetMySystemRoles(ctx context.Context, userID uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error)
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

// GetMySystemRoles lay system roles cua chinh minh
func (s *UserSystemRoleService) GetMySystemRoles(ctx context.Context, userID uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error) {
	// Kiem tra user ton tai
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Lay system roles voi chi tiet
	userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, model.UserSystemRoleStatusActive)
	if err != nil {
		return nil, err
	}

	return s.mapToResponseDTOs(userSystemRoles), nil
}

// AssignSystemRolesToUser gan system roles cho user
func (s *UserSystemRoleService) AssignSystemRolesToUser(
	ctx context.Context,
	userID uuid.UUID,
	req dto.AssignSystemRolesToUserDTO,
	grantedBy uuid.UUID,
) ([]dto.UserSystemRoleResponseDTO, error) {
	// Kiem tra user ton tai
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Validate cac system roles ton tai va active
	validRoles, err := s.systemRoleRepo.GetSystemRoleByIDs(ctx, req.SystemRoleIDs)
	if err != nil {
		return nil, err
	}
	if len(validRoles) != len(req.SystemRoleIDs) {
		return nil, errors.New("one or more system roles not found")
	}
	for _, role := range validRoles {
		if role.Status != "active" {
			return nil, errors.New("system role is not active: " + role.Name)
		}
	}

	// Load cac mapping hien co trong 1 query
	existingMappings, err := s.userSystemRoleRepo.FindByUserAndSystemRoleIDs(ctx, userID, req.SystemRoleIDs)
	if err != nil {
		return nil, err
	}

	// Tao map de lookup O(1)
	existingMap := make(map[uuid.UUID]*model.UserSystemRole, len(existingMappings))
	for i := range existingMappings {
		existingMap[existingMappings[i].SystemRoleID] = &existingMappings[i]
	}

	// Xu ly roles: reactivate inactive, error neu active, collect new
	var rolesToAssign []model.UserSystemRole
	var rolesToReactivate []*model.UserSystemRole
	var alreadyActiveRoles []string
	now := time.Now()

	for _, role := range validRoles {
		existing, exists := existingMap[role.ID]

		if exists {
			if existing.Status == model.UserSystemRoleStatusInactive {
				// Reactivate
				existing.Status = model.UserSystemRoleStatusActive
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

		// Tao moi
		rolesToAssign = append(rolesToAssign, model.UserSystemRole{
			UserID:       userID,
			SystemRoleID: role.ID,
			GrantedAt:    now,
			GrantedBy:    &grantedBy,
			Notes:        req.Notes,
			Status:       model.UserSystemRoleStatusActive,
		})
	}

	// Tra loi neu co role da active
	if len(alreadyActiveRoles) > 0 {
		return nil, fmt.Errorf("roles already assigned to user: %v", alreadyActiveRoles)
	}

	// Thuc hien trong transaction
	if len(rolesToReactivate) > 0 || len(rolesToAssign) > 0 {
		if err := s.userSystemRoleRepo.AssignRolesWithTx(ctx, rolesToReactivate, rolesToAssign); err != nil {
			return nil, err
		}
	}

	return s.GetUserSystemRoles(ctx, userID, model.UserSystemRoleStatusActive)
}

// RevokeSystemRoleFromUser thu hoi system role tu user
func (s *UserSystemRoleService) RevokeSystemRoleFromUser(
	ctx context.Context,
	userID, systemRoleID, revokedBy uuid.UUID,
) error {
	// Kiem tra user ton tai
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	// Kiem tra system role ton tai
	role, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("system role not found")
	}

	// Tim assignment
	assignment, err := s.userSystemRoleRepo.FindByUserAndSystemRole(ctx, userID, systemRoleID)
	if err != nil {
		return err
	}
	if assignment == nil {
		return errors.New("user does not have this system role")
	}

	// Kiem tra da inactive chua
	if assignment.Status == model.UserSystemRoleStatusInactive {
		return errors.New("system role already inactive for this user")
	}

	// Revoke bang cach doi status
	return s.userSystemRoleRepo.UpdateStatus(ctx, assignment.ID, model.UserSystemRoleStatusInactive, &revokedBy)
}

// GetUserSystemRoles lay system roles cua mot user
func (s *UserSystemRoleService) GetUserSystemRoles(
	ctx context.Context,
	userID uuid.UUID,
	status string,
) ([]dto.UserSystemRoleResponseDTO, error) {
	// Kiem tra user ton tai
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Lay system roles voi chi tiet
	userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, status)
	if err != nil {
		return nil, err
	}

	return s.mapToResponseDTOs(userSystemRoles), nil
}

// GetUsersBySystemRole lay users theo system role
func (s *UserSystemRoleService) GetUsersBySystemRole(
	ctx context.Context,
	systemRoleID uuid.UUID,
	page, pageSize int,
	status string,
) (*dto.UserSystemRoleListResponseDTO, error) {
	// Kiem tra system role ton tai
	role, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("system role not found")
	}

	// Set defaults
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Lay users
	userSystemRoles, total, err := s.userSystemRoleRepo.FindBySystemRoleID(ctx, systemRoleID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	return &dto.UserSystemRoleListResponseDTO{
		UserSystemRoles: s.mapToResponseDTOs(userSystemRoles),
		Total:           total,
		Page:            page,
		PageSize:        pageSize,
	}, nil
}

// Helper methods

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

	if usr.RevokedAt != nil {
		revokedAt := usr.RevokedAt.Format(time.RFC3339)
		response.RevokedAt = &revokedAt
	}

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
