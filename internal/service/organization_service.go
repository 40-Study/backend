package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type OrganizationServiceInterface interface {
	CreateOrganization(ctx context.Context, creatorUserID uuid.UUID, req dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error)
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (*dto.OrganizationDetailResponseDTO, error)
	GetAllOrganizations(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.OrganizationListResponseDTO, error)
	UpdateOrganization(ctx context.Context, id uuid.UUID, req dto.UpdateOrganizationDTO) (*dto.OrganizationResponseDTO, error)
	DeleteOrganization(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type OrganizationService struct {
	repo        repository.OrganizationRepositoryInterface
	redisClient *redis.Client
}

// redisClient có thể nil (test): khi đó không vô hiệu hoá token của thành viên lúc xoá tổ chức.
func NewOrganizationService(repo repository.OrganizationRepositoryInterface, redisClient *redis.Client) *OrganizationService {
	return &OrganizationService{repo: repo, redisClient: redisClient}
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
// bộ permission của ORG_OWNER (SSOT data/permissions/org_owner_permissions.json), rồi tạo UserOrganizationRole cho người tạo.
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

		// QA-C-01: tạo đủ bộ vai mặc định (ORG_OWNER kèm quyền chủ tổ chức, TEACHER, STUDENT) — trước đây chỉ có
		// ORG_OWNER nên không thêm được giảng viên/học viên vào tổ chức. Xem repository/default_org_roles.go.
		if _, err := repository.EnsureDefaultOrgRoles(tx, org.ID); err != nil {
			return err
		}
		var ownerRole model.Role
		if err := tx.Where("name = ? AND organization_id = ?", repository.OrgOwnerRoleName, org.ID).
			First(&ownerRole).Error; err != nil {
			return err
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

	// Thành viên phải được lấy TRƯỚC khi xoá: xoá tổ chức gỡ luôn vai trò của họ.
	members, err := s.repo.ActiveMemberUserIDs(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteOrganization(ctx, id, hardDelete); err != nil {
		return err
	}

	// Security: tăng user_version để token đang chọn tổ chức này (active_org_id) hết hiệu lực, như khi thu hồi vai trò
	// (QA3-02). Lỗi Redis chỉ log: tổ chức đã xoá xong, token cũ cũng không còn vai trò nào để dùng.
	if s.redisClient != nil {
		for _, userID := range members {
			if err := s.redisClient.Incr(ctx, constants.KeyUserVersion(userID.String())).Err(); err != nil {
				log.Printf("[OrganizationService] không tăng được user_version của %s sau khi xoá tổ chức %s: %v", userID, id, err)
			}
		}
	}
	return nil
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
		CreatedAt:   utils.FormatTimestamp(org.CreatedAt),
		UpdatedAt:   utils.FormatTimestamp(org.UpdatedAt),
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
		CreatedAt:   utils.FormatTimestamp(org.CreatedAt),
		UpdatedAt:   utils.FormatTimestamp(org.UpdatedAt),
	}
}
