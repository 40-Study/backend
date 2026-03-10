package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ChatServiceInterface interface {
	SendMessage(ctx context.Context, req dto.SendChatMessageDTO) (*model.ChatMessage, error)
	GetMessages(ctx context.Context, sessionID uuid.UUID, page, pageSize int) (*dto.ChatMessageListDTO, error)
	DeleteMessage(ctx context.Context, messageID, deletedBy uuid.UUID) error
	PinMessage(ctx context.Context, messageID uuid.UUID, isPinned bool) error
}

type ChatService struct {
	repo          repository.ChatMessageRepositoryInterface
	analyticsRepo repository.AnalyticsRepositoryInterface
}

func NewChatService(
	repo repository.ChatMessageRepositoryInterface,
	analyticsRepo repository.AnalyticsRepositoryInterface,
) *ChatService {
	return &ChatService{
		repo:          repo,
		analyticsRepo: analyticsRepo,
	}
}

func (s *ChatService) SendMessage(ctx context.Context, req dto.SendChatMessageDTO) (*model.ChatMessage, error) {
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return nil, errors.New("invalid session_id")
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, errors.New("invalid user_id")
	}

	message := &model.ChatMessage{
		SessionID: sessionID,
		UserID:    userID,
		Message:   req.Message,
	}

	if err := s.repo.Create(ctx, message); err != nil {
		return nil, err
	}

	_ = s.analyticsRepo.IncrementTotalMessages(ctx, sessionID)

	return s.repo.GetByID(ctx, message.ID)
}

func (s *ChatService) GetMessages(ctx context.Context, sessionID uuid.UUID, page, pageSize int) (*dto.ChatMessageListDTO, error) {
	messages, total, err := s.repo.GetBySession(ctx, sessionID, page, pageSize)
	if err != nil {
		return nil, err
	}

	var data []dto.ChatMessageResponseDTO
	for _, msg := range messages {
		data = append(data, s.toResponseDTO(msg))
	}

	return &dto.ChatMessageListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ChatService) DeleteMessage(ctx context.Context, messageID, deletedBy uuid.UUID) error {
	message, err := s.repo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if message == nil {
		return errors.New("message not found")
	}

	return s.repo.SoftDelete(ctx, messageID, deletedBy)
}

func (s *ChatService) PinMessage(ctx context.Context, messageID uuid.UUID, isPinned bool) error {
	return s.repo.Pin(ctx, messageID, isPinned)
}

func (s *ChatService) toResponseDTO(msg model.ChatMessage) dto.ChatMessageResponseDTO {
	var userName string
	if msg.User != nil {
		userName = msg.User.UserName
		if msg.User.FullName != nil {
			userName = *msg.User.FullName
		}
	}

	return dto.ChatMessageResponseDTO{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		UserID:    msg.UserID,
		UserName:  userName,
		Message:   msg.Message,
		IsPinned:  msg.IsPinned,
		ParentID:  msg.ParentID,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339),
	}
}
