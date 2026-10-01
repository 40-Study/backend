package database

// Test Postgres THẬT cho migration đổi user_name Google trùng prefix email (issue #105), gọi qua
// RunPostMigrations như lúc API khởi động. Bỏ lời gọi khỏi RunPostMigrations, bỏ điều kiện provider google,
// bỏ cửa sổ "tạo bằng Google" hoặc ghi đè không kiểm lại điều kiện thì test ĐỎ.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func gUser(t *testing.T, db *gorm.DB, userName, emailLocal string, fullName *string) model.User {
	t.Helper()
	u := model.User{Email: emailLocal + "@gmail.test", PasswordHash: "x", UserName: userName, FullName: fullName}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("tạo user: %v", err)
	}
	return u
}

func linkGoogle(t *testing.T, db *gorm.DB, userID uuid.UUID, createdAt time.Time) {
	t.Helper()
	p := model.UserOAuthProvider{UserID: userID, Provider: "google", ProviderUserID: uuid.NewString(), CreatedAt: createdAt}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("liên kết google: %v", err)
	}
}

func userNameOf(t *testing.T, db *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var u model.User
	if err := db.First(&u, "id = ?", id).Error; err != nil {
		t.Fatalf("đọc user: %v", err)
	}
	return u.UserName
}

func TestGoogleUserNameBackfill_DoiTenTrungPrefixEmail_ChayLaiKhongDoiGi(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	name := "Nguyễn Văn Đạt"
	now := time.Now()

	// Đổi: tạo bằng Google (link ngay lúc tạo), user_name == prefix (kể cả khác hoa thường).
	leak := gUser(t, db, "nguyen.van.a", "nguyen.van.a", &name)
	linkGoogle(t, db, leak.ID, leak.CreatedAt)
	leakCase := gUser(t, db, "Tran.B", "tran.b", nil)
	linkGoogle(t, db, leakCase.ID, leakCase.CreatedAt)

	// Giữ: Google nhưng user_name khác prefix (người đã tự đổi).
	changed := gUser(t, db, "ten-tu-chon", "doi.ten", &name)
	linkGoogle(t, db, changed.ID, changed.CreatedAt)
	// Giữ: trùng prefix nhưng KHÔNG có liên kết google (đăng ký mật khẩu, tự đặt tên).
	noGoogle := gUser(t, db, "mat-khau", "mat-khau", &name)
	// Giữ: liên kết Google ra đời LÂU SAU khi tạo tài khoản (đăng ký mật khẩu rồi mới liên kết): tên do họ tự đặt.
	linkedLater := gUser(t, db, "lien.ket.sau", "lien.ket.sau", &name)
	linkGoogle(t, db, linkedLater.ID, linkedLater.CreatedAt.Add(48*time.Hour))
	// Giữ: liên kết facebook không tính.
	fb := gUser(t, db, "fb.user", "fb.user", &name)
	if err := db.Create(&model.UserOAuthProvider{UserID: fb.ID, Provider: "facebook", ProviderUserID: uuid.NewString(), CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	keep := map[string]model.User{"đã tự đổi": changed, "không có google": noGoogle, "link google sau": linkedLater, "chỉ facebook": fb}
	before := map[string]string{}
	for k, u := range keep {
		before[k] = u.UserName
	}

	var firstA, firstB string
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
		a, b := userNameOf(t, db, leak.ID), userNameOf(t, db, leakCase.ID)
		if run == 1 {
			firstA, firstB = a, b
			if !strings.HasPrefix(a, "nguyenvandat_") {
				t.Errorf("tên đổi phải dựa trên họ tên không dấu, nhận %q", a)
			}
			if !strings.HasPrefix(b, "hocvien_") {
				t.Errorf("không có họ tên: phải dùng 'hocvien_...', nhận %q", b)
			}
			if strings.Contains(strings.ToLower(a+b), "nguyen.van.a") || strings.Contains(strings.ToLower(b), "tran") {
				t.Errorf("tên mới không được chứa prefix email: %q %q", a, b)
			}
		} else if a != firstA || b != firstB {
			t.Errorf("chạy lại không được đổi tên lần nữa: %q->%q, %q->%q", firstA, a, firstB, b)
		}
		for k, u := range keep {
			if got := userNameOf(t, db, u.ID); got != before[k] {
				t.Errorf("lần %d, %s: phải giữ %q, nhận %q", run, k, before[k], got)
			}
		}
	}
	if firstA == firstB {
		t.Errorf("hai user phải có tên khác nhau: %q", firstA)
	}
}
