package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type UserSystemRoleServiceInterface interface {
	// Các thao tác gán quyền
	AssignSystemRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignSystemRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error)

	// Các thao tác truy vấn
	GetUserSystemRoleByID(ctx context.Context, id uuid.UUID) (*dto.UserSystemRoleResponseDTO, error)
	GetUserSystemRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserSystemRoleResponseDTO, error)
	GetUsersWithSystemRole(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithSystemRoleResponseDTO, error)
	CheckUserHasSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error)

	// Các thao tác cập nhật trạng thái
	UpdateUserSystemRoleStatus(ctx context.Context, id uuid.UUID, req dto.UpdateUserSystemRoleStatusDTO, updatedBy uuid.UUID) (*dto.UserSystemRoleResponseDTO, error)

	// Các thao tác xóa
	RemoveSystemRoleFromUser(ctx context.Context, id uuid.UUID) error
	RemoveAllSystemRolesFromUser(ctx context.Context, userID uuid.UUID) error
}

type UserSystemRoleService struct {
	repo           repository.UserSystemRoleRepositoryInterface
	userRepo       repository.UserRepositoryInterface
	systemRoleRepo repository.SystemRoleRepositoryInterface
}

func NewUserSystemRoleService(
	repo repository.UserSystemRoleRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
) *UserSystemRoleService {
	return &UserSystemRoleService{
		repo:           repo,
		userRepo:       userRepo,
		systemRoleRepo: systemRoleRepo,
	}
}

// ============ Các Thao Tác Gán Quyền ============

func (s *UserSystemRoleService) AssignSystemRolesToUser(ctx context.Context, userID uuid.UUID, req dto.AssignSystemRolesToUserDTO, grantedBy uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error) {
	// Kiểm tra người dùng tồn tại
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Kiểm tra tất cả các vai trò hệ thống tồn tại
	for _, roleID := range req.SystemRoleIDs {
		systemRole, err := s.systemRoleRepo.GetSystemRoleByID(ctx, roleID)
		if err != nil {
			return nil, err
		}
		if systemRole == nil {
			return nil, errors.New("system role not found: " + roleID.String())
		}

		// Kiểm tra xem đã được gán chưa
		exists, err := s.repo.ExistsByUserAndSystemRole(ctx, userID, roleID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("user already has system role: " + roleID.String())
		}
	}

	// Tạo các gán quyền
	now := time.Now()
	userSystemRoles := make([]model.UserSystemRole, len(req.SystemRoleIDs))
	for i, roleID := range req.SystemRoleIDs {
		userSystemRoles[i] = model.UserSystemRole{
			UserID:       userID,
			SystemRoleID: roleID,
			GrantedAt:    now,
			GrantedBy:    &grantedBy,
			Notes:        req.Notes,
			Status:       "active",
		}
	}

	if err := s.repo.CreateBatch(ctx, userSystemRoles); err != nil {
		return nil, err
	}

	// Lấy tất cả kèm chi tiết
	created, err := s.repo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserSystemRoleResponseDTO, len(created))
	for i, usr := range created {
		result[i] = *toUserSystemRoleResponseDTO(&usr)
	}

	return result, nil
}

// ============ Các Thao Tác Truy Vấn ============

func (s *UserSystemRoleService) GetUserSystemRoleByID(ctx context.Context, id uuid.UUID) (*dto.UserSystemRoleResponseDTO, error) {
	userSystemRole, err := s.repo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	if userSystemRole == nil {
		return nil, errors.New("user system role not found")
	}

	return toUserSystemRoleResponseDTO(userSystemRole), nil
}

func (s *UserSystemRoleService) GetUserSystemRoles(ctx context.Context, userID uuid.UUID, status string) ([]dto.UserSystemRoleResponseDTO, error) {
	userSystemRoles, err := s.repo.FindByUserIDWithDetails(ctx, userID, status)
	if err != nil {
		return nil, err
	}

	result := make([]dto.UserSystemRoleResponseDTO, len(userSystemRoles))
	for i, usr := range userSystemRoles {
		result[i] = *toUserSystemRoleResponseDTO(&usr)
	}

	return result, nil
}

func (s *UserSystemRoleService) GetUsersWithSystemRole(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) (*dto.UsersWithSystemRoleResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Lấy thông tin vai trò hệ thống
	systemRole, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return nil, err
	}
	if systemRole == nil {
		return nil, errors.New("system role not found")
	}

	// Lấy danh sách người dùng có vai trò này
	userSystemRoles, total, err := s.repo.FindBySystemRoleID(ctx, systemRoleID, page, pageSize, status)
	if err != nil {
		return nil, err
	}

	// Tạo phản hồi
	users := make([]dto.UserWithSystemRolesResponseDTO, len(userSystemRoles))
	for i, usr := range userSystemRoles {
		users[i] = dto.UserWithSystemRolesResponseDTO{
			UserID:      usr.UserID,
			Username:    usr.User.UserName,
			Email:       usr.User.Email,
			SystemRoles: []dto.UserSystemRoleResponseDTO{*toUserSystemRoleResponseDTO(&usr)},
		}
	}

	return &dto.UsersWithSystemRoleResponseDTO{
		SystemRoleID:   systemRoleID,
		SystemRoleName: systemRole.Name,
		Users:          users,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func (s *UserSystemRoleService) CheckUserHasSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error) {
	userSystemRole, err := s.repo.FindByUserAndSystemRole(ctx, userID, systemRoleID)
	if err != nil {
		return false, err
	}
	return userSystemRole != nil && userSystemRole.Status == "active", nil
}

// ============ Các Thao Tác Cập Nhật Trạng Thái ============

func (s *UserSystemRoleService) UpdateUserSystemRoleStatus(ctx context.Context, id uuid.UUID, req dto.UpdateUserSystemRoleStatusDTO, updatedBy uuid.UUID) (*dto.UserSystemRoleResponseDTO, error) {
	userSystemRole, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if userSystemRole == nil {
		return nil, errors.New("user system role not found")
	}

	var revokedBy *uuid.UUID
	if req.Status == "revoked" {
		revokedBy = &updatedBy
	}

	if err := s.repo.UpdateStatus(ctx, id, req.Status, revokedBy); err != nil {
		return nil, err
	}

	// Lấy bản ghi đã cập nhật
	updated, err := s.repo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, err
	}

	return toUserSystemRoleResponseDTO(updated), nil
}

// ============ Các Thao Tác Xóa ============

func (s *UserSystemRoleService) RemoveSystemRoleFromUser(ctx context.Context, id uuid.UUID) error {
	userSystemRole, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if userSystemRole == nil {
		return errors.New("user system role not found")
	}

	return s.repo.Delete(ctx, id)
}

func (s *UserSystemRoleService) RemoveAllSystemRolesFromUser(ctx context.Context, userID uuid.UUID) error {
	return s.repo.DeleteByUserID(ctx, userID)
}

// ============ Các Hàm Hỗ Trợ ============

func toUserSystemRoleResponseDTO(usr *model.UserSystemRole) *dto.UserSystemRoleResponseDTO {
	result := &dto.UserSystemRoleResponseDTO{
		ID:           usr.ID,
		UserID:       usr.UserID,
		SystemRoleID: usr.SystemRoleID,
		GrantedAt:    usr.GrantedAt.Format("2006-01-02T15:04:05Z"),
		GrantedBy:    usr.GrantedBy,
		Notes:        usr.Notes,
		Status:       usr.Status,
		RevokedBy:    usr.RevokedBy,
		CreatedAt:    usr.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    usr.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if usr.RevokedAt != nil {
		revokedAt := usr.RevokedAt.Format("2006-01-02T15:04:05Z")
		result.RevokedAt = &revokedAt
	}

	if usr.SystemRole != nil {
		result.SystemRole = toSystemRoleResponseDTO(usr.SystemRole)
	}

	return result
}
