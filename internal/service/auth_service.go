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
	SelectProfile(ctx context.Context, req dto.SelectProfileRequestDto) (*dto.SelectProfileResponseDto, error)
	SelectOrg(ctx context.Context, req dto.SelectOrgRequestDto) (*dto.SelectProfileResponseDto, error)
	SwitchProfile(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchProfileRequestDto) (*dto.SelectProfileResponseDto, error)
	SwitchOrg(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, activeRole string, req dto.SwitchOrgRequestDto) (*dto.SelectProfileResponseDto, error)
	Logout(ctx context.Context, userId, deviceId uuid.UUID) error
	LogoutAllDevice(ctx context.Context, userId uuid.UUID) error
	RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error)
	GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error)
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
// Redis key: register:otp:{email}
// TTL: 5 minutes
type PendingRegistration struct {
	Email        string   `json:"email"`
	PasswordHash string   `json:"password_hash"`
	UserName     string   `json:"user_name"`
	FullName     string   `json:"full_name,omitempty"`
	RoleIDs      []string `json:"role_ids"`
	OTP          string   `json:"otp"`
	CreatedAt    string   `json:"created_at"`
}

func (s *AuthService) RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error {
	if len(req.RoleIDs) > 0 {
		roleUUIDs := make([]uuid.UUID, len(req.RoleIDs))
		for i, idStr := range req.RoleIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				return fmt.Errorf("invalid role_id: %s", idStr)
			}
			roleUUIDs[i] = id
		}
		roles, err := s.systemRoleRepo.GetSystemRoleByIDs(ctx, roleUUIDs)
		if err != nil {
			return fmt.Errorf("failed to validate system roles: %w", err)
		}
		if len(roles) != len(req.RoleIDs) {
			return errors.New("one or more role IDs are invalid")
		}
		for _, r := range roles {
			if r.Status != "active" {
				return errors.New("system role is not active: " + r.Name)
			}
		}
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

	// ===== 6. Generate OTP (6 digits) =====
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	pendingData := PendingRegistration{
		Email:        req.Email,
		PasswordHash: passwordHash,
		UserName:     req.UserName,
		FullName:     req.FullName,
		RoleIDs:      req.RoleIDs,
		OTP:          otp,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}

	// ===== 8. Save to Redis with TTL (5 minutes) =====
	// Key format: register:otp:{email}
	pendingKey := fmt.Sprintf("register:otp:%s", req.Email)
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
	pendingKey := fmt.Sprintf("register:otp:%s", req.Email)
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

	if len(pending.RoleIDs) > 0 {
		roleUUIDs := make([]uuid.UUID, len(pending.RoleIDs))
		for i, idStr := range pending.RoleIDs {
			id, _ := uuid.Parse(idStr)
			roleUUIDs[i] = id
		}
		sysRoles, _ := s.systemRoleRepo.GetSystemRoleByIDs(ctx, roleUUIDs)
		now := time.Now()
		for _, r := range sysRoles {
			if r.Status != "active" {
				continue
			}
			_ = s.userSystemRoleRepo.Create(ctx, &model.UserSystemRole{
				UserID:       user.ID,
				SystemRoleID: r.ID,
				GrantedAt:    now,
				GrantedBy:    nil,
				Status:       model.UserSystemRoleStatusActive,
			})
		}
	}

	_ = s.redisClient.Del(ctx, pendingKey).Err()

	return &dto.RegisterResponseDto{
		ID:       user.ID.String(),
		Email:    user.Email,
		UserName: user.UserName,
		FullName: fullName,
		RoleIDs:  pending.RoleIDs,
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

	// User có nhiều roles → chưa chọn role, trả session_token
	if len(systemRoleDtos) > 1 {
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
		pendingKey := fmt.Sprintf("pending_login:%s", sessionToken)
		if err := s.redisClient.Set(ctx, pendingKey, pendingBytes, 5*time.Minute).Err(); err != nil {
			return nil, fmt.Errorf("failed to save pending login: %w", err)
		}
		return &dto.LoginResponseDto{
			Completed:    false,
			SessionToken: sessionToken,
			SystemRoles:  systemRoleDtos,
		}, nil
	}

	// User chỉ có 1 role → tự động chọn, kiểm tra org
	selectedRole := systemRoleDtos[0]
	return s.finishLoginWithOrgCheck(ctx, user, req.DeviceInfo, selectedRole, systemRoleDtos)
}

// getUserOrgs trả về danh sách org mà user thuộc (generic, không phụ thuộc role)
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

// tryAutoSelectOrg kiểm tra org context sau khi chọn role.
// 0 org → auto-complete với null. 1+ org → yêu cầu chọn (gồm cả lựa chọn "Độc lập").
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
	allRoles []dto.SystemRoleDto,
	activeOrg *dto.OrgContextDto,
) (*dto.SelectProfileResponseDto, error) {

	deviceID, _ := uuid.Parse(deviceInfo.DeviceID)

	userVersionKey := fmt.Sprintf("user_version:%s", user.ID)
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

	refreshKey := fmt.Sprintf("auth:refresh:%s", user.ID)
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
	sessionKey := fmt.Sprintf("session:%s", user.ID)
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

	return &dto.SelectProfileResponseDto{
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
		ActiveRole:  activeRole,
		ActiveOrg:   activeOrg,
		SystemRoles: allRoles,
		CurrentDevice: dto.DeviceSessionDto{
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

	// User thuộc 1+ org → yêu cầu chọn (org hoặc "Độc lập")
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
		pendingKey := fmt.Sprintf("pending_login:%s", sessionToken)
		if err := s.redisClient.Set(ctx, pendingKey, pendingBytes, 5*time.Minute).Err(); err != nil {
			return nil, fmt.Errorf("failed to save pending login: %w", err)
		}
		return &dto.LoginResponseDto{
			Completed:            false,
			SessionToken:         sessionToken,
			SystemRoles:          allRoles,
			RequiresOrgSelection: true,
			Organizations:        orgs,
			ActiveRole:           &selectedRole,
		}, nil
	}

	// 0 org → hoàn tất ngay với active_org=null
	result, err := s.completeLogin(ctx, user, deviceInfo, selectedRole, allRoles, nil)
	if err != nil {
		return nil, err
	}
	entryCtx := s.determineEntryContext(allRoles)
	return &dto.LoginResponseDto{
		Completed:     true,
		SystemRoles:   allRoles,
		AccessToken:   result.AccessToken,
		RefreshToken:  result.RefreshToken,
		User:          &result.User,
		ActiveRole:    &result.ActiveRole,
		ActiveOrg:     result.ActiveOrg,
		EntryContext:  entryCtx,
		CurrentDevice: &result.CurrentDevice,
	}, nil
}

func (s *AuthService) SelectProfile(ctx context.Context, req dto.SelectProfileRequestDto) (*dto.SelectProfileResponseDto, error) {
	pendingKey := fmt.Sprintf("pending_login:%s", req.SessionToken)
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

		return &dto.SelectProfileResponseDto{
			Completed:            false,
			SessionToken:         req.SessionToken,
			RequiresOrgSelection: true,
			Organizations:        orgs,
			ActiveRole:           *selectedRole,
			SystemRoles:          pending.SystemRoles,
		}, nil
	}

	// 0 org → hoàn tất với active_org=null
	result, err := s.completeLogin(ctx, user, pending.DeviceInfo, *selectedRole, pending.SystemRoles, nil)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(pending.SystemRoles)
	_ = s.redisClient.Del(ctx, pendingKey).Err()
	return result, nil
}

func (s *AuthService) SwitchProfile(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, req dto.SwitchProfileRequestDto) (*dto.SelectProfileResponseDto, error) {
	roleID, err := uuid.Parse(req.SystemRoleID)
	if err != nil {
		return nil, errors.New("invalid system_role_id format")
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
	sessionKey := fmt.Sprintf("session:%s", userID)
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
		return &dto.SelectProfileResponseDto{
			Completed:            false,
			RequiresOrgSelection: true,
			Organizations:        orgs,
			ActiveRole:           *selectedRole,
			SystemRoles:          allRoles,
		}, nil
	}

	// 0 org → hoàn tất với active_org=null
	result, err := s.completeLogin(ctx, user, deviceInfo, *selectedRole, allRoles, nil)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(allRoles)
	return result, nil
}

// SelectOrg chọn org sau khi đã chọn role (dùng session_token).
// organization_id rỗng = chọn chế độ "Độc lập" (active_org=null).
func (s *AuthService) SelectOrg(ctx context.Context, req dto.SelectOrgRequestDto) (*dto.SelectProfileResponseDto, error) {
	pendingKey := fmt.Sprintf("pending_login:%s", req.SessionToken)
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

	result, err := s.completeLogin(ctx, user, pending.DeviceInfo, *pending.SelectedRole, pending.SystemRoles, selectedOrg)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(pending.SystemRoles)
	_ = s.redisClient.Del(ctx, pendingKey).Err()
	return result, nil
}

// SwitchOrg đổi org khi đã đăng nhập (giữ nguyên role).
// organization_id rỗng = chuyển về chế độ "Độc lập" (active_org=null).
func (s *AuthService) SwitchOrg(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID, activeRole string, req dto.SwitchOrgRequestDto) (*dto.SelectProfileResponseDto, error) {
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
	sessionKey := fmt.Sprintf("session:%s", userID)
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

	result, err := s.completeLogin(ctx, user, deviceInfo, currentRole, allRoles, selectedOrg)
	if err != nil {
		return nil, err
	}
	result.EntryContext = s.determineEntryContext(allRoles)
	return result, nil
}

func (s *AuthService) Logout(ctx context.Context, userId, deviceId uuid.UUID) error {

	// 1. Remove refresh token of this device
	refreshKey := fmt.Sprintf("auth:refresh:%s", userId)
	if err := s.redisClient.HDel(ctx, refreshKey, deviceId.String()).Err(); err != nil {
		return err
	}

	// 2. Remove device session
	sessionKey := fmt.Sprintf("session:%s", userId)
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
	userVersionKey := fmt.Sprintf("user_version:%s", claims.UserID)
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
	userCacheKey := fmt.Sprintf("user_cache:%s", userID)

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
		Phone:       user.Phone,
		AvatarUrl:   user.AvatarURL,
		DateOfBirth: dob,
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

func (s *AuthService) UpdateMe(ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequestDto) (*dto.UserResponseDto, error) {
	// ===== 1. Build updates map (only non-nil fields) =====
	updates := make(map[string]interface{})

	if req.Username != nil {
		updates["user_name"] = *req.Username
	}

	if req.Phone != nil {
		updates["phone"] = *req.Phone
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
		userCacheKey := fmt.Sprintf("user_cache:%s", userID)
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
		userCacheKey := fmt.Sprintf("user_cache:%s", userID)
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
	userVersionKey := fmt.Sprintf("user_version:%s", userID)
	if err := s.redisClient.Incr(ctx, userVersionKey).Err(); err != nil {
		return err
	}

	// 2. DEL auth:refresh:{userId} → remove all refresh tokens (HASH)
	refreshKey := fmt.Sprintf("auth:refresh:%s", userID)
	_ = s.redisClient.Del(ctx, refreshKey).Err()

	// 3. DEL session:{userId} → remove all device sessions (HASH)
	sessionKey := fmt.Sprintf("session:%s", userID)
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
	otpKey := fmt.Sprintf("password_reset:otp:%s", user.ID)
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
	otpKey := fmt.Sprintf("password_reset:otp:%s", user.ID)
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
	userCacheKey := fmt.Sprintf("user_cache:%s", user.ID)
	_ = s.redisClient.Del(ctx, userCacheKey).Err()

	return nil
}
