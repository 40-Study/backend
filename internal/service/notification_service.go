package service

import (
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/socket"
)

type NotificationServiceInterface interface {
	GetNotifications(userID uuid.UUID, page, pageSize int) (*dto.NotificationListResponseDTO, error)
	GetUnreadCount(userID uuid.UUID) (int64, error)
	MarkAsRead(id, userID uuid.UUID) error
	MarkAllAsRead(userID uuid.UUID) error
	SendNotification(req dto.CreateNotificationDTO) error
	DeleteNotification(id, userID uuid.UUID) error
}

type NotificationService struct {
	repo     repository.NotificationRepositoryInterface
	notifier *socket.Notifier
}

func NewNotificationService(repo repository.NotificationRepositoryInterface, notifier *socket.Notifier) *NotificationService {
	return &NotificationService{repo: repo, notifier: notifier}
}

func (s *NotificationService) GetNotifications(userID uuid.UUID, page, pageSize int) (*dto.NotificationListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	notifications, total, err := s.repo.GetByUserID(userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	unreadCount, err := s.repo.GetUnreadCount(userID)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.NotificationResponseDTO, len(notifications))
	for i, n := range notifications {
		dtos[i] = mapNotificationToDTO(n)
	}

	return &dto.NotificationListResponseDTO{
		Notifications: dtos,
		Total:         total,
		UnreadCount:   unreadCount,
		Page:          page,
		PageSize:      pageSize,
	}, nil
}

func (s *NotificationService) GetUnreadCount(userID uuid.UUID) (int64, error) {
	return s.repo.GetUnreadCount(userID)
}

func (s *NotificationService) MarkAsRead(id, userID uuid.UUID) error {
	return s.repo.MarkAsRead(id, userID)
}

func (s *NotificationService) MarkAllAsRead(userID uuid.UUID) error {
	return s.repo.MarkAllAsRead(userID)
}

func (s *NotificationService) SendNotification(req dto.CreateNotificationDTO) error {
	notifications := make([]model.Notification, len(req.UserIDs))
	for i, uid := range req.UserIDs {
		notifications[i] = model.Notification{
			UserID:           uid,
			Title:            req.Title,
			Content:          req.Content,
			NotificationType: req.NotificationType,
			ReferenceType:    req.ReferenceType,
			ReferenceID:      req.ReferenceID,
		}
	}

	if err := s.repo.CreateBatch(notifications); err != nil {
		return err
	}

	// Send via WebSocket to each user
	for _, n := range notifications {
		s.notifier.NotifyNewNotification(
			n.UserID,
			n.ID,
			n.Title,
			n.Content,
			n.NotificationType,
			n.ReferenceType,
			n.ReferenceID,
		)
	}

	return nil
}

func (s *NotificationService) DeleteNotification(id, userID uuid.UUID) error {
	return s.repo.Delete(id, userID)
}

func mapNotificationToDTO(n model.Notification) dto.NotificationResponseDTO {
	return dto.NotificationResponseDTO{
		ID:               n.ID,
		Title:            n.Title,
		Content:          n.Content,
		NotificationType: n.NotificationType,
		ReferenceType:    n.ReferenceType,
		ReferenceID:      n.ReferenceID,
		IsRead:           n.IsRead,
		ReadAt:           n.ReadAt,
		CreatedAt:        n.CreatedAt,
	}
}
