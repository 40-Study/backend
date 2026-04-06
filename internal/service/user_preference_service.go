package service

import (
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type UserPreferenceServiceInterface interface {
	GetPrivacySettings(userID uuid.UUID) (*dto.PrivacySettingsResponseDTO, error)
	UpdatePrivacySettings(userID uuid.UUID, req dto.UpdatePrivacySettingsDTO) (*dto.PrivacySettingsResponseDTO, error)
}

type UserPreferenceService struct {
	repo repository.UserPreferenceRepositoryInterface
}

func NewUserPreferenceService(repo repository.UserPreferenceRepositoryInterface) *UserPreferenceService {
	return &UserPreferenceService{repo: repo}
}

func (s *UserPreferenceService) GetPrivacySettings(userID uuid.UUID) (*dto.PrivacySettingsResponseDTO, error) {
	pref, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if pref == nil {
		pref = &model.UserPreference{
			UserID:             userID,
			ProfileVisibility:  "public",
			ActivityStatus:     "everyone",
			LeaderboardDisplay: "name",
		}
		if err := s.repo.Upsert(pref); err != nil {
			return nil, err
		}
	}
	return mapPrivacyToDTO(pref), nil
}

func (s *UserPreferenceService) UpdatePrivacySettings(userID uuid.UUID, req dto.UpdatePrivacySettingsDTO) (*dto.PrivacySettingsResponseDTO, error) {
	pref, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if pref == nil {
		pref = &model.UserPreference{
			UserID:             userID,
			ProfileVisibility:  "public",
			ActivityStatus:     "everyone",
			LeaderboardDisplay: "name",
		}
	}
	if req.ProfileVisibility != nil {
		pref.ProfileVisibility = *req.ProfileVisibility
	}
	if req.ActivityStatus != nil {
		pref.ActivityStatus = *req.ActivityStatus
	}
	if req.LeaderboardDisplay != nil {
		pref.LeaderboardDisplay = *req.LeaderboardDisplay
	}
	if err := s.repo.Upsert(pref); err != nil {
		return nil, err
	}
	return mapPrivacyToDTO(pref), nil
}

func mapPrivacyToDTO(p *model.UserPreference) *dto.PrivacySettingsResponseDTO {
	return &dto.PrivacySettingsResponseDTO{
		ProfileVisibility:  p.ProfileVisibility,
		ActivityStatus:     p.ActivityStatus,
		LeaderboardDisplay: p.LeaderboardDisplay,
	}
}
