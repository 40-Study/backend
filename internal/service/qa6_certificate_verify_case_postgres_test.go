package service

// QA6-PG-05: tra cứu chứng chỉ công khai không phân biệt hoa/thường và bỏ khoảng trắng hai đầu (số in trên PDF có
// tiền tố hoa, phần hex thường nên gõ lệch rất dễ). Đổi lại `certificate_number = ?` thì ca hoa/thường/khoảng trắng ĐỎ;
// số lạ vẫn phải không hợp lệ.

import (
	"context"
	"strings"
	"testing"
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
