package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type facebookTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type facebookErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type facebookUserResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"picture"`
}

// ===== FacebookProvider =====

// FacebookOAuthConfig chứa thông tin cấu hình cho Facebook OAuth
// Facebook gọi ClientID là App ID, ClientSecret là App Secret
type FacebookOAuthConfig struct {
	ClientID     string // App ID
	ClientSecret string // App Secret
	RedirectURL  string
}

type FacebookProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client
}

func NewFacebookProvider(cfg *FacebookOAuthConfig, httpClient *http.Client) *FacebookProvider {
	return &FacebookProvider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  cfg.RedirectURL,
		httpClient:   httpClient,
	}
}

func (p *FacebookProvider) Name() string {
	return "facebook"
}

// GetAuthURL tạo URL redirect tới Facebook OAuth dialog
// scope "email,public_profile" để lấy email và thông tin cơ bản
func (p *FacebookProvider) GetAuthURL(state string) string {
	params := url.Values{}
	params.Set("client_id", p.clientID)
	params.Set("redirect_uri", p.redirectURL)
	params.Set("scope", "email,public_profile")
	params.Set("state", state)
	return fmt.Sprint("https://www.facebook.com/v25.0/dialog/oauth?" + params.Encode())
}

// ExchangeCode đổi authorization code lấy access token từ Facebook
// Facebook dùng GET (không phải POST) cho token endpoint
// GET https://graph.facebook.com/v25.0/oauth/access_token?client_id=&client_secret=&code=&redirect_uri=
func (p *FacebookProvider) ExchangeCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("missing oauth code")
	}

	params := url.Values{}
	params.Set("client_id", p.clientID)         // client_id là bắt buộc, lấy từ config
	params.Set("client_secret", p.clientSecret) // client_secret là bắt buộc, lấy từ config
	params.Set("code", code)
	params.Set("redirect_uri", p.redirectURL)

	tokenURL := "https://graph.facebook.com/v25.0/oauth/access_token?" + params.Encode()

	req, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request facebook token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read facebook token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var fbErr facebookErrorResponse
		_ = json.Unmarshal(body, &fbErr)
		if fbErr.Error.Message != "" {
			return "", fmt.Errorf("facebook oauth error: %s", fbErr.Error.Message)
		}
		return "", fmt.Errorf("facebook token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp facebookTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("unmarshal facebook token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", errors.New("facebook access token is empty")
	}

	return tokenResp.AccessToken, nil
}

// GetUserInfo lấy thông tin user từ Facebook Graph API rồi chuẩn hóa thành OAuthUserInfo
// GET https://graph.facebook.com/me?fields=id,name,email,picture.type(large)&access_token=...
func (p *FacebookProvider) GetUserInfo(accessToken string) (*OAuthUserInfo, error) {
	if accessToken == "" {
		return nil, errors.New("access token is empty")
	}

	params := url.Values{}
	// fields id,name,email,picture.type(large) để lấy ID, tên, email và avatar kích thước lớn
	params.Set("fields", "id,name,email,picture.type(large)")
	params.Set("access_token", accessToken)
	apiURL := "https://graph.facebook.com/me?" + params.Encode()

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create facebook user request: %w", err)
	}
	// thực hiện request tới Facebook Graph API để lấy thông tin user
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request facebook user endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("facebook user endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	// decode response JSON vào struct facebookUserResponse để dễ truy cập các trường dữ liệu
	var fbUser facebookUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&fbUser); err != nil {
		return nil, fmt.Errorf("decode facebook user response: %w", err)
	}

	username := "fb_" + fbUser.ID

	var name *string
	if fbUser.Name != "" {
		name = &fbUser.Name
	}
	var avatar *string
	if fbUser.Picture.Data.URL != "" {
		avatar = &fbUser.Picture.Data.URL
	}
	var email *string
	if fbUser.Email != "" {
		email = &fbUser.Email
	}

	return &OAuthUserInfo{
		ProviderUserID: fbUser.ID,
		Email:          email,
		Name:           name,
		AvatarURL:      avatar,
		Username:       username,
	}, nil
}
