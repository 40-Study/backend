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
// này không tình cờ "thắt chặt" một giá trị vốn đã hợp lệ.
var codeUsedOrderStatuses = []string{
	"pending", "processing", "completed", "failed", "cancelled", "refunded", "expired",
}

// TestOrderStatusConstraintCoversCodeUsage (B3-01, review vòng 4) — pin lại: MỌI giá trị
// orders.status mà code Go thực sự dùng (codeUsedOrderStatuses ở trên) PHẢI nằm trong tag
// `check:status IN (...)` của Order.Status. Nếu không, UPDATE orders SET status=<giá trị đó>
// sẽ vi phạm CHECK constraint chk_orders_status ở DB — đúng lỗi B3-01 gốc: PaymentService.
// CheckAndProcessPayment chuyển đơn sang "expired" khi hết hạn mã thanh toán, nhưng "expired"
// từng vắng mặt trong constraint, gây lỗi runtime 100% (UPDATE luôn bị Postgres từ chối, đơn
// kẹt "processing" vĩnh viễn, used_count không bao giờ được release).
//
// Đọc tag qua reflection (không hard-code lại chuỗi CHECK) để test tự phát hiện khi ai đó thêm
// 1 status Go mới vào codeUsedOrderStatuses mà quên nới rộng tag — hoặc ngược lại, sửa tag mà
// quên cập nhật RunPostMigrations (internal/database/migrations.go, không kiểm được bằng test
// thuần Go vì cần DB thật — xem "câu hỏi treo").
func TestOrderStatusConstraintCoversCodeUsage(t *testing.T) {
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

	constraintValues := make(map[string]bool)
	for _, raw := range strings.Split(match[1], ",") {
		v := strings.Trim(strings.TrimSpace(raw), "'")
		constraintValues[v] = true
	}

	for _, used := range codeUsedOrderStatuses {
		if !constraintValues[used] {
			t.Errorf("status %q được code Go dùng nhưng THIẾU trong CHECK constraint tag của Order.Status (%q) — UPDATE orders SET status=%q sẽ vi phạm chk_orders_status ở DB (B3-01)", used, gormTag, used)
		}
	}
}
