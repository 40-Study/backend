package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Phase 1 quản lý người dùng (2026-09-28) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-01-user-management.md

// ErrCannotLockSelf — admin không được tự khoá tài khoản của chính mình.
var ErrCannotLockSelf = errors.New("cannot lock your own account")

// ErrLockReasonRequired — reason bắt buộc khi is_active=false.
var ErrLockReasonRequired = errors.New("reason is required when locking an account")

const systemAdminRoleName = "SYSTEM_ADMIN"

type UserAdminServiceInterface interface {
	ListUsers(ctx context.Context, filter repository.AdminUserListFilter) (*dto.AdminUserListResponseDTO, error)
	GetUserDetail(ctx context.Context, userID uuid.UUID) (*dto.AdminUserDetailDTO, error)
	UpdateUserStatus(ctx context.Context, targetUserID, actorID uuid.UUID, isActive bool, reason *string) (*dto.AdminUserDetailDTO, error)
}

type UserAdminService struct {
	userRepo           repository.UserRepositoryInterface
	systemRoleRepo     repository.SystemRoleRepositoryInterface
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface
	authService        *AuthService
	redisClient        *redis.Client
}

func NewUserAdminService(
	userRepo repository.UserRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface,
	authService *AuthService,
	redisClient *redis.Client,
) *UserAdminService {
	return &UserAdminService{
		userRepo:           userRepo,
		systemRoleRepo:     systemRoleRepo,
		userSystemRoleRepo: userSystemRoleRepo,
		authService:        authService,
		redisClient:        redisClient,
	}
}

// ListUsers — GET /api/users.
func (s *UserAdminService) ListUsers(ctx context.Context, filter repository.AdminUserListFilter) (*dto.AdminUserListResponseDTO, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	filter.Page = page
	filter.Limit = limit

	users, total, err := s.userRepo.AdminListUsers(ctx, filter)
	if err != nil {
		return nil, err
	}

	userIDs := make([]uuid.UUID, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}
	rolesByUser, err := s.userRepo.ActiveSystemRoleNamesByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	items := make([]dto.AdminUserListItemDTO, len(users))
	for i, u := range users {
		items[i] = dto.AdminUserListItemDTO{
			ID:           u.ID,
			Email:        u.Email,
			UserName:     u.UserName,
			FullName:     u.FullName,
			AvatarURL:    u.AvatarURL,
			IsActive:     u.IsActive,
			IsVerified:   u.IsVerified,
			LockedReason: u.LockedReason,
			LockedAt:     u.LockedAt,
			LastLoginAt:  u.LastLoginAt,
			CreatedAt:    u.CreatedAt,
			SystemRoles:  rolesByUser[u.ID],
		}
		if items[i].SystemRoles == nil {
			items[i].SystemRoles = []string{}
		}
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	if totalPages < 1 {
		totalPages = 1
	}

	return &dto.AdminUserListResponseDTO{
		Items:      items,
		TotalCount: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// GetUserDetail — GET /api/users/:id.
func (s *UserAdminService) GetUserDetail(ctx context.Context, userID uuid.UUID) (*dto.AdminUserDetailDTO, error) {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, repository.ErrAdminUserNotFound
	}

	return s.toDetailDTO(ctx, user)
}

// UpdateUserStatus — PUT /api/users/:id/status. Khoá: bắt buộc reason, không tự khoá mình,
// gọi RevokeAllSessions (tái dùng AuthService, không viết lại INCR/DEL Redis lần 2) + set
// marker ACCOUNT_LOCKED cho AuthMiddleware phân biệt lý do 401. Mở khoá: xoá marker.
func (s *UserAdminService) UpdateUserStatus(
	ctx context.Context,
	targetUserID, actorID uuid.UUID,
	isActive bool,
	reason *string,
) (*dto.AdminUserDetailDTO, error) {
	if !isActive {
		if targetUserID == actorID {
			return nil, ErrCannotLockSelf
		}
		if reason == nil || strings.TrimSpace(*reason) == "" {
			return nil, ErrLockReasonRequired
		}
	}

	systemAdminRole, err := s.systemRoleRepo.GetSystemRoleByName(ctx, systemAdminRoleName)
	if err != nil {
		return nil, err
	}
	var systemAdminRoleID uuid.UUID
	if systemAdminRole != nil {
		systemAdminRoleID = systemAdminRole.ID
	}

	user, err := s.userRepo.LockOrUnlockUser(ctx, targetUserID, isActive, reason, actorID, systemAdminRoleID)
	if err != nil {
		return nil, err
	}

	if !isActive {
		// Vô hiệu TOÀN BỘ phiên đăng nhập ngay lập tức — tái dùng nguyên logic INCR
		// user_version + DEL refresh/session của AuthService (không viết lại lần 2).
		if s.authService != nil {
			if err := s.authService.RevokeAllSessions(ctx, targetUserID); err != nil {
				return nil, err
			}
		}
		if s.redisClient != nil {
			_ = s.redisClient.Set(ctx, constants.KeyAccountLocked(targetUserID.String()), "1", 0).Err()
		}
	} else if s.redisClient != nil {
		_ = s.redisClient.Del(ctx, constants.KeyAccountLocked(targetUserID.String())).Err()
	}

	return s.toDetailDTO(ctx, user)
}

func (s *UserAdminService) toDetailDTO(ctx context.Context, user *model.User) (*dto.AdminUserDetailDTO, error) {
	roleDetails, err := s.userSystemRolesWithDetails(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return &dto.AdminUserDetailDTO{
		ID:           user.ID,
		Email:        user.Email,
		UserName:     user.UserName,
		FullName:     user.FullName,
		AvatarURL:    user.AvatarURL,
		Phone:        user.Phone,
		DateOfBirth:  user.DateOfBirth,
		IsActive:     user.IsActive,
		IsVerified:   user.IsVerified,
		LockedReason: user.LockedReason,
		LockedAt:     user.LockedAt,
		LastLoginAt:  user.LastLoginAt,
		CreatedAt:    user.CreatedAt,
		SystemRoles:  roleDetails,
	}, nil
}

// userSystemRolesWithDetails — "id" trả về là SystemRoleID (không phải id của bản ghi gán),
// vì FE gọi lại DELETE /users/:user_id/system-roles/:system_role_id với đúng giá trị này.
func (s *UserAdminService) userSystemRolesWithDetails(ctx context.Context, userID uuid.UUID) ([]dto.AdminUserSystemRoleDTO, error) {
	rows, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, model.UserSystemRoleStatusActive)
	if err != nil {
		return nil, err
	}
	result := make([]dto.AdminUserSystemRoleDTO, len(rows))
	for i, r := range rows {
		name := ""
		if r.SystemRole != nil {
			name = r.SystemRole.Name
		}
		result[i] = dto.AdminUserSystemRoleDTO{
			ID:        r.SystemRoleID,
			Name:      name,
			GrantedAt: r.GrantedAt,
		}
	}
	return result, nil
}
