package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type OrganizationRepositoryInterface interface {
	CreateOrganization(ctx context.Context, org *model.Organization) error
	// WithTransaction (23a, review vòng 1): cho phép service tạo Organization + Role
	// ORG_OWNER + gán quyền + UserOrganizationRole cho người tạo trong CÙNG một transaction —
	// nếu bước nào lỗi, org mới tạo cũng bị rollback (không để lại tổ chức "mồ côi" không ai
	// quản lý được).
	WithTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (*model.Organization, error)
	GetOrganizationByName(ctx context.Context, name string) (*model.Organization, error)
	GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.Organization, int64, error)
	UpdateOrganization(ctx context.Context, org *model.Organization) error
	DeleteOrganization(ctx context.Context, id uuid.UUID, hardDelete bool) error
	GetOrganizationWithRoles(ctx context.Context, orgID uuid.UUID) (*model.Organization, error)
	// ActiveMemberUserIDs: user đang giữ ít nhất một vai trò active trong tổ chức (gọi TRƯỚC khi xoá tổ chức).
	ActiveMemberUserIDs(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error)
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

func (r *OrganizationRepository) WithTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
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

func (r *OrganizationRepository) GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.Organization, int64, error) {
	var orgs []model.Organization
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.Organization{})
	switch status {
	case "deleted":
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	case "all":
		query = query.Unscoped()
	default: // "active" or empty
	}
	if keyword != "" {
		query = utils.ApplyAccentInsensitiveKeywordSearch(query, keyword, "name")
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

func (r *OrganizationRepository) DeleteOrganization(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Lấy danh sách role thuộc organization
			var roleIDs []uuid.UUID
			if err := tx.Model(&model.Role{}).Where("organization_id = ?", id).Pluck("id", &roleIDs).Error; err != nil {
				return err
			}
			// Xóa role_permissions của các role thuộc organization
			if len(roleIDs) > 0 {
				if err := tx.Where("role_id IN ?", roleIDs).Delete(&model.RolePermission{}).Error; err != nil {
					return err
				}
			}
			// Xóa roles thuộc organization
			if err := tx.Unscoped().Where("organization_id = ?", id).Delete(&model.Role{}).Error; err != nil {
				return err
			}
			// Xóa organization
			return tx.Unscoped().Delete(&model.Organization{}, "id = ?", id).Error
		})
	}
	// Xoá mềm: FK CASCADE không chạy nên phải gỡ vai trò của mọi thành viên trong CÙNG transaction, nếu không họ giữ
	// vai trò active trỏ vào tổ chức đã mất (QA3-02).
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ?", id).Delete(&model.UserOrganizationRole{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Organization{}, "id = ?", id).Error
	})
}

func (r *OrganizationRepository) ActiveMemberUserIDs(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Model(&model.UserOrganizationRole{}).
		Where("organization_id = ? AND status = ?", orgID, model.UserOrgRoleStatusActive).
		Distinct().Pluck("user_id", &ids).Error
	return ids, err
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
