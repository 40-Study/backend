package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"study.com/v1/internal/config"
)

// ===== GitHub-specific response structs (private, chỉ dùng trong file này) =====

type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// githubUserResponse — response khi gọi GET /user để lấy thông tin user từ GitHub API
type githubUserResponse struct {
	ID        int     `json:"id"`
	Login     string  `json:"login"`
	Email     *string `json:"email"`
	Name      *string `json:"name"`
	AvatarURL *string `json:"avatar_url"`
}

// githubEmailResponse — mỗi phần tử trong response GET /user/emails đây là api ở trong docs của github
type githubEmailResponse struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// ===== GitHubProvider =====

type GitHubProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	authURL      string       // "https://github.com/login/oauth/authorize"
	tokenURL     string       // "https://github.com/login/oauth/access_token"
	scopes       []string     // ["user:email", "read:user"]
	httpClient   *http.Client // dùng để gửi request HTTP tới GitHub, được inject từ bên ngoài để dễ test
}

func NewGitHubProvider(cfg *config.GithubOAuthConfig, httpClient *http.Client) *GitHubProvider {
	return &GitHubProvider{
		clientID:     cfg.ClientID,     // clientID là bắt buộc, lấy từ config
		clientSecret: cfg.ClientSecret, // clientSecret là bắt buộc, lấy từ config
		redirectURL:  cfg.RedirectURL,  // redirectURL phải trùng với URL đã đăng ký trên GitHub developer, hiện tại là http://localhost:5000/oauth/github/callback
		authURL:      cfg.Endpoint.AuthURL,
		tokenURL:     cfg.Endpoint.TokenURL,
		scopes:       cfg.Scopes, // là mảng string để dễ mở rộng nếu sau này muốn thêm scope, hiện tại ["user:email", "read:user"] để lấy email và thông tin cơ bản
		httpClient:   httpClient, // httpClient được inject từ bên ngoài để dễ test, trong thực tế sẽ là http.DefaultClient
	}
}

// function trả về trên của provider, dùng để lưu vào database và phân biệt khi callback về
func (p *GitHubProvider) Name() string {
	return "github"
}

// GetAuthURL tạo URL redirect tới GitHub với các query param cần thiết
// state đã được OAuthService tạo sẵn và lưu Redis, dùng để chống CSRF
func (p *GitHubProvider) GetAuthURL(state string) string {
	params := url.Values{}
	params.Set("client_id", p.clientID)              // client_id là bắt buộc, lấy từ config
	params.Set("redirect_uri", p.redirectURL)        // redirect_uri phải trùng với URL đã đăng ký trên GitHub developer, hiện tại là http://localhost:5000/oauth/github/callback
	params.Set("scope", strings.Join(p.scopes, " ")) // scope: "user:email" để lấy email, "read:user" để lấy thông tin cơ bản
	params.Set("state", state)                       // state để verify khi callback về, chống CSRF, VD: "randomstring123 + deviceId"
	return p.authURL + "?" + params.Encode()
}

// ExchangeCode đổi authorization code lấy access token từ GitHub
// Gửi POST tới https://github.com/login/oauth/access_token
// body có dạng client_id=?&client_secret=?&code=?&redirect_uri=?
func (p *GitHubProvider) ExchangeCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("missing oauth code")
	}

	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURL)

	req, err := http.NewRequest("POST", p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// gửi request tới GitHub và nhận response
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request github token endpoint: %w", err)
	}
	// đóng response body sau khi đọc xong để tránh rò rỉ tài nguyên
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", fmt.Errorf("read github token response: %w", err)
	}

	// Nếu response trả về lỗi (status code != 200)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp githubTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("unmarshal github token response: %w", err)
	}

	// github có thể trả error trong JSON body thay vì HTTP status code
	if tokenResp.Error != "" {
		return "", fmt.Errorf("github oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return "", errors.New("github access token is empty")
	}

	return tokenResp.AccessToken, nil
}

// GetUserInfo lấy thông tin user từ GitHub API rồi chuẩn hóa thành OAuthUserInfo
// Gọi GET https://api.github.com/user, nếu email bị ẩn thì gọi thêm /user/emails
func (p *GitHubProvider) GetUserInfo(accessToken string) (*OAuthUserInfo, error) {
	if accessToken == "" {
		return nil, errors.New("access token is empty")
	}

	// tạo request GET tới github nhưng đoạn này chưa gửi
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create github user request: %w", err)
	}
	// Thêm header Authorization với giá trị "Bearer "+accessToken để xác thực với GitHub,
	// đồng thời thêm header Accept để yêu cầu GitHub trả về JSON
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	// gửi request tới GitHub và nhận response
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request github user endpoint: %w", err)
	}
	// Đóng response body sau khi đọc xong để tránh rò rỉ tài nguyên
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github user endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	// đọc body và decode vào struct githubUserResponse
	var ghUser githubUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decode github user response: %w", err)
	}

	// Nếu GitHub không trả về email trong response (user ẩn email)
	// thì gọi thêm API /user/emails để lấy email chính
	if ghUser.Email == nil || *ghUser.Email == "" {
		email, err := p.getPrimaryEmail(accessToken)
		if err != nil {
			return nil, fmt.Errorf("get github primary email: %w", err)
		}
		ghUser.Email = &email
	}

	// Chuẩn hóa thành OAuthUserInfo — struct chung cho mọi provider
	return &OAuthUserInfo{
		ProviderUserID: strconv.Itoa(ghUser.ID),
		Email:          ghUser.Email,
		Name:           ghUser.Name,
		AvatarURL:      ghUser.AvatarURL,
		Username:       ghUser.Login,
	}, nil
}

// getPrimaryEmail gọi GET /user/emails để lấy email chính khi user ẩn email trên profile
func (p *GitHubProvider) getPrimaryEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("create github emails request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request github emails endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github emails endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var emails []githubEmailResponse
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decode github emails response: %w", err)
	}

	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email, nil
		}
	}

	return "", errors.New("no verified primary email found on GitHub account")
}
