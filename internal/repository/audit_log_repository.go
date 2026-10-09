package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// AuditLogFilter là bộ lọc ĐÃ PARSE của GET /admin/audit-logs (service lo parse + kiểm tra).
// Page/PageSize đã được service chặn trong khoảng hợp lệ; chuỗi rỗng / nil = không lọc.
type AuditLogFilter struct {
	Page, PageSize int
	Action         string
	ActorID        *uuid.UUID
	TargetType     string
	TargetID       string
	From, To       *time.Time // khoảng đóng [From, To] trên created_at
}

// AuditLogRow = dòng nhật ký + tên/email người thực hiện JOIN từ users (nil khi không còn user).
type AuditLogRow struct {
	model.AuditLog
	ActorName  *string
	ActorEmail *string
}

type AuditLogRepositoryInterface interface {
	Create(ctx context.Context, log *model.AuditLog) error
	List(ctx context.Context, f AuditLogFilter) ([]AuditLogRow, int64, error)
}

type AuditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(ctx context.Context, log *model.AuditLog) error {
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(log).Error
}

// List: LEFT JOIN users để lấy tên/email (dòng vẫn hiện khi user không còn), sắp xếp mới nhất
// trước, id DESC làm tie-break để phân trang ổn định khi nhiều dòng cùng created_at.
func (r *AuditLogRepository) List(ctx context.Context, f AuditLogFilter) ([]AuditLogRow, int64, error) {
	q := r.db.WithContext(ctx).Table("audit_logs AS a")
	if f.Action != "" {
		q = q.Where("a.action = ?", f.Action)
	}
	if f.ActorID != nil {
		q = q.Where("a.actor_id = ?", *f.ActorID)
	}
	if f.TargetType != "" {
		q = q.Where("a.target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		q = q.Where("a.target_id = ?", f.TargetID)
	}
	if f.From != nil {
		q = q.Where("a.created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("a.created_at <= ?", *f.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	rows := []AuditLogRow{}
	err := q.
		Select(`a.*, COALESCE(NULLIF(u.full_name, ''), u.user_name) AS actor_name, u.email AS actor_email`).
		Joins("LEFT JOIN users u ON u.id = a.actor_id").
		Order("a.created_at DESC, a.id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
