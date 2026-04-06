package repository

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserPreferenceRepositoryInterface interface {
	GetByUserID(userID uuid.UUID) (*model.UserPreference, error)
	Upsert(pref *model.UserPreference) error
}

type UserPreferenceRepository struct {
	db *gorm.DB
}

func NewUserPreferenceRepository(db *gorm.DB) *UserPreferenceRepository {
	return &UserPreferenceRepository{db: db}
}

func (r *UserPreferenceRepository) GetByUserID(userID uuid.UUID) (*model.UserPreference, error) {
	var pref model.UserPreference
	err := r.db.Where("user_id = ?", userID).First(&pref).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &pref, nil
}

func (r *UserPreferenceRepository) Upsert(pref *model.UserPreference) error {
	return r.db.Save(pref).Error
}
