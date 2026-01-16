package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"tiger.com/v2/src/config"
	"tiger.com/v2/src/dto"
	"tiger.com/v2/src/model"
	"tiger.com/v2/src/repository"
	"tiger.com/v2/src/utils"
)

type AuthServiceInterface interface {
	RequestRegister(ctx context.Context, req dto.RegisterRequestDto) error
	Register(ctx context.Context, req dto.RegisterDto) (*dto.RegisterResponseDto, error)
	Login(ctx context.Context, req dto.LoginDTO) (*dto.LoginResponseDto, error)
	Logout(ctx context.Context, userId, deviceId uuid.UUID) error
	LogoutAllDevice(ctx context.Context, userId uuid.UUID) error
	RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error)
	GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordDto) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, req dto.ResetPasswordDto) error
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
	email := req.Email
	us, err := s.userRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return errors.New("Database error")
	}
	if us != nil {
		return errors.New("Email already exist")
	}

	registerRedisKey := "register:otp:" + email
	exists, err := s.redisClient.Exists(ctx, registerRedisKey).Result()
	if err != nil {
		return errors.New("Redis not connected")
	}
	if exists == 1 {
		return errors.New("Please wait 1 minute before requesting new OTP")
	}

	registerOTP, err := utils.GenerateOTP(6)
	if err != nil {
		return errors.New("OTP gene fail")
	}
	setKeyErr := s.redisClient.Set(ctx, registerRedisKey, registerOTP, 1*time.Minute).Err()
	if setKeyErr != nil {
		return errors.New("Set key errors")
	}

	go utils.SendRegisterOTP(s.cfg, email, registerOTP)

	return nil
}

func (s *AuthService) Register(ctx context.Context, req dto.RegisterDto) (*dto.RegisterResponseDto, error) {
	redisKey := "register:otp:" + req.Email
	storedOTP, getErr := s.redisClient.Get(ctx, redisKey).Result()
	if getErr != nil {
		return nil, errors.New("OTP expired or not found")
	}
	if storedOTP != req.OTP {
		return nil, errors.New("Invalid OTP")
	}
	s.redisClient.Del(ctx, redisKey)
	if len(req.Password) < 6 {
		return nil, errors.New("password is easy")
	}
	hashPassword, passErr := utils.HashPassword(req.Password)
	if passErr != nil {
		return nil, errors.New("Hash password invalid")
	}

	dateOfBirth, err := time.Parse("2006-01-02", req.DateOfBirth)
	if err != nil {
		return nil, errors.New("Invalid date format, use YYYY-MM-DD")
	}

	user := &model.User{
		Email:        req.Email,
		Username:     req.UserName,
		PasswordHash: hashPassword,
		Phone:        pgtype.Text{String: req.Phone, Valid: true},
		DateOfBirth:  pgtype.Timestamp{Time: dateOfBirth, Valid: true},
		Gender:       pgtype.Text{String: req.Gender, Valid: true},
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	var phone *string
	if user.Phone.Valid {
		phone = &user.Phone.String
	}

	var gender *string
	if user.Gender.Valid {
		gender = &user.Gender.String
	}

	var dob *string
	if user.DateOfBirth.Valid {
		s := user.DateOfBirth.Time.Format("2006-01-02")
		dob = &s
	}
	userDto := dto.UserResponseDto{
		ID:          user.ID,
		Email:       user.Email,
		Username:    user.Username,
		Phone:       phone,
		DateOfBirth: dob,
		Gender:      gender,
	}

	return &dto.RegisterResponseDto{
		User: userDto,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req dto.LoginDTO) (*dto.LoginResponseDto, error) {

	if !utils.IsValidEmail(req.Email) {
		return nil, errors.New("Email or password is incorrect")
	}

	if strings.TrimSpace(req.Password) == "" {
		return nil, errors.New("Email or password is incorrect")
	}

	if req.DeviceId == uuid.Nil {
		return nil, errors.New("DeviceId is required")
	}

	user, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, errors.New("Database error")
	}
	if user == nil {
		return nil, errors.New("Email or password is incorrect")
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		return nil, errors.New("Email or password is incorrect")
	}
	version, err := s.redisClient.Get(ctx, "session_version:"+user.ID.String()).Int()

	if err == redis.Nil {
		s.redisClient.Set(ctx, "session_version:"+user.ID.String(), 1, 0)
		version = 1
	}

	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, user.ID, req.DeviceId, int64(version))
	if err != nil {
		return nil, errors.New("Generate token error")
	}

	rfKey := fmt.Sprintf("refreshToken:%s:%s", user.ID, req.DeviceId)

	s.redisClient.Del(ctx, rfKey)

	if err := s.redisClient.Set(ctx, rfKey, refreshToken, 24*time.Hour).Err(); err != nil {
		return nil, errors.New("Cannot store refresh token")
	}

	sessionKey := fmt.Sprintf("session:%s:%s", user.ID, req.DeviceId)
	sessionValue := fmt.Sprintf("%s|%s|%s|%s", req.DeviceName, req.Platform, req.UserAgent, req.IpAddress)
	s.redisClient.Set(ctx, sessionKey, sessionValue, 30*24*time.Hour)

	dob := user.DateOfBirth.Time.String()

	return &dto.LoginResponseDto{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserResponseDto{
			ID:          user.ID,
			Username:    user.Username,
			Email:       user.Email,
			Phone:       &user.Phone.String,
			AvatarUrl:   &user.AvatarUrl.String,
			DateOfBirth: &dob,
			Gender:      &user.Gender.String,
			CreatedAt:   user.CreatedAt.String(),
		},
	}, nil
}


func (s *AuthService) Logout(ctx context.Context, userId, deviceId uuid.UUID) error {
	if userId == uuid.Nil {
		return errors.New("invalid userId")
	}

	if deviceId == uuid.Nil {
		return errors.New("invalid deviceId")
	}

	rfKey := fmt.Sprintf("refreshToken:%s:%s", userId, deviceId)
	sessionKey := fmt.Sprintf("session:%s:%s", userId, deviceId)
	if deleted, err := s.redisClient.Del(ctx, rfKey).Result(); err != nil {
		return errors.New("redis error while deleting refresh token")
	} else if deleted == 0 {
	}
	if _, err := s.redisClient.Del(ctx, sessionKey).Result(); err != nil {
		return errors.New("redis error while deleting session info")
	}
	return nil
}

func (s *AuthService) LogoutAllDevice(ctx context.Context, userId uuid.UUID) error {
	_, err := s.redisClient.Incr(ctx, "session_version:"+userId.String()).Result()
	return err
}

func (s *AuthService) RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error) {
	claims, err := utils.ParseToken(s.cfg, oldRefreshToken)
	if err != nil {
		return nil, errors.New("Invalid or expired token")
	}
	uid, did, vid := claims.UserID, claims.DeviceID, claims.Version
	rfKey := fmt.Sprintf("refreshToken:%s:%s", uid, did)
	storedToken, err := s.redisClient.Get(ctx, rfKey).Result()
	if err == redis.Nil {
		return nil, errors.New("Refresh token not found or expired")
	}
	if err != nil {
		return nil, errors.New("Redis error")
	}
	if storedToken != oldRefreshToken {
		return nil, errors.New("Invalid refresh token")
	}
	accessToken, refreshToken, err := utils.GenerateTokens(s.cfg, uid, did, vid)
	if err != nil {
		return nil, errors.New("Generate token error")
	}

	if err := s.redisClient.Set(ctx, rfKey, refreshToken, 24*time.Hour).Err(); err != nil {
		return nil, errors.New("Cannot store refresh token")
	}

	return &dto.RefreshTokenResponseDto{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error) {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, errors.New("Database error")
	}
	if user == nil {
		return nil, errors.New("User not found")
	}
	var phone *string
	if user.Phone.Valid {
		phone = &user.Phone.String
	}
	var gender *string
	if user.Gender.Valid {
		gender = &user.Gender.String
	}
	var dob *string
	if user.DateOfBirth.Valid {
		s := user.DateOfBirth.Time.Format("2006-01-02")
		dob = &s
	}
	return &dto.UserResponseDto{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Phone:       phone,
		AvatarUrl:   &user.AvatarUrl.String,
		DateOfBirth: dob,
		Gender:      gender,
		Status:      &user.Status.String,
		CreatedAt:   user.CreatedAt.String(),
	}, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordDto) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return errors.New("Database error")
	}
	if user == nil {
		return errors.New("User not found")
	}
	if !utils.CheckPassword(req.OldPassword, user.PasswordHash) {
		return errors.New("Old password is incorrect")
	}
	newHashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return errors.New("Hash password error")
	}
	if err := s.userRepo.UpdatePasswordHash(ctx, userID, newHashedPassword); err != nil {
		return errors.New("Update password error")
	}
	return nil
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	ok := utils.IsValidEmail(email)
	if !ok {
		return errors.New("email invalid")
	}
	user, err := s.userRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return errors.New("repository is error")
	}
	if user == nil {
		return errors.New("user not found")
	}
	otp, err := utils.GenerateOTP(6)
	redisKey := fmt.Sprintf("reset:password:%s", email)
	_, checkKey := s.redisClient.Get(ctx, redisKey).Result()

	if checkKey == redis.Nil {
		if err := s.redisClient.Set(ctx, redisKey, otp, 1*time.Minute).Err(); err != nil {
			return err
		}
		go utils.SendResetPasswordOTP(s.cfg, email, otp)
		return nil

	} else if checkKey != nil {
		return errors.New("redis errors")
	}
	return errors.New("Try next ont minute")
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordDto) error {
	ok := utils.IsValidEmail(req.Email)
	if !ok {
		return errors.New("email invalid")
	}
	user, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return errors.New("repository is error")
	}
	if user == nil {
		return errors.New("user not found")
	}
	val, err := s.redisClient.Get(ctx, "reset:password:"+req.Email).Result()
	if err == redis.Nil {
		return errors.New("Dont have reset password request")
	} else if err != nil {
		return errors.New("redis errors")
	}
	if req.OTP != val {
		return errors.New("OTP not found")
	}
	newPasswordHash, hashOk := utils.HashPassword(req.NewPassword)
	if hashOk != nil {
		return errors.New("Hash password error")
	}
	if err := s.userRepo.UpdatePasswordHash(ctx, user.ID, newPasswordHash); err != nil {
		return errors.New("Update password error")
	}
	s.redisClient.Del(ctx, "reset:password:"+req.Email)
	return nil
}
