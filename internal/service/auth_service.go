package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	GetAllDevices(ctx context.Context, userID, currentDeviceID uuid.UUID) ([]dto.DeviceSessionDto, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequestDto) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error
}

type AuthService struct {
	cfg          *config.Config
	userRepo     repository.UserRepositoryInterface
	userRoleRepo repository.UserRoleRepositoryInterface
	redisClient  *redis.Client
}

func NewAuthService(
	cfg *config.Config,
	userRepo repository.UserRepositoryInterface,
	userRoleRepo repository.UserRoleRepositoryInterface,
	redisClient *redis.Client,
) *AuthService {
	return &AuthService{
		cfg:          cfg,
		userRepo:     userRepo,
		userRoleRepo: userRoleRepo,
		redisClient:  redisClient,
	}
}

// PendingRegistration stores registration data temporarily in Redis
type PendingRegistration struct {
	FullName     string `json:"full_name"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	Phone        string `json:"phone"`
	DateOfBirth  string `json:"date_of_birth,omitempty"`
	RoleCode     string `json:"role_code"`
	ParentPhone  string `json:"parent_phone,omitempty"`
	OTP          string `json:"otp"`
	CreatedAt    string `json:"created_at"`
}

func (s *AuthService) RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error {
	// ===== 1. Check Redis client =====
	if s.redisClient == nil {
		return errors.New("redis client is not initialized")
	}

	// ===== 2. Check email already exists =====
	existingUser, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return fmt.Errorf("failed to check email: %w", err)
	}
	if existingUser != nil {
		return errors.New("email already registered")
	}

	// ===== 3. Check phone already exists =====
	existingUserByPhone, err := s.userRepo.FindUserByPhone(ctx, req.Phone)
	if err != nil {
		return fmt.Errorf("failed to check phone: %w", err)
	}
	if existingUserByPhone != nil {
		return errors.New("phone number already registered")
	}

	// ===== 4. Hash password =====
	passwordHash, err := utils.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// ===== 5. Generate OTP (6 digits) =====
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// ===== 6. Create pending registration data =====
	pendingData := PendingRegistration{
		FullName:     req.FullName,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Phone:        req.Phone,
		DateOfBirth:  req.DateOfBirth,
		RoleCode:     req.RoleCode,
		ParentPhone:  req.ParentPhone,
		OTP:          otp,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}

	// ===== 7. Save to Redis with TTL (5 minutes) =====
	pendingKey := fmt.Sprintf("register_pending:%s", req.Email)
	pendingBytes, err := json.Marshal(pendingData)
	if err != nil {
		return fmt.Errorf("failed to marshal pending data: %w", err)
	}

	otpTTL := 5 * time.Minute
	if err := s.redisClient.Set(ctx, pendingKey, pendingBytes, otpTTL).Err(); err != nil {
		return fmt.Errorf("failed to save pending registration: %w", err)
	}

	// ===== 8. Send OTP via email =====
	// [DEBUG] Log OTP to console for testing
	fmt.Printf("\n========== [DEBUG] REGISTER OTP ==========\n")
	fmt.Printf("Email: %s\n", req.Email)
	fmt.Printf("OTP: %s\n", otp)
	fmt.Printf("Expires in: 5 minutes\n")
	fmt.Printf("===========================================\n\n")

	// Try to send email, but don't fail if SMTP is not configured
	if err := utils.SendRegisterOTP(s.cfg, req.Email, otp); err != nil {
		// Log warning but continue (OTP is saved in Redis)
		fmt.Printf("[WARNING] Failed to send OTP email: %v\n", err)
		fmt.Printf("[INFO] Use the OTP logged above for testing\n\n")
	}

	return nil
}

func (s *AuthService) Register(ctx context.Context, req dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error) {
	// ===== 1. Check Redis client =====
	if s.redisClient == nil {
		return nil, errors.New("redis client is not initialized")
	}

	// ===== 2. Get pending registration from Redis =====
	pendingKey := fmt.Sprintf("register_pending:%s", req.Email)
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
	if pending.OTP != req.OTPRegister {
		return nil, errors.New("invalid OTP")
	}

	// ===== 5. Get Role by code =====
	role, err := s.userRoleRepo.FindByCode(ctx, pending.RoleCode)
	if err != nil {
		return nil, fmt.Errorf("failed to find role: %w", err)
	}
	if role == nil {
		return nil, errors.New("invalid role")
	}

	// ===== 6. Parse DateOfBirth if provided =====
	var dateOfBirth *time.Time
	if pending.DateOfBirth != "" {
		dob, err := time.Parse("2006-01-02", pending.DateOfBirth)
		if err == nil {
			dateOfBirth = &dob
		}
	}

	// ===== 7. Create user in database =====
	user := &model.User{
		Email:        pending.Email,
		PasswordHash: pending.PasswordHash,
		UserName:     pending.Username,
		FullName:     &pending.FullName,
		Phone:        &pending.Phone,
		DateOfBirth:  dateOfBirth,
		RoleID:       role.ID,
		IsVerified:   true,
		IsActive:     true,
	}

	// Set ParentPhone if provided
	if pending.ParentPhone != "" {
		user.ParentPhone = &pending.ParentPhone
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// ===== 8. Delete pending registration from Redis =====
	_ = s.redisClient.Del(ctx, pendingKey).Err()

	// ===== 9. Parse DeviceID =====
	deviceID, err := uuid.Parse(req.DeviceInfo.DeviceID)
	if err != nil {
		return nil, errors.New("invalid device_id format")
	}

	// ===== 10. Set user_version for token management =====
	userVersionKey := fmt.Sprintf("user_version:%s", user.ID)
	userVersion := int64(1)
	if err := s.redisClient.Set(ctx, userVersionKey, userVersion, 0).Err(); err != nil {
		return nil, fmt.Errorf("failed to set user version: %w", err)
	}

	// ===== 11. Generate JWT tokens =====
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, deviceID, userVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// ===== 12. Store refresh token in Redis =====
	refreshTokenKey := fmt.Sprintf("refresh_token:%s:%s", user.ID, deviceID)
	if err := s.redisClient.Set(ctx, refreshTokenKey, refreshToken, s.cfg.JWTRefreshExpiration).Err(); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	// ===== 13. Save device session =====
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
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := s.redisClient.HSet(ctx, sessionKey, deviceID.String(), sessionBytes).Err(); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	// ===== 14. Build response =====
	status := "active"
	var dob *string
	if user.DateOfBirth != nil {
		f := user.DateOfBirth.Format("2006-01-02")
		dob = &f
	}

	return &dto.RegisterResponseDto{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserResponseDto{
			ID:          user.ID,
			Username:    user.UserName,
			Email:       user.Email,
			Phone:       user.Phone,
			DateOfBirth: dob,
			Status:      &status,
			CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		},
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

	// ===== 1. Delete refresh token from Redis =====
	refreshTokenKey := fmt.Sprintf("refresh_token:%s:%s", userId, deviceId)
	if err := s.redisClient.Del(ctx, refreshTokenKey).Err(); err != nil {
		return err
	}

	// ===== 2. Remove device from session hash =====
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

	// ===== 1. Increment user_version → all tokens become invalid immediately =====
	userVersionKey := fmt.Sprintf("user_version:%s", userId)
	if err := s.redisClient.Incr(ctx, userVersionKey).Err(); err != nil {
		return err
	}

	// ===== 2. Clean up refresh tokens (optional but good practice) =====
	refreshTokenPattern := fmt.Sprintf("refresh_token:%s:*", userId)
	_ = s.deleteKeysByPattern(ctx, refreshTokenPattern) // Ignore error - tokens already invalid by version

	// ===== 3. Delete session hash (all devices) =====
	sessionKey := fmt.Sprintf("session:%s", userId)
	if err := s.redisClient.Del(ctx, sessionKey).Err(); err != nil {
		return err
	}

	return nil
}

func (s *AuthService) deleteKeysByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := s.redisClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := s.redisClient.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
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

func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequestDto) error {
	// ===== 1. Get user from database =====
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if user == nil {
		return errors.New("user not found")
	}

	// ===== 2. Verify old password =====
	if !utils.CheckPassword(req.OldPassword, user.PasswordHash) {
		return errors.New("incorrect current password")
	}

	// ===== 3. Hash new password =====
	newPasswordHash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return errors.New("failed to hash new password")
	}

	// ===== 4. Update password in database =====
	if err := s.userRepo.UpdatePasswordHash(ctx, userID, newPasswordHash); err != nil {
		return err
	}

	// ===== 5. Invalidate user cache =====
	if s.redisClient != nil {
		userCacheKey := fmt.Sprintf("user_cache:%s", userID)
		_ = s.redisClient.Del(ctx, userCacheKey).Err()
	}

	// ===== 6. Handle device sessions based on RevokeOthers flag =====
	// FE can send revoke_others=true to logout all other devices after password change
	if req.RevokeOthers && s.redisClient != nil {
		// Increment user_version to invalidate all tokens
		userVersionKey := fmt.Sprintf("user_version:%s", userID)
		if err := s.redisClient.Incr(ctx, userVersionKey).Err(); err != nil {
			return err
		}

		// Clean up all refresh tokens
		refreshTokenPattern := fmt.Sprintf("refresh_token:%s:*", userID)
		_ = s.deleteKeysByPattern(ctx, refreshTokenPattern)

		// Delete all sessions
		sessionKey := fmt.Sprintf("session:%s", userID)
		_ = s.redisClient.Del(ctx, sessionKey).Err()
	}

	return nil
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error {
	return nil
}
