package service

// QA7 review #155 ở tầng service (Postgres THẬT, schema tạm): M3 khôi phục voucher đã xoá mềm, M4 tên vai trò tự tạo duy nhất
// không phân biệt hoa thường; M1 chặn số mũ khổng lồ trước Round/so sánh ở service.
// Mutation đã kiểm: trả lại GetVoucherByID có scope ở RestoreVoucher (M3); bỏ ensureNameFree (M4); bỏ DecimalExponentWithinBounds ở
// validateDiscountPrice/validatePaidCoursePrice (M1).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

func TestQA7R_M3_RestoreVoucher_KhoiPhucDuocVoucherDaXoaMem_Postgres(t *testing.T) {
	db := isolatedAPISchema(t)
	vs := NewVoucherService(repository.NewVoucherRepository(db), repository.NewUserRepository(db))
	ctx := context.Background()
	v, err := vs.CreateVoucher(ctx, voucherReq("QA7R"+strings.ToUpper(uuid.NewString()[:8])))
	if err != nil {
		t.Fatal(err)
	}

	// Chưa xoá thì không có gì để khôi phục: không tìm thấy (handler: 404 ERR_NOT_FOUND).
	if err := vs.RestoreVoucher(ctx, v.ID); !errors.Is(err, repository.ErrVoucherNotFound) {
		t.Fatalf("khôi phục voucher chưa xoá: err=%v, muốn ErrVoucherNotFound", err)
	}
	if err := vs.DeleteVoucher(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := vs.GetVoucherByID(ctx, v.ID); !errors.Is(err, repository.ErrVoucherNotFound) {
		t.Fatalf("sau khi xoá, tra theo id phải không thấy: err=%v", err)
	}

	if err := vs.RestoreVoucher(ctx, v.ID); err != nil {
		t.Fatalf("khôi phục voucher đã xoá mềm: %v", err)
	}
	if got, err := vs.GetVoucherByID(ctx, v.ID); err != nil || got.ID != v.ID {
		t.Fatalf("sau khi khôi phục phải tra lại được: got=%v err=%v", got, err)
	}
	// Id không tồn tại vẫn là không tìm thấy.
	if err := vs.RestoreVoucher(ctx, uuid.New()); !errors.Is(err, repository.ErrVoucherNotFound) {
		t.Fatalf("khôi phục id lạ: err=%v, muốn ErrVoucherNotFound", err)
	}
}

func TestQA7R_M4_SystemRole_TenTuTao_KhongPhanBietHoaThuong_Postgres(t *testing.T) {
	db := isolatedAPISchema(t)
	svc := NewSystemRoleService(repository.NewSystemRoleRepository(db), nil)
	ctx := context.Background()
	base := "Qa7r_Moderator_" + uuid.NewString()[:6]

	orig, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: base})
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{strings.ToLower(base), strings.ToUpper(base)} {
		if _, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: variant}); !errors.Is(err, ErrSystemRoleNameExists) {
			t.Errorf("tạo %q khi đã có %q: err=%v, muốn ErrSystemRoleNameExists", variant, base, err)
		}
	}

	// Biến thể hoa/thường của một vai ĐÃ XOÁ MỀM cũng bị chặn (unique index vẫn giữ chỗ).
	gone, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: base + "_gone"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSystemRole(ctx, gone.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: strings.ToUpper(base + "_gone")}); !errors.Is(err, ErrSystemRoleNameExists) {
		t.Errorf("biến thể hoa/thường của vai đã xoá mềm: err=%v, muốn ErrSystemRoleNameExists", err)
	}

	// Đổi tên: sang biến thể hoa/thường của vai KHÁC bị chặn; đổi hoa/thường tên của CHÍNH MÌNH thì được.
	other, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: base + "_other"})
	if err != nil {
		t.Fatal(err)
	}
	clash := strings.ToUpper(base)
	if _, err := svc.UpdateSystemRole(ctx, other.ID, dto.UpdateSystemRoleDTO{Name: &clash}); !errors.Is(err, ErrSystemRoleNameExists) {
		t.Errorf("đổi sang %q: err=%v, muốn ErrSystemRoleNameExists", clash, err)
	}
	own := strings.ToLower(base)
	if got, err := svc.UpdateSystemRole(ctx, orig.ID, dto.UpdateSystemRoleDTO{Name: &own}); err != nil || got.Name != own {
		t.Errorf("đổi hoa/thường tên của chính mình: got=%v err=%v", got, err)
	}
}

// M1 ở service: số mũ khổng lồ bị từ chối ngay (trước đây Round/so sánh rescale về 10^|exp|, ~2 s CPU cho 1e20000000).
func TestQA7R_M1_ValidateCoursePrices_SoMuKhongLo_TuChoiNgay(t *testing.T) {
	huge, _ := decimal.NewFromString("1e20000000")
	tiny, _ := decimal.NewFromString("1e-20000000")
	fifty := decimal.NewFromInt(50000)

	start := time.Now()
	for _, d := range []decimal.Decimal{huge, tiny} {
		if err := validateDiscountPrice(fifty, d); !errors.Is(err, ErrDiscountPriceInvalid) {
			t.Errorf("discount exp %d: err=%v, muốn ErrDiscountPriceInvalid", d.Exponent(), err)
		}
		if err := validateDiscountPrice(d, fifty); !errors.Is(err, ErrDiscountPriceInvalid) {
			t.Errorf("price exp %d: err=%v, muốn ErrDiscountPriceInvalid", d.Exponent(), err)
		}
		if err := validatePaidCoursePrice(false, d); !errors.Is(err, ErrPaidCoursePriceInvalid) {
			t.Errorf("paid price exp %d: err=%v, muốn ErrPaidCoursePriceInvalid", d.Exponent(), err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("mất %v, muốn từ chối ngay trước Round/so sánh", elapsed)
	}
	// Giá bình thường (kể cả viết dài "1000.000000") vẫn qua.
	ok, _ := decimal.NewFromString("1000.000000")
	if err := validatePaidCoursePrice(false, ok); err != nil {
		t.Errorf("giá hợp lệ bị từ chối: %v", err)
	}
	if err := validateDiscountPrice(fifty, decimal.NewFromInt(49000)); err != nil {
		t.Errorf("khuyến mãi hợp lệ bị từ chối: %v", err)
	}
}
