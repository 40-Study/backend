package service

import (
	"context"
	"fmt"
	"strings"
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
	when := occurrence.StartsAt.In(sessionDayZone).Format("15:04 · 02/01")
	title := "Đến giờ vào học"
	content := fmt.Sprintf("%s bắt đầu lúc %s. Hãy vào lớp đúng giờ.", occurrence.Title, when)
	if mins > 0 {
		title = "Lớp học bắt đầu sau 15 phút"
		content = fmt.Sprintf("%s · %s. Chuẩn bị vào lớp nhé.", occurrence.Title, when)
	}
	baseKey := fmt.Sprintf("lesson-reminder:%s:%s:%d", occurrence.ReferenceType, occurrence.ID, occurrence.StartsAt.UnixNano())
	for _, userID := range recipients {
		// QA4-N10: phụ huynh nhận MỘT nhắc nhở cho MỖI con trong lớp, ghi tên con (2 con cùng lớp thì vẫn phân
		// biệt được), mở màn chi tiết của con; không nhận quyền vào phòng.
		children, err := s.parentChildrenInClass(ctx, userID, occurrence.ClassID)
		if err != nil {
			return err
		}
		for _, child := range children {
			childContent := childReminderContent(child.Name, occurrence.Title, when, mins)
			key := fmt.Sprintf("%s:%s:%d:child:%s", baseKey, userID, mins, child.ID)
			if err := s.save(ctx, key, userID, title, childContent, "child_schedule", child.ID, now); err != nil {
				return err
			}
		}
		if len(children) > 0 {
			continue
		}
		referenceType := occurrence.ReferenceType
		recipientContent := content
		var isTeacher int64
		if err := s.db.WithContext(ctx).Model(&model.TeacherClass{}).Where("class_id = ? AND teacher_id = ?", occurrence.ClassID, userID).Count(&isTeacher).Error; err != nil {
			return err
		}
		if isTeacher > 0 {
			// QA5 Q5-T07: giảng viên nhận câu theo giọng người dạy, không phải "vào lớp đúng giờ" của học viên.
			recipientContent = teacherReminderContent(occurrence.Title, when, mins)
		}
		if referenceType != "livestream" || mins > 0 || occurrence.Status != "live" {
			referenceType = "student_schedule"
			if isTeacher > 0 {
				referenceType = "teacher_schedule"
			}
		}
		key := fmt.Sprintf("%s:%s:%d", baseKey, userID, mins)
		if err := s.save(ctx, key, userID, title, recipientContent, referenceType, occurrence.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// teacherReminderContent: câu nhắc dành cho giảng viên của lớp (mins>0: báo trước 15 phút).
// childReminderContent là câu nhắc gửi phụ huynh cho một người con (mins > 0: nhắc trước 15 phút).
func childReminderContent(childName, title, when string, mins int) string {
	subject := childReminderSubject(title)
	if mins > 0 {
		return fmt.Sprintf("%s có %s lúc %s.", childName, subject, when)
	}
	return fmt.Sprintf("%s có %s bắt đầu lúc %s.", childName, subject, when)
}

// childReminderSubject (Q9-PG-08): tiêu đề buổi đã mở đầu bằng "Buổi N: ..." thì giữ nguyên ("có Buổi 21: Pandas"),
// không thêm chữ "buổi" lần nữa; tiêu đề khác ("Python") vẫn đọc thành "buổi Python".
func childReminderSubject(title string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), "buổi") {
		return strings.TrimSpace(title)
	}
	return "buổi " + title
}

func teacherReminderContent(title, when string, mins int) string {
	if mins > 0 {
		return fmt.Sprintf("%s · %s. Buổi dạy sắp bắt đầu, hãy chuẩn bị bài giảng.", title, when)
	}
	return fmt.Sprintf("Buổi dạy %s bắt đầu lúc %s. Hãy vào lớp để bắt đầu buổi học.", title, when)
}

type reminderChild struct {
	ID   uuid.UUID
	Name string
}

// parentChildrenInClass: các con (liên kết active, được xem điểm danh, tài khoản còn hoạt động) của userID
// đang học lớp classID.
func (s *LessonReminderService) parentChildrenInClass(ctx context.Context, userID, classID uuid.UUID) ([]reminderChild, error) {
	var children []reminderChild
	err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT u.id, COALESCE(NULLIF(u.full_name, ''), u.user_name) AS name
   FROM parent_student_relations p
   JOIN student_classes sc ON sc.student_id = p.student_user_id AND sc.status = 'active'
   JOIN users u ON u.id = p.student_user_id
   WHERE p.parent_user_id = ? AND p.status = 'active' AND p.can_view_attendance AND sc.class_id = ?
    AND u.deleted_at IS NULL AND u.is_active
   ORDER BY name`, userID, classID).Scan(&children).Error
	return children, err
}

// save ghi thông báo với id tất định từ key (worker chạy song song / thử lại không tạo bản trùng).
func (s *LessonReminderService) save(ctx context.Context, key string, userID uuid.UUID, title, content, referenceType string, referenceID uuid.UUID, now time.Time) error {
	notice := model.Notification{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)), CreatedAt: now.UTC(), UserID: userID,
		Title: title, Content: content, NotificationType: "lesson_reminder", ReferenceType: &referenceType, ReferenceID: &referenceID}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&notice)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 && s.notifier != nil {
		s.notifier.NotifyNewNotification(userID, notice.ID, title, content, notice.NotificationType, notice.ReferenceType, notice.ReferenceID)
	}
	return nil
}
