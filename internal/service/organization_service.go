package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type OrganizationServiceInterface interface {
	CreateOrganization(ctx context.Context, creatorUserID uuid.UUID, req dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error)
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

// orgOwnerPermissionNames (23a, review vòng 1) — ĐÚNG danh sách permission của role ORG_OWNER
// khai báo trong data/roles.json, dùng để tạo Role "ORG_OWNER" theo TỪNG tổ chức (bảng "roles",
// phân biệt với SystemRole "ORG_OWNER" toàn cục đã seed sẵn — xem comment ở CreateOrganization).
var orgOwnerPermissionNames = []string{
	"ORG_MEMBERS_MANAGE",
	"ORG_ROLES_MANAGE",
	"ORG_CATEGORIES_MANAGE",
	"COURSES_APPROVE_OWN_ORG",
	"COURSES_DELETE_ORG",
	"REPORTS_VIEW_ORG",
	"TRACKING_VIEW_ORG_STUDENTS",
}

// CreateOrganization (M-01/23a, review vòng 1): TRƯỚC ĐÂY chỉ INSERT bản ghi Organization,
// không cấp role nào cho người tạo. PUT/DELETE /organizations/:id yêu cầu ORG_ROLES_MANAGE
// VÀ active_org_id khớp org đó (RequireOrgPermission, organization_router.go) — một TEACHER
// (có permission ORG_CREATE) tạo xong tổ chức thì KHÔNG sửa/xóa được chính nó, vì không có
// cách nào tự cấp org role cho mình (route cấp org role đòi hỏi ORG_MEMBERS_MANAGE, mà chưa
// ai có permission đó trong tổ chức vừa tạo — vòng lặp chết, chỉ SYSTEM_ADMIN gỡ được).
//
// Sửa: trong CÙNG một transaction với việc tạo Organization, tìm-hoặc-tạo một Role tên
// "ORG_OWNER" RIÊNG cho tổ chức này (bảng "roles", OrganizationID = org mới, PHÂN BIỆT với
// SystemRole "ORG_OWNER" toàn cục dùng cho luồng B-01/self-service — hai bảng khác nhau: role
// tổ chức này chỉ có hiệu lực khi active_org_id khớp, không cấp quyền toàn hệ thống), gán đúng
// bộ permission của ORG_OWNER theo data/roles.json, rồi tạo UserOrganizationRole cho người tạo.
// Người tạo cần chuyển active_org_id sang tổ chức mới (SwitchOrg/SelectOrg, luồng đã có sẵn)
// để permission có hiệu lực — không cần sửa gì thêm ở đó.
func (s *OrganizationService) CreateOrganization(ctx context.Context, creatorUserID uuid.UUID, req dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
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

	err = s.repo.WithTransaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(org).Error; err != nil {
			return err
		}

		ownerRole := model.Role{
			Name:           "ORG_OWNER",
			OrganizationID: &org.ID,
			Status:         "active",
		}
		if err := tx.Where("name = ? AND organization_id = ?", ownerRole.Name, org.ID).
			FirstOrCreate(&ownerRole).Error; err != nil {
			return err
		}

		var perms []model.Permission
		if err := tx.Where("name IN ?", orgOwnerPermissionNames).Find(&perms).Error; err != nil {
			return err
		}
		for _, perm := range perms {
			rp := model.RolePermission{RoleID: ownerRole.ID, PermissionID: perm.ID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rp).Error; err != nil {
				return err
			}
		}

		uor := &model.UserOrganizationRole{
			UserID:         creatorUserID,
			RoleID:         ownerRole.ID,
			OrganizationID: org.ID,
			GrantedAt:      time.Now(),
			GrantedBy:      &creatorUserID,
			Status:         model.UserOrgRoleStatusActive,
		}
		return tx.Create(uor).Error
	})
	if err != nil {
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
		Status:      org.Status,
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
		Status:      org.Status,
		Roles:       roleDTOs,
		CreatedAt:   org.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   org.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
