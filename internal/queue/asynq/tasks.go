package asynq_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/socket"
)

// ===================== Task Types =====================

const (
	TaskLessonReminder           string = "lesson_reminder"
	TaskScheduleLivestreamRemind string = "schedule_livestream_remind"
	TaskAutoStartLivestream      string = "auto_start_livestream"
	TaskAssignmentDeadline       string = "assignment_deadline"
	TaskPaymentPending           string = "payment_pending"
	TaskDailyCheckin             string = "daily_checkin"
	TaskStreakWarning            string = "streak_warning"
)

// ===================== Payloads =====================

type ClassReminderPayload struct {
	SessionID  uuid.UUID  `json:"session_id"`
	ClassID    uuid.UUID  `json:"class_id"`
	CourseID   *uuid.UUID `json:"course_id,omitempty"`
	ClassName  string     `json:"class_name"`
	Room       string     `json:"room,omitempty"`
	StartTime  string     `json:"start_time"` // HH:MM
	MinsBefore int        `json:"mins_before"`
}

type AssignmentDeadlinePayload struct {
	AssignmentID uuid.UUID   `json:"assignment_id"`
	Title        string      `json:"title"`
	UserIDs      []uuid.UUID `json:"user_ids"`
	MinsBefore   int         `json:"mins_before"`
}

type PaymentPendingPayload struct {
	OrderID     uuid.UUID `json:"order_id"`
	UserID      uuid.UUID `json:"user_id"`
	OrderNumber string    `json:"order_number"`
	TotalAmount string    `json:"total_amount"`
}

type ScheduleLivestreamRemindPayload struct {
	SessionID   uuid.UUID  `json:"session_id"`
	ClassID     *uuid.UUID `json:"class_id,omitempty"`
	CourseID    *uuid.UUID `json:"course_id,omitempty"`
	Title       string     `json:"title"`
	ScheduledAt time.Time  `json:"scheduled_at"`
}

type AutoStartLivestreamPayload struct {
	SessionID uuid.UUID `json:"session_id"`
}

type LivestreamStarter func(ctx context.Context, sessionID uuid.UUID) error

// ===================== Register =====================

func RegisterTasks(q *Queue, notifier *socket.Notifier, classRepo repository.ClassRepositoryInterface, enrollmentRepo repository.EnrollmentRepositoryInterface, redisClient *redis.Client, livestreamStarter LivestreamStarter) {

	q.Handle(TaskScheduleLivestreamRemind, func(ctx context.Context, payload []byte) error {
		var p ScheduleLivestreamRemindPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("unmarshal schedule livestream remind: %w", err)
		}

		var reminderName string
		if p.ClassID != nil {
			class, err := classRepo.GetByID(ctx, *p.ClassID)
			if err != nil || class == nil {
				log.Printf("[task] Schedule livestream remind: class not found %s", p.ClassID)
				return nil
			}
			reminderName = class.Name
		} else if p.CourseID != nil {
			reminderName = p.Title
		} else {
			log.Printf("[task] Schedule livestream remind: no class_id or course_id")
			return nil
		}

		reminderPayload := ClassReminderPayload{
			SessionID: p.SessionID,
			CourseID:  p.CourseID,
			ClassName: reminderName,
			Room:      p.Title,
			StartTime: p.ScheduledAt.Format("15:04"),
		}
		if p.ClassID != nil {
			reminderPayload.ClassID = *p.ClassID
		}

		reminders := []struct {
			mins int
			at   time.Time
		}{
			{30, p.ScheduledAt.Add(-30 * time.Minute)},
			{15, p.ScheduledAt.Add(-15 * time.Minute)},
			{5, p.ScheduledAt.Add(-5 * time.Minute)},
			{1, p.ScheduledAt.Add(-1 * time.Minute)},
		}

		log.Printf("[task] Processing schedule_livestream_remind: class=%s scheduledAt=%v now=%v", reminderName, p.ScheduledAt, time.Now())

		for _, r := range reminders {
			reminderPayload.MinsBefore = r.mins
			if r.at.Before(time.Now()) {
				log.Printf("[task]   SKIP reminder -%d min (at=%v) — already past", r.mins, r.at)
			} else {
				log.Printf("[task]   OK reminder -%d min (at=%v)", r.mins, r.at)
			}
			q.ScheduleAt(TaskLessonReminder, reminderPayload, r.at, "notifications")
		}

		autoStartPayload := AutoStartLivestreamPayload{SessionID: p.SessionID}
		autoStartAt := p.ScheduledAt.Add(-5 * time.Minute)
		if autoStartAt.Before(time.Now()) {
			log.Printf("[task]   SKIP auto-start (at=%v) — already past", autoStartAt)
		} else {
			log.Printf("[task]   OK auto-start (at=%v)", autoStartAt)
		}
		q.ScheduleAt(TaskAutoStartLivestream, autoStartPayload, autoStartAt, "notifications")

		log.Printf("[task] Done scheduling for %s", reminderName)
		return nil
	})

	q.Handle(TaskAutoStartLivestream, func(ctx context.Context, payload []byte) error {
		var p AutoStartLivestreamPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("unmarshal auto start livestream: %w", err)
		}

		log.Printf("[task] Auto-start livestream triggered for session %s", p.SessionID)

		err := livestreamStarter(ctx, p.SessionID)
		if err != nil {
			log.Printf("[task] Auto-start livestream FAILED session=%s err=%v", p.SessionID, err)
		} else {
			log.Printf("[task] Auto-start livestream SUCCESS session=%s — room is now LIVE", p.SessionID)
		}
		return nil
	})

	q.Handle(TaskLessonReminder, func(ctx context.Context, payload []byte) error {
		var p ClassReminderPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("unmarshal class reminder: %w", err)
		}

		var userIDs []uuid.UUID

		if p.CourseID != nil {
			// Course-based: lấy enrolled students
			enrolledIDs, err := enrollmentRepo.GetEnrolledUserIDsByCourseID(ctx, *p.CourseID)
			if err != nil {
				log.Printf("[task] Course reminder: failed to get enrolled users: %v", err)
				return nil
			}
			userIDs = enrolledIDs
		} else {
			// Class-based: flow cũ
			studentCount, _ := classRepo.GetStudentCount(ctx, p.ClassID)
			if studentCount == 0 {
				return nil
			}
			students, _, _ := classRepo.GetStudents(ctx, p.ClassID, 1, int(studentCount))
			for _, sc := range students {
				userIDs = append(userIDs, sc.StudentID)
			}
		}

		// Filter out already-joined users
		redisKey := fmt.Sprintf("livestream:%s:joined", p.SessionID.String())
		joinedUsers, _ := redisClient.SMembers(ctx, redisKey).Result()
		joinedSet := make(map[string]bool)
		for _, uid := range joinedUsers {
			joinedSet[uid] = true
		}

		var filteredIDs []uuid.UUID
		for _, uid := range userIDs {
			if !joinedSet[uid.String()] {
				filteredIDs = append(filteredIDs, uid)
			}
		}

		if len(filteredIDs) == 0 {
			return nil
		}

		room := p.Room
		if room == "" {
			room = "Online"
		}

		noti := socket.NotificationPayload{
			ID:               uuid.New(),
			Title:            fmt.Sprintf("Livestream sắp bắt đầu: %s", p.ClassName),
			Content:          fmt.Sprintf("%s sẽ bắt đầu sau %d phút tại %s (lúc %s)", p.ClassName, p.MinsBefore, room, p.StartTime),
			NotificationType: "livestream_reminder",
			CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		}

		log.Printf("[task] Reminder sent: class=%s, -%d min, start=%s, notify=%d users", p.ClassName, p.MinsBefore, p.StartTime, len(filteredIDs))
		notifier.SendNotificationToMany(filteredIDs, noti)
		return nil
	})

}
