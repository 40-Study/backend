package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type RolePermissionServiceInterface interface {
	// Role operations
	CreateRole(ctx context.Context, req dto.CreateRoleDTO) (*dto.RoleResponseDTO, error)
	GetRoleByID(ctx context.Context, id uuid.UUID) (*dto.RoleDetailResponseDTO, error)
	GetAllRoles(ctx context.Context, page, pageSize int) (*dto.RoleListResponseDTO, error)
	UpdateRole(ctx context.Context, id uuid.UUID, req dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error)
	DeleteRole(ctx context.Context, id uuid.UUID) error

	// Permission operations
	GetPermissionByID(ctx context.Context, id uuid.UUID) (*dto.PermissionResponseDTO, error)
	GetAllPermissions(ctx context.Context, page, pageSize int) (*dto.PermissionListResponseDTO, error)
	UpdatePermission(ctx context.Context, id uuid.UUID, req dto.UpdatePermissionDTO) (*dto.PermissionResponseDTO, error)

	// Role-Permission management
	AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error
	RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, req dto.RemovePermissionsFromRoleDTO) error
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error
	GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]dto.PermissionResponseDTO, error)
}

type RolePermissionService struct {
	repo repository.RolePermissionRepositoryInterface
}

func NewRolePermissionService(repo repository.RolePermissionRepositoryInterface) *RolePermissionService {
	return &RolePermissionService{repo: repo}
}

// ============ Role Operations ============

func (s *RolePermissionService) CreateRole(ctx context.Context, req dto.CreateRoleDTO) (*dto.RoleResponseDTO, error) {
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

	return s.toRoleResponseDTO(role), nil
}

func (s *RolePermissionService) GetRoleByID(ctx context.Context, id uuid.UUID) (*dto.RoleDetailResponseDTO, error) {
	role, err := s.repo.GetRoleWithPermissions(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}

	return s.toRoleDetailResponseDTO(role), nil
}

func (s *RolePermissionService) GetAllRoles(ctx context.Context, page, pageSize int) (*dto.RoleListResponseDTO, error) {
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
		roleDTOs[i] = *s.toRoleResponseDTO(&role)
	}

	return &dto.RoleListResponseDTO{
		Roles:    roleDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *RolePermissionService) UpdateRole(ctx context.Context, id uuid.UUID, req dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error) {
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

	return s.toRoleResponseDTO(role), nil
}

func (s *RolePermissionService) DeleteRole(ctx context.Context, id uuid.UUID) error {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.DeleteRole(ctx, id)
}

// ============ Permission Operations ============

func (s *RolePermissionService) GetPermissionByID(ctx context.Context, id uuid.UUID) (*dto.PermissionResponseDTO, error) {
	permission, err := s.repo.GetPermissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if permission == nil {
		return nil, errors.New("permission not found")
	}

	return s.toPermissionResponseDTO(permission), nil
}

func (s *RolePermissionService) GetAllPermissions(ctx context.Context, page, pageSize int) (*dto.PermissionListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	permissions, total, err := s.repo.GetAllPermissions(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(permissions))
	for i, perm := range permissions {
		permDTOs[i] = *s.toPermissionResponseDTO(&perm)
	}

	return &dto.PermissionListResponseDTO{
		Permissions: permDTOs,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *RolePermissionService) UpdatePermission(ctx context.Context, id uuid.UUID, req dto.UpdatePermissionDTO) (*dto.PermissionResponseDTO, error) {
	permission, err := s.repo.GetPermissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if permission == nil {
		return nil, errors.New("permission not found")
	}

	permission.Description.String = req.Description
	permission.Description.Valid = true

	if err := s.repo.UpdatePermission(ctx, permission); err != nil {
		return nil, err
	}

	return s.toPermissionResponseDTO(permission), nil
}

// ============ Role-Permission Management ============

func (s *RolePermissionService) AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.AddPermissionsToRole(ctx, roleID, req.PermissionIDs)
}

func (s *RolePermissionService) RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, req dto.RemovePermissionsFromRoleDTO) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.RemovePermissionsFromRole(ctx, roleID, req.PermissionIDs)
}

func (s *RolePermissionService) SetRolePermissions(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToRoleDTO) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	return s.repo.SetRolePermissions(ctx, roleID, req.PermissionIDs)
}

func (s *RolePermissionService) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]dto.PermissionResponseDTO, error) {
	permissions, err := s.repo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if permissions == nil {
		return nil, errors.New("role not found")
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(permissions))
	for i, perm := range permissions {
		permDTOs[i] = *s.toPermissionResponseDTO(&perm)
	}

	return permDTOs, nil
}

// ============ Helper Methods ============

func (s *RolePermissionService) toRoleResponseDTO(role *model.Role) *dto.RoleResponseDTO {
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

func (s *RolePermissionService) toRoleDetailResponseDTO(role *model.Role) *dto.RoleDetailResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(role.Permissions))
	for i, perm := range role.Permissions {
		permDTOs[i] = *s.toPermissionResponseDTO(&perm)
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

func (s *RolePermissionService) toPermissionResponseDTO(permission *model.Permission) *dto.PermissionResponseDTO {
	var desc *string
	if permission.Description.Valid {
		desc = &permission.Description.String
	}

	return &dto.PermissionResponseDTO{
		ID:          permission.ID,
		Name:        permission.Name,
		Description: desc,
		CreatedAt:   permission.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   permission.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
