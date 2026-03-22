package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type AuthServiceInterface interface {
	RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error
	Register(ctx context.Context, req dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error)
	Login(ctx context.Context, req dto.LoginRequestDto) (*dto.LoginResponseDto, error)
	SwitchProfile(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchProfileRequestDto) (*dto.SwitchProfileResponseDto, error)
	GetProfiles(ctx context.Context, userID uuid.UUID) ([]dto.ProfileDto, error)
	AddSystemProfile(ctx context.Context, userID uuid.UUID, req dto.AddSystemProfileRequestDto) (*dto.AddSystemProfileResponseDto, error)
	Logout(ctx context.Context, userId, deviceId uuid.UUID) error
	LogoutAllDevice(ctx context.Context, userId uuid.UUID) error
	RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error)
	GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error)
	GetMyProfile(ctx context.Context, userID uuid.UUID, activeRole string, activeOrgID *uuid.UUID) (*dto.MyProfileResponseDto, error)
	GetMySystemRoles(ctx context.Context, userID uuid.UUID) ([]dto.SystemRoleDto, error)
	UpdateMe(ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequestDto) (*dto.UserResponseDto, error)
	GetAllDevices(ctx context.Context, userID, currentDeviceID uuid.UUID) ([]dto.DeviceSessionDto, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequestDto) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error
}

type AuthService struct {
	cfg                *config.Config
	userRepo           repository.UserRepositoryInterface
	roleRepo           repository.RoleRepositoryInterface
	userOrgRoleRepo    repository.UserOrganizationRoleRepositoryInterface
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface
	systemRoleRepo     repository.SystemRoleRepositoryInterface
	redisClient        *redis.Client
}

func NewAuthService(
	cfg *config.Config,
	userRepo repository.UserRepositoryInterface,
	roleRepo repository.RoleRepositoryInterface,
	userOrgRoleRepo repository.UserOrganizationRoleRepositoryInterface,
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
	redisClient *redis.Client,
) *AuthService {
	return &AuthService{
		cfg:                cfg,
		userRepo:           userRepo,
		roleRepo:           roleRepo,
		userOrgRoleRepo:    userOrgRoleRepo,
		userSystemRoleRepo: userSystemRoleRepo,
		systemRoleRepo:     systemRoleRepo,
		redisClient:        redisClient,
	}
}

// PendingRegistration stores registration data temporarily in Redis
// Redis key: auth:register:otp:{email}
// TTL: 5 minutes
type PendingRegistration struct {
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	UserName     string `json:"user_name"`
	FullName     string `json:"full_name,omitempty"`
	RoleID       string `json:"role_id"`
	OTP          string `json:"otp"`
	CreatedAt    string `json:"created_at"`
}

func (s *AuthService) RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error {
	// Validate role_id
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		return errors.New("invalid role_id format")
	}

	roles, err := s.systemRoleRepo.GetSystemRoleByIDs(ctx, []uuid.UUID{roleID})
	if err != nil {
		return fmt.Errorf("failed to validate system role: %w", err)
	}
	if len(roles) == 0 {
		return errors.New("role_id not found")
	}
	if roles[0].Status != "active" {
		return errors.New("system role is not active: " + roles[0].Name)
	}

	existingUser, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return fmt.Errorf("failed to check email: %w", err)
	}
	if existingUser != nil {
		return errors.New("email already registered")
	}

	// Hash password
	passwordHash, err := utils.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Generate OTP (6 digits)
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	pendingData := PendingRegistration{
		Email:        req.Email,
		PasswordHash: passwordHash,
		UserName:     req.UserName,
		FullName:     req.FullName,
		RoleID:       req.RoleID,
		OTP:          otp,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}

	// ===== 8. Save to Redis with TTL (5 minutes) =====
	// Key format: auth:register:otp:{email}
	pendingKey := fmt.Sprintf("auth:register:otp:%s", req.Email)
	pendingBytes, err := json.Marshal(pendingData)
	if err != nil {
		return fmt.Errorf("failed to marshal pending data: %w", err)
	}

	registerOTPTTL := 5 * time.Minute
	if err := s.redisClient.Set(ctx, pendingKey, pendingBytes, registerOTPTTL).Err(); err != nil {
		return fmt.Errorf("failed to save pending registration: %w", err)
	}

	// ===== 9. Send OTP via email (ASYNC) =====
	go func() {
		if err := utils.SendRegisterOTP(s.cfg, req.Email, otp); err != nil {
			log.Printf("[WARN] Failed to send register OTP email to %s: %v", req.Email, err)
		}
	}()

	return nil
}

func (s *AuthService) Register(ctx context.Context, req dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error) {
	// ===== 2. Get pending registration from Redis =====
	pendingKey := fmt.Sprintf("auth:register:otp:%s", req.Email)
	pendingData, err := s.redisClient.Get(ctx, pendingKey).Result()
	if err == redis.Nil {
		return nil, errors.New("registration request not found or expired, please request again")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pending registration: %w", err)
	}

	// ===== 3. Unmarshal pending data =====
	var pending PendingRegistration
	if err := json.Unmarshal([]byte(pendingData), &pending); err != nil {
		return nil, fmt.Errorf("failed to parse pending registration: %w", err)
	}

	if pending.OTP != req.OTP {
		return nil, errors.New("invalid OTP")
	}

	// Prepare FullName pointer
	var fullName *string
	if pending.FullName != "" {
		fullName = &pending.FullName
	}

	// ===== 7. Create user in database =====
	user := &model.User{
		Email:        pending.Email,
		PasswordHash: pending.PasswordHash,
		UserName:     pending.UserName,
		FullName:     fullName,
		IsVerified:   true,
		IsActive:     true,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Gán 1 system role
	roleID, _ := uuid.Parse(pending.RoleID)
	if err := s.userSystemRoleRepo.Create(ctx, &model.UserSystemRole{
		UserID:       user.ID,
		SystemRoleID: roleID,
		GrantedAt:    time.Now(),
		GrantedBy:    nil,
		Status:       model.UserSystemRoleStatusActive,
	}); err != nil {
		return nil, fmt.Errorf("failed to assign system role: %w", err)
	}

	if err := s.redisClient.Del(ctx, pendingKey).Err(); err != nil {
		log.Printf("[WARN] Failed to delete pending registration key: %v", err)
	}

	return &dto.RegisterResponseDto{
		ID:       user.ID.String(),
		Email:    user.Email,
		UserName: user.UserName,
		FullName: fullName,
		RoleID:   pending.RoleID,
	}, nil
}

// GetProfiles trả về danh sách tất cả profiles của user (gộp từ SystemRole + OrgRole)
func (s *AuthService) GetProfiles(ctx context.Context, userID uuid.UUID) ([]dto.ProfileDto, error) {
	var profiles []dto.ProfileDto

	// 1. Lấy system roles
	systemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		log.Printf("[WARN] Failed to load system roles for user %s: %v", userID, err)
	}
	for _, sr := range systemRoles {
		if sr.SystemRole == nil {
			continue
		}
		profiles = append(profiles, dto.ProfileDto{
			ID:          sr.ID,
			Type:        "system",
			DisplayName: sr.SystemRole.Name,
			RoleName:    sr.SystemRole.Name,
		})
	}

	// 2. Lấy org roles
	orgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		log.Printf("[WARN] Failed to load org roles for user %s: %v", userID, err)
	}
	for _, or := range orgRoles {
		if or.Role == nil || or.Organization == nil {
			continue
		}
		orgName := or.Organization.Name
		profiles = append(profiles, dto.ProfileDto{
			ID:               or.ID,
			Type:             "org",
			DisplayName:      or.Role.Name + " " + or.Organization.Name,
			RoleName:         or.Role.Name,
			OrganizationID:   &or.OrganizationID,
			OrganizationName: &orgName,
		})
	}

	return profiles, nil
}

// getLastProfile lấy last profile từ Redis
func (s *AuthService) getLastProfile(ctx context.Context, userID uuid.UUID) *dto.LastProfileDto {
	lastProfileKey := fmt.Sprintf("auth:last_profile:%s", userID)
	data, err := s.redisClient.Get(ctx, lastProfileKey).Result()
	if err != nil {
		return nil
	}
	var lastProfile dto.LastProfileDto
	if err := json.Unmarshal([]byte(data), &lastProfile); err != nil {
		return nil
	}
	return &lastProfile
}

// saveLastProfile lưu last profile vào Redis
func (s *AuthService) saveLastProfile(ctx context.Context, userID uuid.UUID, profile dto.ProfileDto) {
	lastProfile := dto.LastProfileDto{
		Type: profile.Type,
		ID:   profile.ID,
	}
	data, _ := json.Marshal(lastProfile)
	lastProfileKey := fmt.Sprintf("auth:last_profile:%s", userID)
	_ = s.redisClient.Set(ctx, lastProfileKey, data, 0).Err()
}

// findProfileByLastProfile tìm profile trong danh sách dựa trên last profile
func (s *AuthService) findProfileByLastProfile(profiles []dto.ProfileDto, lastProfile *dto.LastProfileDto) *dto.ProfileDto {
	if lastProfile == nil {
		return nil
	}
	for _, p := range profiles {
		if p.Type == lastProfile.Type && p.ID == lastProfile.ID {
			return &p
		}
	}
	return nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req dto.LoginRequestDto,
) (*dto.LoginResponseDto, error) {
	// 1. Validate user
	user, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if user == nil || !utils.CheckPassword(req.Password, user.PasswordHash) {
		return nil, errors.New("invalid email or password")
	}
	if !user.IsActive {
		return nil, errors.New("account is inactive")
	}

	deviceID, err := uuid.Parse(req.DeviceInfo.DeviceID)
	if err != nil {
		return nil, errors.New("invalid device_id format")
	}

	// 2. Lấy danh sách profiles
	profiles, err := s.GetProfiles(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load profiles: %w", err)
	}
	if len(profiles) == 0 {
		return nil, errors.New("user has no active profile assigned")
	}

	// 3. Tự động chọn profile: ưu tiên last_profile, fallback về profile đầu tiên
	lastProfile := s.getLastProfile(ctx, user.ID)
	selectedProfile := s.findProfileByLastProfile(profiles, lastProfile)
	if selectedProfile == nil {
		selectedProfile = &profiles[0]
	}

	// 4. Hoàn tất login
	return s.completeLoginWithProfile(ctx, user, req.DeviceInfo, deviceID, *selectedProfile)
}

// completeLoginWithProfile tạo JWT tokens, lưu session vào Redis
func (s *AuthService) completeLoginWithProfile(
	ctx context.Context,
	user *model.User,
	deviceInfo dto.DeviceInfoDTO,
	deviceID uuid.UUID,
	activeProfile dto.ProfileDto,
) (*dto.LoginResponseDto, error) {
	// Lưu last_profile
	s.saveLastProfile(ctx, user.ID, activeProfile)

	// User version cho token invalidation
	userVersionKey := fmt.Sprintf("auth:user_version:%s", user.ID)
	userVersion := int64(1)
	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == nil {
		userVersion, _ = strconv.ParseInt(userVerStr, 10, 64)
	} else if err == redis.Nil {
		_ = s.redisClient.Set(ctx, userVersionKey, userVersion, 0).Err()
	}

	// Org ID - đã là *uuid.UUID
	activeOrgID := activeProfile.OrganizationID

	// Generate tokens
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, deviceID, activeProfile.RoleName, activeOrgID, userVersion)
	if err != nil {
		return nil, err
	}

	// Lưu refresh token
	refreshKey := fmt.Sprintf("auth:refresh:%s", user.ID)
	if err := s.redisClient.HSet(ctx, refreshKey, deviceID.String(), refreshToken).Err(); err != nil {
		return nil, err
	}
	s.redisClient.Expire(ctx, refreshKey, s.cfg.JWTRefreshExpiration)

	// Lưu device session
	type deviceSession struct {
		DeviceID   uuid.UUID `json:"device_id"`
		DeviceName string    `json:"device_name"`
		UserAgent  string    `json:"user_agent"`
		LoggedInAt string    `json:"logged_in_at"`
	}
	sessionKey := fmt.Sprintf("auth:session:%s", user.ID)
	sessionPayload := deviceSession{
		DeviceID:   deviceID,
		DeviceName: deviceInfo.DeviceName,
		UserAgent:  deviceInfo.UserAgent,
		LoggedInAt: time.Now().Format(time.RFC3339),
	}
	sessionBytes, _ := json.Marshal(sessionPayload)
	_ = s.redisClient.HSet(ctx, sessionKey, deviceID.String(), sessionBytes).Err()

	// Build response
	var dob *string
	if user.DateOfBirth != nil {
		f := user.DateOfBirth.Format("2006-01-02")
		dob = &f
	}

	return &dto.LoginResponseDto{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: &dto.UserResponseDto{
			ID:          user.ID,
			Username:    user.UserName,
			Email:       user.Email,
			Phone:       user.Phone,
			AvatarUrl:   user.AvatarURL,
			DateOfBirth: dob,
			IsActive:    user.IsActive,
			CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		},
		CurrentDevice: &dto.DeviceSessionDto{
			DeviceID:   deviceID.String(),
			DeviceName: deviceInfo.DeviceName,
			UserAgent:  deviceInfo.UserAgent,
			LoggedInAt: time.Now().Format(time.RFC3339),
		},
	}, nil
}

// SwitchProfile đổi profile khi đã đăng nhập
func (s *AuthService) SwitchProfile(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchProfileRequestDto) (*dto.SwitchProfileResponseDto, error) {
	profileID, err := uuid.Parse(req.ProfileID)
	if err != nil {
		return nil, errors.New("invalid profile_id format")
	}

	// Lấy danh sách profiles
	profiles, err := s.GetProfiles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load profiles: %w", err)
	}

	// Tìm profile được chọn
	var selectedProfile *dto.ProfileDto
	for _, p := range profiles {
		if p.Type == req.ProfileType && p.ID == profileID {
			selectedProfile = &p
			break
		}
	}
	if selectedProfile == nil {
		return nil, errors.New("user does not have this profile")
	}

	// Lấy device info từ session
	deviceInfo := dto.DeviceInfoDTO{DeviceID: deviceID.String()}
	sessionKey := fmt.Sprintf("auth:session:%s", userID)
	sessionData, err := s.redisClient.HGet(ctx, sessionKey, deviceID.String()).Result()
	if err == nil {
		var sess struct {
			DeviceName string `json:"device_name"`
			UserAgent  string `json:"user_agent"`
		}
		if json.Unmarshal([]byte(sessionData), &sess) == nil {
			deviceInfo.DeviceName = sess.DeviceName
			deviceInfo.UserAgent = sess.UserAgent
		}
	}

	// Lưu last_profile
	s.saveLastProfile(ctx, userID, *selectedProfile)

	// User version
	userVersionKey := fmt.Sprintf("auth:user_version:%s", userID)
	userVersion := int64(1)
	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == nil {
		userVersion, _ = strconv.ParseInt(userVerStr, 10, 64)
	}

	// Org ID - đã là *uuid.UUID
	activeOrgID := selectedProfile.OrganizationID

	// Generate tokens
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, userID, deviceID, selectedProfile.RoleName, activeOrgID, userVersion)
	if err != nil {
		return nil, err
	}

	// Update refresh token
	refreshKey := fmt.Sprintf("auth:refresh:%s", userID)
	_ = s.redisClient.HSet(ctx, refreshKey, deviceID.String(), refreshToken).Err()
	s.redisClient.Expire(ctx, refreshKey, s.cfg.JWTRefreshExpiration)

	return &dto.SwitchProfileResponseDto{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		ActiveProfile: *selectedProfile,
		CurrentDevice: dto.DeviceSessionDto{
			DeviceID:   deviceID.String(),
			DeviceName: deviceInfo.DeviceName,
			UserAgent:  deviceInfo.UserAgent,
			LoggedInAt: time.Now().Format(time.RFC3339),
		},
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, userId, deviceId uuid.UUID) error {

	// 1. Remove refresh token of this device
	refreshKey := fmt.Sprintf("auth:refresh:%s", userId)
	if err := s.redisClient.HDel(ctx, refreshKey, deviceId.String()).Err(); err != nil {
		return err
	}

	// 2. Remove device session
	sessionKey := fmt.Sprintf("auth:session:%s", userId)
	if err := s.redisClient.HDel(ctx, sessionKey, deviceId.String()).Err(); err != nil {
		return err
	}

	return nil
}

func (s *AuthService) LogoutAllDevice(ctx context.Context, userId uuid.UUID) error {
	return s.revokeAllSessions(ctx, userId)
}

func (s *AuthService) RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error) {

	// ===== 1. Parse old refresh token =====
	claims, err := utils.ParseToken(s.cfg, oldRefreshToken)
	if err != nil {
		return nil, errors.New("invalid or expired refresh token")
	}

	// ===== 2. Check user_version (for logout all) =====
	userVersionKey := fmt.Sprintf("auth:user_version:%s", claims.UserID)
	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == redis.Nil {
		return nil, errors.New("user session not found - please login again")
	}
	if err != nil {
		return nil, err
	}

	currentUserVersion, _ := strconv.ParseInt(userVerStr, 10, 64)
	if currentUserVersion != claims.UserVersion {
		return nil, errors.New("all sessions revoked - please login again")
	}

	// ===== 3. Check if refresh token exists in Redis (Using HGET) =====
	// Key used in Login: auth:refresh:{userID}
	refreshTokenKey := fmt.Sprintf("auth:refresh:%s", claims.UserID)
	storedToken, err := s.redisClient.HGet(ctx, refreshTokenKey, claims.DeviceID.String()).Result()
	if err == redis.Nil {
		return nil, errors.New("refresh token not found - please login again")
	}
	if err != nil {
		return nil, err
	}

	// ===== 4. Verify token matches =====
	if storedToken != oldRefreshToken {
		return nil, errors.New("refresh token mismatch - please login again")
	}

	// ===== 5. Generate new tokens (with same userVersion) =====
	newAccessToken, newRefreshToken, err := utils.GenerateTokens(s.cfg, claims.UserID, claims.DeviceID, claims.ActiveRole, claims.ActiveOrgID, currentUserVersion)
	if err != nil {
		return nil, err
	}

	// ===== 6. Update refresh token in Redis (Using HSET) =====
	if err := s.redisClient.HSet(ctx, refreshTokenKey, claims.DeviceID.String(), newRefreshToken).Err(); err != nil {
		return nil, err
	}
	// Extend TTL for the user's session
	s.redisClient.Expire(ctx, refreshTokenKey, s.cfg.JWTRefreshExpiration)

	return &dto.RefreshTokenResponseDto{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error) {
	userCacheKey := fmt.Sprintf("auth:user_cache:%s", userID)

	// ===== 1. Check Redis cache first (if redis is available) =====
	if s.redisClient != nil {
		cachedUser, err := s.redisClient.Get(ctx, userCacheKey).Result()
		if err == nil {
			// Found in cache → unmarshal and return
			var userResponse dto.UserResponseDto
			if err := json.Unmarshal([]byte(cachedUser), &userResponse); err == nil {
				return &userResponse, nil
			}
			// If unmarshal fails, continue to fetch from database
		} else if err != redis.Nil {
			// Log Redis error but continue to database (graceful degradation)
			// Don't return error - fallback to database
		}
	}

	// ===== 2. Not in cache → fetch from database =====
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, errors.New("user not found")
	}

	// // ===== 3. Build response =====
	// status := "inactive"
	// if user.IsActive {
	// 	status = "active"
	// }

	var dob *string
	if user.DateOfBirth != nil {
		f := user.DateOfBirth.Format("2006-01-02")
		dob = &f
	}

	userResponse := &dto.UserResponseDto{
		ID:          user.ID,
		Username:    user.UserName,
		Email:       user.Email,
		FullName:    user.FullName,
		Phone:       user.Phone,
		AvatarUrl:   user.AvatarURL,
		DateOfBirth: dob,
		Bio:         user.Bio,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt.Format(time.RFC3339),
	}

	// ===== 4. Cache response in Redis (if available) =====
	if s.redisClient != nil {
		userBytes, err := json.Marshal(userResponse)
		if err == nil {
			cacheTTL := 30 * time.Minute
			_ = s.redisClient.Set(ctx, userCacheKey, userBytes, cacheTTL).Err()
		}
	}

	return userResponse, nil
}

// GetMyProfile trả về full profile: user info + system roles + organizations + active context
func (s *AuthService) GetMyProfile(ctx context.Context, userID uuid.UUID, activeRole string, activeOrgID *uuid.UUID) (*dto.MyProfileResponseDto, error) {
	// 1. Get user info
	userDto, err := s.GetMe(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 2. Get system roles
	systemRoles, err := s.GetMySystemRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system roles: %w", err)
	}

	// 3. Get organizations
	orgs, err := s.getUserOrgs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations: %w", err)
	}

	// Convert OrgContextDto to MyOrganizationDto
	orgDtos := make([]dto.MyOrganizationDto, len(orgs))
	for i, org := range orgs {
		orgDtos[i] = dto.MyOrganizationDto{
			ID:   org.ID,
			Name: org.Name,
		}
	}

	// 4. Build active role context
	var activeRoleDto *dto.SystemRoleDto
	for _, sr := range systemRoles {
		if sr.Name == activeRole {
			r := sr
			activeRoleDto = &r
			break
		}
	}

	// 5. Build active org context
	var activeOrgDto *dto.OrgContextDto
	if activeOrgID != nil {
		for _, org := range orgs {
			if org.ID == activeOrgID.String() {
				o := org
				activeOrgDto = &o
				break
			}
		}
	}

	return &dto.MyProfileResponseDto{
		User:          *userDto,
		SystemRoles:   systemRoles,
		Organizations: orgDtos,
		ActiveRole:    activeRoleDto,
		ActiveOrg:     activeOrgDto,
	}, nil
}

// GetMySystemRoles trả về danh sách system roles của user
func (s *AuthService) GetMySystemRoles(ctx context.Context, userID uuid.UUID) ([]dto.SystemRoleDto, error) {
	roles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, err
	}

	roleDtos := make([]dto.SystemRoleDto, len(roles))
	for i, sr := range roles {
		roleDtos[i] = dto.SystemRoleDto{
			ID:   sr.SystemRole.ID.String(),
			Name: sr.SystemRole.Name,
		}
	}
	return roleDtos, nil
}

func (s *AuthService) UpdateMe(ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequestDto) (*dto.UserResponseDto, error) {
	// ===== 1. Build updates map (only non-nil fields) =====
	updates := make(map[string]interface{})

	if req.Username != nil {
		updates["user_name"] = *req.Username
	}

	if req.FullName != nil {
		updates["full_name"] = *req.FullName
	}

	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}

	if req.Bio != nil {
		updates["bio"] = *req.Bio
	}

	if req.AvatarURL != nil {
		updates["avatar_url"] = *req.AvatarURL
	}

	if req.DateOfBirth != nil {
		// Parse date string to time.Time
		dob, err := time.Parse("2006-01-02", *req.DateOfBirth)
		if err != nil {
			return nil, errors.New("invalid date_of_birth format, expected YYYY-MM-DD")
		}
		updates["date_of_birth"] = dob
	}

	// ===== 2. Check if there's anything to update =====
	if len(updates) == 0 {
		return nil, errors.New("no fields to update")
	}

	// ===== 3. Update in database =====
	if err := s.userRepo.UpdateUserProfile(ctx, userID, updates); err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// ===== 4. Invalidate cache (graceful - ignore errors) =====
	if s.redisClient != nil {
		userCacheKey := fmt.Sprintf("auth:user_cache:%s", userID)
		_ = s.redisClient.Del(ctx, userCacheKey).Err()
	}

	// ===== 5. Fetch and return updated user =====
	return s.GetMe(ctx, userID)
}

func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequestDto) error {
	// ===== 1. Validate: Get user from database =====
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return errors.New("user not found")
	}

	// ===== 2. Validate: Check old password =====
	if !utils.CheckPassword(req.OldPassword, user.PasswordHash) {
		return errors.New("incorrect current password")
	}

	// ===== 3. Update: Hash and save new password =====
	newPasswordHash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.userRepo.UpdatePasswordHash(ctx, userID, newPasswordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// ===== 4. Cache: Invalidate user cache =====
	if s.redisClient != nil {
		userCacheKey := fmt.Sprintf("auth:user_cache:%s", userID)
		_ = s.redisClient.Del(ctx, userCacheKey).Err() // Ignore error - cache miss is acceptable
	}

	// ===== 5. Revoke sessions (optional - based on FE flag) =====
	if req.RevokeOthers {
		if err := s.revokeAllSessions(ctx, userID); err != nil {
			return fmt.Errorf("failed to revoke sessions: %w", err)
		}
	}

	return nil
}

// revokeAllSessions invalidates all tokens and clears all device sessions for a user.
// Used when: change password with revoke_others=true, security breach, etc.
func (s *AuthService) revokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	if s.redisClient == nil {
		return nil
	}

	// 1. INCR user_version → all access tokens become invalid immediately
	userVersionKey := fmt.Sprintf("auth:user_version:%s", userID)
	if err := s.redisClient.Incr(ctx, userVersionKey).Err(); err != nil {
		return err
	}

	// 2. DEL auth:refresh:{userId} → remove all refresh tokens (HASH)
	refreshKey := fmt.Sprintf("auth:refresh:%s", userID)
	_ = s.redisClient.Del(ctx, refreshKey).Err()

	// 3. DEL session:{userId} → remove all device sessions (HASH)
	sessionKey := fmt.Sprintf("auth:session:%s", userID)
	_ = s.redisClient.Del(ctx, sessionKey).Err()

	return nil
}

// PasswordResetOTP represents the OTP data stored in Redis
type PasswordResetOTP struct {
	OTP       string `json:"otp"`
	Attempt   int    `json:"attempt"`
	ExpiredAt int64  `json:"expired_at"`
}

const (
	maxOTPAttempts = 5
	otpTTL         = 5 * time.Minute
)

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	// ===== 1. Find user by email (don't reveal if user exists) =====
	user, err := s.userRepo.FindUserByEmail(ctx, email)
	if err != nil {
		// Log error internally but return generic message
		return nil // Don't reveal if email exists
	}

	// If user not found or inactive, return nil (don't reveal)
	if user == nil || !user.IsActive {
		return nil
	}

	// ===== 2. Generate 6-digit OTP =====
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return errors.New("failed to generate OTP")
	}

	// ===== 5. Store OTP in Redis =====
	otpKey := fmt.Sprintf("auth:password_reset:otp:%s", user.ID)
	otpData := PasswordResetOTP{
		OTP:       otp,
		Attempt:   0,
		ExpiredAt: time.Now().Add(otpTTL).Unix(),
	}

	otpBytes, err := json.Marshal(otpData)
	if err != nil {
		return errors.New("failed to create OTP data")
	}

	// Overwrites any existing OTP for this user
	if err := s.redisClient.Set(ctx, otpKey, otpBytes, otpTTL).Err(); err != nil {
		return errors.New("failed to store OTP")
	}

	// ===== 6. Send OTP via email (ASYNC) =====
	go func() {
		if err := utils.SendResetPasswordOTP(s.cfg, email, otp); err != nil {
			log.Printf("[WARN] Failed to send password reset email to %s: %v", email, err)
		}
	}()

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error {
	// ===== 1. Find user by email =====
	user, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return errors.New("invalid request")
	}
	if user == nil {
		return errors.New("invalid request")
	}

	// ===== 2. Get OTP from Redis =====
	otpKey := fmt.Sprintf("auth:password_reset:otp:%s", user.ID)
	otpBytes, err := s.redisClient.Get(ctx, otpKey).Result()
	if err == redis.Nil {
		return errors.New("OTP not found or expired")
	}
	if err != nil {
		return errors.New("failed to verify OTP")
	}

	var otpData PasswordResetOTP
	if err := json.Unmarshal([]byte(otpBytes), &otpData); err != nil {
		return errors.New("invalid OTP data")
	}

	// ===== 3. Check if OTP expired =====
	if time.Now().Unix() > otpData.ExpiredAt {
		_ = s.redisClient.Del(ctx, otpKey).Err()
		return errors.New("OTP has expired")
	}

	// ===== 4. Check attempt limit =====
	if otpData.Attempt >= maxOTPAttempts {
		_ = s.redisClient.Del(ctx, otpKey).Err()
		return errors.New("too many failed attempts, please request a new OTP")
	}

	// ===== 5. Verify OTP =====
	if otpData.OTP != req.Otp {
		// Increment attempt count
		otpData.Attempt++

		if otpData.Attempt >= maxOTPAttempts {
			// Max attempts reached - delete OTP
			_ = s.redisClient.Del(ctx, otpKey).Err()
			return errors.New("too many failed attempts, please request a new OTP")
		}

		// Update attempt count in Redis
		updatedBytes, _ := json.Marshal(otpData)
		remainingTTL := time.Until(time.Unix(otpData.ExpiredAt, 0))
		_ = s.redisClient.Set(ctx, otpKey, updatedBytes, remainingTTL).Err()

		return errors.New("invalid OTP")
	}

	// ===== 6. OTP valid - Hash new password =====
	newPasswordHash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return errors.New("failed to process new password")
	}

	// ===== 7. Update password in database =====
	if err := s.userRepo.UpdatePasswordHash(ctx, user.ID, newPasswordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// ===== 8. Delete OTP from Redis =====
	_ = s.redisClient.Del(ctx, otpKey).Err()

	// // ===== 9. Invalidate all sessions (force re-login on all devices) =====
	// if err := s.revokeAllSessions(ctx, user.ID); err != nil {
	// 	log.Printf("[WARN] Failed to revoke sessions for user %s: %v", user.ID, err)
	// } optional

	// ===== 10. Invalidate user cache =====
	userCacheKey := fmt.Sprintf("auth:user_cache:%s", user.ID)
	_ = s.redisClient.Del(ctx, userCacheKey).Err()

	return nil
}

// AddSystemProfile thêm system profile mới cho user (TEACHER, STUDENT, PARENT, ORG_OWNER)
func (s *AuthService) AddSystemProfile(ctx context.Context, userID uuid.UUID, req dto.AddSystemProfileRequestDto) (*dto.AddSystemProfileResponseDto, error) {
	// 1. Parse và validate system_role_id
	systemRoleID, err := uuid.Parse(req.SystemRoleID)
	if err != nil {
		return nil, errors.New("invalid system_role_id format")
	}

	// 2. Kiểm tra system role có tồn tại và active không
	roles, err := s.systemRoleRepo.GetSystemRoleByIDs(ctx, []uuid.UUID{systemRoleID})
	if err != nil {
		return nil, fmt.Errorf("failed to validate system role: %w", err)
	}
	if len(roles) == 0 {
		return nil, errors.New("system role not found")
	}
	if roles[0].Status != "active" {
		return nil, errors.New("system role is not active")
	}

	// 3. Kiểm tra user đã có role này chưa
	existingRole, err := s.userSystemRoleRepo.FindByUserAndSystemRole(ctx, userID, systemRoleID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing role: %w", err)
	}
	if existingRole != nil {
		return nil, errors.New("user already has this system role")
	}

	// 4. Tạo UserSystemRole mới
	newUserSystemRole := &model.UserSystemRole{
		UserID:       userID,
		SystemRoleID: systemRoleID,
		GrantedAt:    time.Now(),
		GrantedBy:    nil,
		Status:       model.UserSystemRoleStatusActive,
	}
	if err := s.userSystemRoleRepo.Create(ctx, newUserSystemRole); err != nil {
		return nil, fmt.Errorf("failed to create user system role: %w", err)
	}

	// 5. Build profile mới
	newProfile := dto.ProfileDto{
		ID:          newUserSystemRole.ID,
		Type:        "system",
		DisplayName: roles[0].Name,
		RoleName:    roles[0].Name,
	}

	// 6. Lấy danh sách tất cả profiles
	profiles, err := s.GetProfiles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profiles: %w", err)
	}

	return &dto.AddSystemProfileResponseDto{
		Profile:  newProfile,
		Profiles: profiles,
	}, nil
}
