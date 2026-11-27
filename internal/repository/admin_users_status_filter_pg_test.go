package repository

// Q11-ADMIN-03: GET /api/users?status=self_deleted chỉ trả tài khoản CHÍNH CHỦ xoá (self_deleted_at != NULL, vẫn
// is_active=false); status=locked loại các tài khoản đó; active không đổi. Chạy SQL THẬT (schema tạm). Mỗi user mang
// token riêng để kết quả độc lập dữ liệu sẵn có; total và phân trang phải khớp từng nhóm. Bỏ điều kiện
// self_deleted_at trong AdminListUsers thì các ca locked/self_deleted ĐỎ.

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestAdminListUsers_StatusFilter_SelfDeletedTachKhoiLocked(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	token := "sdf" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	now := time.Now()
	ids := map[string]uuid.UUID{}
	mk := func(key string, active bool, selfDeleted bool) {
		u := model.User{Email: key + "-" + token + "@40study.test", PasswordHash: "x", UserName: key + token}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", key, err)
		}
		updates := map[string]any{"is_active": active}
		if selfDeleted {
			updates["self_deleted_at"] = now
		}
		if err := db.Model(&model.User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
			t.Fatalf("cập nhật user %s: %v", key, err)
		}
		ids[key] = u.ID
	}
	mk("active1", true, false)
	mk("locked1", false, false)
	mk("locked2", false, false)
	mk("self1", false, true)
	mk("self2", false, true)
	mk("self3", false, true)

	repo := NewUserRepository(db)
	names := func(t *testing.T, status string, page, limit int) ([]string, int64) {
		t.Helper()
		users, total, err := repo.AdminListUsers(context.Background(), AdminUserListFilter{Keyword: token, Status: status, Page: page, Limit: limit})
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		var got []string
		for _, u := range users {
			for key, id := range ids {
				if u.ID == id {
					got = append(got, key)
				}
			}
		}
		sort.Strings(got)
		return got, total
	}

	for _, tc := range []struct {
		status string
		want   string
	}{
		{"self_deleted", "self1,self2,self3"},
		{"locked", "locked1,locked2"},
		{"active", "active1"},
		{"", "active1,locked1,locked2,self1,self2,self3"},
	} {
		got, total := names(t, tc.status, 1, 100)
		if strings.Join(got, ",") != tc.want || int(total) != len(strings.Split(tc.want, ",")) {
			t.Errorf("status %q -> %v (total=%d), muốn %s", tc.status, got, total, tc.want)
		}
	}

	// Phân trang: total vẫn là tổng của nhóm, mỗi trang chỉ chứa phần tử của nhóm.
	p1, total := names(t, "self_deleted", 1, 2)
	p2, total2 := names(t, "self_deleted", 2, 2)
	if total != 3 || total2 != 3 || len(p1) != 2 || len(p2) != 1 {
		t.Errorf("phân trang self_deleted: p1=%v p2=%v total=%d/%d, muốn 2+1 và total=3", p1, p2, total, total2)
	}
	for _, k := range append(p1, p2...) {
		if !strings.HasPrefix(k, "self") {
			t.Errorf("phân trang self_deleted lẫn user %q", k)
		}
	}
}
