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
	TaskClassSessionReminder     string = "class_session_reminder"
	TaskEventReminder            string = "task:event_reminder"
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

type EventReminderPayload struct {
	EventID uuid.UUID `json:"event_id"`
	UserID  uuid.UUID `json:"user_id"`
	Title   string    `json:"title"`
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

		// In-app reminders are reconciled from the current DB schedule by lesson_reminder_sweep.
		// This legacy task only schedules automatic room opening.

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

	// ===================== Event Reminder =====================
	q.Handle(TaskEventReminder, func(ctx context.Context, payload []byte) error {
		var p EventReminderPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("unmarshal event reminder: %w", err)
		}

		noti := socket.NotificationPayload{
			ID:               uuid.New(),
			Title:            "Nhắc nhở sự kiện",
			Content:          fmt.Sprintf("Sự kiện \"%s\" sắp bắt đầu", p.Title),
			NotificationType: "event_reminder",
			CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		}

		log.Printf("[task] Event reminder sent: event=%s user=%s title=%s", p.EventID, p.UserID, p.Title)
		notifier.SendNotification(p.UserID, noti)
		return nil
	})

	// Drain legacy delayed tasks without emitting a second, stale socket-only reminder.
	// The persistent sweep covers livestreams, concrete class sessions and weekly timetables.
	q.Handle(TaskLessonReminder, func(ctx context.Context, payload []byte) error { return nil })

}
