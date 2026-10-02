package service

// late_refund_items.go — L6 mục 3. Mỗi dòng history payment_after_expiry là MỘT khoản tiền về muộn
// (một giao dịch ngân hàng). Trước đây cờ "cần hoàn tiền" chỉ là một boolean suy từ mốc thời gian
// (cờ mới nhất so với late_refund_done gần nhất): admin không biết có mấy khoản, bao nhiêu tiền, mã
// giao dịch nào, và một lần bấm "đã hoàn" tắt cờ cho TẤT CẢ kể cả khoản chưa hoàn.
//
// Giờ trạng thái được dựng từ chính history, theo TỪNG khoản:
//   - mỗi dòng cờ → một khoản, khoá (ref) là mã giao dịch ngân hàng, hoặc "history:<id dòng>" khi
//     dòng cũ không có mã giao dịch;
//   - dòng late_refund_done MỚI mở đầu bằng "[refunded_tx=ref1,ref2] " và chỉ phủ đúng các khoản
//     đó. Đặt ở ĐẦU Reason để phân tích không bị ghi chú/mã hoàn tự do của admin làm nhiễu;
//   - dòng late_refund_done CŨ (không có tiền tố) giữ nghĩa cũ: phủ mọi khoản được gắn cờ trước hoặc
//     cùng lúc nó. Dữ liệu hiện có vì thế không đổi nghĩa và không cần migration.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// lateFlagReasonHead dựng phần ĐẦU của Reason dòng cờ. Cả hai nơi ghi cờ (flagRefundNeeded,
// expireWithLatePayment) và bộ phân tích bên dưới dùng chung hàm này nên định dạng không thể lệch.
func lateFlagReasonHead(txID, amount, at string) string {
	return fmt.Sprintf("Received bank transaction %s amount %s at %s", txID, amount, at)
}

var (
	lateFlagReasonRE = regexp.MustCompile(`^Received bank transaction (\S*) amount (\S*) at `)
	lateDoneRefsRE   = regexp.MustCompile(`^\[refunded_tx=([^\]]*)\] `)
)

// lateRefundItem — một khoản tiền về muộn của một đơn.
type lateRefundItem struct {
	Ref           string
	TransactionID string
	Amount        string
	FlaggedAt     time.Time
	Refunded      bool
	RefundedAt    *time.Time
}

// lateRefundSummary — toàn bộ khoản tiền về muộn của một đơn.
type lateRefundSummary struct {
	Items         []lateRefundItem
	PendingCount  int
	PendingAmount decimal.Decimal
	// LastDoneAt: lần admin ghi "đã hoàn" gần nhất (mọi loại dòng late_refund_done), nil nếu chưa có.
	LastDoneAt *time.Time
}

// Needed — còn ít nhất một khoản chưa hoàn.
func (s lateRefundSummary) Needed() bool { return s.PendingCount > 0 }

// HasFlags — đơn từng nhận tiền về muộn (có ít nhất một khoản).
func (s lateRefundSummary) HasFlags() bool { return len(s.Items) > 0 }

// PendingRefs — khoá các khoản chưa hoàn, theo thứ tự gắn cờ.
func (s lateRefundSummary) PendingRefs() []string {
	var out []string
	for _, it := range s.Items {
		if !it.Refunded {
			out = append(out, it.Ref)
		}
	}
	return out
}

// lateDoneMarker dựng tiền tố liệt kê các khoản một lần "đã hoàn" phủ tới.
func lateDoneMarker(refs []string) string {
	return "[refunded_tx=" + strings.Join(refs, ",") + "] "
}

// summarizeLateRefund dựng danh sách khoản từ các dòng history của MỘT đơn (thứ tự bất kỳ; chỉ đọc
// hai loại to_status late flag / late refund done, dòng khác bị bỏ qua).
func summarizeLateRefund(rows []model.OrderStatusHistory) lateRefundSummary {
	sorted := append([]model.OrderStatusHistory(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })

	type doneRow struct {
		at   time.Time
		refs map[string]bool // nil = dòng cũ, phủ theo thời gian
	}
	var dones []doneRow
	var summary lateRefundSummary
	for _, r := range sorted {
		if r.ToStatus != lateRefundDoneHistoryStatus {
			continue
		}
		d := doneRow{at: r.CreatedAt}
		if m := lateDoneRefsRE.FindStringSubmatch(r.Reason); m != nil {
			d.refs = map[string]bool{}
			for _, ref := range strings.Split(m[1], ",") {
				if ref = strings.TrimSpace(ref); ref != "" {
					d.refs[ref] = true
				}
			}
		}
		dones = append(dones, d)
		at := r.CreatedAt
		summary.LastDoneAt = &at // đã sắp tăng dần: dòng cuối là mốc lớn nhất
	}

	for _, r := range sorted {
		if r.ToStatus != latePaymentHistoryStatus {
			continue
		}
		item := lateRefundItem{FlaggedAt: r.CreatedAt}
		if m := lateFlagReasonRE.FindStringSubmatch(r.Reason); m != nil {
			item.TransactionID, item.Amount = m[1], m[2]
		}
		item.Ref = item.TransactionID
		if item.Ref == "" {
			item.Ref = "history:" + r.ID.String()
		}
		for _, d := range dones {
			covered := (d.refs == nil && !r.CreatedAt.After(d.at)) || d.refs[item.Ref]
			if covered {
				at := d.at
				item.Refunded, item.RefundedAt = true, &at
				break // dòng sớm nhất phủ khoản này
			}
		}
		if !item.Refunded {
			summary.PendingCount++
			if amount, err := decimal.NewFromString(item.Amount); err == nil {
				summary.PendingAmount = summary.PendingAmount.Add(amount)
			}
		}
		summary.Items = append(summary.Items, item)
	}
	return summary
}
