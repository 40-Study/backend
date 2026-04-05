package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type OAuthProviderRepositoryInterface interface {
	Create(ctx context.Context, provider *model.UserOAuthProvider) error
	FindByProviderAndProviderUserID(ctx context.Context, provider string, providerUserID string) (*model.UserOAuthProvider, error)
	FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.UserOAuthProvider, error)
	FindByUserIDAndProvider(ctx context.Context, userID uuid.UUID, provider string) (*model.UserOAuthProvider, error)
	Update(ctx context.Context, provider *model.UserOAuthProvider) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByUserIDAndProvider(ctx context.Context, userID uuid.UUID, provider string) error
}

type OAuthProviderRepository struct {
	db *gorm.DB
}

func NewOAuthProviderRepository(db *gorm.DB) *OAuthProviderRepository {
	return &OAuthProviderRepository{db: db}
}

func (r *OAuthProviderRepository) Create(ctx context.Context, provider *model.UserOAuthProvider) error {
	return r.db.WithContext(ctx).Create(provider).Error
}

func (r *OAuthProviderRepository) FindByProviderAndProviderUserID(ctx context.Context, provider string, providerUserID string) (*model.UserOAuthProvider, error) {
	var result model.UserOAuthProvider
	err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (r *OAuthProviderRepository) FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.UserOAuthProvider, error) {
	var results []model.UserOAuthProvider
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (r *OAuthProviderRepository) FindByUserIDAndProvider(ctx context.Context, userID uuid.UUID, provider string) (*model.UserOAuthProvider, error) {
	var result model.UserOAuthProvider
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (r *OAuthProviderRepository) Update(ctx context.Context, provider *model.UserOAuthProvider) error {
	return r.db.WithContext(ctx).Save(provider).Error
}

func (r *OAuthProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.UserOAuthProvider{}, "id = ?", id).Error
}

func (r *OAuthProviderRepository) DeleteByUserIDAndProvider(ctx context.Context, userID uuid.UUID, provider string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		Delete(&model.UserOAuthProvider{}).Error
}
