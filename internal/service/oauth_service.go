package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/thirdparty/oauth"
	"study.com/v1/internal/utils"
)

type OAuthService struct {
	cfg                *config.Config
	redisClient        *redis.Client
	userRepo           repository.UserRepositoryInterface
	oauthRepo          repository.OAuthProviderRepositoryInterface
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface
	systemRoleRepo     repository.SystemRoleRepositoryInterface
	authService        *AuthService

	// Map provider name → provider implementation
	// Thay vì hardcode từng provider, dùng map để lookup
	// Key: "github", "google", "facebook"
	providers map[string]oauth.OAuthProvider
}

type OAuthServiceInterface interface {
	// GetAuthURL tạo URL redirect tới provider (GitHub/Google/Facebook) + lưu state vào Redis
	GetAuthURL(ctx context.Context, providerName string, deviceInfo *dto.DeviceInfoDTO) (string, error)
	// HandleCallback xử lý callback từ provider sau khi user authorize
	HandleCallback(ctx context.Context, providerName, code, state string) (*dto.LoginResponseDto, error)
	// ListLinkedAccounts trả về danh sách provider đã liên kết với user
	ListLinkedAccounts(ctx context.Context, userID uuid.UUID) ([]dto.LinkedAccountDTO, error)
	// DisconnectProvider ngắt liên kết một provider khỏi user
	DisconnectProvider(ctx context.Context, userID uuid.UUID, providerName string) error
}

func NewOAuthService(
	cfg *config.Config,
	redisClient *redis.Client,
	userRepo repository.UserRepositoryInterface,
	oauthRepo repository.OAuthProviderRepositoryInterface,
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
	authService *AuthService,
	providers map[string]oauth.OAuthProvider,
) *OAuthService {
	return &OAuthService{
		cfg:                cfg,
		redisClient:        redisClient,
		userRepo:           userRepo,
		oauthRepo:          oauthRepo,
		userSystemRoleRepo: userSystemRoleRepo,
		systemRoleRepo:     systemRoleRepo,
		authService:        authService,
		providers:          providers,
	}
}

// GetAuthURL tạo URL redirect tới provider + lưu state vào Redis chống CSRF
// providerName: "github", "google", "facebook"
// Hoạt động cho mọi provider nhờ interface OAuthProvider
func (s *OAuthService) GetAuthURL(ctx context.Context, providerName string, deviceInfo *dto.DeviceInfoDTO) (string, error) {
	// Tìm provider trong map
	provider, ok := s.providers[providerName]
	if !ok {
		return "", fmt.Errorf("unsupported oauth provider: %s", providerName)
	}

	// state phải đủ dài và đủ ngẫu nhiên để tránh bị đoán trước dẫn đến tấn công CSRF
	state := utils.GenerateShortCode(32)
	if state == "" {
		return "", errors.New("failed to generate oauth state")
	}

	// Lưu device info + tên provider vào Redis với TTL 10 phút
	// để sau này khi callback về có thể kiểm tra tính hợp lệ của state
	// và lấy thông tin thiết bị ra để tạo session
	stateData := dto.OAuthState{
		Provider:   providerName,
		DeviceID:   deviceInfo.DeviceID,
		DeviceName: deviceInfo.DeviceName,
		OS:         deviceInfo.OS,
	}
	stateBytes, err := json.Marshal(stateData)
	if err != nil {
		return "", fmt.Errorf("marshal state data: %w", err)
	}
	if err := s.redisClient.Set(ctx, "oauth:state:"+state, stateBytes, 10*time.Minute).Err(); err != nil {
		return "", fmt.Errorf("save oauth state: %w", err)
	}

	// Gọi provider.GetAuthURL — mỗi provider tự build URL theo API riêng
	authURL := provider.GetAuthURL(state)
	return authURL, nil
}

// HandleCallback xử lý callback từ provider sau khi user authorize
// Nhận code và state, verify state, đổi code lấy token, lấy user info
// Tìm/tạo user trong hệ thống rồi login
//
// Flow:
// 1. Verify state từ Redis (chống CSRF, one-time use)
// 2. Trao đổi code lấy access token từ provider
// 3. Lấy thông tin user từ provider API
// 4. Tìm user trong hệ thống:
//   - Case A: Đã link provider trước đó → login trực tiếp
//   - Case B: Chưa link nhưng email trùng user đã verified → tạo link + login
//   - Case C: User mới hoàn toàn → tạo User + UserSystemRole + UserOAuthProvider
func (s *OAuthService) HandleCallback(ctx context.Context, providerName, code, state string) (*dto.LoginResponseDto, error) {
	if state == "" || code == "" {
		return nil, errors.New("missing code or state")
	}

	// ===== 1. Verify state — chống CSRF =====
	// Lấy state data từ Redis và xóa ngay sau đó để tránh bị sử dụng lại (replay attack)
	stateKey := "oauth:state:" + state
	stateData, err := s.redisClient.Get(ctx, stateKey).Result()
	if err == redis.Nil {
		return nil, errors.New("invalid or expired OAuth state, please try again")
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth state: %w", err)
	}
	// Xóa state khỏi Redis ngay để tránh replay attack
	s.redisClient.Del(ctx, stateKey)

	var oauthState dto.OAuthState
	if err := json.Unmarshal([]byte(stateData), &oauthState); err != nil {
		return nil, fmt.Errorf("parse oauth state: %w", err)
	}

	// Verify provider trong state khớp với provider từ URL callback
	// Tránh trường hợp attacker dùng state của GitHub cho callback của Google
	if oauthState.Provider != providerName {
		return nil, fmt.Errorf("state provider mismatch: expected %s, got %s", oauthState.Provider, providerName)
	}

	// Tìm provider trong map
	provider, ok := s.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("unsupported oauth provider: %s", providerName)
	}

	// Lấy thông tin thiết bị từ state data đã lưu lúc bắt đầu flow
	deviceInfo := dto.DeviceInfoDTO{
		DeviceID:   oauthState.DeviceID,
		DeviceName: oauthState.DeviceName,
		OS:         oauthState.OS,
	}
	// Fallback nếu device info trống (ví dụ: browser không gửi)
	if deviceInfo.DeviceID == "" {
		deviceInfo.DeviceID = uuid.New().String()
	}
	if deviceInfo.DeviceName == "" {
		deviceInfo.DeviceName = "OAuth Browser"
	}
	if deviceInfo.OS == "" {
		deviceInfo.OS = "Unknown"
	}

	// ===== 2. Trao đổi code lấy access token từ provider =====
	accessToken, err := provider.ExchangeCode(code)
	if err != nil {
		return nil, fmt.Errorf("exchange code with %s: %w", providerName, err)
	}

	// ===== 3. Lấy thông tin user từ provider API =====
	// Mỗi provider trả về struct riêng, nhưng đều chuẩn hóa thành OAuthUserInfo
	userInfo, err := provider.GetUserInfo(accessToken)
	if err != nil {
		return nil, fmt.Errorf("get user info from %s: %w", providerName, err)
	}

	// ===== 4. Tìm user trong hệ thống =====
	return s.findOrCreateUser(ctx, providerName, userInfo, deviceInfo)
}

// findOrCreateUser xử lý logic chung cho mọi provider:
// tìm user đã link, link email trùng, hoặc tạo user mới
func (s *OAuthService) findOrCreateUser(
	ctx context.Context,
	providerName string,
	userInfo *oauth.OAuthUserInfo,
	deviceInfo dto.DeviceInfoDTO,
) (*dto.LoginResponseDto, error) {
	// Kiểm tra xem đã có user nào liên kết với provider account này chưa (qua bảng user_oauth_providers)
	existingOAuth, err := s.oauthRepo.FindByProviderAndProviderUserID(ctx, providerName, userInfo.ProviderUserID)
	if err != nil {
		return nil, fmt.Errorf("find oauth provider: %w", err)
	}

	// Case A: Đã link provider trước đó → login trực tiếp, không cần tạo gì thêm
	if existingOAuth != nil {
		user, err := s.userRepo.FindUserByID(ctx, existingOAuth.UserID)
		if err != nil {
			return nil, fmt.Errorf("find user: %w", err)
		}
		if user == nil {
			return nil, errors.New("linked user not found")
		}
		return s.authService.LoginWithOAuth(ctx, user, deviceInfo)
	}

	// Case B: Chưa link provider — kiểm tra xem có user nào trùng email không
	// Nếu đã có user với email này rồi thì sẽ liên kết tài khoản provider với user đó
	if userInfo.Email != nil && *userInfo.Email != "" {
		existingUser, err := s.userRepo.FindUserByEmail(ctx, *userInfo.Email)
		if err != nil {
			return nil, fmt.Errorf("find user by email: %w", err)
		}

		if existingUser != nil {
			// Chỉ auto-link nếu account đã verified — tránh trường hợp
			// attacker đăng ký email trước rồi dùng OAuth để chiếm account
			if !existingUser.IsVerified {
				return nil, errors.New("an account with this email exists but is not verified, please login with password first")
			}

			// Tạo bảng user_oauth_providers để lưu thông tin liên kết
			oauthRecord := &model.UserOAuthProvider{
				UserID:         existingUser.ID,
				Provider:       providerName,
				ProviderUserID: userInfo.ProviderUserID,
				Email:          userInfo.Email,
				AvatarURL:      userInfo.AvatarURL,
			}
			if err := s.oauthRepo.Create(ctx, oauthRecord); err != nil {
				return nil, fmt.Errorf("link %s to existing user: %w", providerName, err)
			}

			return s.authService.LoginWithOAuth(ctx, existingUser, deviceInfo)
		}
	}

	// Case C: User mới hoàn toàn — tạo account với đầy đủ các bảng liên quan
	if userInfo.Email == nil || *userInfo.Email == "" {
		return nil, fmt.Errorf("your %s account does not have a verified email", providerName)
	}

	newUser, err := s.createOAuthUser(ctx, providerName, userInfo)
	if err != nil {
		return nil, fmt.Errorf("create oauth user: %w", err)
	}

	return s.authService.LoginWithOAuth(ctx, newUser, deviceInfo)
}

// createOAuthUser tạo user mới và tất cả các bảng liên quan khi đăng ký qua OAuth:
// 1. User — tạo user với placeholder password hash (bcrypt compare sẽ luôn fail)
// 2. UserSystemRole — gán role mặc định STUDENT
// 3. UserOAuthProvider — lưu liên kết giữa user và provider account
func (s *OAuthService) createOAuthUser(ctx context.Context, providerName string, userInfo *oauth.OAuthUserInfo) (*model.User, error) {
	// Tạo user mới với thông tin từ provider
	// PasswordHash dùng placeholder vì OAuth user không cần password,
	// nếu sau này user muốn set password thì sẽ có endpoint riêng
	user := &model.User{
		Email:        *userInfo.Email,
		PasswordHash: "$oauth$no-password", // Placeholder — bcrypt compare sẽ luôn fail
		UserName:     userInfo.Username,
		FullName:     userInfo.Name,
		AvatarURL:    userInfo.AvatarURL,
		IsVerified:   true, // Đã verify qua provider
		IsActive:     true,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		// Nếu username bị trùng, thử thêm suffix ngẫu nhiên
		if strings.Contains(err.Error(), "user_name") || strings.Contains(err.Error(), "duplicate") {
			user.UserName = userInfo.Username + "_" + utils.GenerateShortCode(4)
			if err := s.userRepo.CreateUser(ctx, user); err != nil {
				return nil, fmt.Errorf("create user: %w", err)
			}
		} else {
			return nil, fmt.Errorf("create user: %w", err)
		}
	}

	// Gán role mặc định STUDENT cho user mới
	// Tìm system role "STUDENT" trong database (đã seed sẵn)
	studentRole, err := s.systemRoleRepo.GetSystemRoleByName(ctx, "STUDENT")
	if err != nil {
		return nil, fmt.Errorf("find student role: %w", err)
	}
	if studentRole == nil {
		return nil, errors.New("default STUDENT system role not found in database")
	}

	// Tạo bản ghi UserSystemRole để gán role cho user
	if err := s.userSystemRoleRepo.Create(ctx, &model.UserSystemRole{
		UserID:       user.ID,
		SystemRoleID: studentRole.ID,
		GrantedAt:    time.Now(),
		Status:       model.UserSystemRoleStatusActive,
	}); err != nil {
		return nil, fmt.Errorf("assign student role: %w", err)
	}

	// Tạo bản ghi UserOAuthProvider để lưu liên kết giữa user mới và provider account
	// Sau này khi user login lại sẽ tìm qua bảng này (Case A ở trên)
	if err := s.oauthRepo.Create(ctx, &model.UserOAuthProvider{
		UserID:         user.ID,
		Provider:       providerName,
		ProviderUserID: userInfo.ProviderUserID,
		Email:          userInfo.Email,
		AvatarURL:      userInfo.AvatarURL,
	}); err != nil {
		return nil, fmt.Errorf("create oauth provider link: %w", err)
	}

	return user, nil
}

// ListLinkedAccounts trả về danh sách các OAuth provider đã liên kết với user
func (s *OAuthService) ListLinkedAccounts(ctx context.Context, userID uuid.UUID) ([]dto.LinkedAccountDTO, error) {
	providers, err := s.oauthRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find linked accounts: %w", err)
	}

	result := make([]dto.LinkedAccountDTO, 0, len(providers))
	for _, p := range providers {
		result = append(result, dto.LinkedAccountDTO{
			Provider:    p.Provider,
			Email:       p.Email,
			ConnectedAt: p.CreatedAt,
		})
	}
	return result, nil
}

// DisconnectProvider ngắt liên kết một OAuth provider khỏi tài khoản user
// Chỉ cho phép nếu user còn ít nhất một phương thức xác thực khác (password hoặc provider khác)
func (s *OAuthService) DisconnectProvider(ctx context.Context, userID uuid.UUID, providerName string) error {
	// Lấy user để kiểm tra có password không
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return errors.New("user not found")
	}

	// Lấy danh sách provider đã liên kết
	providers, err := s.oauthRepo.FindByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find linked accounts: %w", err)
	}

	// Kiểm tra user có password thực sự không (không phải placeholder OAuth)
	hasPassword := user.PasswordHash != "" && user.PasswordHash != "$oauth$no-password" && !strings.HasPrefix(user.PasswordHash, "$oauth$")

	// Cho phép disconnect nếu còn ít nhất 1 provider khác hoặc có password
	if !hasPassword && len(providers) <= 1 {
		return errors.New("cannot disconnect last authentication method")
	}

	return s.oauthRepo.DeleteByUserIDAndProvider(ctx, userID, providerName)
}
