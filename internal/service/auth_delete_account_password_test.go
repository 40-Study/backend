package service

// #142 M1 + Q9-PG-05: DeleteAccount (DELETE /auth/me) trả sai mật khẩu bằng lỗi nghiệp vụ có mã ERR_INVALID_PASSWORD (400, câu
// "invalid password" giữ nguyên cho web cũ) và ghi users.self_deleted_at khi xoá thành công.
// Bỏ ErrInvalidDeletePassword -> test đầu ĐỎ; bỏ dòng ghi SelfDeletedAt -> test sau ĐỎ.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type deleteAccountFakeUserRepo struct {
	lastLoginFakeUserRepo
	saved *model.User
}

func (f *deleteAccountFakeUserRepo) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return f.user, nil
}

func (f *deleteAccountFakeUserRepo) UpdateUser(ctx context.Context, user *model.User) error {
	f.saved = user
	return nil
}

func newDeleteAccountService(t *testing.T, password string) (*AuthService, *deleteAccountFakeUserRepo) {
	t.Helper()
	base, _ := newLastLoginTestService(t, password)
	hash, err := utils.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	repo := &deleteAccountFakeUserRepo{lastLoginFakeUserRepo: lastLoginFakeUserRepo{user: &model.User{
		BaseModel: model.BaseModel{ID: uuid.New()}, Email: "delete@40study.test", PasswordHash: hash, IsActive: true,
	}}}
	svc := NewAuthService(nil, repo, nil, nil, nil, nil, nil, base.redisClient)
	return svc, repo
}

func TestDeleteAccount_SaiMatKhau_LoiNghiepVu400ERR_INVALID_PASSWORD(t *testing.T) {
	svc, repo := newDeleteAccountService(t, "Demo@123")
	err := svc.DeleteAccount(context.Background(), repo.user.ID, dto.DeleteAccountDto{Password: "sai-mat-khau"})
	var known *apperr.KnownError
	if !errors.As(err, &known) {
		t.Fatalf("err=%v, muốn *apperr.KnownError", err)
	}
	if known.Status != http.StatusBadRequest || known.Code != "ERR_INVALID_PASSWORD" || known.Message != "invalid password" {
		t.Fatalf("got status=%d code=%q message=%q, muốn 400 ERR_INVALID_PASSWORD \"invalid password\"", known.Status, known.Code, known.Message)
	}
	if repo.saved != nil {
		t.Fatal("sai mật khẩu không được ghi gì vào tài khoản")
	}
}
