// Package apperr (review vòng 2, MAJOR — plans/reports/review-260928-round2-integration.md,
// mục C): cart_handler.go và review_handler.go trước đây trả nguyên văn `err.Error()` của tầng
// service/repository ra JSON response — kể cả cho lỗi hạ tầng không xác định (DB, GORM, v.v.),
// có thể lộ chi tiết nội bộ (câu lỗi driver, tên cột, constraint) cho client.
//
// KnownError đánh dấu MỘT lỗi nghiệp vụ đã biết trước, an toàn để hiển thị Message nguyên văn
// (validate/không tìm thấy/đã tồn tại/không có quyền). Đặt ở package riêng (không phải
// internal/handler) để internal/service tạo được KnownError mà không phải import ngược
// internal/handler (service bị handler phụ thuộc, không phải chiều ngược lại).
package apperr

import "net/http"

// KnownError là một lỗi nghiệp vụ tầng service/repository đã được nhận diện, Message của nó AN
// TOÀN để trả thẳng cho client. Bất kỳ error nào KHÔNG phải *KnownError (lỗi DB, mạng, v.v.) đều
// được xem là "không xác định" — tầng handler phải log lại và chỉ trả message chung, không bao
// giờ trả err.Error() của nó.
type KnownError struct {
	Status  int
	Message string
}

func (e *KnownError) Error() string { return e.Message }

// NotFound: không tìm thấy đối tượng (khoá học, review, ...). HTTP 404.
func NotFound(message string) *KnownError {
	return &KnownError{Status: http.StatusNotFound, Message: message}
}

// Conflict: trạng thái nghiệp vụ đã tồn tại/xung đột (đã có trong giỏ hàng, đã đăng ký, đã
// review rồi, ...). HTTP 409.
func Conflict(message string) *KnownError {
	return &KnownError{Status: http.StatusConflict, Message: message}
}

// Forbidden: đã xác thực nhưng không có quyền trên đối tượng cụ thể (không phải chủ sở hữu).
// HTTP 403.
func Forbidden(message string) *KnownError {
	return &KnownError{Status: http.StatusForbidden, Message: message}
}

// BadRequest: dữ liệu nghiệp vụ không hợp lệ theo quy tắc domain (khác lỗi parse/validate cấu
// trúc DTO ở tầng handler, vốn đã an toàn để trả nguyên văn vì chỉ phản ánh lại request của
// chính client). HTTP 400.
func BadRequest(message string) *KnownError {
	return &KnownError{Status: http.StatusBadRequest, Message: message}
}
