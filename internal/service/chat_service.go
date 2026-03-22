package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	PinMessage(ctx context.Context, messageID uuid.UUID) error
	UnPinMessage(ctx context.Context, messageID uuid.UUID) error
}

type ChatService struct {
	repo           repository.ChatMessageRepositoryInterface
	analyticsRepo  repository.AnalyticsRepositoryInterface
	livestreamRepo repository.LivestreamRepositoryInterface
	livekitSvc     LivekitServiceInterface
}

func NewChatService(
	repo repository.ChatMessageRepositoryInterface,
	analyticsRepo repository.AnalyticsRepositoryInterface,
	livestreamRepo repository.LivestreamRepositoryInterface,
	livekitSvc LivekitServiceInterface,
) *ChatService {
	return &ChatService{
		repo:           repo,
		analyticsRepo:  analyticsRepo,
		livestreamRepo: livestreamRepo,
		livekitSvc:     livekitSvc,
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

	go s.analyticsRepo.IncrementTotalMessages(ctx, sessionID)

	saved := message
	if err != nil {
		return nil, err
	}

	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		session, err := s.livestreamRepo.GetByID(bg, sessionID)
		if err != nil || session == nil {
			return
		}
		lkPayload := map[string]interface{}{
			"id":        saved.ID,
			"timestamp": saved.CreatedAt.UnixMilli(),
			"message":   saved.Message,
		}
		payload, err := json.Marshal(lkPayload)
		if err != nil {
			return
		}
		// Broadcast via LiveKit data channel
		if err := s.livekitSvc.SendData(bg, session.RoomName, dto.SendDataDTO{
			Data:  string(payload),
			Topic: "lk-chat-message",
		}); err != nil {
			// Log but don't fail - chat is still saved to DB
			fmt.Printf("livekit broadcast failed: %v\n", err)
		}
	}()

	return saved, nil
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

func (s *ChatService) PinMessage(ctx context.Context, messageID uuid.UUID) error {
	return s.repo.Pin(ctx, messageID)
}

func (s *ChatService) UnPinMessage(ctx context.Context, messageID uuid.UUID) error {

	return s.repo.UnPin(ctx, messageID)
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
