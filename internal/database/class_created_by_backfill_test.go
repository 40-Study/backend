package database

// Test Postgres THẬT cho backfill classes.created_by (class_created_by_backfill.go), gọi qua
// RunPostMigrations như lúc API khởi động: bỏ lời gọi khỏi RunPostMigrations, đổi ASC thành DESC,
// bỏ lọc role primary hoặc bỏ điều kiện created_by IS NULL thì test ĐỎ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestClassCreatedByBackfill_GiangVienPrimarySomNhat_ChayLaiKhongDoiGi(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	first, second, assistant, realOwner := backfillUser(t, db, "first"), backfillUser(t, db, "second"),
		backfillUser(t, db, "assistant"), backfillUser(t, db, "owner")
	now := time.Now()
	newClass := func(name string, owner *uuid.UUID) uuid.UUID {
		c := model.Class{Name: "QA-bf-" + name, CreatedBy: owner}
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("tạo lớp: %v", err)
		}
		return c.ID
	}
	assign := func(class, teacher uuid.UUID, role string, at time.Time) {
		if err := db.Create(&model.TeacherClass{TeacherID: teacher, ClassID: class, Role: role, AssignedAt: at}).Error; err != nil {
			t.Fatalf("gán giảng viên: %v", err)
		}
	}

	multi := newClass("nhieu-primary", nil)
	assign(multi, second, "primary", now.Add(-1*time.Hour))
	assign(multi, first, "primary", now.Add(-48*time.Hour)) // sớm nhất trong các primary
	assign(multi, assistant, "assistant", now.Add(-96*time.Hour)) // sớm hơn nhưng là trợ giảng: không được chọn
	onlyAssistant := newClass("chi-tro-giang", nil)
	assign(onlyAssistant, assistant, "assistant", now)
	noTeacher := newClass("khong-giang-vien", nil)
	owned := newClass("da-co-chu", &realOwner)
	assign(owned, first, "primary", now.Add(-time.Hour))

	want := map[string]struct {
		id    uuid.UUID
		owner *uuid.UUID
	}{
		"nhiều primary -> người được gán sớm nhất": {multi, &first},
		"chỉ có trợ giảng -> NULL":                 {onlyAssistant, nil},
		"không có giảng viên -> NULL":              {noTeacher, nil},
		"đã có chủ -> giữ nguyên, không ghi đè":    {owned, &realOwner},
	}
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
		for name, w := range want {
			var c model.Class
			if err := db.First(&c, "id = ?", w.id).Error; err != nil {
				t.Fatalf("đọc lớp: %v", err)
			}
			switch {
			case w.owner == nil && c.CreatedBy != nil:
				t.Errorf("lần %d, %s: muốn NULL, nhận %s", run, name, *c.CreatedBy)
			case w.owner != nil && (c.CreatedBy == nil || *c.CreatedBy != *w.owner):
				t.Errorf("lần %d, %s: muốn %s, nhận %v", run, name, *w.owner, c.CreatedBy)
			}
		}
	}
}