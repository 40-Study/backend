package service

// QA vòng 2 (G4 — N-03): cột users.last_login_at có sẵn nhưng không chỗ nào ghi, nên admin luôn
// thấy "Đăng nhập cuối: —". Test chạy Login THẬT (AuthService thật + miniredis), chỉ thay repo
// bằng fake ghi lại lời gọi UpdateUserProfile. Bỏ recordLastLogin khỏi Login -> test ĐỎ.
//
// Kèm test G6 cho orgRoleDisplayName (display_name vai trò tổ chức không còn "ORG_OWNER - " cụt).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type lastLoginFakeUserRepo struct {
	repository.UserRepositoryInterface
	user    *model.User
	updates []map[string]interface{}
}

func (f *lastLoginFakeUserRepo) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	if f.user != nil && f.user.Email == email {
		return f.user, nil
	}
	return nil, nil
}

func (f *lastLoginFakeUserRepo) UpdateUserProfile(ctx context.Context, userID uuid.UUID, updates map[string]interface{}) error {
	if userID != f.user.ID {
		return errors.New("sai user")
	}
	f.updates = append(f.updates, updates)
	return nil
}

// Trả lỗi để Login đi nhánh "chưa có vai trò" (không cần JWT/cấu hình token) — nhánh này vẫn là
// một lần đăng nhập thành công về mặt xác thực danh tính.
type lastLoginFakeUserSystemRoleRepo struct {
	repository.UserSystemRoleRepositoryInterface
}

func (f *lastLoginFakeUserSystemRoleRepo) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return nil, errors.New("không cần vai trò trong test này")
}

func newLastLoginTestService(t *testing.T, password string) (*AuthService, *lastLoginFakeUserRepo) {
	t.Helper()
	hash, err := utils.HashPassword(password)
	if err != nil {
		t.Fatalf("hash mật khẩu: %v", err)
	}
	userRepo := &lastLoginFakeUserRepo{user: &model.User{
		BaseModel:    model.BaseModel{ID: uuid.New()},
		Email:        "last-login@40study.test",
		PasswordHash: hash,
		IsActive:     true,
	}}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	usr := &lastLoginFakeUserSystemRoleRepo{}
	return NewAuthService(nil, userRepo, nil, nil, usr, nil, usr, rdb), userRepo
}

func lastLoginRequest(password string) dto.LoginRequestDto {
	return dto.LoginRequestDto{
		Email:      "last-login@40study.test",
		Password:   password,
		DeviceInfo: dto.DeviceInfoDTO{DeviceID: "550e8400-e29b-41d4-a716-446655440000"},
	}
}

func TestLogin_ThanhCong_GhiLastLoginAt(t *testing.T) {
	svc, userRepo := newLastLoginTestService(t, "Demo@123")
	before := time.Now()

	if _, err := svc.Login(context.Background(), lastLoginRequest("Demo@123")); err != nil {
		t.Fatalf("Login đúng mật khẩu lỗi: %v", err)
	}

	if len(userRepo.updates) != 1 {
		t.Fatalf("UpdateUserProfile được gọi %d lần, muốn 1 (ghi last_login_at)", len(userRepo.updates))
	}
	got, ok := userRepo.updates[0]["last_login_at"].(time.Time)
	if !ok {
		t.Fatalf("update không có last_login_at kiểu time.Time: %#v", userRepo.updates[0])
	}
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("last_login_at = %v, phải nằm trong lúc gọi Login", got)
	}
}

func TestLogin_SaiMatKhau_KhongGhiLastLoginAt(t *testing.T) {
	svc, userRepo := newLastLoginTestService(t, "Demo@123")

	if _, err := svc.Login(context.Background(), lastLoginRequest("sai-mat-khau")); err == nil {
		t.Fatal("Login sai mật khẩu phải lỗi")
	}
	if len(userRepo.updates) != 0 {
		t.Fatalf("sai mật khẩu mà vẫn ghi %v — last_login_at chỉ ghi khi đăng nhập thành công", userRepo.updates)
	}
}

func TestOrgRoleDisplayName_BoTenToChucRong(t *testing.T) {
	cases := []struct{ role, org, want string }{
		{"ORG_OWNER", "ForteX", "ORG_OWNER - ForteX"},
		{"ORG_OWNER", "", "ORG_OWNER"},
		{"ORG_OWNER", "   ", "ORG_OWNER"},
	}
	for _, tc := range cases {
		if got := orgRoleDisplayName(tc.role, tc.org); got != tc.want {
			t.Fatalf("orgRoleDisplayName(%q, %q) = %q, muốn %q", tc.role, tc.org, got, tc.want)
		}
	}
}
