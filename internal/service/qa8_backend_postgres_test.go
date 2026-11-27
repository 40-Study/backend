package service

// QA8 (Postgres THẬT, schema tạm + miniredis): C18 email không phân biệt hoa thường, C19 tên đăng nhập duy nhất/dành riêng,
// C22 chặn xoá tài khoản khi ví giảng viên còn tiền, C23 vai gán theo tổ chức không gán như vai hệ thống, C21 danh sách buổi học
// theo thứ tự/hôm nay, A01 voucher tạo với is_active=false, T02 tên người mua, tên tổ chức không phân biệt hoa thường.
// Mỗi ca đỏ khi bỏ phần sửa tương ứng (xem comment từng test).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

const qa8Password = "Demo@12345"

type qa8Env struct {
	db   *gorm.DB
	svc  *AuthService
	mr   *miniredis.Miniredis
	rdb  *redis.Client
	otps chan string
}

// newQA8Env: AuthService THẬT với repo user Postgres; vai trò dùng bản giả "chưa có vai" (như auth_last_login_test) để Login dừng
// ở nhánh thành công về danh tính mà không cần cấu hình JWT. Bộ gửi mail OTP đăng ký được thay để bắt mã.
func newQA8Env(t *testing.T) *qa8Env {
	t.Helper()
	db := isolatedAPISchema(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	otps := make(chan string, 8)
	orig := sendRegisterOTPMail
	sendRegisterOTPMail = func(_ *config.Config, _ string, otp string) error { otps <- otp; return nil }
	t.Cleanup(func() { sendRegisterOTPMail = orig })
	usr := &lastLoginFakeUserSystemRoleRepo{}
	svc := NewAuthService(nil, repository.NewUserRepository(db), nil, nil, usr, nil, usr, rdb)
	return &qa8Env{db: db, svc: svc, mr: mr, rdb: rdb, otps: otps}
}

func (e *qa8Env) nextOTP(t *testing.T) string {
	t.Helper()
	select {
	case otp := <-e.otps:
		return otp
	case <-time.After(2 * time.Second):
		t.Fatal("không có email OTP nào được gửi")
		return ""
	}
}

// legacyUser chèn thẳng một dòng (bỏ qua repo) để dựng dữ liệu cũ có email/tên viết hoa.
func (e *qa8Env) legacyUser(t *testing.T, email, userName string, active bool) model.User {
	t.Helper()
	hash, err := utils.HashPassword(qa8Password)
	if err != nil {
		t.Fatal(err)
	}
	u := model.User{Email: email, PasswordHash: hash, UserName: userName, IsVerified: true, IsActive: true}
	if err := e.db.Create(&u).Error; err != nil {
		t.Fatalf("tạo user %q: %v", email, err)
	}
	if !active {
		if err := e.db.Model(&model.User{}).Where("id = ?", u.ID).Update("is_active", false).Error; err != nil {
			t.Fatal(err)
		}
		u.IsActive = false
	}
	return u
}

func qa8Uniq() string { return strings.ToLower(uuid.NewString()[:8]) }

func qa8RegisterReq(email, userName string) dto.RegisterRequestDto {
	return dto.RegisterRequestDto{Email: email, Password: qa8Password, ConfirmPassword: qa8Password, UserName: userName}
}

// ── C18: email ──────────────────────────────────────────────────────────────────────────

// Đỏ khi FindUserByEmail quay về `email = ?` (đăng ký lại bằng email khác hoa thường lọt qua; đăng nhập chữ khác bị 401).
func TestQA8_C18_Login_AnyCaseEmail_Succeeds(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	local := "parent-" + qa8Uniq()
	e.legacyUser(t, strings.ToUpper(local[:1])+local[1:]+"@Demo.com", "qa8p-"+qa8Uniq(), true) // dòng cũ lưu hoa/thường lẫn lộn
	for _, typed := range []string{local + "@demo.com", strings.ToUpper(local) + "@DEMO.COM", "  " + local + "@Demo.Com "} {
		res, err := e.svc.Login(ctx, dto.LoginRequestDto{Email: typed, Password: qa8Password, DeviceInfo: lastLoginRequest(qa8Password).DeviceInfo})
		if err != nil || res == nil {
			t.Errorf("đăng nhập bằng %q: res=%v err=%v, muốn thành công", typed, res, err)
		}
	}
}

// Đỏ khi RequestRegister không chuẩn hoá/dò không phân biệt hoa thường: email đã có (khác hoa thường) vẫn được gửi OTP.
func TestQA8_C18_RequestRegister_ExistingEmailInOtherCase_IsRejected(t *testing.T) {
	e := newQA8Env(t)
	local := "parent-" + qa8Uniq()
	e.legacyUser(t, local+"@demo.com", "qa8p-"+qa8Uniq(), true)
	err := e.svc.RequestRegister(context.Background(), qa8RegisterReq(strings.ToUpper(local)+"@DEMO.COM", "qa8new-"+qa8Uniq()))
	if err == nil || !strings.Contains(err.Error(), "email already registered") {
		t.Fatalf("err=%v, muốn \"email already registered\"", err)
	}
	if len(e.mr.Keys()) != 0 {
		t.Fatalf("không được lưu đăng ký chờ OTP cho email đã có: %v", e.mr.Keys())
	}
}

// Đỏ khi khoá Redis/muối OTP/email lưu giữ nguyên chữ người gõ: xác thực bằng chữ khác không tìm thấy phiên, tài khoản lưu chữ hoa.
func TestQA8_C18_RegisterFlow_StoresLowercase_AndOtherCaseVerifies(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	local := "new-" + qa8Uniq()
	typed := strings.ToUpper(local[:1]) + local[1:] + "@Demo.COM"
	if err := e.svc.RequestRegister(ctx, qa8RegisterReq(typed, "qa8n-"+qa8Uniq())); err != nil {
		t.Fatal(err)
	}
	otp := e.nextOTP(t)
	if !e.mr.Exists(constants.KeyRegisterOTP(local + "@demo.com")) {
		t.Fatalf("khoá đăng ký chờ phải theo email chữ thường; khoá hiện có: %v", e.mr.Keys())
	}
	// Gửi lại + xác thực bằng một kiểu hoa/thường khác hẳn.
	if err := e.svc.ResendRegisterOTP(ctx, strings.ToUpper(local)+"@DEMO.COM"); err != nil {
		t.Fatalf("gửi lại OTP bằng chữ khác: %v", err)
	}
	otp = e.nextOTP(t)
	res, err := e.svc.Register(ctx, dto.VerifyOtpRequestDto{Email: local + "@DEMO.com", OTP: otp})
	if err != nil {
		t.Fatalf("xác thực OTP bằng chữ khác: %v", err)
	}
	if res.Email != local+"@demo.com" {
		t.Fatalf("email lưu = %q, muốn %q", res.Email, local+"@demo.com")
	}
	// Đăng ký lần nữa bằng kiểu hoa/thường thứ ba: đã tồn tại.
	if err := e.svc.RequestRegister(ctx, qa8RegisterReq(strings.ToUpper(local)+"@demo.com", "qa8m-"+qa8Uniq())); err == nil || !strings.Contains(err.Error(), "email already registered") {
		t.Fatalf("đăng ký lại: err=%v, muốn email already registered", err)
	}
}

// Đỏ khi FindUserByEmail quay về `email = ?`: người dùng gõ email khác hoa thường với dòng lưu thì không đặt lại được mật khẩu
// (hoặc đặt lại nhầm chủ). Cấp mã bằng chữ này, xác thực bằng chữ KHÁC.
func TestQA8_C18_PasswordReset_OtherCaseEmail_Works(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	local := "reset-" + qa8Uniq()
	u := e.legacyUser(t, strings.ToUpper(local[:1])+local[1:]+"@Demo.com", "qa8r-"+qa8Uniq(), true)
	if err := e.svc.RequestPasswordReset(ctx, strings.ToUpper(local)+"@DEMO.COM"); err != nil {
		t.Fatal(err)
	}
	key := constants.KeyPasswordReset(u.ID.String())
	raw, err := e.rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("RequestPasswordReset bằng chữ khác không tạo mã đặt lại (khoá %s): %v", key, err)
	}
	var rec PasswordResetOTP
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Email != local+"@demo.com" {
		t.Fatalf("bản ghi đặt lại mang email %q, muốn chữ thường %q", rec.Email, local+"@demo.com")
	}
	// Mã gốc chỉ có trong email gửi đi: thay bằng mã đã biết, cùng muối, rồi xác thực bằng chữ khác nữa.
	otp := "123456"
	rec.OTPHash = utils.HashOTP(otp, rec.Email)
	b, _ := json.Marshal(rec)
	if err := e.rdb.Set(ctx, key, b, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	newPw := "NewPass@12345"
	err = e.svc.ResetPassword(ctx, dto.ResetPasswordRequestDto{Email: strings.ToUpper(local) + "@Demo.COM", Otp: otp, NewPassword: newPw, ConfirmPassword: newPw})
	if err != nil {
		t.Fatalf("ResetPassword bằng chữ khác: %v", err)
	}
}

// ── C19: tên đăng nhập ───────────────────────────────────────────────────────────────────

// Đỏ khi bỏ ensureUserNameAvailable khỏi RequestRegister: tên trùng (khác hoa thường) hoặc dành riêng vẫn được gửi OTP.
func TestQA8_C19_RequestRegister_TakenOrReservedUserName_Conflict(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	taken := "Parent-" + qa8Uniq()
	e.legacyUser(t, "owner-"+qa8Uniq()+"@demo.com", taken, true)
	for _, tc := range []struct {
		name string
		want error
	}{
		{strings.ToUpper(taken), ErrUserNameTaken},
		{strings.ToLower(taken), ErrUserNameTaken},
		{"Admin", ErrUserNameReserved}, {"ROOT", ErrUserNameReserved}, {"support", ErrUserNameReserved},
		{"FortEx", ErrUserNameReserved}, {"40study", ErrUserNameReserved},
	} {
		err := e.svc.RequestRegister(ctx, qa8RegisterReq("fresh-"+qa8Uniq()+"@demo.com", tc.name))
		if !errors.Is(err, tc.want) {
			t.Errorf("tên %q: err=%v, muốn %v", tc.name, err, tc.want)
		}
	}
	if len(e.mr.Keys()) != 0 {
		t.Fatalf("không được lưu đăng ký chờ OTP khi tên bị từ chối: %v", e.mr.Keys())
	}
	if err := e.svc.RequestRegister(ctx, qa8RegisterReq("fresh-"+qa8Uniq()+"@demo.com", "free-"+qa8Uniq())); err != nil {
		t.Fatalf("tên còn trống phải qua: %v", err)
	}
}

// Đỏ khi Register (bước OTP) không kiểm lại: hai người xin OTP cùng tên, người xác thực sau vẫn tạo được tài khoản trùng.
func TestQA8_C19_Register_NameTakenWhileWaitingForOtp_Conflict(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	name := "race-" + qa8Uniq()
	email := "racer-" + qa8Uniq() + "@demo.com"
	if err := e.svc.RequestRegister(ctx, qa8RegisterReq(email, name)); err != nil {
		t.Fatal(err)
	}
	otp := e.nextOTP(t)
	e.legacyUser(t, "winner-"+qa8Uniq()+"@demo.com", strings.ToUpper(name), true) // người khác lấy tên trong lúc chờ
	if _, err := e.svc.Register(ctx, dto.VerifyOtpRequestDto{Email: email, OTP: otp}); !errors.Is(err, ErrUserNameTaken) {
		t.Fatalf("err=%v, muốn ErrUserNameTaken", err)
	}
	var n int64
	e.db.Model(&model.User{}).Where("email = ?", email).Count(&n)
	if n != 0 {
		t.Fatalf("đã tạo %d tài khoản dù tên bị lấy", n)
	}
}

// Tái kích hoạt tài khoản đã xoá được giữ đúng tên cũ của chính nó (không tự xung đột với dòng của mình).
func TestQA8_C19_RequestRegister_Reactivation_KeepsOwnName(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	name := "Comeback-" + qa8Uniq()
	email := "comeback-" + qa8Uniq() + "@demo.com"
	e.legacyUser(t, email, name, false)
	if err := e.svc.RequestRegister(ctx, qa8RegisterReq(strings.ToUpper(email), strings.ToLower(name))); err != nil {
		t.Fatalf("tái kích hoạt giữ tên cũ: %v", err)
	}
}

// Đỏ khi bỏ kiểm ở UpdateMe: PUT /auth/me đổi sang tên người khác/dành riêng vẫn 200; đổi hoa/thường tên của chính mình bị chặn nhầm.
func TestQA8_C19_UpdateMe_UserName(t *testing.T) {
	e := newQA8Env(t)
	ctx := context.Background()
	other := "Other-" + qa8Uniq()
	e.legacyUser(t, "other-"+qa8Uniq()+"@demo.com", other, true)
	me := e.legacyUser(t, "me-"+qa8Uniq()+"@demo.com", "Mine-"+qa8Uniq(), true)
	set := func(name string) error {
		_, err := e.svc.UpdateMe(ctx, me.ID, dto.UpdateMeRequestDto{Username: &name})
		return err
	}
	if err := set(strings.ToUpper(other)); !errors.Is(err, ErrUserNameTaken) {
		t.Errorf("tên người khác: err=%v, muốn ErrUserNameTaken", err)
	}
	if err := set("Admin"); !errors.Is(err, ErrUserNameReserved) {
		t.Errorf("tên dành riêng: err=%v, muốn ErrUserNameReserved", err)
	}
	if err := set(strings.ToUpper(me.UserName)); err != nil {
		t.Errorf("đổi hoa/thường tên của mình: %v", err)
	}
	fresh := "fresh-" + qa8Uniq()
	if err := set(fresh); err != nil {
		t.Fatalf("tên còn trống: %v", err)
	}
	var got model.User
	e.db.First(&got, "id = ?", me.ID)
	if got.UserName != fresh {
		t.Fatalf("user_name = %q, muốn %q", got.UserName, fresh)
	}
	// Tài khoản seed đang mang tên dành riêng giữ được tên khi lưu hồ sơ.
	adminUser := e.legacyUser(t, "adm-"+qa8Uniq()+"@demo.com", "admin", true)
	same := "admin"
	if _, err := e.svc.UpdateMe(ctx, adminUser.ID, dto.UpdateMeRequestDto{Username: &same}); err != nil {
		t.Errorf("tài khoản đang mang tên dành riêng phải giữ được: %v", err)
	}
}
