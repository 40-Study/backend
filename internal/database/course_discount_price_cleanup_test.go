package database

// Test Postgres THẬT cho dọn courses.discount_price (course_discount_price_cleanup.go), gọi qua
// RunPostMigrations như lúc API khởi động: bỏ lời gọi khỏi RunPostMigrations, đổi <= thành <, hoặc bỏ
// điều kiện >= price thì test ĐỎ; bỏ điều kiện lọc (xoá mọi khuyến mãi) cũng ĐỎ vì ca hợp lệ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestCourseDiscountPriceCleanup_NullsInvalidKeepsValid_Idempotent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	instructor := backfillUser(t, db, "disc")
	expiry := time.Now().Add(48 * time.Hour)
	dec := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	newCourse := func(name, price string, discount *decimal.Decimal) uuid.UUID {
		c := model.Course{InstructorID: instructor, Title: "QA-disc " + name, Slug: "qa-disc-" + uuid.NewString(),
			Price: decimal.RequireFromString(price), DiscountPrice: discount, DiscountExpiresAt: &expiry}
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("tạo khoá: %v", err)
		}
		return c.ID
	}

	cases := []struct {
		name      string
		id        uuid.UUID
		wantKept  bool
		wantValue string
	}{
		{"bằng 0", newCourse("zero", "500000", dec("0")), false, ""},
		{"âm", newCourse("negative", "500000", dec("-1")), false, ""},
		{"bằng giá gốc", newCourse("equal", "500000", dec("500000")), false, ""},
		{"lớn hơn giá gốc", newCourse("above", "500000", dec("600000")), false, ""},
		{"hợp lệ", newCourse("valid", "500000", dec("400000")), true, "400000"},
		{"hợp lệ sát giá gốc", newCourse("close", "500000", dec("499999.99")), true, "499999.99"},
		{"không có khuyến mãi", newCourse("none", "500000", nil), false, ""},
	}
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
		for _, c := range cases {
			var got model.Course
			if err := db.First(&got, "id = ?", c.id).Error; err != nil {
				t.Fatalf("đọc khoá: %v", err)
			}
			if c.wantKept {
				if got.DiscountPrice == nil || !got.DiscountPrice.Equal(decimal.RequireFromString(c.wantValue)) || got.DiscountExpiresAt == nil {
					t.Errorf("lần %d, %s: muốn giữ %s và hạn, nhận %v / %v", run, c.name, c.wantValue, got.DiscountPrice, got.DiscountExpiresAt)
				}
				continue
			}
			if got.DiscountPrice != nil {
				t.Errorf("lần %d, %s: muốn NULL, nhận %s", run, c.name, got.DiscountPrice)
			}
			// Dòng không có khuyến mãi từ đầu cũng không được giữ hạn mồ côi sau lần dọn đầu tiên? Không:
			// dọn chỉ chạm dòng CÓ discount_price vi phạm, nên ca "none" giữ nguyên hạn của nó.
			if c.name != "không có khuyến mãi" && got.DiscountExpiresAt != nil {
				t.Errorf("lần %d, %s: muốn hạn khuyến mãi bị bỏ theo, nhận %v", run, c.name, got.DiscountExpiresAt)
			}
		}
	}
}
