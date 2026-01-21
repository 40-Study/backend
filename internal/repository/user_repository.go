package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserRepositoryInterface interface {
	CreateUser(ctx context.Context, user *model.User) error
	FindUserByEmail(ctx context.Context, email string) (*model.User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	UpdateUser(ctx context.Context, user *model.User) error
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newPasswordHash string) error
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *model.User) error {
	return nil
}

func (r *UserRepository) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return nil, nil
}

func (r *UserRepository) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {

	return nil, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, user *model.User) error {
	return nil
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newPasswordHash string) error {
	return nil
}
