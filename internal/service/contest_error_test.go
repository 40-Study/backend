package service

import (
	"errors"
	"testing"
)

// Lỗi dựng riêng kèm details vẫn khớp sentinel qua errors.Is, và không làm bẩn sentinel dùng chung.
func TestContestError_WithDetailsMatchesSentinel(t *testing.T) {
	err := error(ErrContestVoucherUnusable.withDetails("chi tiết", map[string]string{"k": "v"}))
	if !errors.Is(err, ErrContestVoucherUnusable) {
		t.Fatal("lỗi kèm details phải khớp ErrContestVoucherUnusable")
	}
	if errors.Is(err, ErrContestNotEnded) {
		t.Fatal("khác code thì không được khớp")
	}
	if ErrContestVoucherUnusable.Details != nil || ErrContestVoucherUnusable.Message == "chi tiết" {
		t.Fatal("withDetails đã sửa sentinel dùng chung")
	}
}
