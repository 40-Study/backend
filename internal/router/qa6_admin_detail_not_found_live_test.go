package router

import "testing"

// QA6-A06 (C8): admin mở /contests/manage/:id với id không có -> 404 ERR_NOT_FOUND, id sai định dạng -> 400 ERR_INVALID_ID,
// cả hai kèm message tiếng Việt. Trước đây là CONTEST_NOT_FOUND / INVALID_ID tiếng Anh, khác đơn hàng và người dùng nên
// màn admin không dựng được một trạng thái "Không tìm thấy" chung. Đổi lại GetManage về mã cũ thì test ĐỎ.
func TestQA6_AdminContestDetail_KhongThayVaIDSai(t *testing.T) {
	e := newCtEnv(t)
	missing := e.must("id khong co", e.do("GET", "/api/contests/manage/00000000-0000-0000-0000-000000000000", "admin", ""), 404, "ERR_NOT_FOUND")
	if m, _ := missing.body["message"].(string); m != "Không tìm thấy cuộc thi." {
		t.Errorf("message 404 = %q", m)
	}
	bad := e.must("id sai dinh dang", e.do("GET", "/api/contests/manage/not-a-uuid", "admin", ""), 400, "ERR_INVALID_ID")
	if m, _ := bad.body["message"].(string); m != "Mã cuộc thi không hợp lệ." {
		t.Errorf("message 400 = %q", m)
	}
}
