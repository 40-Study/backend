package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

type ProfileServiceInterface interface {
	// GetChildren lấy danh sách con của phụ huynh
	GetChildren(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.PaginatedChildrenResponse, error)
	// GetOrganizations lấy danh sách tổ chức của user
	GetOrganizations(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.PaginatedOrganizationsResponse, error)
}

type ProfileService struct {
	parentStudentRepo repository.ParentStudentRepositoryInterface
	userOrgRoleRepo   repository.UserOrganizationRoleRepositoryInterface
}

func NewProfileService(
	parentStudentRepo repository.ParentStudentRepositoryInterface,
	userOrgRoleRepo repository.UserOrganizationRoleRepositoryInterface,
) *ProfileService {
	return &ProfileService{
		parentStudentRepo: parentStudentRepo,
		userOrgRoleRepo:   userOrgRoleRepo,
	}
}

// GetChildren lấy danh sách con của phụ huynh với pagination
func (s *ProfileService) GetChildren(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.PaginatedChildrenResponse, error) {
	// Validate pagination params
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	relations, total, err := s.parentStudentRepo.GetChildrenByParentID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get children: %w", err)
	}

	children := make([]dto.ChildDto, len(relations))
	for i, rel := range relations {
		children[i] = dto.ChildDto{
			ID:           rel.Student.ID.String(),
			Username:     rel.Student.UserName,
			FullName:     rel.Student.FullName,
			AvatarUrl:    rel.Student.AvatarURL,
			Relationship: rel.Relationship,
		}
	}

	return &dto.PaginatedChildrenResponse{
		Children: children,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetOrganizations lấy danh sách tổ chức của user với pagination
func (s *ProfileService) GetOrganizations(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.PaginatedOrganizationsResponse, error) {
	// Validate pagination params
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Lấy tất cả org roles của user
	orgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations: %w", err)
	}

	// Deduplicate organizations (user có thể có nhiều roles trong 1 org)
	// Giữ lại role đầu tiên tìm được
	orgMap := make(map[string]dto.MyOrganizationDto)
	for _, or := range orgRoles {
		orgID := or.Organization.ID.String()
		if _, exists := orgMap[orgID]; !exists {
			orgMap[orgID] = dto.MyOrganizationDto{
				ID:         orgID,
				Name:       or.Organization.Name,
				MemberRole: or.Role.Name,
				Status:     or.Organization.Status,
			}
		}
	}

	// Convert map to slice
	allOrgs := make([]dto.MyOrganizationDto, 0, len(orgMap))
	for _, org := range orgMap {
		allOrgs = append(allOrgs, org)
	}

	// In-memory pagination
	total := int64(len(allOrgs))
	start := (page - 1) * pageSize
	end := start + pageSize

	var orgs []dto.MyOrganizationDto
	if start >= len(allOrgs) {
		orgs = []dto.MyOrganizationDto{}
	} else if end > len(allOrgs) {
		orgs = allOrgs[start:]
	} else {
		orgs = allOrgs[start:end]
	}

	return &dto.PaginatedOrganizationsResponse{
		Organizations: orgs,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
	}, nil
}
