package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/socket"
)

// LessonReminderService reads the current schedule, rather than trusting delayed task payloads.
// Rescheduling/cancelling therefore takes effect immediately. Notifications are persisted even
// when recipients are offline; deterministic IDs make concurrent workers and retries idempotent.
type LessonReminderService struct {
	db       *gorm.DB
	notifier *socket.Notifier
}

func NewLessonReminderService(db *gorm.DB, notifier *socket.Notifier) *LessonReminderService {
	return &LessonReminderService{db: db, notifier: notifier}
}

type reminderOccurrence struct {
	ID            uuid.UUID
	ClassID       uuid.UUID
	HostID        *uuid.UUID
	Title         string
	StartsAt      time.Time
	ReferenceType string
	Status        string
}

func (s *LessonReminderService) Sweep(ctx context.Context, now time.Time) error {
	// Look forward 15 minutes and recover a short worker outage. All wall-clock schedules are VN time.
	var occurrences []reminderOccurrence
	err := s.db.WithContext(ctx).Raw(`
 WITH occurrences AS (
  SELECT l.id, l.class_id, l.host_id, l.title, l.scheduled_at AS starts_at, 'livestream' AS reference_type, l.status
  FROM livestream_sessions l JOIN classes c ON c.id = l.class_id
  WHERE l.deleted_at IS NULL AND c.deleted_at IS NULL AND c.status = 'active'
   AND l.status IN ('scheduled','live') AND l.scheduled_at BETWEEN ? AND ?
  UNION ALL
  SELECT cs.id, cs.class_id, NULL::uuid, coalesce(cs.topic, c.name),
   (cs.date + cs.start_time) AT TIME ZONE 'Asia/Ho_Chi_Minh', 'class_session', cs.status
  FROM class_sessions cs JOIN classes c ON c.id = cs.class_id
  WHERE cs.deleted_at IS NULL AND c.deleted_at IS NULL AND c.status = 'active'
   AND cs.status IN ('scheduled','in_progress') AND cs.livestream_session_id IS NULL
   AND (cs.date + cs.start_time) AT TIME ZONE 'Asia/Ho_Chi_Minh' BETWEEN ? AND ?
  UNION ALL
  SELECT sc.id, sc.class_id, NULL::uuid, c.name,
   (d.day::date + sc.start_time) AT TIME ZONE 'Asia/Ho_Chi_Minh', 'class_schedule', 'scheduled'
  FROM class_schedules sc JOIN classes c ON c.id = sc.class_id
  CROSS JOIN generate_series((?::timestamptz AT TIME ZONE 'Asia/Ho_Chi_Minh')::date - 1,
    (?::timestamptz AT TIME ZONE 'Asia/Ho_Chi_Minh')::date + 1, interval '1 day') d(day)
  WHERE sc.deleted_at IS NULL AND c.deleted_at IS NULL AND c.status = 'active' AND sc.is_active
   AND extract(dow FROM d.day) = sc.day_of_week AND d.day::date >= sc.effective_from
   AND (sc.effective_until IS NULL OR d.day::date <= sc.effective_until)
   AND NOT EXISTS (SELECT 1 FROM class_sessions cs WHERE cs.schedule_id = sc.id
    AND cs.deleted_at IS NULL AND cs.date = d.day::date)
   AND (d.day::date + sc.start_time) AT TIME ZONE 'Asia/Ho_Chi_Minh' BETWEEN ? AND ?
 ) SELECT * FROM occurrences`, now.Add(-2*time.Minute), now.Add(15*time.Minute),
		now.Add(-2*time.Minute), now.Add(15*time.Minute), now, now,
		now.Add(-2*time.Minute), now.Add(15*time.Minute)).Scan(&occurrences).Error
	if err != nil {
		return fmt.Errorf("read lesson reminders: %w", err)
	}
	for _, occurrence := range occurrences {
		phase := 0
		if now.Before(occurrence.StartsAt) {
			// Only emit the early notice near its threshold, never several late notices together.
			if now.Sub(occurrence.StartsAt.Add(-15*time.Minute)) > 2*time.Minute {
				continue
			}
			phase = 15
		}
		if err := s.deliver(ctx, occurrence, phase, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *LessonReminderService) deliver(ctx context.Context, occurrence reminderOccurrence, mins int, now time.Time) error {
	var recipients []uuid.UUID
	// Class membership is authoritative. Never notify other classes enrolled in the same course.
	err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT u.id FROM users u JOIN (
  SELECT teacher_id AS user_id FROM teacher_classes WHERE class_id = @class
  UNION SELECT student_id FROM student_classes WHERE class_id = @class AND status = 'active'
  UNION SELECT p.parent_user_id FROM parent_student_relations p
   JOIN student_classes sc ON sc.student_id = p.student_user_id AND sc.status = 'active'
   WHERE sc.class_id = @class AND p.status = 'active' AND p.can_view_attendance
  UNION SELECT CAST(@host AS uuid)
 ) r ON r.user_id = u.id WHERE u.deleted_at IS NULL AND u.is_active`,
		map[string]interface{}{"class": occurrence.ClassID, "host": occurrence.HostID}).Scan(&recipients).Error
	if err != nil {
		return fmt.Errorf("read reminder recipients: %w", err)
	}
	loc := time.FixedZone("ICT", 7*60*60)
	title := "Đến giờ vào học"
	content := fmt.Sprintf("%s bắt đầu lúc %s. Hãy vào lớp đúng giờ.", occurrence.Title, occurrence.StartsAt.In(loc).Format("15:04 · 02/01"))
	if mins > 0 {
		title = "Lớp học bắt đầu sau 15 phút"
		content = fmt.Sprintf("%s · %s. Chuẩn bị vào lớp nhé.", occurrence.Title, occurrence.StartsAt.In(loc).Format("15:04 · 02/01"))
	}
	for _, userID := range recipients {
		referenceType := occurrence.ReferenceType
		// Parents get their child's detail screen; they do not receive a room join permission.
		var child struct{ StudentUserID uuid.UUID }
		if err := s.db.WithContext(ctx).Raw(`SELECT p.student_user_id FROM parent_student_relations p
   JOIN student_classes sc ON sc.student_id = p.student_user_id AND sc.status = 'active'
   WHERE p.parent_user_id = ? AND p.status = 'active' AND p.can_view_attendance AND sc.class_id = ? LIMIT 1`, userID, occurrence.ClassID).Scan(&child).Error; err != nil {
			return err
		}
		referenceID := occurrence.ID
		if child.StudentUserID != uuid.Nil {
			referenceType = "child_schedule"
			referenceID = child.StudentUserID
		}
		if referenceType != "child_schedule" && (referenceType != "livestream" || mins > 0 || occurrence.Status != "live") {
			referenceType = "student_schedule"
			var isTeacher int64
			if err := s.db.WithContext(ctx).Model(&model.TeacherClass{}).Where("class_id = ? AND teacher_id = ?", occurrence.ClassID, userID).Count(&isTeacher).Error; err != nil {
				return err
			}
			if isTeacher > 0 {
				referenceType = "teacher_schedule"
			}
		}
		key := fmt.Sprintf("lesson-reminder:%s:%s:%d:%s:%d", occurrence.ReferenceType, occurrence.ID, occurrence.StartsAt.UnixNano(), userID, mins)
		notice := model.Notification{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)), CreatedAt: now.UTC(), UserID: userID,
			Title: title, Content: content, NotificationType: "lesson_reminder", ReferenceType: &referenceType, ReferenceID: &referenceID}
		result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&notice)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 && s.notifier != nil {
			s.notifier.NotifyNewNotification(userID, notice.ID, title, content, notice.NotificationType, notice.ReferenceType, notice.ReferenceID)
		}
	}
	return nil
}
