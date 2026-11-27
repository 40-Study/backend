package service

// Q10-ADMIN-03: admin phải phân biệt được "người dùng tự xoá tài khoản" với "admin khoá" (trước đây cả hai đều là
// is_active=false và UI chỉ đọc is_active nên admin có thể vô tình khôi phục tài khoản tự xoá như thể đó là một lần
// khoá). DTO admin (danh sách + chi tiết) lộ self_deleted_at (nullable): có giá trị chỉ khi chính chủ đã xoá; nil với
// tài khoản hoạt động và tài khoản bị admin khoá. Mở khoá qua cùng endpoint xoá dấu này. Postgres thật.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestQ10_AdminUserDTO_PG_PhanBietTuXoaVoiAdminKhoa(t *testing.T) {
	tx := isolatedAPISchema(t)
	ctx := context.Background()

	token := "q10sd" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	mk := func(kind string) model.User {
		u := model.User{Email: kind + "." + token + "@40study.test", PasswordHash: "x", UserName: kind + "_" + token}
		if err := tx.Create(&u).Error; err != nil {
			t.Fatalf("tạo %s: %v", kind, err)
		}
		return u
	}
	active, selfDeleted, adminLocked, actor := mk("active"), mk("selfdel"), mk("locked"), mk("actor")

	deletedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	reason, lockedAt := "vi phạm", time.Now().Add(-2*time.Hour).UTC().Truncate(time.Second)
	if err := tx.Model(&selfDeleted).Updates(map[string]any{"is_active": false, "self_deleted_at": deletedAt}).Error; err != nil {
		t.Fatalf("đánh dấu tự xoá: %v", err)
	}
	if err := tx.Model(&adminLocked).Updates(map[string]any{"is_active": false, "locked_reason": reason, "locked_at": lockedAt, "locked_by": actor.ID}).Error; err != nil {
		t.Fatalf("khoá bởi admin: %v", err)
	}

	svc := NewUserAdminService(repository.NewUserRepository(tx), repository.NewSystemRoleRepository(tx), repository.NewUserSystemRoleRepository(tx), nil, nil)

	list, err := svc.ListUsers(ctx, repository.AdminUserListFilter{Keyword: token, Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	byID := map[uuid.UUID]dto.AdminUserListItemDTO{}
	for _, it := range list.Items {
		byID[it.ID] = it
	}
	if got := byID[selfDeleted.ID].SelfDeletedAt; got == nil || !got.Equal(deletedAt) {
		t.Errorf("danh sách: tài khoản tự xoá self_deleted_at = %v, muốn %v", got, deletedAt)
	}
	for name, id := range map[string]uuid.UUID{"đang hoạt động": active.ID, "admin khoá": adminLocked.ID} {
		if got := byID[id].SelfDeletedAt; got != nil {
			t.Errorf("danh sách: tài khoản %s phải có self_deleted_at = nil, ra %v", name, got)
		}
	}

	detail, err := svc.GetUserDetail(ctx, selfDeleted.ID)
	if err != nil || detail.SelfDeletedAt == nil || !detail.SelfDeletedAt.Equal(deletedAt) {
		t.Fatalf("chi tiết tự xoá: %+v err=%v, muốn self_deleted_at = %v", detail, err, deletedAt)
	}
	if lockedDetail, err := svc.GetUserDetail(ctx, adminLocked.ID); err != nil || lockedDetail.SelfDeletedAt != nil || lockedDetail.LockedReason == nil {
		t.Errorf("chi tiết admin khoá: %+v err=%v, muốn self_deleted_at nil và có lý do khoá", lockedDetail, err)
	}

	// JSON: khoá phải luôn có mặt (null khi không tự xoá) để web phân nhánh theo `=== null` thay vì "thiếu khoá".
	raw, _ := json.Marshal(byID[active.ID])
	if !strings.Contains(string(raw), `"self_deleted_at":null`) {
		t.Errorf("JSON danh sách thiếu self_deleted_at:null: %s", raw)
	}

	// #159 L1: admin khoá một tài khoản ĐÃ tự xoá -> hành động mới nhất thắng: có lý do khoá, không còn dấu tự xoá
	// (nếu còn, web hiện "Đã tự xoá" và đăng nhập báo ERR_ACCOUNT_DELETED dù admin đã khoá sau đó).
	lockedAfterDelete := mk("lockafterdel")
	if err := tx.Model(&lockedAfterDelete).Updates(map[string]any{"is_active": false, "self_deleted_at": deletedAt}).Error; err != nil {
		t.Fatalf("đánh dấu tự xoá (khoá sau): %v", err)
	}
	why := "vi phạm điều khoản"
	locked, err := svc.UpdateUserStatus(ctx, lockedAfterDelete.ID, actor.ID, false, &why)
	if err != nil {
		t.Fatalf("admin khoá tài khoản đã tự xoá: %v", err)
	}
	if locked.IsActive || locked.SelfDeletedAt != nil || locked.LockedReason == nil || *locked.LockedReason != why {
		t.Errorf("sau admin khoá: %+v, muốn is_active=false, self_deleted_at=nil, locked_reason=%q", locked, why)
	}
	var reread model.User
	if err := tx.First(&reread, "id = ?", lockedAfterDelete.ID).Error; err != nil || reread.SelfDeletedAt != nil {
		t.Errorf("đọc lại từ DB: self_deleted_at=%v err=%v, muốn nil", reread.SelfDeletedAt, err)
	}

	// Khôi phục qua đúng endpoint mở khoá: xoá dấu tự xoá.
	restored, err := svc.UpdateUserStatus(ctx, selfDeleted.ID, actor.ID, true, nil)
	if err != nil {
		t.Fatalf("mở khoá tài khoản tự xoá: %v", err)
	}
	if !restored.IsActive || restored.SelfDeletedAt != nil {
		t.Errorf("sau mở khoá: is_active=%v self_deleted_at=%v, muốn true/nil", restored.IsActive, restored.SelfDeletedAt)
	}
}
