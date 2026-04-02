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
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type AuthServiceInterface interface {
	// Auth
	RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error
	Register(ctx context.Context, req dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error)
	Login(ctx context.Context, req dto.LoginRequestDto) (*dto.LoginResponseDto, error)
	RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error)
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error
	// Role selection
	GetMyRoles(ctx context.Context, userID uuid.UUID) (*dto.GetMyRolesResponseDto, error)
	SelectRole(ctx context.Context, req dto.SelectRoleRequestDto) (*dto.SelectRoleResponseDto, error)
	SwitchRole(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchRoleRequestDto) (*dto.SelectRoleResponseDto, error)
	// Profile management
	GetSystemRoleOptions(ctx context.Context) ([]dto.SystemRoleOptionDto, error)
	GetMyProfiles(ctx context.Context, userID uuid.UUID) ([]dto.ProfileDto, error)
	CreateProfile(ctx context.Context, userID uuid.UUID, req dto.CreateProfileRequestDto) (*dto.ProfileDto, error)
	DeleteProfile(ctx context.Context, userID uuid.UUID, profileID uuid.UUID) error
	// User info
	GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error)
	UpdateMe(ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequestDto) (*dto.UserResponseDto, error)
	// Session
	GetAllDevices(ctx context.Context, userID, currentDeviceID uuid.UUID) ([]dto.DeviceSessionDto, error)
	Logout(ctx context.Context, userId, deviceId uuid.UUID) error
	LogoutAllDevice(ctx context.Context, userId uuid.UUID) error
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequestDto) error
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

// ==================== LOGIN ATTEMPT TRACKING ====================
// Security: Prevents brute force attacks by locking accounts after too many failed attempts

const (
	maxLoginAttempts    = 5
	loginLockoutMinutes = 15
	loginWindowMinutes  = 5
)

func (s *AuthService) recordFailedLogin(ctx context.Context, email string) {
	attemptsKey := constants.KeyLoginAttempts(email)
	lockKey := constants.KeyLoginLocked(email)

	count, err := s.redisClient.Incr(ctx, attemptsKey).Result()
	if err != nil {
		log.Printf("[WARN] Failed to record login attempt for %s: %v", email, err)
		return
	}

	// Set expiry on first attempt
	if count == 1 {
		s.redisClient.Expire(ctx, attemptsKey, time.Duration(loginWindowMinutes)*time.Minute)
	}

	// Lock account if max attempts exceeded
	if int(count) >= maxLoginAttempts {
		s.redisClient.Set(ctx, lockKey, "1", time.Duration(loginLockoutMinutes)*time.Minute)
		log.Printf("[SECURITY] Account locked due to %d failed login attempts: %s", count, email)
	}
}

// clearFailedLogin removes failed attempt counter on successful login
func (s *AuthService) clearFailedLogin(ctx context.Context, email string) {
	attemptsKey := constants.KeyLoginAttempts(email)
	s.redisClient.Del(ctx, attemptsKey)
}

// PendingRegistration stores registration data temporarily in Redis
// Redis key: register:otp:{email}
// TTL: 5 minutes
// Security: OTP is stored as hash (SHA256 with email salt) to prevent plaintext exposure
type PendingRegistration struct {
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	UserName     string `json:"user_name"`
	FullName     string `json:"full_name,omitempty"`
	RoleID       string `json:"role_id"`
	OTPHash      string `json:"otp_hash"` // Hashed OTP, not plaintext
	Attempts     int    `json:"attempts"` // OTP verification attempts
	CreatedAt    string `json:"created_at"`
}

func (s *AuthService) RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error {
	// Validate system role
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		return fmt.Errorf("invalid role_id: %s", req.RoleID)
	}
	roles, err := s.systemRoleRepo.GetSystemRoleByIDs(ctx, []uuid.UUID{roleID})
	if err != nil {
		return fmt.Errorf("failed to validate system role: %w", err)
	}
	if len(roles) != 1 {
		return errors.New("role ID is invalid")
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

	// ===== 5. Hash password =====
	passwordHash, err := utils.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Hash OTP before storing (security: prevent plaintext exposure if Redis is compromised)
	otpHash := utils.HashOTP(otp, req.Email)

	pendingData := PendingRegistration{
		Email:        req.Email,
		PasswordHash: passwordHash,
		UserName:     req.UserName,
		FullName:     req.FullName,
		RoleID:       req.RoleID,
		OTPHash:      otpHash,
		Attempts:     0,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}

	// ===== 8. Save to Redis with TTL (5 minutes) =====
	// Key format: register:otp:{email}
	pendingKey := constants.KeyRegisterOTP(req.Email)
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
	pendingKey := constants.KeyRegisterOTP(req.Email)
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

	// ===== 4. Check OTP attempts (max 5) =====
	const maxOTPAttempts = 5
	if pending.Attempts >= maxOTPAttempts {
		// Delete pending registration to force user to request new OTP
		_ = s.redisClient.Del(ctx, pendingKey).Err()
		return nil, errors.New("too many failed attempts, please request a new OTP")
	}

	// ===== 5. Verify OTP using constant-time comparison =====
	if !utils.VerifyOTP(req.OTP, pending.OTPHash, req.Email) {
		// Increment attempts and save back
		pending.Attempts++
		pendingBytes, _ := json.Marshal(pending)
		ttl, _ := s.redisClient.TTL(ctx, pendingKey).Result()
		if ttl > 0 {
			_ = s.redisClient.Set(ctx, pendingKey, pendingBytes, ttl).Err()
		}
		remaining := maxOTPAttempts - pending.Attempts
		return nil, fmt.Errorf("invalid OTP, %d attempts remaining", remaining)
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

	// Assign the selected system role to user
	roleID, _ := uuid.Parse(pending.RoleID)
	now := time.Now()
	_ = s.userSystemRoleRepo.Create(ctx, &model.UserSystemRole{
		UserID:       user.ID,
		SystemRoleID: roleID,
		GrantedAt:    now,
		GrantedBy:    nil,
		Status:       model.UserSystemRoleStatusActive,
	})

	_ = s.redisClient.Del(ctx, pendingKey).Err()

	return &dto.RegisterResponseDto{
		ID:       user.ID.String(),
		Email:    user.Email,
		UserName: user.UserName,
		FullName: fullName,
		RoleID:   pending.RoleID,
	}, nil
}

// PendingLogin stores login data temporarily in Redis while user selects a profile/org
type PendingLogin struct {
	UserID       string              `json:"user_id"`
	DeviceInfo   dto.DeviceInfoDTO   `json:"device_info"`
	SystemRoles  []dto.SystemRoleDto `json:"system_roles"`
	SelectedRole *dto.SystemRoleDto  `json:"selected_role,omitempty"`
	CreatedAt    string              `json:"created_at"`
}

func (s *AuthService) Login(
	ctx context.Context,
	req dto.LoginRequestDto,
) (*dto.LoginResponseDto, error) {

	// ===== 1. Check if account is locked due to failed attempts =====
	lockKey := constants.KeyLoginLocked(req.Email)
	if locked, _ := s.redisClient.Exists(ctx, lockKey).Result(); locked > 0 {
		ttl, _ := s.redisClient.TTL(ctx, lockKey).Result()
		return nil, fmt.Errorf("account temporarily locked due to too many failed attempts, try again in %d minutes", int(ttl.Minutes())+1)
	}

	// ===== 2. Validate credentials =====
	user, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}

	// Track failed login attempts
	if user == nil || !utils.CheckPassword(req.Password, user.PasswordHash) {
		s.recordFailedLogin(ctx, req.Email)
		return nil, errors.New("invalid email or password")
	}

	if !user.IsActive {
		return nil, errors.New("account is inactive")
	}

	// ===== 3. Clear failed attempts on successful login =====
	s.clearFailedLogin(ctx, req.Email)

	_, err = uuid.Parse(req.DeviceInfo.DeviceID)
	if err != nil {
		return nil, errors.New("invalid device_id format")
	}

	systemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, user.ID, "active")
	if err != nil {
		log.Printf("[WARN] Failed to load system roles for user %s: %v", user.ID, err)
		systemRoles = []model.UserSystemRole{}
	}

	systemRoleDtos := make([]dto.SystemRoleDto, len(systemRoles))
	for i, sr := range systemRoles {
		systemRoleDtos[i] = dto.SystemRoleDto{
			ID:   sr.SystemRole.ID.String(),
			Name: sr.SystemRole.Name,
		}
	}

	// NEW: Build unified roles list (system + organization roles)
	unifiedRoles, err := s.buildUnifiedRoles(ctx, user.ID)
	if err != nil {
		log.Printf("[WARN] Failed to build unified roles for user %s: %v", user.ID, err)
		unifiedRoles = []dto.UnifiedRoleDto{}
	}

	// User có nhiều roles → chưa chọn role, trả session_token
	if len(unifiedRoles) > 1 {
		sessionToken := uuid.New().String()
		pending := PendingLogin{
			UserID:      user.ID.String(),
			DeviceInfo:  req.DeviceInfo,
			SystemRoles: systemRoleDtos,
			CreatedAt:   time.Now().Format(time.RFC3339),
		}
		pendingBytes, err := json.Marshal(pending)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal pending login: %w", err)
		}
		pendingKey := constants.KeyPendingLogin(sessionToken)
		if err := s.redisClient.Set(ctx, pendingKey, pendingBytes, 5*time.Minute).Err(); err != nil {
			return nil, fmt.Errorf("failed to save pending login: %w", err)
		}
		// trả về khi user có nhiều hơn 1 role để chọn, client sẽ gọi API chọn role tiếp theo
		return &dto.LoginResponseDto{
			Completed:    true,
			SessionToken: sessionToken,
			Roles:        unifiedRoles,
		}, nil
	}

	if len(unifiedRoles) == 0 {
		return nil, errors.New("user has no active role assigned")
	}

	singleRole := unifiedRoles[0]
	result, err := s.completeLoginUnified(ctx, user, req.DeviceInfo, singleRole)
	if err != nil {
		return nil, err
	}

	// trả về khi user chỉ có 1 role duy nhất, auto-login luôn mà không cần chọn role nữa
	return &dto.LoginResponseDto{
		Completed:     true,
		Roles:         unifiedRoles,
		AccessToken:   result.AccessToken,
		RefreshToken:  result.RefreshToken,
		User:          &result.User,
		ActiveRole:    &singleRole,
		CurrentDevice: result.CurrentDevice,
	}, nil
}

func (s *AuthService) getUserOrgs(ctx context.Context, userID uuid.UUID) ([]dto.OrgContextDto, error) {
	userOrgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var orgs []dto.OrgContextDto
	for _, uor := range userOrgRoles {
		if uor.Organization == nil {
			continue
		}
		orgID := uor.OrganizationID.String()
		if seen[orgID] {
			continue
		}
		seen[orgID] = true
		orgs = append(orgs, dto.OrgContextDto{
			ID:   orgID,
			Name: uor.Organization.Name,
		})
	}
	return orgs, nil
}

func (s *AuthService) tryAutoSelectOrg(ctx context.Context, userID uuid.UUID) ([]dto.OrgContextDto, error) {
	orgs, err := s.getUserOrgs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, nil
	}
	return orgs, nil
}

// completeLogin tạo JWT tokens, lưu session vào Redis, trả SelectProfileResponseDto
func (s *AuthService) completeLogin(
	ctx context.Context,
	user *model.User,
	deviceInfo dto.DeviceInfoDTO,
	activeRole dto.SystemRoleDto,
	activeOrg *dto.OrgContextDto,
) (*dto.SelectRoleResponseDto, error) {

	deviceID, _ := uuid.Parse(deviceInfo.DeviceID)

	userVersionKey := constants.KeyUserVersion(user.ID.String())
	userVersion := int64(1)
	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == nil {
		userVersion, _ = strconv.ParseInt(userVerStr, 10, 64)
	} else if err == redis.Nil {
		_ = s.redisClient.Set(ctx, userVersionKey, userVersion, 0).Err()
	}

	var activeOrgID *uuid.UUID
	if activeOrg != nil {
		parsed, _ := uuid.Parse(activeOrg.ID)
		activeOrgID = &parsed
	}

	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, deviceID, activeRole.Name, activeOrgID, userVersion)
	if err != nil {
		return nil, err
	}

	refreshKey := constants.KeyRefresh(user.ID.String())
	if err := s.redisClient.HSet(ctx, refreshKey, deviceID.String(), refreshToken).Err(); err != nil {
		return nil, err
	}
	s.redisClient.Expire(ctx, refreshKey, s.cfg.JWTRefreshExpiration)

	type deviceSession struct {
		DeviceID   uuid.UUID `json:"device_id"`
		DeviceName string    `json:"device_name"`
		UserAgent  string    `json:"user_agent"`
		LoggedInAt string    `json:"logged_in_at"`
	}
	sessionKey := constants.KeySession(user.ID.String())
	sessionPayload := deviceSession{
		DeviceID:   deviceID,
		DeviceName: deviceInfo.DeviceName,
		UserAgent:  deviceInfo.UserAgent,
		LoggedInAt: time.Now().Format(time.RFC3339),
	}
	sessionBytes, err := json.Marshal(sessionPayload)
	if err != nil {
		return nil, err
	}
	if err := s.redisClient.HSet(ctx, sessionKey, deviceID.String(), sessionBytes).Err(); err != nil {
		return nil, err
	}

	var dob *string
	if user.DateOfBirth != nil {
		f := user.DateOfBirth.Format("2006-01-02")
		dob = &f
	}

	return &dto.SelectRoleResponseDto{
		Completed:    true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserResponseDto{
			ID:          user.ID,
			Username:    user.UserName,
			Email:       user.Email,
			Phone:       user.Phone,
			AvatarUrl:   user.AvatarURL,
			DateOfBirth: dob,
			IsActive:    user.IsActive,
			CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		},
		ActiveRole: dto.UnifiedRoleDto{
			ID:          activeRole.ID,
			Type:        "system",
			RoleName:    activeRole.Name,
			DisplayName: activeRole.Name,
		},
		ActiveOrg:   activeOrg,
		CurrentDevice: &dto.DeviceSessionDto{
			DeviceID:   deviceID.String(),
			DeviceName: deviceInfo.DeviceName,
			UserAgent:  deviceInfo.UserAgent,
			LoggedInAt: time.Now().Format(time.RFC3339),
		},
	}, nil
}

// finishLoginWithOrgCheck kiểm tra org context sau khi đã chọn role.
// 0 org → hoàn tất login (active_org=null). 1+ org → yêu cầu chọn org (hoặc "Độc lập").
func (s *AuthService) finishLoginWithOrgCheck(
	ctx context.Context,
	user *model.User,
	deviceInfo dto.DeviceInfoDTO,
	selectedRole dto.SystemRoleDto,
	allRoles []dto.SystemRoleDto,
) (*dto.LoginResponseDto, error) {
	orgs, err := s.tryAutoSelectOrg(ctx, user.ID)
	if err != nil {
		log.Printf("[WARN] Failed to check orgs for user %s: %v", user.ID, err)
	}

	// Convert SystemRoleDto to UnifiedRoleDto for new flow
	activeRoleUnified := dto.UnifiedRoleDto{
		ID:          selectedRole.ID,
		Type:        "system",
		RoleName:    selectedRole.Name,
		DisplayName: selectedRole.Name,
	}

	// User thuộc 1+ org → yêu cầu chọn (org hoặc "Độc lập") - DEPRECATED flow
	if len(orgs) > 0 {
		sessionToken := uuid.New().String()
		pending := PendingLogin{
			UserID:       user.ID.String(),
			DeviceInfo:   deviceInfo,
			SystemRoles:  allRoles,
			SelectedRole: &selectedRole,
			CreatedAt:    time.Now().Format(time.RFC3339),
		}
		pendingBytes, _ := json.Marshal(pending)
		pendingKey := constants.KeyPendingLogin(sessionToken)
		if err := s.redisClient.Set(ctx, pendingKey, pendingBytes, 5*time.Minute).Err(); err != nil {
			return nil, fmt.Errorf("failed to save pending login: %w", err)
		}
		return &dto.LoginResponseDto{
			Completed:            false,
			SessionToken:         sessionToken,
			RequiresOrgSelection: true,
			Organizations:        orgs,
			ActiveRole:           &activeRoleUnified,
		}, nil
	}

	// 0 org → hoàn tất ngay với active_org=null
	result, err := s.completeLogin(ctx, user, deviceInfo, selectedRole, nil)
	if err != nil {
		return nil, err
	}
	entryCtx := s.determineEntryContext(allRoles)
	return &dto.LoginResponseDto{
		Completed:     true,
		AccessToken:   result.AccessToken,
		RefreshToken:  result.RefreshToken,
		User:          &result.User,
		ActiveRole:    &activeRoleUnified,
		EntryContext:  entryCtx,
		CurrentDevice: result.CurrentDevice,
	}, nil
}

func (s *AuthService) SelectProfile(ctx context.Context, req dto.SelectProfileRequestDto) (*dto.SelectRoleResponseDto, error) {
	pendingKey := constants.KeyPendingLogin(req.SessionToken)
	pendingData, err := s.redisClient.Get(ctx, pendingKey).Result()
	if err == redis.Nil {
		return nil, errors.New("session expired or invalid, please login again")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pending login: %w", err)
	}

	var pending PendingLogin
	if err := json.Unmarshal([]byte(pendingData), &pending); err != nil {
		return nil, fmt.Errorf("failed to parse pending login: %w", err)
	}

	var selectedRole *dto.SystemRoleDto
	for _, role := range pending.SystemRoles {
		if role.ID == req.SystemRoleID {
			selectedRole = &role
			break
		}
	}
	if selectedRole == nil {
		return nil, errors.New("invalid system_role_id: user does not have this role")
	}

	userID, _ := uuid.Parse(pending.UserID)
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	// Kiểm tra org context
	orgs, err := s.tryAutoSelectOrg(ctx, userID)
	if err != nil {
		log.Printf("[WARN] Failed to check orgs for user %s: %v", userID, err)
	}

	if len(orgs) > 0 {
		// Có org → cập nhật pending với role đã chọn, yêu cầu chọn org (hoặc "Độc lập")
		pending.SelectedRole = selectedRole
		pendingBytes, _ := json.Marshal(pending)
		_ = s.redisClient.Set(ctx, pendingKey, pendingBytes, 5*time.Minute).Err()

		return &dto.SelectRoleResponseDto{
			Completed:            false,
			SessionToken:         req.SessionToken,
			RequiresOrgSelection: true,
			Organizations:        orgs,
			ActiveRole: dto.UnifiedRoleDto{
				ID:          selectedRole.ID,
				Type:        "system",
				RoleName:    selectedRole.Name,
				DisplayName: selectedRole.Name,
			},
		}, nil
	}

	// 0 org → hoàn tất với active_org=null
	result, err := s.completeLogin(ctx, user, pending.DeviceInfo, *selectedRole, nil)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(pending.SystemRoles)
	_ = s.redisClient.Del(ctx, pendingKey).Err()
	return result, nil
}

func (s *AuthService) SwitchProfile(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchProfileRequestDto) (*dto.SelectRoleResponseDto, error) {
	roleID, err := uuid.Parse(req.ProfileID)
	if err != nil {
		return nil, errors.New("invalid profile_id format")
	}

	userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to load system roles: %w", err)
	}

	var selectedRole *dto.SystemRoleDto
	allRoles := make([]dto.SystemRoleDto, len(userSystemRoles))
	for i, sr := range userSystemRoles {
		allRoles[i] = dto.SystemRoleDto{
			ID:   sr.SystemRole.ID.String(),
			Name: sr.SystemRole.Name,
		}
		if sr.SystemRole.ID == roleID {
			selectedRole = &allRoles[i]
		}
	}
	if selectedRole == nil {
		return nil, errors.New("user does not have this system role")
	}

	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	deviceInfo := dto.DeviceInfoDTO{DeviceID: deviceID.String()}
	sessionKey := constants.KeySession(userID.String())
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

	// Khi switch role → check org
	orgs, _ := s.tryAutoSelectOrg(ctx, userID)
	if len(orgs) > 0 {
		// Có org → trả danh sách để FE chọn org hoặc "Độc lập", gọi switch-org
		return &dto.SelectRoleResponseDto{
			Completed:            false,
			RequiresOrgSelection: true,
			Organizations:        orgs,
			ActiveRole: dto.UnifiedRoleDto{
				ID:          selectedRole.ID,
				Type:        "system",
				RoleName:    selectedRole.Name,
				DisplayName: selectedRole.Name,
			},
		}, nil
	}

	// 0 org → hoàn tất với active_org=null
	result, err := s.completeLogin(ctx, user, deviceInfo, *selectedRole, nil)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(allRoles)
	return result, nil
}

// SelectOrg chọn org sau khi đã chọn role (dùng session_token).
// organization_id rỗng = chọn chế độ "Độc lập" (active_org=null).
func (s *AuthService) SelectOrg(ctx context.Context, req dto.SelectOrgRequestDto) (*dto.SelectRoleResponseDto, error) {
	pendingKey := constants.KeyPendingLogin(req.SessionToken)
	pendingData, err := s.redisClient.Get(ctx, pendingKey).Result()
	if err == redis.Nil {
		return nil, errors.New("session expired or invalid, please login again")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pending login: %w", err)
	}

	var pending PendingLogin
	if err := json.Unmarshal([]byte(pendingData), &pending); err != nil {
		return nil, fmt.Errorf("failed to parse pending login: %w", err)
	}

	if pending.SelectedRole == nil {
		return nil, errors.New("no role selected yet, please call select-profile first")
	}

	userID, _ := uuid.Parse(pending.UserID)

	var selectedOrg *dto.OrgContextDto

	// organization_id rỗng = chế độ "Độc lập"
	if req.OrganizationID != "" {
		orgs, err := s.getUserOrgs(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to load organizations: %w", err)
		}
		for _, org := range orgs {
			if org.ID == req.OrganizationID {
				selectedOrg = &org
				break
			}
		}
		if selectedOrg == nil {
			return nil, errors.New("invalid organization_id: user does not belong to this organization")
		}
	}

	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	result, err := s.completeLogin(ctx, user, pending.DeviceInfo, *pending.SelectedRole, selectedOrg)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(pending.SystemRoles)
	_ = s.redisClient.Del(ctx, pendingKey).Err()
	return result, nil
}

// SwitchOrg đổi org khi đã đăng nhập (giữ nguyên role).
// organization_id rỗng = chuyển về chế độ "Độc lập" (active_org=null).
func (s *AuthService) SwitchOrg(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, activeRole string, req dto.SwitchOrgRequestDto) (*dto.SelectRoleResponseDto, error) {
	var selectedOrg *dto.OrgContextDto

	if req.OrganizationID != "" {
		orgID, err := uuid.Parse(req.OrganizationID)
		if err != nil {
			return nil, errors.New("invalid organization_id format")
		}

		orgs, err := s.getUserOrgs(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to load organizations: %w", err)
		}

		for _, org := range orgs {
			parsed, _ := uuid.Parse(org.ID)
			if parsed == orgID {
				selectedOrg = &org
				break
			}
		}
		if selectedOrg == nil {
			return nil, errors.New("user does not belong to this organization")
		}
	}

	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	// Lấy tất cả system roles
	userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to load system roles: %w", err)
	}

	var currentRole dto.SystemRoleDto
	allRoles := make([]dto.SystemRoleDto, len(userSystemRoles))
	for i, sr := range userSystemRoles {
		allRoles[i] = dto.SystemRoleDto{
			ID:   sr.SystemRole.ID.String(),
			Name: sr.SystemRole.Name,
		}
		if sr.SystemRole.Name == activeRole {
			currentRole = allRoles[i]
		}
	}

	deviceInfo := dto.DeviceInfoDTO{DeviceID: deviceID.String()}
	sessionKey := constants.KeySession(userID.String())
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

	result, err := s.completeLogin(ctx, user, deviceInfo, currentRole, selectedOrg)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(allRoles)
	return result, nil
}

func (s *AuthService) Logout(ctx context.Context, userId, deviceId uuid.UUID) error {

	// 1. Remove refresh token of this device
	refreshKey := constants.KeyRefresh(userId.String())
	if err := s.redisClient.HDel(ctx, refreshKey, deviceId.String()).Err(); err != nil {
		return err
	}

	// 2. Remove device session
	sessionKey := constants.KeySession(userId.String())
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
	userVersionKey := constants.KeyUserVersion(claims.UserID.String())
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
	refreshTokenKey := constants.KeyRefresh(claims.UserID.String())
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
	userCacheKey := constants.KeyUserCache(userID.String())

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

	var activeRoleDto *dto.SystemRoleDto
	for _, sr := range systemRoles {
		if sr.Name == activeRole {
			r := sr
			activeRoleDto = &r
			break
		}
	}

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
		userCacheKey := constants.KeyUserCache(userID.String())
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
		userCacheKey := constants.KeyUserCache(userID.String())
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
	userVersionKey := constants.KeyUserVersion(userID.String())
	if err := s.redisClient.Incr(ctx, userVersionKey).Err(); err != nil {
		return err
	}

	// 2. DEL auth:refresh:{userId} → remove all refresh tokens (HASH)
	refreshKey := constants.KeyRefresh(userID.String())
	_ = s.redisClient.Del(ctx, refreshKey).Err()

	// 3. DEL auth:session:{userId} → remove all device sessions (HASH)
	sessionKey := constants.KeySession(userID.String())
	_ = s.redisClient.Del(ctx, sessionKey).Err()

	return nil
}

// determineEntryContext xác định context điều hướng dựa trên system roles
func (s *AuthService) determineEntryContext(systemRoles []dto.SystemRoleDto) *dto.EntryContext {
	if len(systemRoles) == 0 {
		return nil
	}

	// Priority mapping: ORG_OWNER > PARENT > TEACHER > STUDENT
	priorityMap := map[string]int{
		"ORG_OWNER": 4,
		"PARENT":    3,
		"TEACHER":   2,
		"STUDENT":   1,
	}

	var primaryRole string
	maxPriority := 0

	for _, role := range systemRoles {
		if p, ok := priorityMap[role.Name]; ok && p > maxPriority {
			maxPriority = p
			primaryRole = role.Name
		}
	}

	if primaryRole == "" {
		primaryRole = systemRoles[0].Name
	}

	ctx := &dto.EntryContext{PrimaryRole: primaryRole}

	switch primaryRole {
	case "PARENT":
		ctx.RequiresSetup = true
		ctx.SetupEndpoint = "/me/children"
	case "ORG_OWNER":
		ctx.RequiresSetup = true
		ctx.SetupEndpoint = "/me/organizations"
	default:
		ctx.RequiresSetup = false
	}

	return ctx
}

// PasswordResetOTP represents the OTP data stored in Redis
// PasswordResetOTP stores password reset data in Redis
// Security: OTP is stored as hash to prevent plaintext exposure
type PasswordResetOTP struct {
	OTPHash   string `json:"otp_hash"` // SHA256 hash of OTP with email salt
	Email     string `json:"email"`    // Email used as salt for hashing
	Attempt   int    `json:"attempt"`
	ExpiredAt int64  `json:"expired_at"`
}

const (
	maxPasswordResetAttempts = 5
	otpTTL                   = 5 * time.Minute
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

	// Hash OTP before storing (security: prevent plaintext exposure)
	otpHash := utils.HashOTP(otp, email)

	// ===== 5. Store hashed OTP in Redis =====
	otpKey := constants.KeyPasswordReset(user.ID.String())
	otpData := PasswordResetOTP{
		OTPHash:   otpHash,
		Email:     email,
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
	otpKey := constants.KeyPasswordReset(user.ID.String())
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
	if otpData.Attempt >= maxPasswordResetAttempts {
		_ = s.redisClient.Del(ctx, otpKey).Err()
		return errors.New("too many failed attempts, please request a new OTP")
	}

	// ===== 5. Verify OTP using constant-time comparison =====
	if !utils.VerifyOTP(req.Otp, otpData.OTPHash, otpData.Email) {
		// Increment attempt count
		otpData.Attempt++

		if otpData.Attempt >= maxPasswordResetAttempts {
			// Max attempts reached - delete OTP
			_ = s.redisClient.Del(ctx, otpKey).Err()
			return errors.New("too many failed attempts, please request a new OTP")
		}

		// Update attempt count in Redis
		updatedBytes, _ := json.Marshal(otpData)
		remainingTTL := time.Until(time.Unix(otpData.ExpiredAt, 0))
		_ = s.redisClient.Set(ctx, otpKey, updatedBytes, remainingTTL).Err()

		remaining := maxPasswordResetAttempts - otpData.Attempt
		return fmt.Errorf("invalid OTP, %d attempts remaining", remaining)
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
	userCacheKey := constants.KeyUserCache(user.ID.String())
	_ = s.redisClient.Del(ctx, userCacheKey).Err()

	return nil
}

// ========== UNIFIED ROLE SELECTION (NEW FLOW) ==========

// GetMyRoles returns unified list of system roles and organization roles
func (s *AuthService) GetMyRoles(ctx context.Context, userID uuid.UUID) (*dto.GetMyRolesResponseDto, error) {
	roles, err := s.buildUnifiedRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &dto.GetMyRolesResponseDto{Roles: roles}, nil
}

// buildUnifiedRoles fetches both SystemRoles and OrganizationRoles, returns as UnifiedRoleDto list
func (s *AuthService) buildUnifiedRoles(ctx context.Context, userID uuid.UUID) ([]dto.UnifiedRoleDto, error) {
	var roles []dto.UnifiedRoleDto

	// 1. Get system roles (e.g., "Giáo viên tự do", "Học sinh")
	systemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to load system roles: %w", err)
	}
	for _, sr := range systemRoles {
		if sr.SystemRole == nil {
			continue
		}
		roles = append(roles, dto.UnifiedRoleDto{
			ID:          sr.ID.String(),
			Type:        "system",
			RoleName:    sr.SystemRole.Name,
			DisplayName: sr.SystemRole.Name, // e.g., "Giáo viên tự do"
		})
	}

	// 2. Get organization roles (e.g., "Kế toán - Trường PTIT")
	orgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to load organization roles: %w", err)
	}
	for _, or := range orgRoles {
		if or.Role == nil || or.Organization == nil {
			continue
		}
		orgID := or.OrganizationID.String()
		orgName := or.Organization.Name
		roles = append(roles, dto.UnifiedRoleDto{
			ID:               or.ID.String(),
			Type:             "organization",
			RoleName:         or.Role.Name,
			OrganizationID:   &orgID,
			OrganizationName: &orgName,
			DisplayName:      fmt.Sprintf("%s - %s", or.Role.Name, or.Organization.Name),
		})
	}

	return roles, nil
}

// SelectRole selects a role during login flow (using session_token)
func (s *AuthService) SelectRole(ctx context.Context, req dto.SelectRoleRequestDto) (*dto.SelectRoleResponseDto, error) {
	// 1. Get pending login from Redis
	pendingKey := constants.KeyPendingLogin(req.SessionToken)
	pendingData, err := s.redisClient.Get(ctx, pendingKey).Result()
	if err == redis.Nil {
		return nil, errors.New("session expired or invalid, please login again")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pending login: %w", err)
	}

	var pending PendingLogin
	if err := json.Unmarshal([]byte(pendingData), &pending); err != nil {
		return nil, fmt.Errorf("failed to parse pending login: %w", err)
	}

	userID, _ := uuid.Parse(pending.UserID)
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		return nil, errors.New("invalid role_id format")
	}

	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	// 2. Validate role ownership and build active role
	var activeRole dto.UnifiedRoleDto
	if req.RoleType == "system" {
		// Validate system role
		userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
		if err != nil {
			return nil, fmt.Errorf("failed to load system roles: %w", err)
		}
		found := false
		for _, sr := range userSystemRoles {
			if sr.ID == roleID && sr.SystemRole != nil {
				activeRole = dto.UnifiedRoleDto{
					ID:          sr.ID.String(),
					Type:        "system",
					RoleName:    sr.SystemRole.Name,
					DisplayName: sr.SystemRole.Name,
				}
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("user does not have this system role")
		}
	} else if req.RoleType == "organization" {
		// Validate organization role
		orgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
		if err != nil {
			return nil, fmt.Errorf("failed to load organization roles: %w", err)
		}
		found := false
		for _, or := range orgRoles {
			if or.ID == roleID && or.Role != nil && or.Organization != nil {
				orgID := or.OrganizationID.String()
				orgName := or.Organization.Name
				activeRole = dto.UnifiedRoleDto{
					ID:               or.ID.String(),
					Type:             "organization",
					RoleName:         or.Role.Name,
					OrganizationID:   &orgID,
					OrganizationName: &orgName,
					DisplayName:      fmt.Sprintf("%s - %s", or.Role.Name, or.Organization.Name),
				}
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("user does not have this organization role")
		}
	} else {
		return nil, errors.New("invalid role_type: must be 'system' or 'organization'")
	}

	// 3. Complete login - generate tokens
	result, err := s.completeLoginUnified(ctx, user, pending.DeviceInfo, activeRole)
	if err != nil {
		return nil, err
	}

	// 4. Delete pending login from Redis
	_ = s.redisClient.Del(ctx, pendingKey).Err()

	return result, nil
}

// SwitchRole switches role while already logged in
func (s *AuthService) SwitchRole(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchRoleRequestDto) (*dto.SelectRoleResponseDto, error) {
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		return nil, errors.New("invalid role_id format")
	}

	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	// 1. Validate role ownership and build active role
	var activeRole dto.UnifiedRoleDto
	if req.RoleType == "system" {
		userSystemRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
		if err != nil {
			return nil, fmt.Errorf("failed to load system roles: %w", err)
		}
		found := false
		for _, sr := range userSystemRoles {
			if sr.ID == roleID && sr.SystemRole != nil {
				activeRole = dto.UnifiedRoleDto{
					ID:          sr.ID.String(),
					Type:        "system",
					RoleName:    sr.SystemRole.Name,
					DisplayName: sr.SystemRole.Name,
				}
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("user does not have this system role")
		}
	} else if req.RoleType == "organization" {
		orgRoles, err := s.userOrgRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
		if err != nil {
			return nil, fmt.Errorf("failed to load organization roles: %w", err)
		}
		found := false
		for _, or := range orgRoles {
			if or.ID == roleID && or.Role != nil && or.Organization != nil {
				orgID := or.OrganizationID.String()
				orgName := or.Organization.Name
				activeRole = dto.UnifiedRoleDto{
					ID:               or.ID.String(),
					Type:             "organization",
					RoleName:         or.Role.Name,
					OrganizationID:   &orgID,
					OrganizationName: &orgName,
					DisplayName:      fmt.Sprintf("%s - %s", or.Role.Name, or.Organization.Name),
				}
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("user does not have this organization role")
		}
	} else {
		return nil, errors.New("invalid role_type: must be 'system' or 'organization'")
	}

	// 2. Get device info from session
	deviceInfo := dto.DeviceInfoDTO{DeviceID: deviceID.String()}
	sessionKey := constants.KeySession(userID.String())
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

	// 3. Complete login with new role
	return s.completeLoginUnified(ctx, user, deviceInfo, activeRole)
}

// completeLoginUnified generates JWT tokens for unified role selection
func (s *AuthService) completeLoginUnified(
	ctx context.Context,
	user *model.User,
	deviceInfo dto.DeviceInfoDTO,
	activeRole dto.UnifiedRoleDto,
) (*dto.SelectRoleResponseDto, error) {
	deviceID, _ := uuid.Parse(deviceInfo.DeviceID)

	// Get/increment user version
	userVersionKey := constants.KeyUserVersion(user.ID.String())
	userVersion := int64(1)
	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == nil {
		userVersion, _ = strconv.ParseInt(userVerStr, 10, 64)
	} else if err == redis.Nil {
		_ = s.redisClient.Set(ctx, userVersionKey, userVersion, 0).Err()
	}

	// Determine active org for JWT
	var activeOrgID *uuid.UUID
	if activeRole.Type == "organization" && activeRole.OrganizationID != nil {
		parsed, _ := uuid.Parse(*activeRole.OrganizationID)
		activeOrgID = &parsed
	}

	// Generate tokens - use RoleName for JWT (for backward compatibility with middleware)
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, deviceID, activeRole.RoleName, activeOrgID, userVersion)
	if err != nil {
		return nil, err
	}

	// Save refresh token
	refreshKey := constants.KeyRefresh(user.ID.String())
	if err := s.redisClient.HSet(ctx, refreshKey, deviceID.String(), refreshToken).Err(); err != nil {
		return nil, err
	}
	s.redisClient.Expire(ctx, refreshKey, s.cfg.JWTRefreshExpiration)

	// Save session info
	type deviceSession struct {
		DeviceID   uuid.UUID `json:"device_id"`
		DeviceName string    `json:"device_name"`
		UserAgent  string    `json:"user_agent"`
		LoggedInAt string    `json:"logged_in_at"`
	}
	sess := deviceSession{
		DeviceID:   deviceID,
		DeviceName: deviceInfo.DeviceName,
		UserAgent:  deviceInfo.UserAgent,
		LoggedInAt: time.Now().Format(time.RFC3339),
	}
	sessBytes, _ := json.Marshal(sess)
	sessionKey := constants.KeySession(user.ID.String())
	_ = s.redisClient.HSet(ctx, sessionKey, deviceID.String(), sessBytes).Err()
	s.redisClient.Expire(ctx, sessionKey, s.cfg.JWTRefreshExpiration)

	// Build response
	var dobStr *string
	if user.DateOfBirth != nil {
		formatted := user.DateOfBirth.Format("2006-01-02")
		dobStr = &formatted
	}

	return &dto.SelectRoleResponseDto{
		Completed:    true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserResponseDto{
			ID:          user.ID,
			Username:    user.UserName,
			Email:       user.Email,
			FullName:    user.FullName,
			Phone:       user.Phone,
			AvatarUrl:   user.AvatarURL,
			DateOfBirth: dobStr,
			Bio:         user.Bio,
			IsActive:    user.IsActive,
			CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		},
		ActiveRole: activeRole,
		CurrentDevice: &dto.DeviceSessionDto{
			DeviceID:   deviceID.String(),
			DeviceName: deviceInfo.DeviceName,
			UserAgent:  deviceInfo.UserAgent,
			LoggedInAt: sess.LoggedInAt,
			IsCurrent:  true,
		},
	}, nil
}

// ========== PROFILE MANAGEMENT ==========

// protectedRoles are roles that cannot be self-assigned via API
var protectedRoles = map[string]bool{
	"SYSTEM_ADMIN": true,
}

// GetSystemRoleOptions returns available system roles for creating profiles (excludes protected roles)
func (s *AuthService) GetSystemRoleOptions(ctx context.Context) ([]dto.SystemRoleOptionDto, error) {
	roles, _, err := s.systemRoleRepo.GetAllSystemRoles(ctx, 1, 100, "", "active")
	if err != nil {
		return nil, fmt.Errorf("failed to get system roles: %w", err)
	}

	result := make([]dto.SystemRoleOptionDto, 0, len(roles))
	for _, role := range roles {
		if protectedRoles[role.Name] {
			continue
		}
		var desc *string
		if role.Description.Valid {
			desc = &role.Description.String
		}
		result = append(result, dto.SystemRoleOptionDto{
			ID:          role.ID.String(),
			Name:        role.Name,
			Description: desc,
		})
	}
	return result, nil
}

// GetMyProfiles returns all profiles (UserSystemRoles) of a user
func (s *AuthService) GetMyProfiles(ctx context.Context, userID uuid.UUID) ([]dto.ProfileDto, error) {
	userRoles, err := s.userSystemRoleRepo.FindByUserIDWithDetails(ctx, userID, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to get user profiles: %w", err)
	}

	result := make([]dto.ProfileDto, 0, len(userRoles))
	for _, ur := range userRoles {
		if ur.SystemRole == nil {
			continue
		}
		var desc *string
		if ur.SystemRole.Description.Valid {
			desc = &ur.SystemRole.Description.String
		}
		result = append(result, dto.ProfileDto{
			ID:           ur.ID.String(),
			SystemRoleID: ur.SystemRoleID.String(),
			RoleName:     ur.SystemRole.Name,
			Description:  desc,
			Status:       ur.Status,
			CreatedAt:    ur.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

// CreateProfile creates a new profile for user with given system role
func (s *AuthService) CreateProfile(ctx context.Context, userID uuid.UUID, req dto.CreateProfileRequestDto) (*dto.ProfileDto, error) {
	systemRoleID, err := uuid.Parse(req.SystemRoleID)
	if err != nil {
		return nil, errors.New("invalid system_role_id")
	}

	// Check if system role exists
	systemRole, err := s.systemRoleRepo.GetSystemRoleByID(ctx, systemRoleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system role: %w", err)
	}
	if systemRole == nil {
		return nil, errors.New("system role not found")
	}

	// Block protected roles from being self-assigned
	if protectedRoles[systemRole.Name] {
		return nil, errors.New("this role cannot be self-assigned")
	}

	// Check if user already has this profile
	existing, err := s.userSystemRoleRepo.FindByUserAndSystemRole(ctx, userID, systemRoleID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing profile: %w", err)
	}
	if existing != nil {
		return nil, errors.New("you already have this profile")
	}

	// Create new profile
	userSystemRole := &model.UserSystemRole{
		UserID:       userID,
		SystemRoleID: systemRoleID,
		Status:       "active",
		GrantedAt:    time.Now(),
	}
	if err := s.userSystemRoleRepo.Create(ctx, userSystemRole); err != nil {
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	var desc *string
	if systemRole.Description.Valid {
		desc = &systemRole.Description.String
	}
	return &dto.ProfileDto{
		ID:           userSystemRole.ID.String(),
		SystemRoleID: systemRoleID.String(),
		RoleName:     systemRole.Name,
		Description:  desc,
		Status:       userSystemRole.Status,
		CreatedAt:    userSystemRole.CreatedAt.Format(time.RFC3339),
	}, nil
}

// DeleteProfile removes a profile from user
func (s *AuthService) DeleteProfile(ctx context.Context, userID uuid.UUID, profileID uuid.UUID) error {
	// Check if profile exists and belongs to user
	profile, err := s.userSystemRoleRepo.FindByID(ctx, profileID)
	if err != nil {
		return fmt.Errorf("failed to find profile: %w", err)
	}
	if profile == nil {
		return errors.New("profile not found")
	}
	if profile.UserID != userID {
		return errors.New("profile does not belong to you")
	}

	// Check if user has at least 2 profiles (can't delete last one)
	profiles, err := s.userSystemRoleRepo.FindByUserID(ctx, userID, "active")
	if err != nil {
		return fmt.Errorf("failed to count profiles: %w", err)
	}
	if len(profiles) <= 1 {
		return errors.New("cannot delete your last profile")
	}

	// Soft delete
	if err := s.userSystemRoleRepo.Delete(ctx, profileID); err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	// Security: Increment user_version to invalidate existing tokens
	// Forces user to re-login with updated profiles
	userVersionKey := constants.KeyUserVersion(userID.String())
	s.redisClient.Incr(ctx, userVersionKey)

	return nil
}
