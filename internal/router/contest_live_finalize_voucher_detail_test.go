package router

// Re-review vòng 2 (chủ dự án chốt 29/09): voucher giải không phát được vẫn chặn cả lần chốt với
// 409 CONTEST_VOUCHER_UNAVAILABLE, nhưng response nói rõ người thắng (id, tên, hạng) và voucher
// (id, mã, lý do) để admin sửa giải rồi chốt lại. Phát thưởng đi qua ContestRewardService THẬT.

import (
	"encoding/json"
	"strings"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

func TestContestLive_FinalizeVoucherErrorNamesWinnerAndVoucher(t *testing.T) {
	e := newCtEnv(t)
	vouchers := service.NewVoucherService(repository.NewVoucherRepository(e.db), repository.NewUserRepository(e.db))
	e.issuer.delegate = service.NewContestRewardService(vouchers, nil) // chỉ dùng IssueAwardTx
	good, exhausted := e.newVoucher(true), e.newVoucher(true)
	id := e.finalizeFixture(good, exhausted)
	// Hết tổng lượt dùng SAU khi duyệt (duyệt đã kiểm voucher còn dùng được).
	e.db.Model(&model.Voucher{}).Where("id = ?", exhausted).Updates(map[string]interface{}{"usage_limit": 1, "used_count": 1})
	var code string
	e.db.Model(&model.Voucher{}).Where("id = ?", exhausted).Pluck("code", &code)

	r := e.must("chot voi voucher het luot", e.do("POST", "/api/admin/contests/"+id.String()+"/finalize", "admin", ""),
		409, "CONTEST_VOUCHER_UNAVAILABLE")
	var body struct {
		Message string `json:"message"`
		Details *struct {
			UserID      string `json:"user_id"`
			UserName    string `json:"user_name"`
			Rank        *int   `json:"rank"`
			VoucherID   string `json:"voucher_id"`
			VoucherCode string `json:"voucher_code"`
			Reason      string `json:"reason"`
		} `json:"details"`
	}
	if err := json.Unmarshal([]byte(r.raw), &body); err != nil {
		t.Fatalf("parse: %v", err)
	}
	d := body.Details
	if d == nil {
		t.Fatalf("409 thieu details nguoi thang/voucher: %s", r.raw)
	}
	// Giải hạng 2 mang voucher hết lượt; hạng 2 là student1 hoặc student2 (cùng 2/3, tie-break theo giờ nộp).
	winner := ""
	for _, who := range []string{"student1", "student2"} {
		if e.ids[who].String() == d.UserID {
			winner = who
		}
	}
	if winner == "" || d.UserName != "QA-contest "+winner || d.Rank == nil || *d.Rank != 2 {
		t.Fatalf("details nguoi thang sai: %+v", d)
	}
	if d.VoucherID != exhausted.String() || d.VoucherCode != code || d.Reason != service.VoucherGrantReasonUsageLimit {
		t.Fatalf("details voucher sai: %+v (muon %s/%s, ly do het tong luot)", d, exhausted, code)
	}
	for _, want := range []string{code, d.UserName, "hạng 2"} {
		if !strings.Contains(body.Message, want) {
			t.Fatalf("message thieu %q: %s", want, body.Message)
		}
	}
	// Vẫn chặn cả lần chốt: rollback sạch.
	var c model.Contest
	e.db.First(&c, "id = ?", id)
	if c.FinalizedAt != nil || e.count("contest_awards", id) != 0 || e.userVoucherCount() != 0 || e.issuer.notifyCalls() != 0 {
		t.Fatalf("chot bi chan ma van ghi du lieu: finalized=%v awards=%d vouchers=%d", c.FinalizedAt, e.count("contest_awards", id), e.userVoucherCount())
	}
}
