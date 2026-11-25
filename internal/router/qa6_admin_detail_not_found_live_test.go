package router

import "testing"

// QA6 review M1/M2: GET /contests/manage/:id GIỮ mã lỗi cũ (404 CONTEST_NOT_FOUND, 400 INVALID_ID) vì trang giảng viên trên
// web map đúng hai mã này và /contests/manage/:id/participants cũng trả CONTEST_NOT_FOUND. Chỉ chi tiết đơn hàng/người dùng
// của admin dùng bộ mã C8 (ERR_NOT_FOUND / ERR_INVALID_ID, xem qa6_admin_detail_not_found_handler_test.go). Đổi lại
// GetManage sang ERR_NOT_FOUND / ERR_INVALID_ID thì test ĐỎ.
func TestQA6_ContestManageDetail_GiuMaLoiCu(t *testing.T) {
	e := newCtEnv(t)
	e.must("id khong co", e.do("GET", "/api/contests/manage/00000000-0000-0000-0000-000000000000", "admin", ""), 404, "CONTEST_NOT_FOUND")
	e.must("id sai dinh dang", e.do("GET", "/api/contests/manage/not-a-uuid", "admin", ""), 400, "INVALID_ID")
}
