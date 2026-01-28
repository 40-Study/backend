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
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequestDto) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error
}

type AuthService struct {
	cfg         *config.Config
	userRepo    repository.UserRepositoryInterface
	redisClient *redis.Client
}

func NewAuthService(
	cfg *config.Config,
	userRepo repository.UserRepositoryInterface,
	redisClient *redis.Client,
) *AuthService {
	return &AuthService{
		cfg:         cfg,
		userRepo:    userRepo,
		redisClient: redisClient,
	}
}

func (s *AuthService) RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error {
	return nil
}

func (s *AuthService) Register(ctx context.Context, req dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error) {
	return nil, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req dto.LoginRequestDto,
) (*dto.LoginResponseDto, error) {

	if s.redisClient == nil {
		return nil, errors.New("redis client is not initialized")
	}

	// ===== 1. Check user =====
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

	// ===== 2. Parse DeviceID from string to UUID =====
	deviceID, err := uuid.Parse(req.DeviceInfo.DeviceID)
	if err != nil {
		return nil, errors.New("invalid device_id format")
	}

	// ===== 3. Get/Set user_version (for logout all) =====
	userVersionKey := fmt.Sprintf("user_version:%s", user.ID)
	userVersion := int64(1)

	userVerStr, err := s.redisClient.Get(ctx, userVersionKey).Result()
	if err == nil {
		userVersion, _ = strconv.ParseInt(userVerStr, 10, 64)
	} else if err == redis.Nil {
		// First login ever → create user_version = 1
		if err := s.redisClient.Set(ctx, userVersionKey, userVersion, 0).Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}


	// ===== 4. Generate JWT =====
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, deviceID, userVersion)
	if err != nil {
		return nil, err
	}

	// ===== 5. Store refresh token in Redis =====
	// Key: refresh_token:{userId}:{deviceId}
	refreshTokenKey := fmt.Sprintf("refresh_token:%s:%s", user.ID, deviceID)
	if err := s.redisClient.Set(ctx, refreshTokenKey, refreshToken, s.cfg.JWTRefreshExpiration).Err(); err != nil {
		return nil, err
	}

	// ===== 6. Save device session metadata (optional - for device management) =====
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

	// Use HSet to store multiple devices per user
	if err := s.redisClient.HSet(ctx, sessionKey, deviceID.String(), sessionBytes).Err(); err != nil {
		return nil, err
	}

	// ===== 7. Get all active devices from Redis =====
	allSessions, err := s.redisClient.HGetAll(ctx, sessionKey).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	var activeDevices []dto.DeviceSessionDto
	for _, sessionData := range allSessions {
		var session deviceSession
		if err := json.Unmarshal([]byte(sessionData), &session); err == nil {
			activeDevices = append(activeDevices, dto.DeviceSessionDto{
				DeviceID:   session.DeviceID.String(),
				DeviceName: session.DeviceName,
				UserAgent:  session.UserAgent,
				LoggedInAt: session.LoggedInAt,
			})
		}
	}

	// ===== 8. Build response =====
	status := "inactive"
	if user.IsActive {
		status = "active"
	}

	var dob *string
	if user.DateOfBirth != nil {
		f := user.DateOfBirth.Format("2006-01-02")
		dob = &f
	}


	currentDevice := dto.DeviceSessionDto{
		DeviceID:   deviceID.String(),
		DeviceName: req.DeviceInfo.DeviceName,
		UserAgent:  req.DeviceInfo.UserAgent,
		LoggedInAt: sessionPayload.LoggedInAt,
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
		ActiveDevices: activeDevices,
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

	return nil
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error {
	return nil
}
