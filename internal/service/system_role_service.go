package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type SystemRoleServiceInterface interface {
	CreateSystemRole(ctx context.Context, req dto.CreateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error)
	GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*dto.SystemRoleDetailResponseDTO, error)
	GetAllSystemRoles(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.SystemRoleListResponseDTO, error)
	UpdateSystemRole(ctx context.Context, id uuid.UUID, req dto.UpdateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error)
	DeleteSystemRole(ctx context.Context, id uuid.UUID, hardDelete bool) error
	RestoreSystemRole(ctx context.Context, id uuid.UUID) error

	AddPermissionsToSystemRole(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToSystemRoleDTO) error
	RemovePermissionsFromSystemRole(ctx context.Context, roleID uuid.UUID, req dto.RemovePermissionsFromSystemRoleDTO) error
	SetSystemRolePermissions(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToSystemRoleDTO) error
	GetSystemRolePermissions(ctx context.Context, roleID uuid.UUID) ([]dto.PermissionResponseDTO, error)
}

type SystemRoleService struct {
	repo           repository.SystemRoleRepositoryInterface
	permissionRepo repository.PermissionRepositoryInterface
}

func NewSystemRoleService(repo repository.SystemRoleRepositoryInterface, permissionRepo repository.PermissionRepositoryInterface) *SystemRoleService {
	return &SystemRoleService{repo: repo, permissionRepo: permissionRepo}
}

func (s *SystemRoleService) CreateSystemRole(ctx context.Context, req dto.CreateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	role := &model.SystemRole{
		Name: req.Name,
	}
	if req.Description != "" {
		role.Description.String = req.Description
		role.Description.Valid = true
	}

	if err := s.repo.CreateSystemRole(ctx, role); err != nil {
		return nil, err
	}

	return toSystemRoleResponseDTO(role), nil
}

func (s *SystemRoleService) GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*dto.SystemRoleDetailResponseDTO, error) {
	role, err := s.repo.GetSystemRoleWithPermissions(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("system role not found")
	}

	return toSystemRoleDetailResponseDTO(role), nil
}

func (s *SystemRoleService) GetAllSystemRoles(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.SystemRoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	roles, total, err := s.repo.GetAllSystemRoles(ctx, page, pageSize, keyword, status)
	if err != nil {
		return nil, err
	}

	roleDTOs := make([]dto.SystemRoleResponseDTO, len(roles))
	for i, role := range roles {
		roleDTOs[i] = *toSystemRoleResponseDTO(&role)
	}

	return &dto.SystemRoleListResponseDTO{
		Roles:    roleDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *SystemRoleService) UpdateSystemRole(ctx context.Context, id uuid.UUID, req dto.UpdateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	role, err := s.repo.GetSystemRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("system role not found")
	}

	if req.Name != nil {
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description.String = *req.Description
		role.Description.Valid = true
	}

	if err := s.repo.UpdateSystemRole(ctx, role); err != nil {
		return nil, err
	}

	return toSystemRoleResponseDTO(role), nil
}

func (s *SystemRoleService) DeleteSystemRole(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	role, err := s.repo.GetSystemRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("system role not found")
	}

	return s.repo.DeleteSystemRole(ctx, id, hardDelete)
}

func (s *SystemRoleService) RestoreSystemRole(ctx context.Context, id uuid.UUID) error {
	return s.repo.RestoreSystemRole(ctx, id)
}

// ============ Quản Lý Quyền-Vai Trò Hệ Thống ============

func (s *SystemRoleService) AddPermissionsToSystemRole(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToSystemRoleDTO) error {
	role, err := s.repo.GetSystemRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("system role not found")
	}

	count, err := s.permissionRepo.CountPermissionsByIDs(ctx, req.PermissionIDs)
	if err != nil {
		return err
	}
	if int(count) != len(req.PermissionIDs) {
		return errors.New("one or more permission IDs do not exist")
	}

	return s.repo.AddPermissionsToSystemRole(ctx, roleID, req.PermissionIDs)
}

func (s *SystemRoleService) RemovePermissionsFromSystemRole(ctx context.Context, roleID uuid.UUID, req dto.RemovePermissionsFromSystemRoleDTO) error {
	role, err := s.repo.GetSystemRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("system role not found")
	}

	return s.repo.RemovePermissionsFromSystemRole(ctx, roleID, req.PermissionIDs)
}

func (s *SystemRoleService) SetSystemRolePermissions(ctx context.Context, roleID uuid.UUID, req dto.AddPermissionsToSystemRoleDTO) error {
	role, err := s.repo.GetSystemRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("system role not found")
	}

	count, err := s.permissionRepo.CountPermissionsByIDs(ctx, req.PermissionIDs)
	if err != nil {
		return err
	}
	if int(count) != len(req.PermissionIDs) {
		return errors.New("one or more permission IDs do not exist")
	}

	return s.repo.SetSystemRolePermissions(ctx, roleID, req.PermissionIDs)
}

func (s *SystemRoleService) GetSystemRolePermissions(ctx context.Context, roleID uuid.UUID) ([]dto.PermissionResponseDTO, error) {
	permissions, err := s.repo.GetPermissionsBySystemRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if permissions == nil {
		return nil, errors.New("system role not found")
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(permissions))
	for i, perm := range permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return permDTOs, nil
}

func toSystemRoleResponseDTO(role *model.SystemRole) *dto.SystemRoleResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	return &dto.SystemRoleResponseDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: desc,
		Status:      role.Status,
		CreatedAt:   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toSystemRoleDetailResponseDTO(role *model.SystemRole) *dto.SystemRoleDetailResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(role.Permissions))
	for i, perm := range role.Permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return &dto.SystemRoleDetailResponseDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: desc,
		Status:      role.Status,
		Permissions: permDTOs,
		CreatedAt:   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
