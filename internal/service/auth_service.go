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
	cfg          *config.Config
	userRepo     repository.UserRepositoryInterface
	roleRepo     repository.RoleRepositoryInterface
	userRoleRepo repository.UserRoleRepositoryInterface
	redisClient  *redis.Client
}

func NewAuthService(
	cfg *config.Config,
	userRepo repository.UserRepositoryInterface,
	roleRepo repository.RoleRepositoryInterface,
	userRoleRepo repository.UserRoleRepositoryInterface,
	redisClient *redis.Client,
) *AuthService {
	return &AuthService{
		cfg:          cfg,
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		userRoleRepo: userRoleRepo,
		redisClient:  redisClient,
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
	RoleCodes    []string `json:"role_codes"`
	OTP          string   `json:"otp"`
	CreatedAt    string   `json:"created_at"`
}

func (s *AuthService) RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error {
	
	// ===== 2. Validate role_codes (chỉ cho phép "student" hoặc "parent") =====
	allowedRoles := map[string]bool{"student": true, "parent": true}
	for _, code := range req.RoleCodes {
		if !allowedRoles[code] {
			return fmt.Errorf("invalid role code: %s, only 'student' or 'parent' allowed", code)
		}
	}

	// ===== 3. Check if roles exist in database =====
	roles, err := s.roleRepo.FindByCodes(ctx, req.RoleCodes)
	if err != nil {
		return fmt.Errorf("failed to validate roles: %w", err)
	}
	if len(roles) != len(req.RoleCodes) {
		return errors.New("one or more role codes are invalid")
	}

	// ===== 4. Check email already exists =====
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

	// ===== 7. Create pending registration data =====
	pendingData := PendingRegistration{
		Email:        req.Email,
		PasswordHash: passwordHash,
		UserName:     req.UserName,
		FullName:     req.FullName,
		RoleCodes:    req.RoleCodes,
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
	// ===== 1. Check Redis client =====
	if s.redisClient == nil {
		return nil, errors.New("redis client is not initialized")
	}

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

	// ===== 4. Verify OTP =====
	if pending.OTP != req.OTP {
		return nil, errors.New("invalid OTP")
	}

	// ===== 5. Get Roles by codes =====
	roles, err := s.roleRepo.FindByCodes(ctx, pending.RoleCodes)
	if err != nil {
		return nil, fmt.Errorf("failed to find roles: %w", err)
	}
	if len(roles) != len(pending.RoleCodes) {
		return nil, errors.New("one or more roles are invalid")
	}

	// ===== 6. Prepare FullName pointer =====
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

	// ===== 8. Create user_roles entries =====
	now := time.Now()
	userRoles := make([]model.UserRole, len(roles))
	for i, role := range roles {
		userRoles[i] = model.UserRole{
			UserID:         user.ID,
			RoleID:         role.ID,
			OrganizationID: nil, // NULL = global role
			GrantedAt:      now,
			GrantedBy:      nil, // Self-registered
			Notes:          nil,
		}
	}

	if err := s.userRoleRepo.CreateUserRoles(ctx, userRoles); err != nil {
		// Rollback: delete user if user_roles creation fails
		// In production, use database transaction
		return nil, fmt.Errorf("failed to assign roles: %w", err)
	}

	// ===== 9. Delete pending registration from Redis =====
	_ = s.redisClient.Del(ctx, pendingKey).Err()

	// ===== 10. Collect role codes for response =====
	roleCodes := make([]string, len(roles))
	for i, role := range roles {
		roleCodes[i] = role.Code
	}

	// ===== 11. Build response =====
	return &dto.RegisterResponseDto{
		ID:        user.ID.String(),
		Email:     user.Email,
		UserName:  user.UserName,
		FullName:  fullName,
		RoleCodes: roleCodes,
	}, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req dto.LoginRequestDto,
) (*dto.LoginResponseDto, error) {

	// ===== 1. Validate user credentials =====
	user, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}

	if user == nil || !utils.CheckPassword(req.Password, user.PasswordHash) {
		return nil, errors.New("invalid email or password")
	}

	// ===== 2. Reject inactive users =====
	if !user.IsActive {
		return nil, errors.New("account is inactive")
	}

	// ===== 3. Parse device_id =====
	deviceID, err := uuid.Parse(req.DeviceInfo.DeviceID)
	if err != nil {
		return nil, errors.New("invalid device_id format")
	}

	// ===== 4. Get or initialize user_version =====
	userVersionKey := fmt.Sprintf("user_version:%s", user.ID)
	userVersion := int64(1)

	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == nil {
		userVersion, _ = strconv.ParseInt(userVerStr, 10, 64)
	} else if err == redis.Nil {
		if err := s.redisClient.Set(ctx, userVersionKey, userVersion, 0).Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	// ===== 5. Generate JWT tokens =====
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, deviceID, userVersion)
	if err != nil {
		return nil, err
	}

	// ===== 6. Store refresh token in Redis HSET (PLAINTEXT) =====
	refreshKey := fmt.Sprintf("auth:refresh:%s", user.ID)
	if err := s.redisClient.HSet(ctx, refreshKey, deviceID.String(), refreshToken).Err(); err != nil {
		return nil, err
	}

	// ===== 7. Set TTL on hash key =====
	if err := s.redisClient.Expire(ctx, refreshKey, s.cfg.JWTRefreshExpiration).Err(); err != nil {
		return nil, err
	}

	// ===== 8. Store device metadata in separate hash =====
	type deviceSession struct {
		DeviceID   uuid.UUID `json:"device_id"`
		DeviceName string    `json:"device_name"`
		UserAgent  string    `json:"user_agent"`
		LoggedInAt string    `json:"logged_in_at"`
	}

	sessionKey := fmt.Sprintf("session:%s", user.ID)
	sessionPayload := deviceSession{
		DeviceID:   deviceID,
		DeviceName: req.DeviceInfo.DeviceName,
		UserAgent:  req.DeviceInfo.UserAgent,
		LoggedInAt: time.Now().Format(time.RFC3339),
	}

	sessionBytes, err := json.Marshal(sessionPayload)
	if err != nil {
		return nil, err
	}

	if err := s.redisClient.HSet(ctx, sessionKey, deviceID.String(), sessionBytes).Err(); err != nil {
		return nil, err
	}

	// ===== 9. Build response =====
	status := "active"

	var dob *string
	if user.DateOfBirth != nil {
		f := user.DateOfBirth.Format("2006-01-02")
		dob = &f
	}

	currentDevice := dto.DeviceSessionDto{
		DeviceID:   deviceID.String(),
		DeviceName: req.DeviceInfo.DeviceName,
		UserAgent:  req.DeviceInfo.UserAgent,
		LoggedInAt: time.Now().Format(time.RFC3339),
	}

	return &dto.LoginResponseDto{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserResponseDto{
			ID:          user.ID,
			Username:    user.UserName,
			Email:       user.Email,
			Phone:       user.Phone,
			AvatarUrl:   user.AvatarURL,
			DateOfBirth: dob,
			Status:      &status,
			CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		},
		CurrentDevice: currentDevice,
	}, nil
}


func (s *AuthService) Logout(ctx context.Context, userId, deviceId uuid.UUID) error {
	if s.redisClient == nil {
		return errors.New("redis client is not initialized")
	}

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
	if s.redisClient == nil {
		return errors.New("redis client is not initialized")
	}
	return s.revokeAllSessions(ctx, userId)
}


func (s *AuthService) RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error) {
	if s.redisClient == nil {
		return nil, errors.New("redis client is not initialized")
	}

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

	// ===== 3. Check if refresh token exists in Redis =====
	refreshTokenKey := fmt.Sprintf("refresh_token:%s:%s", claims.UserID, claims.DeviceID)
	storedToken, err := s.redisClient.Get(ctx, refreshTokenKey).Result()
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
	newAccessToken, newRefreshToken, err := utils.GenerateTokens(s.cfg, claims.UserID, claims.DeviceID, currentUserVersion)
	if err != nil {
		return nil, err
	}

	// ===== 6. Update refresh token in Redis =====
	if err := s.redisClient.Set(ctx, refreshTokenKey, newRefreshToken, s.cfg.JWTRefreshExpiration).Err(); err != nil {
		return nil, err
	}

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

	// ===== 3. Build response =====
	status := "inactive"
	if user.IsActive {
		status = "active"
	}

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
		Status:      &status,
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

// PasswordResetOTP represents the OTP data stored in Redis
type PasswordResetOTP struct {
	OTP       string `json:"otp"`
	Attempt   int    `json:"attempt"`
	ExpiredAt int64  `json:"expired_at"`
}

const (
	maxOTPAttempts   = 5
	otpTTL           = 5 * time.Minute
	otpRateLimitTTL  = 60 * time.Second // 1 request per minute
)

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	// ===== 1. Check rate limit (1 request per minute per email) =====
	rateLimitKey := fmt.Sprintf("password_reset:rate:%s", email)
	exists, _ := s.redisClient.Exists(ctx, rateLimitKey).Result()
	if exists > 0 {
		return errors.New("please wait before requesting another OTP")
	}

	// ===== 2. Find user by email (don't reveal if user exists) =====
	user, err := s.userRepo.FindUserByEmail(ctx, email)
	if err != nil {
		// Log error internally but return generic message
		return nil // Don't reveal if email exists
	}

	// If user not found or inactive, return nil (don't reveal)
	if user == nil || !user.IsActive {
		return nil
	}

	// ===== 3. Set rate limit =====
	_ = s.redisClient.Set(ctx, rateLimitKey, "1", otpRateLimitTTL).Err()

	// ===== 4. Generate 6-digit OTP =====
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

	// ===== 9. Invalidate all sessions (force re-login on all devices) =====
	if err := s.revokeAllSessions(ctx, user.ID); err != nil {
		log.Printf("[WARN] Failed to revoke sessions for user %s: %v", user.ID, err)
	}

	// ===== 10. Invalidate user cache =====
	userCacheKey := fmt.Sprintf("user_cache:%s", user.ID)
	_ = s.redisClient.Del(ctx, userCacheKey).Err()

	return nil
}
