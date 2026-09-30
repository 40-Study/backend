package seeds

import (
	"fmt"
	"time"

	"study.com/v1/internal/model"
)

// demoSessionLookaheadDays: số ngày tới vẫn sinh buổi học, đủ phủ tuần này và tuần sau trên trang lịch.
const demoSessionLookaheadDays = 14

// seedDemoClassSessions sinh buổi học cụ thể cho mọi ngày khớp lịch tuần, từ ngày khai giảng tới
// hôm nay + 14 ngày. Buổi trước hôm nay: completed + điểm danh (trang my-attendance, điểm danh của
// giáo viên, phụ huynh). Từ hôm nay: scheduled (GetUpcomingSessions của phụ huynh lọc
// status='scheduled' AND date >= CURRENT_DATE). Khoá tự nhiên: lớp + ngày.
func (s *Seeder) seedDemoClassSessions(spec classSpec, class model.Class, teacher model.User, students []model.User, schedules map[int]model.ClassSchedule) error {
	today := demoToday()
	start := dateOrToday(class.StartDate)
	last := today.AddDate(0, 0, demoSessionLookaheadDays)

	number := 0
	for d := start; !d.After(last); d = d.AddDate(0, 0, 1) {
		schedule, ok := schedules[int(d.Weekday())]
		if !ok {
			continue
		}
		number++ // số buổi tính từ ngày khai giảng nên ổn định giữa các lần chạy
		isPast := d.Before(today)

		session, err := s.upsertDemoSession(spec, class, schedule, d, number, isPast)
		if err != nil {
			return err
		}
		if !isPast {
			continue
		}
		for i, student := range students {
			if err := s.upsertDemoAttendance(spec, session, d, teacher, student, number, i); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Seeder) upsertDemoSession(spec classSpec, class model.Class, schedule model.ClassSchedule, day time.Time, number int, isPast bool) (model.ClassSession, error) {
	status := model.SessionScheduled
	if isPast {
		status = model.SessionCompleted
	}
	topic := spec.Topics[(number-1)%len(spec.Topics)]
	startAt, err := clockStamp(day, spec.StartTime)
	if err != nil {
		return model.ClassSession{}, err
	}
	endAt, err := clockStamp(day, spec.EndTime)
	if err != nil {
		return model.ClassSession{}, err
	}
	session := model.ClassSession{
		ClassID:       class.ID,
		ScheduleID:    &schedule.ID,
		SessionNumber: number,
		Date:          day,
		StartTime:     startAt,
		EndTime:       endAt,
		Status:        status,
		Topic:         ptr(fmt.Sprintf("Buổi %d: %s", number, topic)),
	}
	if isPast {
		session.Notes = ptr("Đã hoàn thành nội dung buổi học, bài tập về nhà xem trên hệ thống.")
	}

	dateKey := day.Format("2006-01-02")
	if err := s.db.Where("class_id = ? AND date = ?", class.ID, dateKey).
		Attrs(session).FirstOrCreate(&session).Error; err != nil {
		return model.ClassSession{}, fmt.Errorf("failed to seed class session %s: %w", dateKey, err)
	}

	// Buổi tạo ở lần chạy trước (khi còn là tương lai) nay đã qua: chuyển sang completed để điểm
	// danh bên dưới khớp trạng thái buổi.
	if isPast && session.Status == model.SessionScheduled {
		if err := s.db.Model(&model.ClassSession{}).Where("id = ?", session.ID).
			Update("status", model.SessionCompleted).Error; err != nil {
			return model.ClassSession{}, fmt.Errorf("failed to complete past session %s: %w", dateKey, err)
		}
		session.Status = model.SessionCompleted
	}
	return session, nil
}

// upsertDemoAttendance ghi điểm danh một học sinh cho một buổi đã qua (khoá: buổi + học sinh).
func (s *Seeder) upsertDemoAttendance(spec classSpec, session model.ClassSession, day time.Time, teacher, student model.User, number, studentIdx int) error {
	status, lateMinutes, note := demoAttendancePattern(number, studentIdx)

	expected, err := clockStamp(day, spec.StartTime)
	if err != nil {
		return err
	}
	att := model.SessionAttendance{
		SessionID:    session.ID,
		StudentID:    student.ID,
		Status:       status,
		ExpectedTime: ptr(expected),
		LateMinutes:  lateMinutes,
		Location:     ptr(spec.Location),
		VerifiedBy:   &teacher.ID,
	}
	if note != "" {
		att.Note = ptr(note)
	}
	startAt, err := atClock(day, spec.StartTime)
	if err != nil {
		return err
	}
	endAt, err := atClock(day, spec.EndTime)
	if err != nil {
		return err
	}
	if status == model.AttendancePresent || status == model.AttendanceLate {
		checkIn := startAt.Add(time.Duration(lateMinutes) * time.Minute)
		att.CheckInTime = &checkIn
		att.CheckOutTime = &endAt
	}
	// created_at = lúc tan buổi học: /me/attendances sắp xếp theo created_at DESC.
	att.CreatedAt = endAt

	if err := s.db.Where("session_id = ? AND student_id = ?", session.ID, student.ID).
		Attrs(att).FirstOrCreate(&att).Error; err != nil {
		return fmt.Errorf("failed to seed attendance: %w", err)
	}
	return nil
}

// demoAttendancePattern cho kết quả điểm danh cố định theo (số buổi, học sinh): phần lớn có mặt,
// thỉnh thoảng đi muộn, vắng có phép và vắng không phép — đủ để thống kê trên trang có ý nghĩa.
func demoAttendancePattern(number, studentIdx int) (model.AttendanceStatus, int, string) {
	switch (number*3 + studentIdx*5) % 10 {
	case 0:
		return model.AttendanceAbsent, 0, "Vắng không báo trước"
	case 4:
		return model.AttendanceLate, 12, "Kẹt xe, vào lớp muộn"
	case 7:
		return model.AttendanceExcused, 0, "Xin nghỉ ốm, phụ huynh đã báo giáo viên"
	case 9:
		return model.AttendanceLate, 5, ""
	default:
		return model.AttendancePresent, 0, ""
	}
}

// atClock ghép ngày với giờ dạng "HH:MM:SS" (giờ máy chủ). Giờ sai định dạng là lỗi dữ liệu demo:
// trả lỗi thay vì âm thầm dùng 00:00.
func atClock(day time.Time, clock string) (time.Time, error) {
	t, err := time.Parse("15:04:05", clock)
	if err != nil {
		return time.Time{}, fmt.Errorf("giờ demo %q sai định dạng HH:MM:SS: %w", clock, err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, time.Local), nil
}

// clockStamp trả giờ học dưới dạng timestamp RFC3339 (ngày + giờ, múi giờ máy chủ).
//
// Vì sao không ghi "19:00:00": model khai `gorm:"type:time"` nhưng GORM coi "time" là kiểu logic
// schema.Time nên AutoMigrate tạo cột start_time/end_time/expected_time là TIMESTAMPTZ (đã kiểm
// information_schema trên DB dev) — chuỗi giờ trơn bị Postgres từ chối (SQLSTATE 22007). Test có sẵn
// (s5_schedule_authz_postgres_test.go) cũng ghi timestamp đầy đủ.
func clockStamp(day time.Time, clock string) (string, error) {
	t, err := atClock(day, clock)
	if err != nil {
		return "", err
	}
	return t.Format(time.RFC3339), nil
}
