package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// Giá trị audience của thông báo hệ thống (contract C3). SSOT cho service và repository.
const (
	BroadcastAudienceAll   = "all"
	BroadcastAudienceRoles = "roles"
)

// BroadcastAudienceRepositoryInterface xác định tập người nhận của một đợt thông báo hệ thống.
// Người nhận = tài khoản đang hoạt động (users.is_active) và, với audience=roles, đang giữ ÍT NHẤT MỘT
// trong các vai trò hệ thống được nêu ở trạng thái active (user_system_roles.status).
//
// promotionOptInOnly (M3, review 261009): true khi đợt gửi là loại "promotion" — chỉ giữ người đã BẬT nhận khuyến mãi
// (notification_settings.push_promotions = true). Mặc định cột này là false và người chưa có hàng cài đặt cũng được
// coi là mặc định, tức chưa đồng ý. Phải truyền cùng giá trị cho cả đếm xem trước lẫn gửi thật.
type BroadcastAudienceRepositoryInterface interface {
	// CountRecipients đếm người nhận (dùng cho bước xem trước).
	CountRecipients(ctx context.Context, audience string, roles []string, promotionOptInOnly bool) (int64, error)
	// StreamRecipientIDs trả tối đa limit id người nhận có id > afterID, tăng dần (keyset pagination: mỗi
	// người đúng một lần dù bảng users thay đổi giữa các lượt). afterID = uuid.Nil nghĩa là từ đầu.
	StreamRecipientIDs(ctx context.Context, audience string, roles []string, promotionOptInOnly bool, afterID uuid.UUID, limit int) ([]uuid.UUID, error)
	// ExistingRoleNames trả lại những tên trong names có thật trong system_roles (để service báo UNKNOWN_ROLE).
	ExistingRoleNames(ctx context.Context, names []string) ([]string, error)
}

type BroadcastAudienceRepository struct {
	db *gorm.DB
}

func NewBroadcastAudienceRepository(db *gorm.DB) *BroadcastAudienceRepository {
	return &BroadcastAudienceRepository{db: db}
}

// recipientsQuery dựng truy vấn người nhận. Subquery vai trò lọc deleted_at tường minh vì Table() của GORM
// không tự thêm điều kiện xoá mềm (khác Model()).
func (r *BroadcastAudienceRepository) recipientsQuery(ctx context.Context, audience string, roles []string, promotionOptInOnly bool) *gorm.DB {
	q := r.db.WithContext(ctx).Model(&model.User{}).Where("users.is_active = ?", true)
	if promotionOptInOnly {
		q = q.Where("EXISTS (?)",
			r.db.WithContext(ctx).
				Table("notification_settings ns").
				Select("1").
				Where("ns.user_id = users.id AND ns.push_promotions = ? AND ns.deleted_at IS NULL", true),
		)
	}
	if audience != BroadcastAudienceRoles {
		return q
	}
	return q.Where("users.id IN (?)",
		r.db.WithContext(ctx).
			Table("user_system_roles usr").
			Select("usr.user_id").
			Joins("JOIN system_roles sr ON sr.id = usr.system_role_id AND sr.deleted_at IS NULL").
			Where("sr.name IN ? AND usr.status = ? AND usr.deleted_at IS NULL", roles, model.UserSystemRoleStatusActive),
	)
}

func (r *BroadcastAudienceRepository) CountRecipients(ctx context.Context, audience string, roles []string, promotionOptInOnly bool) (int64, error) {
	var n int64
	if err := r.recipientsQuery(ctx, audience, roles, promotionOptInOnly).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *BroadcastAudienceRepository) StreamRecipientIDs(ctx context.Context, audience string, roles []string, promotionOptInOnly bool, afterID uuid.UUID, limit int) ([]uuid.UUID, error) {
	q := r.recipientsQuery(ctx, audience, roles, promotionOptInOnly)
	if afterID != uuid.Nil {
		q = q.Where("users.id > ?", afterID)
	}
	var ids []uuid.UUID
	if err := q.Order("users.id ASC").Limit(limit).Pluck("users.id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *BroadcastAudienceRepository) ExistingRoleNames(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var found []string
	if err := r.db.WithContext(ctx).Model(&model.SystemRole{}).Where("name IN ?", names).Pluck("name", &found).Error; err != nil {
		return nil, err
	}
	return found, nil
}
