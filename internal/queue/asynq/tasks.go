package asynq_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/socket"
)

// ===================== Task Types =====================

const (
	TaskClassReminder    = "class_reminder"
	TaskAssignmentDeadline = "assignment_deadline"
	TaskDailyCheckin     = "daily_checkin"
	TaskStreakWarning     = "streak_warning"
	TaskPaymentPending   = "payment_pending"
)

// ===================== Payloads =====================

type ClassReminderPayload struct {
	ClassID   uuid.UUID   `json:"class_id"`
	ClassName string      `json:"class_name"`
	Room      string      `json:"room,omitempty"`
	StartTime string      `json:"start_time"` // HH:MM
	UserIDs   []uuid.UUID `json:"user_ids"`
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

// ===================== Register =====================

func RegisterTasks(q *Queue, notifier *socket.Notifier) {



	q.Handle(TaskClassReminder, func(ctx context.Context, payload []byte) error {
		var p ClassReminderPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("unmarshal class reminder: %w", err)
		}

		room := p.Room
		if room == "" {
			room = "Online"
		}

		noti := socket.NotificationPayload{
			ID:               uuid.New(),
			Title:            fmt.Sprintf("Lịch học sắp bắt đầu: %s", p.ClassName),
			Content:          fmt.Sprintf("Lớp %s sẽ bắt đầu sau %d phút tại %s (lúc %s)", p.ClassName, p.MinsBefore, room, p.StartTime),
			NotificationType: "class_reminder",
			CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		}

		log.Printf("[task] Class reminder: class=%s, %d min before, users=%d", p.ClassName, p.MinsBefore, len(p.UserIDs))
		notifier.SendNotificationToMany(p.UserIDs, noti)
		return nil
	})

	// q.Handle(TaskAssignmentDeadline, func(ctx context.Context, payload []byte) error {
	// 	var p AssignmentDeadlinePayload
	// 	if err := json.Unmarshal(payload, &p); err != nil {
	// 		return fmt.Errorf("unmarshal assignment deadline: %w", err)
	// 	}

	// 	refType := "assignment"
	// 	noti := socket.NotificationPayload{
	// 		ID:               uuid.New(),
	// 		Title:            fmt.Sprintf("Deadline sắp tới: %s", p.Title),
	// 		Content:          fmt.Sprintf("Bài tập \"%s\" sẽ hết hạn sau %d phút", p.Title, p.MinsBefore),
	// 		NotificationType: "assignment_deadline",
	// 		ReferenceType:    &refType,
	// 		ReferenceID:      &p.AssignmentID,
	// 		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	// 	}

	// 	log.Printf("[task] Assignment deadline: %s, %d min before, users=%d", p.Title, p.MinsBefore, len(p.UserIDs))
	// 	notifier.SendNotificationToMany(p.UserIDs, noti)
	// 	return nil
	// })

	// q.Handle(TaskPaymentPending, func(ctx context.Context, payload []byte) error {
	// 	var p PaymentPendingPayload
	// 	if err := json.Unmarshal(payload, &p); err != nil {
	// 		return fmt.Errorf("unmarshal payment pending: %w", err)
	// 	}

	// 	refType := "order"
	// 	noti := socket.NotificationPayload{
	// 		ID:               uuid.New(),
	// 		Title:            "Đơn hàng chờ thanh toán",
	// 		Content:          fmt.Sprintf("Đơn hàng #%s (%s) đang chờ thanh toán", p.OrderNumber, p.TotalAmount),
	// 		NotificationType: "payment_pending",
	// 		ReferenceType:    &refType,
	// 		ReferenceID:      &p.OrderID,
	// 		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	// 	}

	// 	log.Printf("[task] Payment pending: order=%s, user=%s", p.OrderNumber, p.UserID)
	// 	notifier.SendNotification(p.UserID, noti)
	// 	return nil
	// })


	// q.Schedule(TaskDailyCheckin, DailyAt(7, 0), func(ctx context.Context, _ []byte) error {
	// 	noti := socket.NotificationPayload{
	// 		ID:               uuid.New(),
	// 		Title:            "Điểm danh hàng ngày",
	// 		Content:          "Đừng quên điểm danh hôm nay để giữ streak!",
	// 		NotificationType: "daily_checkin",
	// 		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	// 	}

	// 	onlineUsers := notifier.GetOnlineUsers()
	// 	log.Printf("[task] Daily checkin reminder: %d online users", len(onlineUsers))
	// 	notifier.SendNotificationToMany(onlineUsers, noti)
	// 	return nil
	// }, "low")

	// q.Schedule(TaskStreakWarning, DailyAt(20, 0), func(ctx context.Context, _ []byte) error {
	// 	// Gửi nhắc streak cho tất cả user online
	// 	noti := socket.NotificationPayload{
	// 		ID:               uuid.New(),
	// 		Title:            "Streak sắp mất!",
	// 		Content:          "Hoàn thành bài học hôm nay để duy trì streak!",
	// 		NotificationType: "streak_warning",
	// 		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	// 	}

	// 	onlineUsers := notifier.GetOnlineUsers()
	// 	log.Printf("[task] Streak warning: %d online users", len(onlineUsers))
	// 	notifier.SendNotificationToMany(onlineUsers, noti)
	// 	return nil
	// }, "notifications")
}
