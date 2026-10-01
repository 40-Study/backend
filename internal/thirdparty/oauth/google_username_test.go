package oauth

import "testing"

// Issue #105: user_name không được sinh từ phần trước '@' của email Google.
func TestGoogleUserToOAuthInfo_KhongLayUserNameTuEmail(t *testing.T) {
	info := googleUserToOAuthInfo(googleUserResponse{ID: "g1", Email: "nguyen.van.a@gmail.com", Name: "Nguyễn Văn A"})
	if info.Username != "" {
		t.Errorf("Username phải để trống để service tự sinh, nhận %q", info.Username)
	}
	if info.Email == nil || *info.Email != "nguyen.van.a@gmail.com" || info.Name == nil || *info.Name != "Nguyễn Văn A" {
		t.Errorf("email/tên phải giữ nguyên: %+v", info)
	}
}
