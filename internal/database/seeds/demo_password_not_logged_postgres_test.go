package seeds

// Log của seed (chạy trên máy chung, CI) không được chứa mật khẩu demo. Chạy trọn SeedAll + SeedDemoData hai lần (lần
// đầu tạo, lần sau đi nhánh "đã có") với log thu vào bộ đệm. Thêm lại DemoPassword vào bất kỳ log.Printf nào thì ĐỎ.

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"study.com/v1/internal/database"
	"study.com/v1/internal/testutil/pgtest"
)

func TestSeedDemoData_LogNeverContainsDemoPassword(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)

	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	for run := 1; run <= 2; run++ {
		if err := s.SeedAll(s2DataDir); err != nil {
			t.Fatalf("lần %d: SeedAll: %v", run, err)
		}
		if err := s.SeedDemoData(); err != nil {
			t.Fatalf("lần %d: SeedDemoData: %v", run, err)
		}
	}
	out := buf.String()
	if !strings.Contains(out, "Demo data seeded successfully") {
		t.Fatalf("không thu được log seed (%d byte) — test không chứng minh được gì", len(out))
	}
	if strings.Contains(out, DemoPassword) {
		t.Fatal("log seed chứa mật khẩu demo")
	}
}
