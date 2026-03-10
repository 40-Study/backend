package service

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

type AnalyticsServiceInterface interface {
	GetLivestreamAnalytics(ctx context.Context, sessionID uuid.UUID) (*dto.AnalyticsResponseDTO, error)
	GetAssignmentAnalytics(ctx context.Context, assignmentID uuid.UUID) (*dto.AssignmentAnalyticsDTO, error)
	GetParticipantAnalytics(ctx context.Context, sessionID uuid.UUID) (*dto.ParticipantAnalyticsDTO, error)
}

type AnalyticsService struct {
	analyticsRepo   repository.AnalyticsRepositoryInterface
	participantRepo repository.ParticipantRepositoryInterface
	submissionRepo  repository.SubmissionRepositoryInterface
	assignmentRepo  repository.AssignmentRepositoryInterface
}

func NewAnalyticsService(
	analyticsRepo repository.AnalyticsRepositoryInterface,
	participantRepo repository.ParticipantRepositoryInterface,
	submissionRepo repository.SubmissionRepositoryInterface,
	assignmentRepo repository.AssignmentRepositoryInterface,
) *AnalyticsService {
	return &AnalyticsService{
		analyticsRepo:   analyticsRepo,
		participantRepo: participantRepo,
		submissionRepo:  submissionRepo,
		assignmentRepo:  assignmentRepo,
	}
}

func (s *AnalyticsService) GetLivestreamAnalytics(ctx context.Context, sessionID uuid.UUID) (*dto.AnalyticsResponseDTO, error) {
	analytics, err := s.analyticsRepo.GetBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if analytics == nil {
		return &dto.AnalyticsResponseDTO{
			SessionID: sessionID,
		}, nil
	}

	return &dto.AnalyticsResponseDTO{
		SessionID:        analytics.SessionID,
		PeakViewers:      analytics.PeakViewers,
		TotalViewers:     analytics.TotalViewers,
		TotalMessages:    analytics.TotalMessages,
		AvgWatchTimeSecs: analytics.AvgWatchTimeSecs,
	}, nil
}

func (s *AnalyticsService) GetAssignmentAnalytics(ctx context.Context, assignmentID uuid.UUID) (*dto.AssignmentAnalyticsDTO, error) {
	assignment, err := s.assignmentRepo.GetByID(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, nil
	}

	stats, err := s.analyticsRepo.GetSubmissionStats(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	totalSubmissions := int64(0)
	acceptedCount := int64(0)
	acceptanceRate := 0.0

	if v, ok := stats["total_submissions"].(int64); ok {
		totalSubmissions = v
	}
	if v, ok := stats["accepted_count"].(int64); ok {
		acceptedCount = v
	}
	if v, ok := stats["acceptance_rate"].(float64); ok {
		acceptanceRate = v
	}

	return &dto.AssignmentAnalyticsDTO{
		AssignmentID:     assignmentID,
		TotalSubmissions: int(totalSubmissions),
		AcceptedCount:    int(acceptedCount),
		AcceptanceRate:   acceptanceRate,
		Difficulty:       string(assignment.Difficulty),
	}, nil
}

func (s *AnalyticsService) GetParticipantAnalytics(ctx context.Context, sessionID uuid.UUID) (*dto.ParticipantAnalyticsDTO, error) {
	totalJoined, err := s.participantRepo.CountBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	activeCount, err := s.participantRepo.CountActiveBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	participants, err := s.participantRepo.GetActiveBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	byRole := make(map[string]int)
	for _, p := range participants {
		byRole[string(p.Role)]++
	}

	return &dto.ParticipantAnalyticsDTO{
		SessionID:   sessionID,
		ActiveCount: int(activeCount),
		TotalJoined: int(totalJoined),
		ByRole:      byRole,
	}, nil
}
