package service

// Fixture chung cho các test quyền truy cập của lane S2 (test case ẩn, bài làm quiz, bài nộp).
// Chạy trên Postgres THẬT trong schema tạm (pgtest.IsolatedSchema, DROP khi xong). Không có
// Postgres: Skip ở local, FAIL khi CI=true.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

type s2Fixture struct {
	t  *testing.T
	db *gorm.DB
}

func newS2Fixture(t *testing.T) *s2Fixture {
	t.Helper()
	return &s2Fixture{t: t, db: pgtest.IsolatedSchema(t, migrateLikeAPIBoot)}
}

func (f *s2Fixture) user(kind string) model.User {
	f.t.Helper()
	s := uuid.NewString()[:8]
	u := model.User{Email: "qa-s2-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-s2-" + kind + "-" + s}
	if err := f.db.Create(&u).Error; err != nil {
		f.t.Fatalf("tạo user %s: %v", kind, err)
	}
	return u
}

func (f *s2Fixture) course(instructor model.User) model.Course {
	f.t.Helper()
	s := uuid.NewString()[:8]
	c := model.Course{InstructorID: instructor.ID, Title: "QA-s2 " + s, Slug: "qa-s2-" + s, Price: decimal.Zero, Status: "published"}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo course: %v", err)
	}
	return c
}

// lessonOf tạo section + lesson thuộc course, trả về lesson.
func (f *s2Fixture) lessonOf(c model.Course) model.Lesson {
	f.t.Helper()
	sec := model.Section{CourseID: c.ID, Title: "s", DisplayOrder: 1}
	if err := f.db.Create(&sec).Error; err != nil {
		f.t.Fatalf("tạo section: %v", err)
	}
	l := model.Lesson{SectionID: sec.ID, Title: "l", DisplayOrder: 1}
	if err := f.db.Create(&l).Error; err != nil {
		f.t.Fatalf("tạo lesson: %v", err)
	}
	return l
}
