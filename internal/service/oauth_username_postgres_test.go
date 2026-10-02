package service

// Test cho issue #105: tài khoản Google mới KHÔNG được có user_name sinh từ email, và user_name phải duy nhất.
//   - TestPickUniqueUserName_*: hàm thuần, ép va chạm. Bỏ bước kiểm tra tồn tại (vd `if !taken || true`), đổi số
//     lần thử, hoặc nuốt lỗi gen/exists thì ĐỎ.
//   - TestCreateOAuthUser_Google_*: Postgres thật. Đổi lại sinh từ email, hoặc bỏ nhánh sinh tên thì ĐỎ; va chạm
//     với tên đã có (không phân biệt hoa thường) phải sinh lại; hết số lần thử thì lỗi chứ không tạo user trùng.
// (Test sinh ngẫu nhiên nhiều tên KHÔNG chứng minh duy nhất vì 36^6 hầu như không va chạm, nên không dùng.)

import (
	"errors"
	"regexp"
	"strings"
	"testing"

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

func TestPickUniqueUserName_VaChamRoiThanhCong(t *testing.T) {
	calls := 0
	gen := func() (string, error) { calls++; return "ten" + string(rune('a'+calls)), nil }
	exists := func(n string) (bool, error) { return calls < 8, nil } // 7 lần đầu va chạm, lần 8 trống
	got, err := pickUniqueUserName(gen, exists, 8)
	if err != nil || got != "teni" || calls != 8 {
		t.Fatalf("muốn thành công ở lần thử thứ 8: got=%q err=%v calls=%d", got, err, calls)
	}
}

func TestPickUniqueUserName_HetSoLanThu_BaoLoiKhongTraTenTrung(t *testing.T) {
	calls := 0
	gen := func() (string, error) { calls++; return "dadung", nil }
	exists := func(string) (bool, error) { return true, nil }
	got, err := pickUniqueUserName(gen, exists, 8)
	if !errors.Is(err, errOAuthUserNameExhausted) || got != "" || calls != 8 {
		t.Fatalf("muốn errOAuthUserNameExhausted sau đúng 8 lần: got=%q err=%v calls=%d", got, err, calls)
	}
}

func TestPickUniqueUserName_LoiGenHoacExists_LanRa(t *testing.T) {
	boom := errors.New("boom")
	if _, err := pickUniqueUserName(func() (string, error) { return "", boom }, func(string) (bool, error) { return false, nil }, 3); !errors.Is(err, boom) {
		t.Errorf("lỗi gen phải lan ra: %v", err)
	}
	if _, err := pickUniqueUserName(func() (string, error) { return "x", nil }, func(string) (bool, error) { return false, boom }, 3); !errors.Is(err, boom) {
		t.Errorf("lỗi exists phải lan ra, không được coi là 'chưa dùng': %v", err)
	}
}

func TestCreateOAuthUser_Google_UserNameKhongTuEmail(t *testing.T) {
	svc, _ := newOAuthUsernameSvc(t)
	email, name := "nguyen.van.a@gmail.com", "Nguyễn Văn A"
	// Username rỗng đúng như googleUserToOAuthInfo trả về.
	u, err := svc.createOAuthUser(t.Context(), "google", &oauth.OAuthUserInfo{ProviderUserID: "g-1", Email: &email, Name: &name})
	if err != nil {
		t.Fatalf("createOAuthUser: %v", err)
	}
	if strings.Contains(strings.ToLower(u.UserName), "nguyen.van.a") {
		t.Errorf("user_name lộ email: %q", u.UserName)
	}
	if !regexp.MustCompile(`^nguyenvana[a-z0-9]{6}$`).MatchString(u.UserName) {
		t.Errorf("user_name phải là <tên không dấu><6 ký tự ngẫu nhiên>, không có '_', nhận %q", u.UserName)
	}
}

func TestCreateOAuthUser_Google_VaChamVoiTenDaCo_SinhLai(t *testing.T) {
	svc, users := newOAuthUsernameSvc(t)
	// Người dùng đầu chiếm "trungten000001"; bộ sinh lần đầu lại ra đúng tên đó (khác hoa thường) rồi mới ra tên trống.
	first, second := "trungten000001", "trungten000002"
	calls := 0
	svc.userNameGen = func(*string) (string, error) {
		calls++
		if calls == 1 {
			return "TRUNGTEN000001", nil
		}
		return second, nil
	}
	e1, e2 := "u1@gmail.com", "u2@gmail.com"
	if _, err := svc.createOAuthUser(t.Context(), "github", &oauth.OAuthUserInfo{ProviderUserID: "gh-1", Email: &e1, Username: first}); err != nil {
		t.Fatal(err)
	}
	u, err := svc.createOAuthUser(t.Context(), "google", &oauth.OAuthUserInfo{ProviderUserID: "g-2", Email: &e2})
	if err != nil {
		t.Fatal(err)
	}
	if u.UserName != second || calls != 2 {
		t.Fatalf("phải bỏ tên trùng và dùng %q sau 2 lần sinh: got %q calls=%d", second, u.UserName, calls)
	}
	if ok, _ := users.UserNameExists(t.Context(), "TrungTen000002"); !ok {
		t.Error("UserNameExists phải không phân biệt hoa thường")
	}
	if ok, _ := users.UserNameExists(t.Context(), "khong-ton-tai-xyz"); ok {
		t.Error("UserNameExists báo có cho tên chưa dùng")
	}
}

func TestCreateOAuthUser_Google_HetSoLanThu_KhongTaoUser(t *testing.T) {
	svc, users := newOAuthUsernameSvc(t)
	taken := "daco000000"
	e0 := "u0@gmail.com"
	if _, err := svc.createOAuthUser(t.Context(), "github", &oauth.OAuthUserInfo{ProviderUserID: "gh-0", Email: &e0, Username: taken}); err != nil {
		t.Fatal(err)
	}
	svc.userNameGen = func(*string) (string, error) { return taken, nil }
	e1 := "u1@gmail.com"
	_, err := svc.createOAuthUser(t.Context(), "google", &oauth.OAuthUserInfo{ProviderUserID: "g-1", Email: &e1})
	if !errors.Is(err, errOAuthUserNameExhausted) {
		t.Fatalf("muốn errOAuthUserNameExhausted, nhận %v", err)
	}
	if u, _ := users.FindUserByEmail(t.Context(), e1); u != nil {
		t.Error("hết số lần thử thì không được tạo user")
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
}
