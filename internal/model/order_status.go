package model

// OrderStatuses (I-01, review vòng 5) — NGUỒN SỰ THẬT DUY NHẤT cho danh sách giá trị hợp lệ của
// cột orders.status. TRƯỚC ĐÂY (vòng 4, B3-01) danh sách này bị LẶP LẠI thủ công ở 3 nơi độc
// lập: tag `check:` của Order.Status (payment.go), câu SQL literal trong RunPostMigrations
// (internal/database/migrations.go), và codeUsedOrderStatuses (order_status_test.go). Bằng
// chứng review vòng 4 (mutation #4): xóa 'expired' khỏi câu SQL literal trong RunPostMigrations
// (giữ nguyên tag lẫn test) khiến TOÀN BỘ suite vẫn xanh — không có gì đối chiếu SQL đó với bất
// kỳ đâu, đúng cơ chế gây ra bug B3-01 gốc (tag Go đúng, constraint DB sai) và có thể tái diễn
// nguyên xi ở lần thêm status kế tiếp nếu không bịt tận gốc.
//
// Từ vòng 5: slice này là SSOT thật —
//   - RunPostMigrations (internal/database/migrations.go) SINH câu SQL constraint TỪ slice này
//     (buildOrderStatusConstraintSQL) thay vì tự chép lại chuỗi, nên KHÔNG THỂ lệch khỏi slice
//     bằng cách chỉ sửa migrations.go một mình.
//   - Tag `check:` của Order.Status (payment.go) VẪN PHẢI là string literal tĩnh — giới hạn cú
//     pháp Go struct tag, không thể sinh động từ slice ở compile time — nhưng
//     TestOrderStatusTagMatchesSSOT (order_status_test.go) đối chiếu tag với slice này ở CẢ HAI
//     CHIỀU mỗi lần `go test` chạy: sửa 1 bên mà quên bên kia sẽ đỏ ngay, không cần DB thật.
//   - codeUsedOrderStatuses (order_status_test.go, danh sách LIỆT KÊ THỦ CÔNG mọi giá trị code
//     Go thực sự GÁN cho order.Status — không thể tự động suy ra bằng reflection vì đó là literal
//     rải rác trong nhiều file service/repository) cũng được đối chiếu hai chiều với slice này
//     (M-01, TestCodeUsedOrderStatusesMatchesSSOT) — bắt cả 2 hướng lệch: SSOT thiếu status code
//     dùng, HOẶC SSOT thừa status không code nào dùng (rồi phải grep tay xác nhận đó là chủ đích
//     — ví dụ "refunded" chỉ ĐỌC chứ chưa có code path nào GÁN, vẫn giữ vì hợp lệ từ trước).
var OrderStatuses = []string{
	"pending", "processing", "completed", "failed", "refunded", "cancelled", "expired",
}
