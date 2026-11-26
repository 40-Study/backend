package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
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

// ErrSystemRoleProtected (QA A1): vai trò hệ thống dựng sẵn (STUDENT, TEACHER, SYSTEM_ADMIN...) không
// được xoá qua API quản trị. Handler trả 409 SYSTEM_ROLE_PROTECTED.
var ErrSystemRoleProtected = errors.New("built-in system role cannot be deleted")

// ErrSystemRoleNameExists (QA7-A02, C14): tạo/đổi tên vai trò hệ thống trùng tên đã có (kể cả vai đã xoá mềm: unique index vẫn
// giữ chỗ) hoặc trùng tên vai dựng sẵn (so không phân biệt hoa thường, như tên org role). Handler trả 409 ERR_ROLE_NAME_EXISTS.
var ErrSystemRoleNameExists = apperr.ConflictCode("ERR_ROLE_NAME_EXISTS", "Tên vai trò đã tồn tại (kể cả vai trò đã xoá hoặc tên hệ thống dành riêng). Hãy chọn tên khác.")

type SystemRoleService struct {
	repo           repository.SystemRoleRepositoryInterface
	permissionRepo repository.PermissionRepositoryInterface
}

func NewSystemRoleService(repo repository.SystemRoleRepositoryInterface, permissionRepo repository.PermissionRepositoryInterface) *SystemRoleService {
	return &SystemRoleService{repo: repo, permissionRepo: permissionRepo}
}

func (s *SystemRoleService) CreateSystemRole(ctx context.Context, req dto.CreateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	if model.OrgRoleNameReserved(req.Name) {
		return nil, ErrSystemRoleNameExists
	}
	role := &model.SystemRole{
		Name: req.Name,
	}
	if req.Description != "" {
		role.Description.String = req.Description
		role.Description.Valid = true
	}

	if err := s.repo.CreateSystemRole(ctx, role); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSystemRoleNameExists
		}
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
		return nil, apperr.NotFound("Không tìm thấy vai trò.")
	}

	if req.Name != nil {
		// Giữ nguyên tên hiện tại là hợp lệ (kể cả vai dựng sẵn); đổi sang tên dành riêng thì bị chặn như lúc tạo.
		if *req.Name != role.Name && model.OrgRoleNameReserved(*req.Name) {
			return nil, ErrSystemRoleNameExists
		}
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description.String = *req.Description
		role.Description.Valid = true
	}

	if err := s.repo.UpdateSystemRole(ctx, role); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSystemRoleNameExists
		}
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
	if model.IsBuiltInSystemRole(role.Name) {
		return ErrSystemRoleProtected
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
		CreatedAt:   utils.FormatTimestamp(role.CreatedAt),
		UpdatedAt:   utils.FormatTimestamp(role.UpdatedAt),
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
		CreatedAt:   utils.FormatTimestamp(role.CreatedAt),
		UpdatedAt:   utils.FormatTimestamp(role.UpdatedAt),
	}
}
