package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type OrganizationServiceInterface interface {
	CreateOrganization(ctx context.Context, req dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error)
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (*dto.OrganizationDetailResponseDTO, error)
	GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.OrganizationListResponseDTO, error)
	UpdateOrganization(ctx context.Context, id uuid.UUID, req dto.UpdateOrganizationDTO) (*dto.OrganizationResponseDTO, error)
	DeleteOrganization(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type OrganizationService struct {
	repo repository.OrganizationRepositoryInterface
}

func NewOrganizationService(repo repository.OrganizationRepositoryInterface) *OrganizationService {
	return &OrganizationService{repo: repo}
}

func (s *OrganizationService) CreateOrganization(ctx context.Context, req dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	existing, err := s.repo.GetOrganizationByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("organization with this name already exists")
	}

	org := &model.Organization{
		Name: req.Name,
	}
	if req.Description != "" {
		org.Description.String = req.Description
		org.Description.Valid = true
	}

	if err := s.repo.CreateOrganization(ctx, org); err != nil {
		return nil, err
	}

	return toOrganizationResponseDTO(org), nil
}

func (s *OrganizationService) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*dto.OrganizationDetailResponseDTO, error) {
	org, err := s.repo.GetOrganizationWithRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	return toOrganizationDetailResponseDTO(org), nil
}

func (s *OrganizationService) GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.OrganizationListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	orgs, total, err := s.repo.GetAllOrganizations(ctx, page, pageSize, keyword, status)
	if err != nil {
		return nil, err
	}

	orgDTOs := make([]dto.OrganizationResponseDTO, len(orgs))
	for i, org := range orgs {
		orgDTOs[i] = *toOrganizationResponseDTO(&org)
	}

	return &dto.OrganizationListResponseDTO{
		Organizations: orgDTOs,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
	}, nil
}

func (s *OrganizationService) UpdateOrganization(ctx context.Context, id uuid.UUID, req dto.UpdateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	org, err := s.repo.GetOrganizationByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, errors.New("organization not found")
	}

	if req.Name != nil {
		existing, err := s.repo.GetOrganizationByName(ctx, *req.Name)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != id {
			return nil, errors.New("organization with this name already exists")
		}
		org.Name = *req.Name
	}

	if req.Description != nil {
		org.Description.String = *req.Description
		org.Description.Valid = true
	}

	if err := s.repo.UpdateOrganization(ctx, org); err != nil {
		return nil, err
	}

	return toOrganizationResponseDTO(org), nil
}

func (s *OrganizationService) DeleteOrganization(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	org, err := s.repo.GetOrganizationByID(ctx, id)
	if err != nil {
		return err
	}
	if org == nil {
		return errors.New("organization not found")
	}

	return s.repo.DeleteOrganization(ctx, id, hardDelete)
}

// ============ Helper Methods ============

func toOrganizationResponseDTO(org *model.Organization) *dto.OrganizationResponseDTO {
	var desc *string
	if org.Description.Valid {
		desc = &org.Description.String
	}

	return &dto.OrganizationResponseDTO{
		ID:          org.ID,
		Name:        org.Name,
		Description: desc,
		CreatedAt:   org.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   org.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toOrganizationDetailResponseDTO(org *model.Organization) *dto.OrganizationDetailResponseDTO {
	var desc *string
	if org.Description.Valid {
		desc = &org.Description.String
	}

	roleDTOs := make([]dto.RoleResponseDTO, len(org.Roles))
	for i, role := range org.Roles {
		roleDTOs[i] = *toRoleResponseDTO(&role)
	}

	return &dto.OrganizationDetailResponseDTO{
		ID:          org.ID,
		Name:        org.Name,
		Description: desc,
		Roles:       roleDTOs,
		CreatedAt:   org.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   org.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
