package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type PermissionServiceInterface interface {
	GetPermissionByID(ctx context.Context, id uuid.UUID) (*dto.PermissionResponseDTO, error)
	GetAllPermissions(ctx context.Context, page, pageSize int, keyword string) (*dto.PermissionListResponseDTO, error)
	UpdatePermission(ctx context.Context, id uuid.UUID, req dto.UpdatePermissionDTO) (*dto.PermissionResponseDTO, error)
}

type PermissionService struct {
	repo repository.PermissionRepositoryInterface
}

func NewPermissionService(repo repository.PermissionRepositoryInterface) *PermissionService {
	return &PermissionService{repo: repo}
}

func (s *PermissionService) GetPermissionByID(ctx context.Context, id uuid.UUID) (*dto.PermissionResponseDTO, error) {
	permission, err := s.repo.GetPermissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if permission == nil {
		return nil, errors.New("permission not found")
	}

	return toPermissionResponseDTO(permission), nil
}

func (s *PermissionService) GetAllPermissions(ctx context.Context, page, pageSize int, keyword string) (*dto.PermissionListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	permissions, total, err := s.repo.GetAllPermissions(ctx, page, pageSize, keyword)
	if err != nil {
		return nil, err
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(permissions))
	for i, perm := range permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return &dto.PermissionListResponseDTO{
		Permissions: permDTOs,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *PermissionService) UpdatePermission(ctx context.Context, id uuid.UUID, req dto.UpdatePermissionDTO) (*dto.PermissionResponseDTO, error) {
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

	return toPermissionResponseDTO(permission), nil
}

func toPermissionResponseDTO(permission *model.Permission) *dto.PermissionResponseDTO {
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
