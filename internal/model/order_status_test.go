package model

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// codeUsedOrderStatuses (B3-01, review vòng 4): LIỆT KÊ THỦ CÔNG mọi giá trị "orders.status" mà
// service/repository code Go từng gán (UpdateStatus/UpdatePaymentInfo/Create) hoặc so sánh trực
// tiếp (order.Status == "...") — cập nhật danh sách này MỖI KHI thêm một trạng thái order mới.
// Grep xác nhận (2026-09-09, review vòng 4): "pending"/"processing"/"completed"/"failed"/
// "cancelled"/"expired" đều được service code GÁN; "refunded" hiện chỉ được SO SÁNH (đọc), chưa
// có code path nào gán, nhưng đã có sẵn trong constraint từ trước — giữ trong danh sách để test
// này không tình cờ "thắt chặt" một giá trị vốn đã hợp lệ. Không thể tự động suy ra danh sách
// này bằng reflection (literal rải rác nhiều file service/repository) — vẫn cần grep tay khi
// thêm status mới, TestCodeUsedOrderStatusesMatchesSSOT bên dưới chỉ bắt được LỆCH giữa danh
// sách này và OrderStatuses (SSOT), không tự phát hiện được literal Go mới nào chưa được liệt kê
// vào ĐÂY (M-01, review vòng 5 — hạn chế đã biết, ghi rõ thay vì giấu).
var codeUsedOrderStatuses = []string{
	"pending", "processing", "completed", "failed", "cancelled", "refunded", "expired",
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// assertSetsEqual báo lỗi CẢ HAI CHIỀU: phần tử có trong "got" nhưng thiếu trong "want", VÀ phần
// tử có trong "want" nhưng thiếu trong "got" — một test chỉ kiểm 1 chiều (như bản gốc vòng 4)
// chỉ bắt được 1 trong 2 kiểu lệch (M-01, review vòng 5): rút 1 phần tử khỏi BÊN NÀO cũng phải
// đỏ, không chỉ riêng chiều "thiếu".
func assertSetsEqual(t *testing.T, gotName string, got []string, wantName string, want []string) {
	t.Helper()
	gotSet, wantSet := toSet(got), toSet(want)
	for v := range wantSet {
		if !gotSet[v] {
			t.Errorf("%s thiếu %q — %s có nhưng %s không có", gotName, v, wantName, gotName)
		}
	}
	for v := range gotSet {
		if !wantSet[v] {
			t.Errorf("%s thừa %q — %s có nhưng %s không có (kiểm tra lại có phải chủ đích không)", gotName, v, gotName, wantName)
		}
	}
}

func extractTagStatusValues(t *testing.T) []string {
	t.Helper()
	field, ok := reflect.TypeOf(Order{}).FieldByName("Status")
	if !ok {
		t.Fatal("Order struct không còn field Status — cập nhật lại test này")
	}
	gormTag := field.Tag.Get("gorm")

	re := regexp.MustCompile(`check:status IN \(([^)]*)\)`)
	match := re.FindStringSubmatch(gormTag)
	if match == nil {
		t.Fatalf("không tìm thấy check:status IN (...) trong gorm tag của Order.Status: %q", gormTag)
	}

	values := make([]string, 0)
	for _, raw := range strings.Split(match[1], ",") {
		values = append(values, strings.Trim(strings.TrimSpace(raw), "'"))
	}
	return values
}

// TestOrderStatusTagMatchesSSOT (B3-01 vòng 4, viết lại theo I-01 review vòng 5) — đối chiếu tag
// `check:status IN (...)` của Order.Status (payment.go) với OrderStatuses (order_status.go,
// SSOT) ở CẢ HAI CHIỀU bằng reflection (không hard-code lại chuỗi CHECK trong test).
//
// Trước vòng 5, test này (TestOrderStatusConstraintCoversCodeUsage) chỉ kiểm MỘT chiều (tag
// thiếu status code dùng) và so trực tiếp với codeUsedOrderStatuses thay vì một SSOT thật —
// không có gì bắt được việc RunPostMigrations tự chép một bản SAO KHÁC của cùng danh sách này
// (mutation #4, review vòng 4: xóa 'expired' khỏi SQL literal trong RunPostMigrations vẫn xanh
// toàn bộ suite). Từ vòng 5, RunPostMigrations SINH SQL từ chính OrderStatuses
// (buildOrderStatusConstraintSQL, internal/database/migrations_test.go tự kiểm phần đó riêng)
// nên test này chỉ còn cần đối chiếu ĐÚNG MỘT ranh giới còn lại không thể tự sinh: tag Go
// (bắt buộc string literal tĩnh) so với slice.
//
// Tự kiểm chứng mutation CẢ HAI CHIỀU (ghi lại kết quả, không giữ mutation trong working tree):
//  1. Xóa "expired" khỏi OrderStatuses (order_status.go) — GIỮ NGUYÊN tag → đỏ ("OrderStatuses
//     thiếu... tag có nhưng OrderStatuses không có" — đúng chiều "tag thừa so với SSOT").
//  2. Khôi phục OrderStatuses, xóa 'expired' khỏi tag check: (payment.go) — GIỮ NGUYÊN
//     OrderStatuses → đỏ ("tag thiếu... OrderStatuses có nhưng tag không có" — đúng chiều
//     "SSOT thừa so với tag").
func TestOrderStatusTagMatchesSSOT(t *testing.T) {
	tagValues := extractTagStatusValues(t)
	assertSetsEqual(t, "tag check:status IN (...) của Order.Status", tagValues, "OrderStatuses (SSOT)", OrderStatuses)
}

// TestCodeUsedOrderStatusesMatchesSSOT (M-01, review vòng 5) — companion test cho
// codeUsedOrderStatuses: đối chiếu HAI CHIỀU với OrderStatuses (SSOT). Trước vòng 5,
// codeUsedOrderStatuses chỉ được dùng để kiểm 1 chiều "tag có đủ status code dùng không" — rút
// 1 phần tử khỏi codeUsedOrderStatuses vẫn để suite xanh (mutation #6, review vòng 4) vì không
// có gì đối chiếu NGƯỢC LẠI (SSOT có status nào code không hề dùng, hoặc bản thân
// codeUsedOrderStatuses tự ý bỏ sót 1 giá trị so với SSOT).
func TestCodeUsedOrderStatusesMatchesSSOT(t *testing.T) {
	assertSetsEqual(t, "codeUsedOrderStatuses", codeUsedOrderStatuses, "OrderStatuses (SSOT)", OrderStatuses)
}
