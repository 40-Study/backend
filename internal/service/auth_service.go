package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
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
	return nil
}

func (s *AuthService) Register(ctx context.Context, req dto.RegisterDto) (*dto.RegisterResponseDto, error) {
	return nil, nil
}

func (s *AuthService) Login(ctx context.Context, req dto.LoginDTO) (*dto.LoginResponseDto, error) {
	return nil, nil
}

func (s *AuthService) Logout(ctx context.Context, userId, deviceId uuid.UUID) error {
	return nil
}

func (s *AuthService) LogoutAllDevice(ctx context.Context, userId uuid.UUID) error {
	return nil
}

func (s *AuthService) RefreshToken(ctx context.Context, oldRefreshToken string) (*dto.RefreshTokenResponseDto, error) {
	return nil, nil
}

func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponseDto, error) {
	return nil, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordDto) error {

	return nil
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordDto) error {
	return nil
}
