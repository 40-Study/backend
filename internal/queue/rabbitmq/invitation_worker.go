package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

// ===================== Constants =====================

const (
	InvitationExchange   = "invitation"
	InvitationSendQueue  = "invitation.send"
	InvitationSendKey    = "invitation.send"
	InvitationEventQueue = "invitation.event"
	InvitationEventKey   = "invitation.event"
)

// ===================== Messages =====================

// InvitationSendMessage — đẩy vào queue khi tạo lời mời, worker sẽ gửi email + noti
type InvitationSendMessage struct {
	InvitationID uuid.UUID `json:"invitation_id"`
	StudentID    uuid.UUID `json:"student_id"`
	StudentName  string    `json:"student_name"`
	InviteeEmail string    `json:"invitee_email"`
	Relationship string    `json:"relationship"`
	Token        string    `json:"token"`
	Message      *string   `json:"message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// InvitationEventMessage — đẩy vào queue khi phụ huynh accept/reject, worker gửi noti cho student
type InvitationEventMessage struct {
	InvitationID uuid.UUID `json:"invitation_id"`
	StudentID    uuid.UUID `json:"student_id"`
	ParentName   string    `json:"parent_name"`
	ParentEmail  string    `json:"parent_email"`
	Action       string    `json:"action"` // accepted, rejected
	CreatedAt    time.Time `json:"created_at"`
}

// ===================== Queue Setup =====================

// SetupInvitationQueues khai báo exchange + queue cho invitation worker
func SetupInvitationQueues(svc *RabbitMQService) error {
	if svc == nil {
		return fmt.Errorf("RabbitMQ not available")
	}

	if err := svc.SetupQueue(InvitationExchange, InvitationSendQueue, InvitationSendKey); err != nil {
		return fmt.Errorf("setup invitation send queue: %w", err)
	}
	if err := svc.SetupQueue(InvitationExchange, InvitationEventQueue, InvitationEventKey); err != nil {
		return fmt.Errorf("setup invitation event queue: %w", err)
	}

	log.Println("[RabbitMQ] Invitation queues setup successfully")
	return nil
}

// ===================== Notifier Interface =====================

// InvitationNotifier gửi notification qua WebSocket + lưu DB
type InvitationNotifier interface {
	SendNotification(req dto.CreateNotificationDTO) error
}

// ===================== Worker =====================

// InvitationWorker consume messages từ RabbitMQ và xử lý: gửi email, bắn noti
type InvitationWorker struct {
	rabbitMQ *RabbitMQService
	cfg      *config.Config
	notifier InvitationNotifier
}

func NewInvitationWorker(
	rabbitMQ *RabbitMQService,
	cfg *config.Config,
	notifier InvitationNotifier,
) *InvitationWorker {
	return &InvitationWorker{
		rabbitMQ: rabbitMQ,
		cfg:      cfg,
		notifier: notifier,
	}
}

// Start khởi chạy 2 consumer: send invitation + handle event
func (w *InvitationWorker) Start(ctx context.Context) error {
	// Consumer 1: gửi email lời mời
	if err := w.rabbitMQ.ConsumeMessages(ctx, InvitationSendQueue, w.handleSendInvitation); err != nil {
		return fmt.Errorf("start invitation send consumer: %w", err)
	}
	log.Println("[InvitationWorker] Send consumer started")

	// Consumer 2: xử lý event accept/reject
	if err := w.rabbitMQ.ConsumeMessages(ctx, InvitationEventQueue, w.handleInvitationEvent); err != nil {
		return fmt.Errorf("start invitation event consumer: %w", err)
	}
	log.Println("[InvitationWorker] Event consumer started")

	return nil
}

// handleSendInvitation — gửi email cho phụ huynh + noti cho student
func (w *InvitationWorker) handleSendInvitation(ctx context.Context, data []byte) error {
	var msg InvitationSendMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[InvitationWorker] Failed to unmarshal send message: %v", err)
		return nil // Không retry message hỏng format
	}

	log.Printf("[InvitationWorker] Processing invitation: student=%s email=%s", msg.StudentName, msg.InviteeEmail)

	// 1. Gửi email mời phụ huynh
	if err := utils.SendInvitationEmail(w.cfg, msg.InviteeEmail, msg.StudentName, msg.Relationship, msg.Token); err != nil {
		log.Printf("[InvitationWorker] FAILED send email to %s: %v", msg.InviteeEmail, err)
		return err // Retry — RabbitMQ sẽ requeue
	}
	log.Printf("[InvitationWorker] Email sent to %s", msg.InviteeEmail)

	// 2. Tạo notification cho student: "Đã gửi lời mời thành công"
	refType := "parent_invitation"
	w.notifier.SendNotification(dto.CreateNotificationDTO{
		UserIDs:          []uuid.UUID{msg.StudentID},
		Title:            "Lời mời phụ huynh đã được gửi",
		Content:          fmt.Sprintf("Email mời %s (%s) đã được gửi tới %s", msg.Relationship, msg.InviteeEmail, msg.InviteeEmail),
		NotificationType: "parent_invitation_sent",
		ReferenceType:    &refType,
		ReferenceID:      &msg.InvitationID,
	})

	log.Printf("[InvitationWorker] Done: invitation=%s", msg.InvitationID)
	return nil
}

// handleInvitationEvent — phụ huynh accept/reject → noti cho student
func (w *InvitationWorker) handleInvitationEvent(ctx context.Context, data []byte) error {
	var msg InvitationEventMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[InvitationWorker] Failed to unmarshal event message: %v", err)
		return nil
	}

	log.Printf("[InvitationWorker] Processing event: action=%s parent=%s student=%s", msg.Action, msg.ParentEmail, msg.StudentID)

	refType := "parent_invitation"

	switch msg.Action {
	case "accepted":
		// Noti cho student
		w.notifier.SendNotification(dto.CreateNotificationDTO{
			UserIDs:          []uuid.UUID{msg.StudentID},
			Title:            "Phụ huynh đã chấp nhận lời mời!",
			Content:          fmt.Sprintf("%s (%s) đã chấp nhận làm phụ huynh của bạn.", msg.ParentName, msg.ParentEmail),
			NotificationType: "parent_invitation_accepted",
			ReferenceType:    &refType,
			ReferenceID:      &msg.InvitationID,
		})

		// Gửi email xác nhận cho student (optional)
		// utils.SendEmail(w.cfg, ...)

	case "rejected":
		w.notifier.SendNotification(dto.CreateNotificationDTO{
			UserIDs:          []uuid.UUID{msg.StudentID},
			Title:            "Phụ huynh đã từ chối lời mời",
			Content:          fmt.Sprintf("%s đã từ chối lời mời phụ huynh.", msg.ParentEmail),
			NotificationType: "parent_invitation_rejected",
			ReferenceType:    &refType,
			ReferenceID:      &msg.InvitationID,
		})
	}

	log.Printf("[InvitationWorker] Event processed: %s for invitation=%s", msg.Action, msg.InvitationID)
	return nil
}
