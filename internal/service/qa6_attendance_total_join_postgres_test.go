package service

// QA6 review L5: tổng (`total`) và các dòng của trang lịch sử điểm danh dùng CÙNG phép JOIN class_sessions. Trước đây total đếm
// trước JOIN nên một dòng điểm danh mất buổi học (session_id không còn) làm total=2 mà trang chỉ có 1 dòng.
// Mutation đã kiểm: đưa Joins về sau Count làm test ĐỎ (total=2, len=1).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func TestQA6_MyAttendances_TotalKhopTrang_DongMatBuoiHoc_Postgres(t *testing.T) {
	e := newS4ClassEnv(t)
	svc := newS5ScheduleSvc(e)
	ctx := context.Background()

	sess := model.ClassSession{ClassID: e.class.ID, SessionNumber: 1, Date: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), StartTime: "08:00", EndTime: "09:00", Status: model.SessionCompleted}
	if err := e.f.db.Create(&sess).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.f.db.Create(&model.SessionAttendance{SessionID: sess.ID, StudentID: e.student.ID, Status: model.AttendancePresent}).Error; err != nil {
		t.Fatal(err)
	}

	// FK ON DELETE CASCADE ngăn dòng mồ côi: gỡ ràng buộc trong schema tạm của test để dựng đúng ca "buổi học không còn".
	var fk string
	if err := e.f.db.Raw(`SELECT conname FROM pg_constraint WHERE contype = 'f' AND conrelid = 'session_attendances'::regclass AND confrelid = 'class_sessions'::regclass`).Scan(&fk).Error; err != nil || fk == "" {
		t.Fatalf("không tìm thấy FK session_attendances -> class_sessions: %q err=%v", fk, err)
	}
	if err := e.f.db.Exec(`ALTER TABLE session_attendances DROP CONSTRAINT "` + fk + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.f.db.Create(&model.SessionAttendance{SessionID: uuid.New(), StudentID: e.student.ID, Status: model.AttendanceAbsent}).Error; err != nil {
		t.Fatal(err)
	}

	rows, total, err := svc.GetMyAttendances(ctx, e.student.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if int(total) != len(rows) || len(rows) != 1 {
		t.Errorf("total=%d len=%d, muốn total == số dòng của trang == 1 (dòng mất buổi học không được đếm)", total, len(rows))
	}
}
