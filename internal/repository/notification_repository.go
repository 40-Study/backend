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

// MarkAsRead trả gorm.ErrRecordNotFound khi không có thông báo nào của userID với id này (không tồn tại hoặc của
// người khác: hai trường hợp không phân biệt được), để handler trả 404 thay vì 200 giả.
func (r *NotificationRepository) MarkAsRead(id, userID uuid.UUID) error {
	now := time.Now()
	res := r.db.Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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

// DeleteResolvedFriendRequestNotices xoá thông báo "lời mời kết bạn" đã hết hiệu lực (model.ResolvedFriendRequestNoticeSQL)
// của các user cho trước; không truyền user nào thì quét toàn bảng (bước sửa một lần ở package database dùng SQL
// chung, không gọi hàm này). Gọi sau khi lời mời bị huỷ/từ chối/chấp nhận/chặn để người nhận không còn thấy thông
// báo dẫn tới tab Lời mời trống (QA hồi quy A-14).
func (r *NotificationRepository) DeleteResolvedFriendRequestNotices(userIDs ...uuid.UUID) error {
	q := r.db.Where(model.ResolvedFriendRequestNoticeSQL)
	if len(userIDs) > 0 {
		q = q.Where("user_id IN ?", userIDs)
	}
	return q.Delete(&model.Notification{}).Error
}

// Delete chỉ xoá thông báo của chính userID; 0 dòng bị xoá (không tồn tại hoặc của người khác) trả gorm.ErrRecordNotFound.
func (r *NotificationRepository) Delete(id, userID uuid.UUID) error {
	res := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Notification{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
