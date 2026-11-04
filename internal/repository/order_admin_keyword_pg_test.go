package repository

// QA3-01: ô tìm kiếm ở /admin/orders chỉ lọc 20 dòng đã tải, nên đơn nằm ở trang 2+ (vd FX-000226 ở trang 20 của 606
// đơn) không tìm ra. Backend nhận `q` (AdminOrderFilter.Keyword): khớp mã đơn HOẶC email/tên người mua, bỏ dấu và hoa/thường,
// phân trang + tổng đếm theo kết quả đã lọc. Chạy SQL THẬT (schema tạm).

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestListAdmin_Keyword_MatchesOrderNumberOrBuyer(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	token := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:6])

	mkUser := func(key, fullName string) model.User {
		u := model.User{Email: key + "-" + strings.ToLower(token) + "@40study.test", PasswordHash: "x", UserName: key + strings.ToLower(token), FullName: &fullName}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		return u
	}
	mkOrder := func(u model.User, number, status string) uuid.UUID {
		o := model.Order{UserID: u.ID, OrderNumber: number, Subtotal: decimal.NewFromInt(100000), TotalAmount: decimal.NewFromInt(100000), Status: status}
		if err := db.Create(&o).Error; err != nil {
			t.Fatalf("tạo đơn %s: %v", number, err)
		}
		return o.ID
	}
	an, binh := mkUser("an", "Nguyễn Văn An"), mkUser("binh", "Trần Thị Bình")
	o1 := mkOrder(an, "FX-"+token+"226", "pending")
	o2 := mkOrder(an, "FX-"+token+"300", "completed")
	o3 := mkOrder(binh, "FX-"+token+"227", "completed")

	repo := NewOrderRepository(db)
	list := func(f AdminOrderFilter) (map[uuid.UUID]bool, int64) {
		t.Helper()
		f.Page, f.Limit = 1, 50
		orders, total, err := repo.ListAdmin(f)
		if err != nil {
			t.Fatalf("ListAdmin(%+v): %v", f, err)
		}
		got := map[uuid.UUID]bool{}
		for _, o := range orders {
			got[o.ID] = true
		}
		return got, total
	}
	want := func(label string, f AdminOrderFilter, ids ...uuid.UUID) {
		t.Helper()
		got, total := list(f)
		if len(got) != len(ids) || int(total) != len(ids) {
			t.Errorf("%s: %d đơn (total=%d), muốn %d", label, len(got), total, len(ids))
			return
		}
		for _, id := range ids {
			if !got[id] {
				t.Errorf("%s: thiếu đơn %s", label, id)
			}
		}
	}

	want("mã đơn đầy đủ", AdminOrderFilter{Keyword: "FX-" + token + "226"}, o1)
	want("mã đơn viết thường, chứa chuỗi", AdminOrderFilter{Keyword: strings.ToLower("FX-" + token + "22")}, o1, o3)
	want("tên người mua không dấu", AdminOrderFilter{Keyword: "nguyen van an"}, o1, o2)
	want("tên người mua có dấu, hoa", AdminOrderFilter{Keyword: "TRẦN THỊ BÌNH"}, o3)
	want("email người mua", AdminOrderFilter{Keyword: "binh" + "-" + strings.ToLower(token) + "@40study"}, o3)
	want("kết hợp với status (AND)", AdminOrderFilter{Keyword: "nguyen van an", Status: "completed"}, o2)
	want("không khớp gì", AdminOrderFilter{Keyword: "khong-co-" + token})
	want("ký tự đặc biệt khớp literal", AdminOrderFilter{Keyword: "%"})
	want("không có q thì không lọc", AdminOrderFilter{}, o1, o2, o3)

	// Phân trang theo kết quả đã lọc: total đếm trên tập khớp, mỗi trang đúng cỡ.
	orders, total, err := repo.ListAdmin(AdminOrderFilter{Keyword: "nguyen van an", Page: 1, Limit: 1})
	if err != nil || len(orders) != 1 || total != 2 {
		t.Errorf("phân trang: %d đơn, total=%d (err=%v), muốn 1 đơn và total=2", len(orders), total, err)
	}
}
