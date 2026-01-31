package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type OrganizationRepositoryInterface interface {
	CreateOrganization(ctx context.Context, org *model.Organization) error
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (*model.Organization, error)
	GetOrganizationByName(ctx context.Context, name string) (*model.Organization, error)
	GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string) ([]model.Organization, int64, error)
	UpdateOrganization(ctx context.Context, org *model.Organization) error
	DeleteOrganization(ctx context.Context, id uuid.UUID) error
	GetOrganizationWithRoles(ctx context.Context, orgID uuid.UUID) (*model.Organization, error)
}

type OrganizationRepository struct {
	db *gorm.DB
}

func NewOrganizationRepository(db *gorm.DB) *OrganizationRepository {
	return &OrganizationRepository{db: db}
}

func (r *OrganizationRepository) CreateOrganization(ctx context.Context, org *model.Organization) error {
	return r.db.WithContext(ctx).Create(org).Error
}

func (r *OrganizationRepository) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*model.Organization, error) {
	var org model.Organization
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

func (r *OrganizationRepository) GetOrganizationByName(ctx context.Context, name string) (*model.Organization, error) {
	var org model.Organization
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

func (r *OrganizationRepository) GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string) ([]model.Organization, int64, error) {
	var orgs []model.Organization
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.Organization{})
	if keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&orgs).Error; err != nil {
		return nil, 0, err
	}

	return orgs, total, nil
}

func (r *OrganizationRepository) UpdateOrganization(ctx context.Context, org *model.Organization) error {
	return r.db.WithContext(ctx).Save(org).Error
}

func (r *OrganizationRepository) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Organization{}, "id = ?", id).Error
}

func (r *OrganizationRepository) GetOrganizationWithRoles(ctx context.Context, orgID uuid.UUID) (*model.Organization, error) {
	org, err := r.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, nil
	}

	var roles []model.Role
	if err := r.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&roles).Error; err != nil {
		return nil, err
	}
	org.Roles = roles
	return org, nil
}
