package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type WhiteboardServiceInterface interface {
	GetSnapshot(ctx context.Context, sessionID uuid.UUID) (*dto.WhiteboardSnapshotResponseDTO, error)
	SaveSnapshot(ctx context.Context, req dto.WhiteboardSnapshotDTO) error
	BroadcastEvent(ctx context.Context, sessionID uuid.UUID, event dto.WhiteboardEventDTO, livekitSvc LivekitServiceInterface) error
}

type WhiteboardService struct {
	repo        repository.WhiteboardRepositoryInterface
	sessionRepo repository.LivestreamRepositoryInterface
	redis       *redis.Client
}

func NewWhiteboardService(
	repo repository.WhiteboardRepositoryInterface,
	sessionRepo repository.LivestreamRepositoryInterface,
	redis *redis.Client,
) *WhiteboardService {
	return &WhiteboardService{
		repo:        repo,
		sessionRepo: sessionRepo,
		redis:       redis,
	}
}

func (s *WhiteboardService) GetSnapshot(ctx context.Context, sessionID uuid.UUID) (*dto.WhiteboardSnapshotResponseDTO, error) {
	snapshot, err := s.repo.GetSnapshot(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return &dto.WhiteboardSnapshotResponseDTO{
			SessionID:    sessionID,
			SnapshotData: "{}",
			Version:      0,
			SavedAt:      time.Now().Format(time.RFC3339),
		}, nil
	}

	return &dto.WhiteboardSnapshotResponseDTO{
		ID:           snapshot.ID,
		SessionID:    snapshot.SessionID,
		SnapshotData: snapshot.SnapshotData,
		Version:      snapshot.Version,
		SavedAt:      snapshot.SavedAt.Format(time.RFC3339),
	}, nil
}

func (s *WhiteboardService) SaveSnapshot(ctx context.Context, req dto.WhiteboardSnapshotDTO) error {
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return err
	}

	snapshot := &model.WhiteboardSnapshot{
		SessionID:    sessionID,
		SnapshotData: req.SnapshotData,
		Version:      req.Version,
	}

	return s.repo.SaveSnapshot(ctx, snapshot)
}

func (s *WhiteboardService) BroadcastEvent(ctx context.Context, sessionID uuid.UUID, event dto.WhiteboardEventDTO, livekitSvc LivekitServiceInterface) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return nil
	}

	if session.Settings.WhiteboardLocked {
		return nil
	}

	eventData := map[string]interface{}{
		"type":    event.Type,
		"action":  event.Action,
		"payload": event.Payload,
	}
	eventJSON, _ := json.Marshal(eventData)

	sendReq := dto.SendDataDTO{
		Data:  string(eventJSON),
		Topic: "whiteboard",
	}

	return livekitSvc.SendData(ctx, session.RoomName, sendReq)
}
