package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type AchievementServiceInterface interface {
	ListAchievements(ctx context.Context) ([]dto.AchievementDTO, error)
	GetMyAchievements(ctx context.Context, userID uuid.UUID) ([]dto.AchievementWithStatusDTO, error)
	UnlockAchievement(ctx context.Context, userID, achievementID uuid.UUID) (*dto.UnlockAchievementResponse, error)
}

type AchievementService struct {
	repo *repository.AchievementRepository
}

func NewAchievementService(repo *repository.AchievementRepository) *AchievementService {
	return &AchievementService{repo: repo}
}

func (s *AchievementService) ListAchievements(ctx context.Context) ([]dto.AchievementDTO, error) {
	achievements, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	return toAchievementDTOs(achievements), nil
}

func (s *AchievementService) GetMyAchievements(ctx context.Context, userID uuid.UUID) ([]dto.AchievementWithStatusDTO, error) {
	// Get all active achievements
	all, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	// Get what the user has unlocked
	userAchievements, err := s.repo.GetUserAchievements(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Build unlocked set
	unlockedMap := make(map[uuid.UUID]*model.UserAchievement, len(userAchievements))
	for i := range userAchievements {
		ua := &userAchievements[i]
		unlockedMap[ua.AchievementID] = ua
	}

	result := make([]dto.AchievementWithStatusDTO, 0, len(all))
	for _, a := range all {
		item := dto.AchievementWithStatusDTO{
			AchievementDTO: toAchievementDTO(a),
		}
		if ua, ok := unlockedMap[a.ID]; ok {
			item.Unlocked = true
			item.EarnedAt = &ua.EarnedAt
			item.Progress = ua.Progress
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *AchievementService) UnlockAchievement(ctx context.Context, userID, achievementID uuid.UUID) (*dto.UnlockAchievementResponse, error) {
	// Verify achievement exists
	achievement, err := s.repo.GetByID(ctx, achievementID)
	if err != nil {
		return nil, err
	}
	if achievement == nil {
		return nil, errors.New("achievement not found")
	}

	// Idempotent: already unlocked is not an error
	has, err := s.repo.HasAchievement(ctx, userID, achievementID)
	if err != nil {
		return nil, err
	}
	if has {
		return nil, errors.New("achievement already unlocked")
	}

	ua, err := s.repo.UnlockAchievement(ctx, userID, achievementID)
	if err != nil {
		return nil, err
	}

	return &dto.UnlockAchievementResponse{
		AchievementID: achievementID,
		UserID:        userID,
		EarnedAt:      ua.EarnedAt,
		Message:       "Achievement unlocked successfully",
	}, nil
}

// ============================================================
// Mappers
// ============================================================

func toAchievementDTO(a model.Achievement) dto.AchievementDTO {
	return dto.AchievementDTO{
		ID:          a.ID,
		Name:        a.Name,
		Slug:        a.Slug,
		Description: a.Description,
		IconURL:     a.IconURL,
		BadgeURL:    a.BadgeURL,
		Category:    a.Category,
		Points:      a.Points,
		Requirement: a.Requirement,
		Threshold:   a.Threshold,
	}
}

func toAchievementDTOs(achievements []model.Achievement) []dto.AchievementDTO {
	result := make([]dto.AchievementDTO, len(achievements))
	for i, a := range achievements {
		result[i] = toAchievementDTO(a)
	}
	return result
}
