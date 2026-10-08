package handler

// Test cho QA T13 (261008): GET /courses/:id và GET /classes/:id với id không phải UUID (vd
// "does-not-exist") trả 400 "Invalid ... ID". Với endpoint chi tiết theo id, một id không parse
// được UUID thì chắc chắn không trỏ tới tài nguyên nào => 404 như id UUID hợp lệ nhưng không có.
// Handler trả lời trước khi chạm service nên service = nil là đủ.
//
// Mutation muốn bắt: trả lại StatusBadRequest ở nhánh uuid.Parse của GetCourseByID/GetClassByID
// phải làm test ĐỎ.

import (
	"testing"

	"github.com/google/uuid"
)

func TestGetDetailByID_IDKhongPhaiUUID_Tra404(t *testing.T) {
	course := NewCourseHandler(nil, nil)
	class := NewClassHandler(nil, nil)
	app := mountWithCaller("GET", "/courses/:id", uuid.New(), course.GetCourseByID)
	if code := doJSON(t, app, "GET", "/courses/does-not-exist", ""); code != 404 {
		t.Errorf("GET /courses/does-not-exist: muốn 404, nhận %d", code)
	}
	app = mountWithCaller("GET", "/classes/:id", uuid.New(), class.GetClassByID)
	if code := doJSON(t, app, "GET", "/classes/does-not-exist", ""); code != 404 {
		t.Errorf("GET /classes/does-not-exist: muốn 404, nhận %d", code)
	}
}
