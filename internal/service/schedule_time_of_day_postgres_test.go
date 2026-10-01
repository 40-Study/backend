package service

// Giờ lịch học qua service trên Postgres thật (schema tạm, migrate như API khởi động).
// Trước bản sửa cột là TIMESTAMPTZ: tạo lịch/buổi với "14:00" lỗi SQLSTATE 22007 và đọc ra
// "2026-...T14:00:00+07:00" — test này ĐỎ.

import (
	"context"
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestSchedule_GioHHMM_TaoVaDocLai(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)

	sch, err := svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
		DayOfWeek: 2, StartTime: "14:00", EndTime: "15:30:00", EffectiveFrom: "2030-01-01"})
	if err != nil {
		t.Fatalf("CreateSchedule 14:00: %v", err)
	}
	if sch.StartTime != "14:00" || sch.EndTime != "15:30" {
		t.Errorf("CreateSchedule trả %q-%q, muốn 14:00-15:30", sch.StartTime, sch.EndTime)
	}
	var raw string
	e.f.db.Raw("SELECT start_time::text FROM class_schedules WHERE id = ?", sch.ID).Scan(&raw)
	if raw != "14:00:00" {
		t.Errorf("DB lưu %q, muốn 14:00:00", raw)
	}

	one, err := svc.GetScheduleByID(ctx, sch.ID, e.owner.ID, false)
	if err != nil || one.StartTime != "14:00" || one.EndTime != "15:30" {
		t.Errorf("GetScheduleByID: %+v %v, muốn 14:00-15:30", one, err)
	}
	tt, err := svc.GetClassTimetable(ctx, e.class.ID, e.student.ID, false)
	if err != nil || len(tt.Entries) != 1 || tt.Entries[0].StartTime != "14:00" || tt.Entries[0].EndTime != "15:30" {
		t.Errorf("GetClassTimetable: %+v %v, muốn 1 mục 14:00-15:30", tt, err)
	}
	mine, err := svc.GetMyTimetable(ctx, e.student.ID, "STUDENT")
	if err != nil || len(mine.Entries) != 1 || mine.Entries[0].StartTime != "14:00" {
		t.Errorf("GetMyTimetable: %+v %v, muốn 1 mục 14:00", mine, err)
	}

	start := "07:05"
	upd, err := svc.UpdateSchedule(ctx, sch.ID, e.owner.ID, false, dto.UpdateClassScheduleDTO{StartTime: &start})
	if err != nil || upd.StartTime != "07:05" {
		t.Errorf("UpdateSchedule: %+v %v, muốn 07:05", upd, err)
	}

	// Sinh buổi từ lịch lặp: 2030-01-01 là thứ Ba (day_of_week 2), giờ chép từ lịch.
	if _, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2030-01-01", EndDate: "2030-01-07"}); err != nil {
		t.Fatalf("GenerateSessions: %v", err)
	}
	var gen model.ClassSession
	if err := e.f.db.Where("class_id = ? AND schedule_id = ?", e.class.ID, sch.ID).First(&gen).Error; err != nil ||
		gen.StartTime != "07:05" || gen.EndTime != "15:30" {
		t.Errorf("buổi sinh từ lịch: %q-%q (%v), muốn 07:05-15:30", gen.StartTime, gen.EndTime, err)
	}

	ses, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2030-02-01", StartTime: "14:00", EndTime: "16:00"})
	if err != nil {
		t.Fatalf("CreateSession 14:00: %v", err)
	}
	got, err := svc.GetSessionByID(ctx, ses.ID, e.owner.ID, false)
	if err != nil || got.StartTime != "14:00" || got.EndTime != "16:00" {
		t.Errorf("GetSessionByID: %+v %v, muốn 14:00-16:00", got, err)
	}
}

func TestSchedule_GioSaiDinhDangBiTuChoi(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)

	for _, bad := range []string{"2026-01-01T08:00:00Z", "25:00", ""} {
		if _, err := svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
			DayOfWeek: 1, StartTime: bad, EndTime: "09:00", EffectiveFrom: "2030-01-01"}); err == nil {
			t.Errorf("CreateSchedule start_time=%q phải lỗi", bad)
		}
		if _, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{
			Date: "2030-01-01", StartTime: "08:00", EndTime: bad}); err == nil {
			t.Errorf("CreateSession end_time=%q phải lỗi", bad)
		}
	}
	var n int64
	e.f.db.Model(&model.ClassSchedule{}).Where("class_id = ?", e.class.ID).Count(&n)
	var m int64
	e.f.db.Model(&model.ClassSession{}).Where("class_id = ?", e.class.ID).Count(&m)
	if n != 0 || m != 0 {
		t.Errorf("giờ sai vẫn ghi %d lịch, %d buổi", n, m)
	}
}
