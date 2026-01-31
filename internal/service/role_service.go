package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type RoleServiceInterface interface {
	CreateRole(ctx context.Context, req dto.CreateRoleDTO) (*dto.RoleResponseDTO, error)
	GetRoleByID(ctx context.Context, id uuid.UUID) (*dto.RoleDetailResponseDTO, error)
	GetAllRoles(ctx context.Context, page, pageSize int) (*dto.RoleListResponseDTO, error)
	UpdateRole(ctx context.Context, id uuid.UUID, req dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error)
	DeleteRole(ctx context.Context, id uuid.UUID) error

	// Role-Permission management
	AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error
	RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, req dto.RemovePermissionsFromRoleDTO) error
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error
	GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]dto.PermissionResponseDTO, error)
}

type RoleService struct {
	repo repository.RoleRepositoryInterface
}

func NewRoleService(repo repository.RoleRepositoryInterface) *RoleService {
	return &RoleService{repo: repo}
}

func (s *RoleService) CreateRole(ctx context.Context, req dto.CreateRoleDTO) (*dto.RoleResponseDTO, error) {
	existing, err := s.repo.GetRoleByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("role with this name already exists")
	}

	role := &model.Role{
		Name: req.Name,
	}
	if req.Description != "" {
		role.Description.String = req.Description
		role.Description.Valid = true
	}

	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}

	return toRoleResponseDTO(role), nil
}

func (s *RoleService) GetRoleByID(ctx context.Context, id uuid.UUID) (*dto.RoleDetailResponseDTO, error) {
	role, err := s.repo.GetRoleWithPermissions(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}

	return toRoleDetailResponseDTO(role), nil
}

func (s *RoleService) GetAllRoles(ctx context.Context, page, pageSize int) (*dto.RoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	roles, total, err := s.repo.GetAllRoles(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}

	roleDTOs := make([]dto.RoleResponseDTO, len(roles))
	for i, role := range roles {
		roleDTOs[i] = *toRoleResponseDTO(&role)
	}

	return &dto.RoleListResponseDTO{
		Roles:    roleDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *RoleService) UpdateRole(ctx context.Context, id uuid.UUID, req dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error) {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}

	if req.Name != nil {
		existing, err := s.repo.GetRoleByName(ctx, *req.Name)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != id {
			return nil, errors.New("role with this name already exists")
		}
		role.Name = *req.Name
	}

	if req.Description != nil {
		role.Description.String = *req.Description
		role.Description.Valid = true
	}

	if err := s.repo.UpdateRole(ctx, role); err != nil {
		return nil, err
	}

	return toRoleResponseDTO(role), nil
}

func (s *RoleService) DeleteRole(ctx context.Context, id uuid.UUID) error {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.DeleteRole(ctx, id)
}

// ============ Role-Permission Management ============

func (s *RoleService) AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.AddPermissionsToRole(ctx, roleID, req.PermissionIDs)
}

func (s *RoleService) RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, req dto.RemovePermissionsFromRoleDTO) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.RemovePermissionsFromRole(ctx, roleID, req.PermissionIDs)
}

func (s *RoleService) SetRolePermissions(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.SetRolePermissions(ctx, roleID, req.PermissionIDs)
}

func (s *RoleService) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]dto.PermissionResponseDTO, error) {
	permissions, err := s.repo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if permissions == nil {
		return nil, errors.New("role not found")
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(permissions))
	for i, perm := range permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return permDTOs, nil
}

func toRoleResponseDTO(role *model.Role) *dto.RoleResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	return &dto.RoleResponseDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: desc,
		CreatedAt:   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toRoleDetailResponseDTO(role *model.Role) *dto.RoleDetailResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(role.Permissions))
	for i, perm := range role.Permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return &dto.RoleDetailResponseDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: desc,
		Permissions: permDTOs,
		CreatedAt:   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
