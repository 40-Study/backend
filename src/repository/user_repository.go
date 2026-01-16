package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
	"tiger.com/v2/src/model"
)

type UserRepositoryInterface interface {
	CreateUser(ctx context.Context, user *model.User) error
	FindUserByEmail(ctx context.Context, email string) (*model.User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	UpdateUser(ctx context.Context, user *model.User) error
	FindPermissionByUserID(ctx context.Context, userID uuid.UUID) ([]model.Permission, error)
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newPasswordHash string) error
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepositoryInterface {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		if err := tx.Create(user).Error; err != nil {
			return err
		}

		pref := model.UserPreference{
			UserID:                 user.ID,
			Theme:                  pgtype.Text{String: "light", Valid: true},
			Language:               pgtype.Text{String: "vi", Valid: true},
			NotificationEmail:      pgtype.Bool{Bool: true, Valid: true},
			NotificationPush:       pgtype.Bool{Bool: true, Valid: true},
			NotificationSms:        pgtype.Bool{Bool: false, Valid: true},
			NewsletterSubscribed:   pgtype.Bool{Bool: false, Valid: true},
			PreferredPaymentMethod: pgtype.Text{String: "", Valid: false},
			PrivacyShowProfile:     pgtype.Bool{Bool: true, Valid: true},
			PrivacyShowActivity:    pgtype.Bool{Bool: false, Valid: true},
			Preferences:            map[string]interface{}{},
		}

		if err := tx.Create(&pref).Error; err != nil {
			return err
		}

		points := model.UserPoint{
			UserID:          user.ID,
			TotalPoints:     0,
			AvailablePoints: 0,
			LifetimeEarned:  0,
			LifetimeSpent:   0,
			TotalExpired:    0,
			LastExpiredAt:   nil,
		}

		if err := tx.Create(&points).Error; err != nil {
			return err
		}

		return nil
	})
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
	return &user, nil
}

func (r *UserRepository) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, user *model.User) error {
	if user.ID == uuid.Nil {
		return errors.New("user ID is required for update")
	}

	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", user.ID).
		Omit("email", "username", "password_hash", "created_at", "id").
		Updates(user).Error
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newPasswordHash string) error {
	if userID == uuid.Nil {
		return errors.New("user ID is required for update")
	}
	return r.db.WithContext(ctx).Exec(`
		UPDATE users
		SET password_hash = ?
		WHERE id = ?
	`, newPasswordHash, userID,
	).Error
}

func (r *UserRepository) FindPermissionByUserID(ctx context.Context, userID uuid.UUID) ([]model.Permission, error) {
	var permissions []model.Permission
	err := r.db.WithContext(ctx).Raw(
		`SELECT DISTINCT p.* FROM permissions AS p
		JOIN role_permissions AS rp ON p.id = rp.permission_id
		JOIN user_roles AS ur ON rp.role_id = ur.role_id
		JOIN users AS u ON u.id = ur.user_id
		WHERE u.id = $1 AND rp.deleted_at IS NULL AND ur.deleted_at IS NULL
		`, userID,
	).Scan(&permissions).Error
	if err != nil {
		return nil, err
	}
	return permissions, nil
}
