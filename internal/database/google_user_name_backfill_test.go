package database

// Test Postgres THẬT cho migration đổi user_name Google trùng prefix email (issue #105).
//   - TestGoogleUserNameBackfill_DoiTenTrungPrefixEmail_*: qua RunPostMigrations như lúc API khởi động. Bỏ lời gọi
//     khỏi RunPostMigrations, bỏ điều kiện provider google hoặc cửa sổ "tạo bằng Google" thì ĐỎ.
//   - TestApplyGoogleRenames_KiemLaiDieuKienTrongUpdate: ép tình huống người dùng tự đổi tên GIỮA lúc quét và lúc
//     ghi (quét -> đổi tên -> ghi). Bỏ điều kiện kiểm lại trong câu UPDATE thì ĐỎ (test tuần tự qua
//     RunPostMigrations không bắt được việc này).
//   - TestGoogleUserNameBackfill_NhieuLo_*: nhiều lô, tên không trùng nhau và không trùng tên đang dùng.

import (
	"regexp"
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
			if !regexp.MustCompile(`^nguyenvandat[a-z0-9]{6}$`).MatchString(a) {
				t.Errorf("tên đổi phải dựa trên họ tên không dấu, nhận %q", a)
			}
			if !regexp.MustCompile(`^hocvien[a-z0-9]{6}$`).MatchString(b) {
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

// Quét -> người dùng tự đổi tên -> ghi: dòng đã tự đổi KHÔNG bị ghi đè, dòng còn nguyên thì đổi.
func TestApplyGoogleRenames_KiemLaiDieuKienTrongUpdate(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	stay := gUser(t, db, "giu.nguyen", "giu.nguyen", nil)
	linkGoogle(t, db, stay.ID, stay.CreatedAt)
	changedMidway := gUser(t, db, "tu.doi", "tu.doi", nil)
	linkGoogle(t, db, changedMidway.ID, changedMidway.CreatedAt)

	secs := googleCreatedWindow.Seconds()
	rows, err := scanGoogleEmailPrefixUsers(db, secs)
	if err != nil || len(rows) != 2 {
		t.Fatalf("quét: rows=%d err=%v, muốn 2 dòng", len(rows), err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", changedMidway.ID).Update("user_name", "ten-tu-chon").Error; err != nil {
		t.Fatal(err)
	}
	plan, err := planGoogleRenames(db, rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyGoogleRenames(db, plan, secs); err != nil {
		t.Fatal(err)
	}
	if got := userNameOf(t, db, changedMidway.ID); got != "ten-tu-chon" {
		t.Errorf("người vừa tự đổi tên bị ghi đè: %q", got)
	}
	if got := userNameOf(t, db, stay.ID); got == "giu.nguyen" {
		t.Errorf("dòng còn nguyên phải được đổi, vẫn là %q", got)
	}
}

// 1.200 dòng = 3 lô: mọi tên mới khác nhau, không trùng tên đang dùng (kể cả khác hoa thường), không còn prefix email.
func TestGoogleUserNameBackfill_NhieuLo_TenKhacNhauKhongTrung(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	if err := db.Exec(`INSERT INTO users (id, email, password_hash, user_name, full_name, created_at, updated_at)
		SELECT gen_random_uuid(), 'bulk' || g || '@gmail.test', 'x', 'bulk' || g, 'Nguyen Van A', now(), now()
		FROM generate_series(1, 1200) g`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO user_oauth_providers (id, user_id, provider, provider_user_id, created_at)
		SELECT gen_random_uuid(), id, 'google', 'p-' || id, created_at FROM users`).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatal(err)
	}
	var total, distinct, leaked int64
	db.Raw(`SELECT count(*), count(DISTINCT lower(user_name)),
		count(*) FILTER (WHERE lower(user_name) = lower(split_part(email, '@', 1))) FROM users`).Row().Scan(&total, &distinct, &leaked)
	if total != 1200 || distinct != 1200 || leaked != 0 {
		t.Fatalf("total=%d distinct=%d còn prefix email=%d, muốn 1200/1200/0", total, distinct, leaked)
	}
}

func TestNewFreeUserName_TranhTrungVaHetSoLan(t *testing.T) {
	taken := map[string]struct{}{"da.co": {}}
	calls := 0
	gen := func(*string) (string, error) {
		calls++
		if calls < 3 {
			return "DA.CO", nil // trùng không phân biệt hoa thường
		}
		return "moi", nil
	}
	if got, err := newFreeUserName(nil, taken, gen); err != nil || got != "moi" || calls != 3 {
		t.Fatalf("got=%q err=%v calls=%d", got, err, calls)
	}
	always := func(*string) (string, error) { return "da.co", nil }
	if _, err := newFreeUserName(nil, taken, always); err == nil {
		t.Fatal("hết số lần thử phải báo lỗi")
	}
}
