package service

// QA8 (Postgres THẬT, schema tạm), phần còn lại: C22 xoá tài khoản khi ví giảng viên còn tiền, C23 vai theo tổ chức không gán
// được như vai hệ thống, C21 danh sách buổi học, A01 voucher tạo với is_active=false, T02 tên người mua, tên tổ chức không phân
// biệt hoa thường. Mỗi ca nêu thay đổi làm nó ĐỎ.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// ── C22: xoá tài khoản ───────────────────────────────────────────────────────────────────

// Đỏ khi bỏ ensureNoTeacherMoney (tài khoản còn tiền vẫn bị khoá) hoặc chặn cả giảng viên đã rút hết / người không bán khoá nào.
func TestQA8_C22_DeleteAccount_BlockedWhileWalletHasMoney(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	svc := NewAuthService(nil, repository.NewUserRepository(f.db), nil, nil, nil, nil, nil, nil)
	svc.SetTeacherMoneyReader(repository.NewWalletRepository(f.db))
	hash, err := utils.HashPassword(qa8Password)
	if err != nil {
		t.Fatal(err)
	}
	withPassword := func(id uuid.UUID) uuid.UUID {
		if err := f.db.Model(&model.User{}).Where("id = ?", id).Update("password_hash", hash).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	active := func(id uuid.UUID) bool {
		var u model.User
		f.db.First(&u, "id = ?", id)
		return u.IsActive
	}
	del := func(id uuid.UUID, pw string) error {
		return svc.DeleteAccount(ctx, id, dto.DeleteAccountDto{Password: pw})
	}

	withBalance := withPassword(f.teacher(true))
	f.order("completed", 0, orderLine{f.course(withBalance), 1_000_000})
	if err := del(withBalance, qa8Password); !errors.Is(err, ErrAccountHasBalance) {
		t.Fatalf("còn số dư: err=%v, muốn ErrAccountHasBalance", err)
	}
	if !active(withBalance) {
		t.Fatal("tài khoản còn số dư bị khoá dù xoá bị chặn")
	}
	if err := del(withBalance, "Sai@12345"); err == nil || errors.Is(err, ErrAccountHasBalance) {
		t.Fatalf("sai mật khẩu phải báo sai mật khẩu, không lộ tình trạng ví: %v", err)
	}

	pending := withPassword(f.teacher(true))
	f.order("completed", 0, orderLine{f.course(pending), 500_000})
	f.payout(pending, model.PayoutStatusPending, 500_000) // số dư khả dụng 0 nhưng còn yêu cầu rút đang xử lý
	if err := del(pending, qa8Password); !errors.Is(err, ErrAccountHasBalance) {
		t.Fatalf("còn yêu cầu rút pending: err=%v, muốn ErrAccountHasBalance", err)
	}

	paidOut := withPassword(f.teacher(true))
	f.order("completed", 0, orderLine{f.course(paidOut), 500_000})
	f.payout(paidOut, model.PayoutStatusCompleted, 500_000)
	if err := del(paidOut, qa8Password); err != nil {
		t.Fatalf("đã rút hết: %v", err)
	}
	if active(paidOut) {
		t.Fatal("tài khoản đã rút hết phải bị vô hiệu hoá")
	}

	student := withPassword(f.user("student"))
	if err := del(student, qa8Password); err != nil {
		t.Fatalf("người dùng không có thu nhập: %v", err)
	}
}

// ── C23: vai theo tổ chức ────────────────────────────────────────────────────────────────

// Đỏ khi bỏ kiểm IsOrgScopedSystemRole: ORG_OWNER vẫn gán được như vai hệ thống (không còn ERR_ROLE_NOT_ASSIGNABLE).
func TestQA8_C23_AssignSystemRoles_OrgScopedRoleRejected(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()
	svc := NewUserSystemRoleService(repository.NewUserSystemRoleRepository(db), repository.NewUserRepository(db), repository.NewSystemRoleRepository(db))
	mkRole := func(name string) model.SystemRole {
		r := model.SystemRole{Name: name + "-" + uuid.NewString()[:6], Status: "active"}
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		return r
	}
	ownerRole := model.SystemRole{Name: "ORG_OWNER", Status: "active"}
	if err := db.Where("name = ?", ownerRole.Name).FirstOrCreate(&ownerRole).Error; err != nil {
		t.Fatal(err)
	}
	plain := mkRole("QA8_PLAIN")
	u := model.User{Email: "qa8-c23-" + qa8Uniq() + "@demo.com", PasswordHash: "x", UserName: "qa8c23-" + qa8Uniq(), IsActive: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	granter := u.ID // người cấp phải là user có thật (khoá ngoại fk_user_system_roles_granter)

	_, err := svc.AssignSystemRolesToUser(ctx, u.ID, dto.AssignSystemRolesToUserDTO{SystemRoleIDs: []uuid.UUID{ownerRole.ID}}, granter)
	if !errors.Is(err, ErrSystemRoleNotAssignable) {
		t.Fatalf("gán ORG_OWNER: err=%v, muốn ErrSystemRoleNotAssignable", err)
	}
	// Lẫn một vai thường: cả lô bị từ chối, không gán nửa chừng.
	_, err = svc.AssignSystemRolesToUser(ctx, u.ID, dto.AssignSystemRolesToUserDTO{SystemRoleIDs: []uuid.UUID{plain.ID, ownerRole.ID}}, granter)
	if !errors.Is(err, ErrSystemRoleNotAssignable) {
		t.Fatalf("lô có ORG_OWNER: err=%v", err)
	}
	var n int64
	db.Model(&model.UserSystemRole{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 0 {
		t.Fatalf("đã gán %d vai dù lô bị từ chối", n)
	}
	if _, err := svc.AssignSystemRolesToUser(ctx, u.ID, dto.AssignSystemRolesToUserDTO{SystemRoleIDs: []uuid.UUID{plain.ID}}, granter); err != nil {
		t.Fatalf("vai thường vẫn gán được: %v", err)
	}
}

// ── C21: danh sách buổi học ──────────────────────────────────────────────────────────────

func vnToday() time.Time {
	n := time.Now().UTC().Add(7 * time.Hour)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func qa8Sessions(t *testing.T, e *s4ClassEnv, daysFromToday ...int) {
	t.Helper()
	for i, d := range daysFromToday {
		s := model.ClassSession{ClassID: e.class.ID, SessionNumber: i + 1, Date: vnToday().AddDate(0, 0, d), StartTime: "08:00", EndTime: "09:00", Status: model.SessionScheduled}
		if err := e.f.db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func dayOffsets(t *testing.T, res *dto.ClassSessionListDTO) []int {
	t.Helper()
	out := make([]int, len(res.Data))
	for i, s := range res.Data {
		d, err := time.Parse("2006-01-02", s.Date[:10])
		if err != nil {
			t.Fatalf("ngày %q: %v", s.Date, err)
		}
		out[i] = int(d.Sub(vnToday()).Hours() / 24)
	}
	return out
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Đỏ khi bỏ Desc (vẫn tăng dần), bỏ AroundToday (trang 1 thay vì trang chứa hôm nay), hoặc đổi mặc định khỏi tăng dần.
func TestQA8_C21_SessionsByClass_OrderAndAroundToday(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	qa8Sessions(t, e, -5, -4, -3, -2, -1, 0, 1, 2, 3, 4) // 10 buổi, hôm nay là phần tử thứ 6
	list := func(page int, o repository.SessionListOptions) *dto.ClassSessionListDTO {
		res, err := svc.GetSessionsByClass(ctx, e.class.ID, e.owner.ID, false, page, 3, o)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	if got := dayOffsets(t, list(1, repository.SessionListOptions{})); !eqInts(got, []int{-5, -4, -3}) {
		t.Errorf("mặc định phải tăng dần từ buổi sớm nhất: %v", got)
	}
	res := list(1, repository.SessionListOptions{Desc: true})
	if got := dayOffsets(t, res); !eqInts(got, []int{4, 3, 2}) || res.Total != 10 {
		t.Errorf("order=desc: %v total=%d", got, res.Total)
	}
	res = list(1, repository.SessionListOptions{AroundToday: true})
	if got := dayOffsets(t, res); !eqInts(got, []int{-2, -1, 0}) || res.Page != 2 {
		t.Errorf("around=today (tăng dần): %v page=%d, muốn trang 2 chứa hôm nay", got, res.Page)
	}
	res = list(9, repository.SessionListOptions{AroundToday: true, Desc: true})
	if got := dayOffsets(t, res); !eqInts(got, []int{1, 0, -1}) || res.Page != 2 {
		t.Errorf("around=today + desc: %v page=%d (tham số page bị bỏ qua)", got, res.Page)
	}
}

// Đỏ khi không kẹp trang: mọi buổi đã qua và tổng chia hết cho cỡ trang thì trả trang rỗng ngoài cuối.
func TestQA8_C21_SessionsByClass_AroundToday_AllPast_LastPage(t *testing.T) {
	e := newS4ClassEnv(t)
	qa8Sessions(t, e, -6, -5, -4, -3, -2, -1)
	res, err := newS5ScheduleSvc(e).GetSessionsByClass(context.Background(), e.class.ID, e.owner.ID, false, 1, 3, repository.SessionListOptions{AroundToday: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := dayOffsets(t, res); !eqInts(got, []int{-3, -2, -1}) || res.Page != 2 {
		t.Fatalf("mọi buổi đã qua: %v page=%d, muốn trang cuối (2) có 3 buổi", got, res.Page)
	}
}

// ── A01: voucher ─────────────────────────────────────────────────────────────────────────

// Đỏ khi CreateVoucher quay về Create thuần: GORM thay false bằng default true của cột nên voucher "tắt" thành đang chạy.
func TestQA8_A01_CreateVoucher_KeepsIsActiveFalse(t *testing.T) {
	db := isolatedAPISchema(t)
	vs := NewVoucherService(repository.NewVoucherRepository(db), repository.NewUserRepository(db))
	ctx := context.Background()
	off, on := false, true

	req := voucherReq("QA8OFF" + strings.ToUpper(uuid.NewString()[:6]))
	req.IsActive = &off
	req.AcceptAllPaymentMethods = false
	v, err := vs.CreateVoucher(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if v.IsActive || v.AcceptAllPaymentMethods {
		t.Fatalf("voucher trả về: is_active=%v accept_all=%v, muốn cả hai false", v.IsActive, v.AcceptAllPaymentMethods)
	}
	var stored model.Voucher
	if err := db.First(&stored, "id = ?", v.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.IsActive || stored.AcceptAllPaymentMethods {
		t.Fatalf("trong DB: is_active=%v accept_all_payment_methods=%v, muốn cả hai false", stored.IsActive, stored.AcceptAllPaymentMethods)
	}

	reqOn := voucherReq("QA8ON" + strings.ToUpper(uuid.NewString()[:6]))
	reqOn.IsActive = &on
	von, err := vs.CreateVoucher(ctx, reqOn)
	if err != nil {
		t.Fatal(err)
	}
	var storedOn model.Voucher
	db.First(&storedOn, "id = ?", von.ID)
	if !storedOn.IsActive || !storedOn.AcceptAllPaymentMethods {
		t.Fatalf("voucher bật phải giữ true: is_active=%v accept_all=%v", storedOn.IsActive, storedOn.AcceptAllPaymentMethods)
	}
}

// ── T02: tên người mua ───────────────────────────────────────────────────────────────────

// Đỏ khi query quay về users.user_name: ví giảng viên hiện tên đăng nhập ("student1") thay vì họ tên.
func TestQA8_T02_TeacherTransactions_BuyerNameIsFullName(t *testing.T) {
	f := newWithdrawalFixture(t)
	teacher := f.teacher(true)
	course := f.course(teacher)
	orderID := f.order("completed", 0, orderLine{course, 100_000})
	var order model.Order
	if err := f.db.First(&order, "id = ?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	buyerID := order.UserID
	buyerName := func() string {
		rows, _, err := repository.NewWalletRepository(f.db).GetTeacherTransactions(teacher, "", 1, 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		return rows[0].BuyerName
	}
	var userName string
	f.db.Table("users").Where("id = ?", buyerID).Select("user_name").Scan(&userName)

	if got := buyerName(); got != userName {
		t.Fatalf("chưa có họ tên: buyer_name=%q, muốn tên đăng nhập %q", got, userName)
	}
	full := "Lê Văn C"
	f.db.Model(&model.User{}).Where("id = ?", buyerID).Update("full_name", full)
	if got := buyerName(); got != full {
		t.Fatalf("buyer_name=%q, muốn họ tên %q", got, full)
	}
	f.db.Model(&model.User{}).Where("id = ?", buyerID).Update("full_name", "   ")
	if got := buyerName(); got != userName {
		t.Fatalf("full_name chỉ có khoảng trắng: buyer_name=%q, muốn tên đăng nhập %q", got, userName)
	}
}

// ── Tên tổ chức ──────────────────────────────────────────────────────────────────────────

// Đỏ khi GetOrganizationByName quay về `name = ?` (tạo/đổi tên khác hoa thường lọt qua) hoặc đổi tên về chính mình bị chặn nhầm.
func TestQA8_OrgName_CaseInsensitiveUnique(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()
	svc := NewOrganizationService(repository.NewOrganizationRepository(db))
	owner := model.User{Email: "qa8-org-" + qa8Uniq() + "@demo.com", PasswordHash: "x", UserName: "qa8org-" + qa8Uniq(), IsActive: true}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	base := "Trung Tâm QA8 " + qa8Uniq()
	if _, err := svc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: base}); err != nil {
		t.Fatal(err)
	}
	for _, dup := range []string{strings.ToUpper(base), strings.ToLower(base), base} {
		if _, err := svc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: dup}); !errors.Is(err, ErrOrgNameExists) {
			t.Errorf("tạo %q: err=%v, muốn ErrOrgNameExists", dup, err)
		}
	}
	other, err := svc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: base + " 2"})
	if err != nil {
		t.Fatal(err)
	}
	clash := strings.ToLower(base)
	if _, err := svc.UpdateOrganization(ctx, other.ID, dto.UpdateOrganizationDTO{Name: &clash}); !errors.Is(err, ErrOrgNameExists) {
		t.Errorf("đổi tên trùng khác hoa thường: err=%v, muốn ErrOrgNameExists", err)
	}
	same := strings.ToUpper(base + " 2")
	if _, err := svc.UpdateOrganization(ctx, other.ID, dto.UpdateOrganizationDTO{Name: &same}); err != nil {
		t.Errorf("đổi hoa/thường tên của chính mình: %v", err)
	}
}
