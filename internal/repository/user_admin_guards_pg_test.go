package repository

// Review đối kháng (plans/reports/review-260928-users-pr72-pr28.md, finding #1 BLOCKER): tái
// hiện ĐÚNG kịch bản tuần tự (KHÔNG cần 2 goroutine/race) làm hệ thống còn 0 SYSTEM_ADMIN đăng
// nhập được. Kết nối Postgres THẬT (không mock) vì lỗi nằm ở CALL SITE (dữ liệu feed vào hàm
// thuần evaluateLastSystemAdminGuard), không nằm ở chính hàm thuần đó — hàm thuần đã có test
// riêng ở user_admin_guards_test.go và luôn đúng với input tay soạn đúng. Toàn bộ chạy trong 1
// transaction rồi ROLLBACK — không ghi gì xuống DB thật, không đụng tài khoản demo.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// TestRevokeActiveAssignment_ChanKhiAdminConLaiDaBiKhoaTruoc: tái hiện finding #1 BLOCKER — 1
// user (adminActive) là SYSTEM_ADMIN duy nhất còn is_active=true trong hệ thống (admin còn lại,
// adminLocked, giữ role SYSTEM_ADMIN active nhưng TÀI KHOẢN đã bị khoá từ trước — thao tác khoá
// đó hợp lệ tại thời điểm nó xảy ra). adminActive tự gỡ vai trò SYSTEM_ADMIN của chính mình
// (route DELETE .../system-roles/:id có sẵn từ trước, web #28 gọi thẳng từ UI) — PHẢI bị chặn
// (ErrLastSystemAdmin), vì sau thao tác sẽ không còn AI đăng nhập được với vai trò SYSTEM_ADMIN.
//
// Trước bản vá (activeStatus[a.UserID] = true gán CỨNG cho mọi holder): test này FAIL (err=nil,
// guard cho qua sai). Sau bản vá (fetchIsActiveByUserIDs đọc is_active THẬT): test PASS.
func TestRevokeActiveAssignment_ChanKhiAdminConLaiDaBiKhoaTruoc(t *testing.T) {
	db := openTestPostgres(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	defer sqlDB.Close()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("db.Begin(): %v", tx.Error)
	}
	defer tx.Rollback()

	suffix := uuid.NewString()
	ctx := context.Background()

	// Role SYSTEM_ADMIN test riêng (tên duy nhất) — KHÔNG dùng role SYSTEM_ADMIN thật của DB dev,
	// để đếm "còn bao nhiêu admin hoạt động" chỉ tính đúng 2 user seed trong test này, không lẫn
	// admin@demo.com / dữ liệu thật khác đang có sẵn trong bảng.
	testAdminRole := model.SystemRole{Name: "SYSTEM_ADMIN_REVIEW_TEST_" + suffix}
	if err := tx.Create(&testAdminRole).Error; err != nil {
		t.Fatalf("tao testAdminRole: %v", err)
	}
	otherRole := model.SystemRole{Name: "OTHER_ROLE_REVIEW_TEST_" + suffix}
	if err := tx.Create(&otherRole).Error; err != nil {
		t.Fatalf("tao otherRole: %v", err)
	}

	adminActive := model.User{
		Email:        "review-active-" + suffix + "@40study.test",
		PasswordHash: "x",
		UserName:     "review-active-" + suffix,
	}
	if err := tx.Create(&adminActive).Error; err != nil {
		t.Fatalf("tao adminActive: %v", err)
	}

	adminLocked := model.User{
		Email:        "review-locked-" + suffix + "@40study.test",
		PasswordHash: "x",
		UserName:     "review-locked-" + suffix,
	}
	if err := tx.Create(&adminLocked).Error; err != nil {
		t.Fatalf("tao adminLocked: %v", err)
	}
	// QUAN TRỌNG: IsActive có gorm:"default:true" — set false trực tiếp trong struct rồi Create()
	// sẽ bị GORM COI LÀ zero-value và ÂM THẦM bỏ qua, để Postgres tự áp default (true), không
	// phải false như ý muốn. Phải Update() riêng một cột sau khi đã tạo xong.
	if err := tx.Model(&adminLocked).Update("is_active", false).Error; err != nil {
		t.Fatalf("khoa adminLocked: %v", err)
	}

	// adminActive giữ 2 vai trò active (testAdminRole + otherRole) để KHÔNG dính guard "vai trò
	// cuối cùng của chính user này" — phép thử này chỉ nhắm đúng guard "còn >=1 SYSTEM_ADMIN
	// hoạt động của HỆ THỐNG".
	assignments := []model.UserSystemRole{
		{UserID: adminActive.ID, SystemRoleID: testAdminRole.ID, Status: model.UserSystemRoleStatusActive},
		{UserID: adminActive.ID, SystemRoleID: otherRole.ID, Status: model.UserSystemRoleStatusActive},
		{UserID: adminLocked.ID, SystemRoleID: testAdminRole.ID, Status: model.UserSystemRoleStatusActive},
	}
	for i := range assignments {
		if err := tx.Create(&assignments[i]).Error; err != nil {
			t.Fatalf("tao assignment %d: %v", i, err)
		}
	}

	t.Logf("Trang thai truoc: adminActive.is_active=true (SYSTEM_ADMIN active), adminLocked.is_active=false (SYSTEM_ADMIN active nhung tai khoan da khoa)")

	repo := NewUserSystemRoleRepository(tx)
	revokedBy := uuid.New()
	_, err = repo.RevokeActiveAssignment(ctx, adminActive.ID, testAdminRole.ID, revokedBy, true)

	if err != ErrLastSystemAdmin {
		t.Fatalf(
			"RevokeActiveAssignment(adminActive tu go SYSTEM_ADMIN, trong khi adminLocked la admin "+
				"con lai duy nhat nhung DA BI KHOA) = err %v, muon ErrLastSystemAdmin — neu err=nil "+
				"nghia la guard cho qua sai, he thong con 0 SYSTEM_ADMIN dang nhap duoc sau thao tac nay",
			err,
		)
	}

	// Xac nhan assignment KHONG bi mutate khi guard chan (van con active, chua bi Save thanh
	// inactive) — guard phai chan TRUOC khi ghi, khong phai ghi roi moi bao loi.
	var stillActive model.UserSystemRole
	if err := tx.Where("user_id = ? AND system_role_id = ?", adminActive.ID, testAdminRole.ID).
		First(&stillActive).Error; err != nil {
		t.Fatalf("doc lai assignment sau khi bi chan: %v", err)
	}
	if stillActive.Status != model.UserSystemRoleStatusActive {
		t.Fatalf("assignment bi doi thanh %q dù guard đã chặn — muốn vẫn là %q", stillActive.Status, model.UserSystemRoleStatusActive)
	}
}
