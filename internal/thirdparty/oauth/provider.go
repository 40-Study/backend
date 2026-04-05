package oauth

// OAuthUserInfo — thông tin user chuẩn hóa mà mọi provider đều trả về
// Bất kể GitHub, Google hay Facebook, cuối cùng đều cần mấy field này
// OAuthService chỉ làm việc với struct này, không biết chi tiết response của từng provider
type OAuthUserInfo struct {
	ProviderUserID string  // ID user trên provider (GitHub: "12345", Google: "abc123")
	Email          *string // email chính (có thể nil nếu user ẩn email)
	Name           *string // tên hiển thị
	AvatarURL      *string // ảnh đại diện
	Username       string  // username (GitHub: login, Google: email prefix, FB: "fb_"+id)
}

// OAuthProvider — interface chung cho tất cả OAuth provider
// Mỗi provider (GitHub, Google, FB) implement interface này
// OAuthService không cần biết chi tiết API của từng provider, chỉ gọi qua interface
//
// Thêm provider mới (Apple, Discord, ...): chỉ cần tạo 1 file implement 4 methods này
type OAuthProvider interface {
	// Name trả về tên provider: "github", "google", "facebook"
	// Dùng để lưu vào bảng user_oauth_providers.provider
	Name() string

	// GetAuthURL tạo URL redirect tới provider để user authorize
	// state đã được OAuthService tạo sẵn (random, lưu Redis, chống CSRF)
	// Return: URL hoàn chỉnh để redirect browser tới trang đăng nhập của provider
	GetAuthURL(state string) string

	// ExchangeCode đổi authorization code lấy access token từ provider
	// code: authorization code mà provider gửi về qua callback URL
	// Return: access token string để gọi API lấy thông tin user
	ExchangeCode(code string) (string, error)

	// GetUserInfo dùng access token để lấy thông tin user từ provider API
	// Mỗi provider có API khác nhau (GitHub: /user, Google: /userinfo, FB: /me)
	// nhưng kết quả đều được chuẩn hóa thành OAuthUserInfo
	GetUserInfo(accessToken string) (*OAuthUserInfo, error)
}
