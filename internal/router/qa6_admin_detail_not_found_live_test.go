package router

import "testing"

// QA6 review M1/M2 + QA8 A08 (C24): GET /contests/manage/:id GIỮ 404 CONTEST_NOT_FOUND (trang giảng viên và slug map mã này,
// /contests/manage/:id/participants cũng trả nó; trang chi tiết admin đã coi mọi 404 là "không tìm thấy"), còn id sai định dạng là
// 400 ERR_INVALID_ID như đơn hàng/người dùng (QA8 A08: mã cũ INVALID_ID không được trang admin nhận ra, hiện khối lỗi + "Thử lại").
// Đổi lại withContestID sang INVALID_ID, hay GetManage sang ERR_NOT_FOUND, thì test ĐỎ.
func TestQA6_ContestManageDetail_MaLoi(t *testing.T) {
	e := newCtEnv(t)
	e.must("id khong co", e.do("GET", "/api/contests/manage/00000000-0000-0000-0000-000000000000", "admin", ""), 404, "CONTEST_NOT_FOUND")
	e.must("id sai dinh dang", e.do("GET", "/api/contests/manage/not-a-uuid", "admin", ""), 400, "ERR_INVALID_ID")
}
