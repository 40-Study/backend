package service

// Test Postgres THẬT cho issue #105: tài khoản Google mới KHÔNG được có user_name sinh từ email, và
// user_name phải duy nhất. Đổi lại cách sinh sang email prefix hoặc bỏ kiểm tra tồn tại thì test ĐỎ.

import (
	"regexp"
	"strings"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/thirdparty/oauth"
)

func newOAuthUsernameSvc(t *testing.T) (*OAuthService, *repository.UserRepository) {
	t.Helper()
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	users := repository.NewUserRepository(db)
	return &OAuthService{userRepo: users, oauthRepo: repository.NewOAuthProviderRepository(db)}, users
}

func TestCreateOAuthUser_Google_UserNameKhongTuEmail(t *testing.T) {
	svc, _ := newOAuthUsernameSvc(t)
	email, name := "nguyen.van.a@gmail.com", "Nguyễn Văn A"
	// Username rỗng đúng như googleUserToOAuthInfo trả về.
	u, err := svc.createOAuthUser(t.Context(), "google", &oauth.OAuthUserInfo{ProviderUserID: "g-1", Email: &email, Name: &name})
	if err != nil {
		t.Fatalf("createOAuthUser: %v", err)
	}
	if strings.Contains(strings.ToLower(u.UserName), "nguyen.van.a") || strings.Contains(strings.ToLower(u.UserName), "nguyenvana@") {
		t.Errorf("user_name lộ email: %q", u.UserName)
	}
	if !regexp.MustCompile(`^nguyenvana_[a-z0-9]{6}$`).MatchString(u.UserName) {
		t.Errorf("user_name phải là <tên không dấu>_<6 ký tự ngẫu nhiên>, nhận %q", u.UserName)
	}
}

func TestCreateOAuthUser_Google_UserNameKhongTrung(t *testing.T) {
	svc, users := newOAuthUsernameSvc(t)
	seen := map[string]bool{}
	for i := 0; i < 15; i++ {
		email := "same." + string(rune('a'+i)) + "@gmail.com"
		name := "Cùng Một Tên"
		u, err := svc.createOAuthUser(t.Context(), "google", &oauth.OAuthUserInfo{ProviderUserID: "g-" + email, Email: &email, Name: &name})
		if err != nil {
			t.Fatalf("lần %d: %v", i, err)
		}
		if seen[u.UserName] {
			t.Fatalf("user_name trùng: %q", u.UserName)
		}
		seen[u.UserName] = true
	}
	ok, err := users.UserNameExists(t.Context(), strings.ToUpper(firstKey(seen)))
	if err != nil || !ok {
		t.Errorf("UserNameExists phải không phân biệt hoa thường: ok=%v err=%v", ok, err)
	}
	if ok, _ := users.UserNameExists(t.Context(), "khong-ton-tai-xyz"); ok {
		t.Error("UserNameExists báo có cho tên chưa dùng")
	}
}

// Provider có username riêng (GitHub, Facebook) vẫn dùng đúng giá trị đó.
func TestCreateOAuthUser_ProviderCoUsername_GiuNguyen(t *testing.T) {
	svc, _ := newOAuthUsernameSvc(t)
	email := "gh@example.com"
	u, err := svc.createOAuthUser(t.Context(), "github", &oauth.OAuthUserInfo{ProviderUserID: "gh-1", Email: &email, Username: "octocat"})
	if err != nil {
		t.Fatal(err)
	}
	if u.UserName != "octocat" {
		t.Errorf("GitHub login phải giữ nguyên, nhận %q", u.UserName)
	}
	var _ = model.User{}
}

func firstKey(m map[string]bool) string {
	for k := range m {
		return k
	}
	return ""
}
