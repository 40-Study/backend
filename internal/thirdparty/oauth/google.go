package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

type googleUserResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type GoogleProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client
}

func NewGoogleProvider(cfg *GoogleOAuthConfig, httpClient *http.Client) *GoogleProvider {
	return &GoogleProvider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  cfg.RedirectURL,
		httpClient:   httpClient,
	}
}

func (p *GoogleProvider) Name() string {
	return "google"
}

// trả ra URL để redirect browser tới trang đăng nhập của Google, kèm state để chống CSRF
func (p *GoogleProvider) GetAuthURL(state string) string {
	params := url.Values{}
	params.Set("client_id", p.clientID)
	params.Set("redirect_uri", p.redirectURL)
	params.Set("response_type", "code")
	params.Set("scope", "openid email profile")
	params.Set("state", state)
	params.Set("access_type", "offline") // để lấy refresh token nếu cần sau này
	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

// ExchangeCode đổi authorization code lấy access token từ Google
// POST tới https://oauth2.googleapis.com/token
func (p *GoogleProvider) ExchangeCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("missing oauth code")
	}

	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURL)
	form.Set("grant_type", "authorization_code")
	
	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request google token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read google token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp googleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("unmarshal google token response: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("google oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return "", errors.New("google access token is empty")
	}

	return tokenResp.AccessToken, nil
}

// GetUserInfo lấy thông tin user từ Google API rồi chuẩn hóa thành OAuthUserInfo
// GET https://www.googleapis.com/oauth2/v2/userinfo
func (p *GoogleProvider) GetUserInfo(accessToken string) (*OAuthUserInfo, error) {
	if accessToken == "" {
		return nil, errors.New("access token is empty")
	}

	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("create google userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request google userinfo endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google userinfo endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var gUser googleUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&gUser); err != nil {
		return nil, fmt.Errorf("decode google userinfo response: %w", err)
	}

	// Lấy phần trước @ làm username (Google không có username riêng)
	username := gUser.Email
	if idx := strings.Index(gUser.Email, "@"); idx > 0 {
		username = gUser.Email[:idx]
	}

	var name *string
	if gUser.Name != "" {
		name = &gUser.Name
	}
	var avatar *string
	if gUser.Picture != "" {
		avatar = &gUser.Picture
	}
	var email *string
	if gUser.Email != "" {
		email = &gUser.Email
	}

	return &OAuthUserInfo{
		ProviderUserID: gUser.ID,
		Email:          email,
		Name:           name,
		AvatarURL:      avatar,
		Username:       username,
	}, nil
}
