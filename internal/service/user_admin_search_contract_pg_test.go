package service

// ADM-A4 (QA vòng 2 261013): ô gán vai trò ở /admin/roles chọn người theo tên hoặc email qua GET /users?keyword= (đã có,
// không viết endpoint thứ hai). Test này pin đúng phần hợp đồng mà UI phụ thuộc, trên Postgres thật (ILIKE là hành vi
// của Postgres): khớp được TÊN ĐẦY ĐỦ, EMAIL và user_name, không phân biệt hoa thường, vẫn thấy tài khoản đang bị
// khoá (admin cần thấy để biết vì sao không gán được), và phân trang theo limit.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestAdminListUsers_TimTheoTenEmailUserName_KhongPhanBietHoaThuong(t *testing.T) {
	tx := isolatedAPISchema(t)

	token := "srch" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	full := "Tran Thi " + token
	alice := model.User{Email: "alice." + token + "@40study.test", PasswordHash: "x", UserName: "alice_" + token, FullName: &full}
	locked := model.User{Email: "locked." + token + "@40study.test", PasswordHash: "x", UserName: "locked_" + token}
	if err := tx.Create(&alice).Error; err != nil {
		t.Fatalf("tao alice: %v", err)
	}
	if err := tx.Create(&locked).Error; err != nil {
		t.Fatalf("tao locked: %v", err)
	}
	if err := tx.Model(&locked).Update("is_active", false).Error; err != nil {
		t.Fatalf("khoa user: %v", err)
	}

	repo := repository.NewUserRepository(tx)
	find := func(keyword string, limit int) ([]model.User, int64) {
		users, total, err := repo.AdminListUsers(context.Background(), repository.AdminUserListFilter{Keyword: keyword, Page: 1, Limit: limit})
		if err != nil {
			t.Fatalf("keyword %q: %v", keyword, err)
		}
		return users, total
	}
	has := func(users []model.User, id uuid.UUID) bool {
		for _, u := range users {
			if u.ID == id {
				return true
			}
		}
		return false
	}

	for _, kw := range []string{"Tran Thi " + token, strings.ToUpper("tran thi " + token), "alice." + token, "ALICE." + token + "@40STUDY.TEST", "alice_" + token} {
		users, total := find(kw, 10)
		if total != 1 || !has(users, alice.ID) {
			t.Errorf("keyword %q: total=%d, muon dung alice", kw, total)
		}
	}
	users, total := find(token, 10)
	if total != 2 || !has(users, alice.ID) || !has(users, locked.ID) {
		t.Errorf("tai khoan bi khoa van phai hien: total=%d", total)
	}
	if page, total := find(token, 1); len(page) != 1 || total != 2 {
		t.Errorf("limit=1: %d dong, total=%d (muon 1 dong, tong 2)", len(page), total)
	}
	if _, total := find(token+"-khong-co-that", 10); total != 0 {
		t.Errorf("keyword khong khop phai ra 0, ra %d", total)
	}
}
