package repository

// QA6 #132 M1: FindByOrganizationID sắp xếp chỉ theo granted_at DESC, nên các thành viên cùng granted_at (gán hàng loạt,
// import) không có thứ tự xác định giữa các trang: trang sau có thể lặp/sót người. Phải có tiebreak id ASC.
// Chạy SQL THẬT (schema tạm). Dòng được chèn theo id GIẢM dần để thứ tự heap (thứ tự Postgres trả khi không có tiebreak)
// ngược với id ASC, nên test đỏ nếu thiếu tiebreak.

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestFindByOrganizationID_TiedGrantedAt_PagesByIDAsc(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()

	org := model.Organization{Name: "qa6-paging-" + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("tạo tổ chức: %v", err)
	}
	role := model.Role{Name: "ORG_MEMBER", OrganizationID: &org.ID}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("tạo role: %v", err)
	}

	const members = 5
	ids := make([]uuid.UUID, members)
	for i := range ids {
		ids[i] = uuid.New()
	}
	// Sắp id giảm dần rồi chèn theo thứ tự đó: heap order = id DESC.
	for i := 0; i < members; i++ {
		for j := i + 1; j < members; j++ {
			if bytes.Compare(ids[i][:], ids[j][:]) < 0 {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	grantedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for i, id := range ids {
		u := apvUser(t, db, "qa6-pg-"+string(rune('a'+i)))
		uor := model.UserOrganizationRole{
			ID: id, UserID: u.ID, RoleID: role.ID, OrganizationID: org.ID,
			Status: model.UserOrgRoleStatusActive, GrantedAt: grantedAt,
		}
		if err := db.Create(&uor).Error; err != nil {
			t.Fatalf("gán thành viên %d: %v", i, err)
		}
	}

	want := make([]uuid.UUID, members) // id ASC = đảo ngược của ids
	for i, id := range ids {
		want[members-1-i] = id
	}

	repo := NewUserOrganizationRoleRepository(db)
	var got []uuid.UUID
	for page := 1; page <= 3; page++ {
		rows, total, err := repo.FindByOrganizationID(ctx, org.ID, page, 2, "")
		if err != nil || total != members {
			t.Fatalf("trang %d: total=%d err=%v, muốn total=%d", page, total, err, members)
		}
		for _, r := range rows {
			got = append(got, r.ID)
		}
	}
	if len(got) != members {
		t.Fatalf("ghép 3 trang được %d dòng, muốn %d", len(got), members)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("vị trí %d: id=%s, muốn %s (granted_at trùng phải xếp theo id ASC, không lặp/sót giữa các trang)", i, got[i], want[i])
		}
	}
}
