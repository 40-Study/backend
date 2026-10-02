package service

// L8 mục 7: gói test dùng lại một schema đã migrate (isolatedAPISchema) thay vì migrate + DROP cho từng test. Test
// này chứng minh schema dùng lại SAU KHI một test đã làm bẩn nó giống hệt một schema vừa migrate sạch: cùng cấu trúc
// (dấu vân tay: cột, chỉ mục, ràng buộc...), cùng số dòng ở MỌI bảng, cùng dòng baseline (data_migrations). Nếu ai
// thêm vào Migrate thứ mà bước reset không đưa về được (sequence, bảng có dữ liệu mồi...), test này đỏ.

import (
	"context"
	"testing"

	"study.com/v1/internal/testutil/pgtest"
)

func tableRowCounts(t *testing.T, f *orderFixture) map[string]int64 {
	t.Helper()
	var tables []string
	if err := f.db.Raw("SELECT tablename FROM pg_tables WHERE schemaname = current_schema()").Scan(&tables).Error; err != nil {
		t.Fatalf("liệt kê bảng: %v", err)
	}
	out := make(map[string]int64, len(tables))
	for _, tbl := range tables {
		var n int64
		if err := f.db.Raw(`SELECT count(*) FROM "` + tbl + `"`).Scan(&n).Error; err != nil {
			t.Fatalf("đếm %s: %v", tbl, err)
		}
		out[tbl] = n
	}
	return out
}

func TestAPISchemaReuse_ResetEqualsFreshMigrate(t *testing.T) {
	// Test này chứng minh chính cơ chế dùng lại schema (lượt sau nhận đúng schema lượt trước). PGTEST_REUSE=0 là lối
	// thoát có chủ đích: mỗi test một schema mới, nên "không dùng lại" là hành vi đúng, không phải lỗi (L9 mục 4).
	if !pgtest.ReuseEnabled() {
		t.Skip("PGTEST_REUSE=0: cơ chế dùng lại schema đang tắt, không có gì để so sánh")
	}
	var reusedSchema string
	t.Run("dirty", func(t *testing.T) {
		f := newOrderFixture(t)
		reusedSchema = currentSchemaOf(t, f)
		student := f.user()
		order := f.createOrder(student, f.course("L8 làm bẩn schema"))
		if _, err := f.paymentService().CreatePaymentIntent(context.Background(), student, order.ID, false, "qr_transfer"); err != nil {
			t.Fatalf("CreatePaymentIntent: %v", err)
		}
		f.exec("INSERT INTO data_migrations (name) VALUES ('l8-extra-row')") // dòng thêm vào bảng baseline
		f.exec("UPDATE data_migrations SET name = name || '-edited'")        // và dòng baseline bị sửa
		if n := tableRowCounts(t, f)["orders"]; n == 0 {
			t.Fatal("test làm bẩn không ghi được dữ liệu, test này không chứng minh gì")
		}
	})
	t.Run("compare", func(t *testing.T) {
		reused := newOrderFixture(t)
		if got := currentSchemaOf(t, reused); got != reusedSchema {
			t.Fatalf("lượt sau nhận schema %s, muốn dùng lại %s (PGTEST_REUSE=0?)", got, reusedSchema)
		}
		fresh := &orderFixture{t: t, db: pgtest.IsolatedSchema(t, migrateLikeAPIBoot)}

		fpReused, err := pgtest.SchemaFingerprint(reused.db, currentSchemaOf(t, reused))
		if err != nil {
			t.Fatal(err)
		}
		fpFresh, err := pgtest.SchemaFingerprint(fresh.db, currentSchemaOf(t, fresh))
		if err != nil {
			t.Fatal(err)
		}
		if fpReused != fpFresh {
			t.Fatalf("cấu trúc schema dùng lại khác schema vừa migrate sạch: %s vs %s", fpReused, fpFresh)
		}

		cReused, cFresh := tableRowCounts(t, reused), tableRowCounts(t, fresh)
		if len(cReused) != len(cFresh) {
			t.Fatalf("số bảng: dùng lại %d, sạch %d", len(cReused), len(cFresh))
		}
		for tbl, want := range cFresh {
			if got := cReused[tbl]; got != want {
				t.Errorf("bảng %s: dùng lại %d dòng, schema sạch %d dòng", tbl, got, want)
			}
		}

		names := func(f *orderFixture) []string {
			var out []string
			if err := f.db.Raw("SELECT name FROM data_migrations ORDER BY name").Scan(&out).Error; err != nil {
				t.Fatalf("đọc data_migrations: %v", err)
			}
			return out
		}
		a, b := names(reused), names(fresh)
		if len(a) != len(b) || (len(a) > 0 && a[0] != b[0]) {
			t.Fatalf("data_migrations: dùng lại %v, sạch %v", a, b)
		}
		// Chạy thêm một giao dịch bình thường trên schema dùng lại: không còn dấu vết nào của test trước (id, ràng buộc).
		student := reused.user()
		reused.createOrder(student, reused.course("L8 sau reset"))
	})
}

func currentSchemaOf(t *testing.T, f *orderFixture) string {
	t.Helper()
	var s string
	if err := f.db.Raw("SELECT current_schema()").Scan(&s).Error; err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	return s
}
