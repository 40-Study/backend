package service

// QA4-N12: GET /parent/children/:id/schedule trả upcoming_sessions[].class_name và room luôn rỗng (không
// Preload Class/Schedule), và /attendance trả check_in_time theo UTC ("12:00" cho buổi 19:00 giờ Việt Nam).
// Postgres THẬT. Mutation đã kiểm: bỏ Preload("Class").Preload("Schedule") hoặc bỏ .In(sessionDayZone)
// làm test ĐỎ.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestParentChildSchedule_ClassRoomAndVNCheckIn_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	parent, student := f.user("n12-parent"), f.user("n12-student")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	class := model.Class{Name: "Toán 9A", Status: "active"}
	must(f.db.Create(&class).Error)
	must(f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}).Error)
	must(f.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: student.ID, Status: "active", CanViewAttendance: true}).Error)

	scheduleID, upcomingID, pastID := uuid.New(), uuid.New(), uuid.New()
	must(f.db.Exec(`INSERT INTO class_schedules (id,class_id,day_of_week,start_time,end_time,room,is_active,effective_from)
  VALUES (?, ?, 1, '19:00','20:30','P.204',true, CURRENT_DATE - 30)`, scheduleID, class.ID).Error)
	must(f.db.Exec(`INSERT INTO class_sessions (id,class_id,schedule_id,session_number,date,start_time,end_time,status) VALUES
  (?, ?, ?, 2, CURRENT_DATE + 7, '19:00','20:30','scheduled'),
  (?, ?, ?, 1, CURRENT_DATE - 7, '19:00','20:30','completed')`, upcomingID, class.ID, scheduleID, pastID, class.ID, scheduleID).Error)
	// 12:00 UTC = 19:00 giờ Việt Nam.
	checkIn := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	must(f.db.Create(&model.SessionAttendance{SessionID: pastID, StudentID: student.ID, Status: model.AttendancePresent, CheckInTime: &checkIn}).Error)

	svc := &ParentDashboardService{parentStudentRepo: repository.NewParentStudentRepository(f.db), scheduleRepo: repository.NewScheduleRepository(f.db)}
	schedule, err := svc.GetChildSchedule(ctx, parent.ID, student.ID)
	must(err)
	if len(schedule.UpcomingSessions) != 1 {
		t.Fatalf("upcoming = %+v, muốn đúng 1 buổi", schedule.UpcomingSessions)
	}
	if up := schedule.UpcomingSessions[0]; up.ClassName != "Toán 9A" || up.Room != "P.204" {
		t.Fatalf("upcoming class_name=%q room=%q, muốn Toán 9A / P.204", up.ClassName, up.Room)
	}

	att, err := svc.GetChildAttendance(ctx, parent.ID, student.ID, 1, 20)
	must(err)
	if len(att.Records) != 1 || att.Records[0].CheckInTime == nil || *att.Records[0].CheckInTime != "19:00" {
		t.Fatalf("attendance = %+v, muốn check_in_time 19:00 (giờ Việt Nam)", att.Records)
	}
}
