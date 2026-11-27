package service

// QA6-PG-05: tra cứu chứng chỉ công khai không phân biệt hoa/thường và bỏ khoảng trắng hai đầu (số in trên PDF có
// tiền tố hoa, phần hex thường nên gõ lệch rất dễ). Đổi lại `certificate_number = ?` thì ca hoa/thường/khoảng trắng ĐỎ;
// số lạ vẫn phải không hợp lệ.

import (
	"context"
	"strings"
	"testing"

	"study.com/v1/internal/repository"
)

func TestQA6_VerifyCertificate_KhongPhanBietHoaThuongVaKhoangTrang(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()
	for name, input := range map[string]string{
		"nguyên văn": rc.number, "hoa hết": strings.ToUpper(rc.number), "thường hết": strings.ToLower(rc.number), "có khoảng trắng": "  " + rc.number + " \n",
	} {
		v, err := rc.certSvc.VerifyCertificate(ctx, input)
		if err != nil || !v.Valid || v.CertificateNumber != rc.number {
			t.Errorf("%s (%q): verify=%+v err=%v, muốn hợp lệ và trả số gốc %q", name, input, v, err, rc.number)
		}
	}
	if v, err := rc.certSvc.VerifyCertificate(ctx, rc.number+"x"); err != nil || v.Valid {
		t.Errorf("số lạ: verify=%+v err=%v, muốn không hợp lệ", v, err)
	}
}

// Q10-PG-05: mã dán từ app chat hay kèm ký tự vô hình (zero-width space, BOM, word joiner) và NBSP; JS trim() và
// strings.TrimSpace của Go không bỏ zero-width space nên trước đây ra "Không tìm thấy chứng chỉ" mà người dùng không
// thấy lý do. Mọi biến thể phải ra đúng chứng chỉ gốc.
func TestQ10_VerifyCertificate_PG_BoKyTuVoHinhVaNBSP(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()
	zw, bom, wj, nbsp := "\u200b", "\ufeff", "\u2060", "\u00a0"
	for name, input := range map[string]string{
		"zero-width space cuối": rc.number + zw, "zero-width space đầu": zw + rc.number, "BOM": bom + rc.number,
		"word joiner giữa mã": rc.number[:6] + wj + rc.number[6:], "NBSP hai đầu": nbsp + rc.number + nbsp,
		"NBSP giữa mã":  rc.number[:6] + nbsp + rc.number[6:],
		"hỗn hợp + hoa": " " + zw + nbsp + strings.ToUpper(rc.number) + zw + " ",
	} {
		v, err := rc.certSvc.VerifyCertificate(ctx, input)
		if err != nil || !v.Valid || v.CertificateNumber != rc.number {
			t.Errorf("%s (%q): verify=%+v err=%v, muốn hợp lệ và trả số gốc %q", name, input, v, err, rc.number)
		}
		if stored, err := repository.NewCertificateRepository(rc.db).GetCertificateByNumber(ctx, input); err != nil || stored == nil {
			t.Errorf("%s: repository không tra được %q: %v", name, input, err)
		}
	}
	if v, err := rc.certSvc.VerifyCertificate(ctx, zw+rc.number+"x"+zw); err != nil || v.Valid {
		t.Errorf("số lạ có ký tự vô hình vẫn phải không hợp lệ: verify=%+v err=%v", v, err)
	}
}
