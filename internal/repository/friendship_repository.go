package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// eligibleStudentSQL — điều kiện (alias bảng users là `u`) để một tài khoản được tham gia tính năng
// Bạn bè: đang hoạt động, có vai trò hệ thống STUDENT và KHÔNG mang bất kỳ vai trò hệ thống active nào khác
// (ALLOWLIST, không phải denylist TEACHER/PARENT/SYSTEM_ADMIN: vai trò tạo thêm qua CreateSystemRole mà người
// dùng mang kèm STUDENT cũng bị loại).
// Q1: chỉ học viên kết bạn với học viên (trẻ vị thành niên: không mở kênh cho người lớn). Chuỗi là hằng
// compile-time.
const eligibleStudentSQL = `u.is_active = TRUE AND u.deleted_at IS NULL
	AND EXISTS (SELECT 1 FROM user_system_roles usr JOIN system_roles sr ON sr.id = usr.system_role_id
		WHERE usr.user_id = u.id AND usr.status = 'active' AND usr.deleted_at IS NULL
		AND sr.deleted_at IS NULL AND sr.name = 'STUDENT')
	AND NOT EXISTS (SELECT 1 FROM user_system_roles usr JOIN system_roles sr ON sr.id = usr.system_role_id
		WHERE usr.user_id = u.id AND usr.status = 'active' AND usr.deleted_at IS NULL
		AND sr.deleted_at IS NULL AND sr.name IN ('TEACHER', 'SYSTEM_ADMIN'))`

// FriendUserRow — cột công khai của một học viên (KHÔNG có email/phone).
type FriendUserRow struct {
	UserID    uuid.UUID `gorm:"column:user_id"`
	UserName  string    `gorm:"column:user_name"`
	FullName  *string   `gorm:"column:full_name"`
	AvatarURL *string   `gorm:"column:avatar_url"`
}

// FriendRow — một dòng danh sách bạn bè.
type FriendRow struct {
	FriendUserRow
	FriendshipID uuid.UUID `gorm:"column:friendship_id"`
	Since        time.Time `gorm:"column:since"`
}

// FriendRequestRow — một dòng danh sách lời mời.
type FriendRequestRow struct {
	FriendUserRow
	ID        uuid.UUID `gorm:"column:id"`
	Direction string    `gorm:"column:direction"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// FriendSearchRow — kết quả tìm kiếm (chưa gắn relationship).
type FriendSearchRow = FriendUserRow

// emailPrefixUserNameSQL — user_name của tài khoản đăng ký bằng Google được sinh từ phần trước '@' của email
// (thirdparty/oauth), nên hiển thị nó là lộ một phần email. CHỈ che khi tài khoản có liên kết Google VÀ user_name
// trùng phần trước '@' (không che nhầm người tự đặt user_name giống prefix email).
const emailPrefixUserNameSQL = `(lower(u.user_name) = lower(split_part(u.email, '@', 1))
	AND EXISTS (SELECT 1 FROM user_oauth_providers op WHERE op.user_id = u.id AND op.provider = 'google'))`

// friendUserColumns — cột công khai của một học viên; user_name được che bằng emailPrefixUserNameSQL ở MỌI
// response bạn bè (gửi lời mời, danh sách lời mời/chặn/bạn, tìm kiếm) qua một chỗ duy nhất.
const friendUserColumns = "u.id AS user_id, CASE WHEN " + emailPrefixUserNameSQL + " THEN '' ELSE u.user_name END AS user_name, u.full_name, u.avatar_url"

type FriendshipRepository struct {
	db *gorm.DB
}

func NewFriendshipRepository(db *gorm.DB) *FriendshipRepository {
	return &FriendshipRepository{db: db}
}

// Transaction chạy fn với một repository gắn transaction; lỗi trả về từ fn sẽ rollback.
func (r *FriendshipRepository) Transaction(ctx context.Context, fn func(tx *FriendshipRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&FriendshipRepository{db: tx})
	})
}

// LockUsers giữ advisory lock (đến hết transaction) cho từng user, theo thứ tự id tăng dần để hai
// request chéo nhau (A->B và B->A) không deadlock. Dùng advisory lock thay vì khoá dòng users để
// không tranh chấp với các luồng khác đang cập nhật users (đăng nhập...). Serialize được: đếm hạn
// mức (20/24h, 30 chờ, 500 bạn) rồi ghi, và hai người gửi cho nhau cùng lúc.
func (r *FriendshipRepository) LockUsers(ctx context.Context, ids ...uuid.UUID) error {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = id.String()
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "friendship:"+k).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *FriendshipRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Friendship, error) {
	var f model.Friendship
	err := r.db.WithContext(ctx).First(&f, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &f, err
}

// FindByPair trả dòng của cặp (a,b) bất kể chiều gửi; nil nếu chưa có.
func (r *FriendshipRepository) FindByPair(ctx context.Context, a, b uuid.UUID) (*model.Friendship, error) {
	var f model.Friendship
	err := r.db.WithContext(ctx).
		Where("(requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?)", a, b, b, a).
		First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &f, err
}

func (r *FriendshipRepository) Create(ctx context.Context, f *model.Friendship) error {
	return r.db.WithContext(ctx).Create(f).Error
}

// Save ghi đè mọi cột của dòng (dùng khi chuyển trạng thái / gửi lại lời mời).
func (r *FriendshipRepository) Save(ctx context.Context, f *model.Friendship) error {
	return r.db.WithContext(ctx).Save(f).Error
}

// DeleteAcceptedPair xoá dòng ACCEPTED của cặp; trả số dòng bị xoá.
func (r *FriendshipRepository) DeleteAcceptedPair(ctx context.Context, a, b uuid.UUID) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("status = ? AND ((requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?))",
			model.FriendshipStatusAccepted, a, b, b, a).
		Delete(&model.Friendship{})
	return res.RowsAffected, res.Error
}

// EndActiveRelation kết thúc quan hệ ĐANG HIỆU LỰC của cặp khi chặn: dòng ACCEPTED bị xoá, dòng PENDING
// chuyển CANCELLED (giữ requested_at để hạn mức 20/24h còn đếm, đặt responded_at để cooldown thu hồi chạy).
// Dòng DECLINED/CANCELLED giữ nguyên: chúng là lịch sử chống spam (cooldown 7 ngày/1 giờ và hạn mức), xoá đi
// thì "chặn rồi bỏ chặn" lách được cả hai.
func (r *FriendshipRepository) EndActiveRelation(ctx context.Context, a, b uuid.UUID, now time.Time) error {
	pair := "((requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?))"
	if err := r.db.WithContext(ctx).
		Where("status = ? AND "+pair, model.FriendshipStatusAccepted, a, b, b, a).
		Delete(&model.Friendship{}).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("status = ? AND "+pair, model.FriendshipStatusPending, a, b, b, a).
		Updates(map[string]any{"status": model.FriendshipStatusCancelled, "responded_at": now}).Error
}

func (r *FriendshipRepository) CountFriends(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("status = ? AND (requester_id = ? OR addressee_id = ?)", model.FriendshipStatusAccepted, userID, userID).
		Count(&n).Error
	return n, err
}

func (r *FriendshipRepository) CountIncomingPending(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("status = ? AND addressee_id = ?", model.FriendshipStatusPending, userID).Count(&n).Error
	return n, err
}

func (r *FriendshipRepository) CountOutgoingPending(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("status = ? AND requester_id = ?", model.FriendshipStatusPending, userID).Count(&n).Error
	return n, err
}

// CountRequestedSince đếm lời mời userID đã gửi từ `since` (mọi trạng thái, kể cả đã thu hồi/từ chối:
// đó chính là lý do FriendshipStatusCancelled tồn tại). excludeID bỏ một dòng khỏi phép đếm — dòng sắp
// bị ghi đè khi gửi lại thì không tính hai lần.
func (r *FriendshipRepository) CountRequestedSince(ctx context.Context, userID uuid.UUID, since time.Time, excludeID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("requester_id = ? AND requested_at > ? AND id <> ?", userID, since, excludeID).Count(&n).Error
	return n, err
}

// IsEligibleStudent — người dùng có được tham gia tính năng Bạn bè không (xem eligibleStudentSQL).
func (r *FriendshipRepository) IsEligibleStudent(ctx context.Context, userID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("users u").
		Where("u.id = ? AND "+eligibleStudentSQL, userID).Count(&n).Error
	return n > 0, err
}

// FindEligibleTarget đọc học viên đích. requireVisible=true loại người đặt hồ sơ `hidden` (đích của lời
// mời/tìm kiếm: người ẩn hồ sơ không muốn bị tìm thấy). Không hợp lệ → nil.
func (r *FriendshipRepository) FindEligibleTarget(ctx context.Context, userID uuid.UUID, requireVisible bool) (*FriendUserRow, error) {
	q := r.db.WithContext(ctx).Table("users u").Select(friendUserColumns).
		Where("u.id = ? AND "+eligibleStudentSQL, userID)
	if requireVisible {
		q = q.Where(notHiddenSQL)
	}
	var row FriendUserRow
	res := q.Limit(1).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// FindActiveUser đọc một tài khoản còn hoạt động, KHÔNG xét vai trò (dùng cho Chặn/Bỏ chặn/Huỷ kết bạn).
func (r *FriendshipRepository) FindActiveUser(ctx context.Context, userID uuid.UUID) (*FriendUserRow, error) {
	var row FriendUserRow
	res := r.db.WithContext(ctx).Table("users u").Select(friendUserColumns).
		Where("u.id = ? AND u.is_active = TRUE AND u.deleted_at IS NULL", userID).Limit(1).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

const notHiddenSQL = `NOT EXISTS (SELECT 1 FROM user_preferences up WHERE up.user_id = u.id AND up.profile_visibility = 'hidden')`

// Từ khoá tìm kiếm đi qua escapeLike (contest_repository.go) để "50%" hay "a_b" tìm đúng chuỗi đã gõ.

// SearchStudents — tìm học viên theo tên/họ tên. Loại chính mình, người ẩn hồ sơ, người có block hai chiều.
// Sắp theo tên đăng nhập để kết quả ổn định.
func (r *FriendshipRepository) SearchStudents(ctx context.Context, viewerID uuid.UUID, keyword string, limit int) ([]FriendSearchRow, error) {
	like := "%" + escapeLike(keyword) + "%"
	var rows []FriendSearchRow
	// Tài khoản đăng ký bằng Google có user_name = phần trước '@' của email (thirdparty/oauth): khớp hay trả giá
	// trị đó là lộ một phần email và cho phép quét danh bạ theo tiền tố email. Với tài khoản đó tìm kiếm chỉ
	// khớp theo họ tên và user_name trả rỗng (web hiển thị full_name).
	emailDerived := emailPrefixUserNameSQL
	err := r.db.WithContext(ctx).Table("users u").
		Select(friendUserColumns).
		Where("u.id <> ? AND "+eligibleStudentSQL+" AND "+notHiddenSQL, viewerID).
		Where(`((NOT `+emailDerived+` AND u.user_name ILIKE ? ESCAPE '\') OR u.full_name ILIKE ? ESCAPE '\')`, like, like).
		Where(`NOT EXISTS (SELECT 1 FROM user_blocks b WHERE (b.blocker_id = ? AND b.blocked_id = u.id) OR (b.blocker_id = u.id AND b.blocked_id = ?))`, viewerID, viewerID).
		Order("u.user_name ASC, u.id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// RelationsWith trả các dòng friendships giữa viewerID và từng id trong others (một truy vấn).
func (r *FriendshipRepository) RelationsWith(ctx context.Context, viewerID uuid.UUID, others []uuid.UUID) ([]model.Friendship, error) {
	if len(others) == 0 {
		return nil, nil
	}
	var rows []model.Friendship
	err := r.db.WithContext(ctx).
		Where("(requester_id = ? AND addressee_id IN ?) OR (addressee_id = ? AND requester_id IN ?)", viewerID, others, viewerID, others).
		Find(&rows).Error
	return rows, err
}

// ListFriends — danh sách bạn (ACCEPTED) của userID, mới kết bạn trước. keyword lọc theo tên (tuỳ chọn).
func (r *FriendshipRepository) ListFriends(ctx context.Context, userID uuid.UUID, keyword string, page, limit int) ([]FriendRow, int64, error) {
	base := r.db.WithContext(ctx).Table("friendships f").
		Joins("JOIN users u ON u.id = CASE WHEN f.requester_id = ? THEN f.addressee_id ELSE f.requester_id END", userID).
		Where("f.status = ? AND (f.requester_id = ? OR f.addressee_id = ?)", model.FriendshipStatusAccepted, userID, userID).
		Where("u.is_active = TRUE AND u.deleted_at IS NULL")
	if keyword != "" {
		like := "%" + escapeLike(keyword) + "%"
		base = base.Where(`(u.user_name ILIKE ? ESCAPE '\' OR u.full_name ILIKE ? ESCAPE '\')`, like, like)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []FriendRow
	err := base.Select(friendUserColumns + ", f.id AS friendship_id, COALESCE(f.responded_at, f.updated_at) AS since").
		Order("since DESC, u.user_name ASC").Offset((page - 1) * limit).Limit(limit).Scan(&rows).Error
	return rows, total, err
}

// ListRequests — lời mời PENDING; direction "incoming" (gửi cho userID) hoặc "outgoing" (userID gửi đi).
func (r *FriendshipRepository) ListRequests(ctx context.Context, userID uuid.UUID, outgoing bool, page, limit int) ([]FriendRequestRow, int64, error) {
	self, other, direction := "addressee_id", "requester_id", "incoming"
	if outgoing {
		self, other, direction = "requester_id", "addressee_id", "outgoing"
	}
	base := r.db.WithContext(ctx).Table("friendships f").
		Joins("JOIN users u ON u.id = f."+other).
		Where("f.status = ? AND f."+self+" = ?", model.FriendshipStatusPending, userID).
		Where("u.is_active = TRUE AND u.deleted_at IS NULL")
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []FriendRequestRow
	// direction là một trong hai literal compile-time ở trên, nối chuỗi an toàn (tham số ? trong SELECT
	// khiến Postgres không suy được kiểu).
	err := base.Select(friendUserColumns + ", f.id AS id, '" + direction + "' AS direction, f.requested_at AS created_at").
		Order("f.requested_at DESC, f.id ASC").Offset((page - 1) * limit).Limit(limit).Scan(&rows).Error
	return rows, total, err
}
