package service

// QA6 round 3 (M1): huỷ buổi qua PUT /sessions/:id đi cùng luật R4 như CancelSession: buổi đã tới giờ bắt đầu (ICT) bị
// từ chối ErrSessionAlreadyStarted và không đổi gì; buổi còn trước giờ bắt đầu huỷ được VÀ có cancelled_at.
// Mutation đã kiểm: bỏ requireSessionNotStarted trong UpdateSession làm ca "đã bắt đầu" ĐỎ; bỏ gán CancelledAt làm ca "ngày mai" ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestQA6_UpdateSession_HuyQuaPUTDiQuaLuatR4_Postgres(t *testing.T) {
	e := newS4ClassEnv(t)
	svc := newS5ScheduleSvc(e)
	ctx := context.Background()
	today := time.Now().In(sessionDayZone)
	mk := func(n int, day time.Time, start, end string) model.ClassSession {
		s := model.ClassSession{ClassID: e.class.ID, SessionNumber: n, Date: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
			StartTime: model.TimeOfDay(start), EndTime: model.TimeOfDay(end), Status: model.SessionScheduled}
		if err := e.f.db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	yesterday := mk(1, today.AddDate(0, 0, -1), "08:00", "09:00")
	tomorrow := mk(2, today.AddDate(0, 0, 1), "08:00", "09:00")
	cancelled := "cancelled"
	nextWeek := today.AddDate(0, 0, 7).Format("2006-01-02")

	for name, req := range map[string]dto.UpdateClassSessionDTO{
		"chỉ huỷ": {Status: &cancelled},
		"vừa dời sang tương lai vừa huỷ": {Status: &cancelled, Date: &nextWeek},
	} {
		if _, err := svc.UpdateSession(ctx, yesterday.ID, e.coTeacher.ID, false, req); !errors.Is(err, ErrSessionAlreadyStarted) {
			t.Errorf("%s: err=%v, muốn ErrSessionAlreadyStarted", name, err)
		}
		var after model.ClassSession
		if err := e.f.db.First(&after, "id = ?", yesterday.ID).Error; err != nil {
			t.Fatal(err)
		}
		if after.Status != model.SessionScheduled || after.CancelledAt != nil || !after.Date.Equal(yesterday.Date) {
			t.Errorf("%s: buổi bị từ chối không được đổi gì: status=%s cancelled_at=%v date=%v", name, after.Status, after.CancelledAt, after.Date)
		}
	}

	if _, err := svc.UpdateSession(ctx, tomorrow.ID, e.coTeacher.ID, false, dto.UpdateClassSessionDTO{Status: &cancelled}); err != nil {
		t.Fatalf("buổi ngày mai phải huỷ được qua PUT: %v", err)
	}
	var after model.ClassSession
	if err := e.f.db.First(&after, "id = ?", tomorrow.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != model.SessionCancelled || after.CancelledAt == nil {
		t.Errorf("buổi ngày mai: status=%s cancelled_at=%v, muốn cancelled và có cancelled_at", after.Status, after.CancelledAt)
	}
	// Sửa ghi chú trên buổi cũ (không huỷ) không bị luật R4 chặn.
	topic := "Ôn tập"
	if _, err := svc.UpdateSession(ctx, yesterday.ID, e.coTeacher.ID, false, dto.UpdateClassSessionDTO{Topic: &topic}); err != nil {
		t.Errorf("sửa chủ đề buổi cũ bị chặn nhầm: %v", err)
	}
}
