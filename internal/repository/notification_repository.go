package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type NotificationRepositoryInterface interface {
	GetByUserID(userID uuid.UUID, page, pageSize int) ([]model.Notification, int64, error)
	GetUnreadCount(userID uuid.UUID) (int64, error)
	MarkAsRead(id, userID uuid.UUID) error
	MarkAllAsRead(userID uuid.UUID) error
	Create(notification *model.Notification) error
	CreateBatch(notifications []model.Notification) error
	Delete(id, userID uuid.UUID) error
	GetSettingsByUserID(userID uuid.UUID) (*model.NotificationSettings, error)
	UpsertSettings(settings *model.NotificationSettings) error
}

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) GetByUserID(userID uuid.UUID, page, pageSize int) ([]model.Notification, int64, error) {
	var notifications []model.Notification
	var total int64

	query := r.db.Model(&model.Notification{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&notifications).Error
	return notifications, total, err
}

func (r *NotificationRepository) GetUnreadCount(userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&count).Error
	return count, err
}

func (r *NotificationRepository) MarkAsRead(id, userID uuid.UUID) error {
	now := time.Now()
	return r.db.Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

func (r *NotificationRepository) MarkAllAsRead(userID uuid.UUID) error {
	now := time.Now()
	return r.db.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = false", userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

func (r *NotificationRepository) Create(notification *model.Notification) error {
	return r.db.Create(notification).Error
}

func (r *NotificationRepository) CreateBatch(notifications []model.Notification) error {
	return r.db.Create(&notifications).Error
}

func (r *NotificationRepository) Delete(id, userID uuid.UUID) error {
	return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Notification{}).Error
}

func (r *NotificationRepository) GetSettingsByUserID(userID uuid.UUID) (*model.NotificationSettings, error) {
	var settings model.NotificationSettings
	err := r.db.Where("user_id = ?", userID).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &settings, nil
}

func (r *NotificationRepository) UpsertSettings(settings *model.NotificationSettings) error {
	return r.db.Save(settings).Error
}
