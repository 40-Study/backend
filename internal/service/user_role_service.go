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

// UserRoleServiceInterface — inject interface này vào bất kỳ service/handler nào cần
type UserRoleServiceInterface interface {
	GetAllRolesOfUser(ctx context.Context, userID uuid.UUID) (*dto.UserAllRolesResponseDTO, error)
}

type UserRoleService struct {
	userRepo        repository.UserRepositoryInterface
	userSysRoleRepo repository.UserSystemRoleRepositoryInterface
	userOrgRoleRepo repository.UserOrganizationRoleRepositoryInterface
}

func NewUserRoleService(
	userRepo repository.UserRepositoryInterface,
	userSysRoleRepo repository.UserSystemRoleRepositoryInterface,
	userOrgRoleRepo repository.UserOrganizationRoleRepositoryInterface,
) *UserRoleService {
	return &UserRoleService{
		userRepo:        userRepo,
		userSysRoleRepo: userSysRoleRepo,
		userOrgRoleRepo: userOrgRoleRepo,
	}
}

// GetAllRolesOfUser lấy toàn bộ roles của user — cả system lẫn org roles.
// Inject UserRoleServiceInterface vào bất kỳ service/handler nào cần dùng.
func (s *UserRoleService) GetAllRolesOfUser(ctx context.Context, userID uuid.UUID) (*dto.UserAllRolesResponseDTO, error) {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	sysRoles, err := s.userSysRoleRepo.FindByUserIDWithDetails(ctx, userID, model.UserSystemRoleStatusActive)
	if err != nil {
		return nil, err
	}

	orgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, model.UserOrgRoleStatusActive)
	if err != nil {
		return nil, err
	}

	return &dto.UserAllRolesResponseDTO{
		UserID:      userID,
		SystemRoles: mapUserSystemRoleDTOs(sysRoles),
		OrgRoles:    mapUserOrgRoleDTOs(orgRoles),
	}, nil
}

// ============ Package-level mappers — dùng chung cho mọi service trong package ============

func mapUserSystemRoleDTOs(list []model.UserSystemRole) []dto.UserSystemRoleResponseDTO {
	result := make([]dto.UserSystemRoleResponseDTO, len(list))
	for i, usr := range list {
		result[i] = mapUserSystemRoleDTO(usr)
	}
	return result
}

func mapUserSystemRoleDTO(usr model.UserSystemRole) dto.UserSystemRoleResponseDTO {
	r := dto.UserSystemRoleResponseDTO{
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
		s := usr.RevokedAt.Format(time.RFC3339)
		r.RevokedAt = &s
	}
	if usr.SystemRole != nil {
		var desc *string
		if usr.SystemRole.Description.Valid {
			desc = &usr.SystemRole.Description.String
		}
		r.SystemRole = &dto.SystemRoleResponseDTO{
			ID:          usr.SystemRole.ID,
			Name:        usr.SystemRole.Name,
			Description: desc,
			Status:      usr.SystemRole.Status,
			CreatedAt:   usr.SystemRole.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   usr.SystemRole.UpdatedAt.Format(time.RFC3339),
		}
	}
	return r
}

func mapUserOrgRoleDTOs(list []model.UserOrganizationRole) []dto.UserOrgRoleResponseDTO {
	result := make([]dto.UserOrgRoleResponseDTO, len(list))
	for i := range list {
		if mapped := mapUserOrgRoleDTO(&list[i]); mapped != nil {
			result[i] = *mapped
		}
	}
	return result
}

func mapUserOrgRoleDTO(uor *model.UserOrganizationRole) *dto.UserOrgRoleResponseDTO {
	r := &dto.UserOrgRoleResponseDTO{
		ID:             uor.ID,
		UserID:         uor.UserID,
		RoleID:         uor.RoleID,
		OrganizationID: uor.OrganizationID,
		GrantedAt:      uor.GrantedAt.Format(time.RFC3339),
		GrantedBy:      uor.GrantedBy,
		Notes:          uor.Notes,
		Status:         uor.Status,
		RevokedBy:      uor.RevokedBy,
		CreatedAt:      uor.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      uor.UpdatedAt.Format(time.RFC3339),
	}
	if uor.RevokedAt != nil {
		s := uor.RevokedAt.Format(time.RFC3339)
		r.RevokedAt = &s
	}
	if uor.Role != nil {
		var desc *string
		if uor.Role.Description.Valid {
			desc = &uor.Role.Description.String
		}
		r.Role = &dto.OrgRoleResponseDTO{
			ID:          uor.Role.ID,
			Name:        uor.Role.Name,
			Description: desc,
			Status:      uor.Role.Status,
		}
	}
	if uor.Organization != nil {
		r.Organization = &dto.OrgInfoResponseDTO{
			ID:   uor.Organization.ID,
			Name: uor.Organization.Name,
		}
	}
	return r
}
