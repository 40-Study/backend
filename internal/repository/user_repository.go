package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// ErrAdminUserNotFound (Phase 1 quản lý người dùng, 2026-09-28) — user không tồn tại, dùng
// riêng cho các thao tác admin (list/detail/khoá) để handler map đúng 404.
var ErrAdminUserNotFound = errors.New("user not found")

// AdminUserListFilter — tham số lọc GET /api/users (contract phase-01-user-management.md).
type AdminUserListFilter struct {
	Keyword string // khớp email/user_name/full_name, không phân biệt hoa/thường và dấu
	Role    string // tên SystemRole, join user_system_roles
	Status  string // "active" | "locked" | "self_deleted" -> is_active + self_deleted_at (Q11-ADMIN-03)
	Page    int
	Limit   int
}

type UserRepositoryInterface interface {
	CreateUser(ctx context.Context, user *model.User) error
	FindUserByEmail(ctx context.Context, email string) (*model.User, error)
	// FindActiveUserByEmailCI (QA-C-01): khớp email CHÍNH XÁC không phân biệt hoa thường, chỉ user đang hoạt động (chưa
	// bị khoá is_active=false); user đã xoá mềm bị GORM loại sẵn. nil khi không có.
	FindActiveUserByEmailCI(ctx context.Context, email string) (*model.User, error)
	FindUserByPhone(ctx context.Context, phone string) (*model.User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	// UserNameExists — đã có tài khoản (kể cả đã xoá mềm: tên cũ vẫn có thể còn nằm ở dữ liệu liên quan)
	// dùng user_name này chưa, so không phân biệt hoa thường. Dùng để sinh user_name OAuth không trùng.
	UserNameExists(ctx context.Context, userName string) (bool, error)
	// WithUserNameLock chạy fn trong MỘT transaction giữ pg_advisory_xact_lock(hashtext(lower(userName))): hai thao tác
	// kiểm tra-rồi-ghi cùng một tên (không phân biệt hoa thường) bị tuần tự hoá, vì chưa có unique index trên user_name
	// (seed còn tên trùng). fn nhận repo gắn với transaction; mọi đọc/ghi tên phải đi qua repo đó.
	WithUserNameLock(ctx context.Context, userName string, fn func(tx UserRepositoryInterface) error) error
	UpdateUser(ctx context.Context, user *model.User) error
	UpdateUserProfile(ctx context.Context, userID uuid.UUID, updates map[string]interface{}) error
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newPasswordHash string) error

	// ===== Phase 1 quản lý người dùng (admin) =====
	AdminListUsers(ctx context.Context, filter AdminUserListFilter) ([]model.User, int64, error)
	// ActiveSystemRoleNamesByUserIDs trả về map userID -> danh sách tên SystemRole đang active,
	// dùng để đính kèm "system_roles" vào từng item danh sách (1 query cho cả trang, tránh N+1).
	ActiveSystemRoleNamesByUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	// LockOrUnlockUser khoá/mở khoá 1 tài khoản trong 1 transaction: khoá dòng user (FOR UPDATE),
	// và khi khoá (isActive=false) khoá thêm toàn bộ dòng active của systemAdminRoleID để tuần tự
	// hoá bất biến "luôn còn >=1 SYSTEM_ADMIN đang hoạt động" với các thao tác đồng thời khác
	// (xem evaluateLastSystemAdminGuard, user_admin_guards.go).
	LockOrUnlockUser(
		ctx context.Context,
		targetUserID uuid.UUID,
		isActive bool,
		reason *string,
		actorID uuid.UUID,
		systemAdminRoleID uuid.UUID,
	) (*model.User, error)
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// CreateUser luôn lưu email đã chuẩn hoá (trim + chữ thường): mọi đường tạo tài khoản (đăng ký, OAuth) đi qua đây nên không
// có đường nào ghi được "Parent1@Demo.com" bên cạnh "parent1@demo.com" (QA8 PG-01, C18).
func (r *UserRepository) CreateUser(ctx context.Context, user *model.User) error {
	user.Email = utils.NormalizeEmail(user.Email)
	return r.db.WithContext(ctx).Create(user).Error
}

// FindUserByEmail khớp email KHÔNG phân biệt hoa thường (QA8 PG-01, C18): lower(email) = <email đã chuẩn hoá>, khớp index
// uq_users_email_ci. Hàng cũ lưu chữ hoa vẫn tìm thấy.
func (r *UserRepository) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("lower(email) = ?", utils.NormalizeEmail(email)).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindActiveUserByEmailCI(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("lower(email) = lower(?) AND is_active = ?", email, true).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) UserNameExists(ctx context.Context, userName string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Unscoped().Model(&model.User{}).Where("lower(user_name) = lower(?)", userName).Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *UserRepository) WithUserNameLock(ctx context.Context, userName string, fn func(tx UserRepositoryInterface) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(lower(?)))", strings.TrimSpace(userName)).Error; err != nil {
			return fmt.Errorf("lock user name: %w", err)
		}
		return fn(&UserRepository{db: tx})
	})
}

func (r *UserRepository) FindUserByPhone(ctx context.Context, phone string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&user).Error
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
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// UpdateUserProfile updates only specified fields (partial update)
func (r *UserRepository) UpdateUserProfile(ctx context.Context, userID uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// AdminListUsers — GET /api/users: tìm/lọc/phân trang cho trang quản trị người dùng.
// Lọc theo role JOIN user_system_roles (chỉ status=active) để không đếm role đã bị gỡ.
func (r *UserRepository) AdminListUsers(ctx context.Context, filter AdminUserListFilter) ([]model.User, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&model.User{})

	if filter.Keyword != "" {
		// MINOR đã sửa (review-260928-users-pr72-pr28.md finding #3): ILIKE coi `%`/`_` trong
		// input là wildcard thật (Postgres), nên keyword chứa các ký tự này trả kết quả sai
		// (vd. keyword="%" khớp mọi user). Escape `\` TRƯỚC (chính nó là ký tự escape mặc định
		// của ILIKE), rồi mới escape `%`/`_` — cả 3 cặp thay thế chạy 1 lượt duy nhất qua
		// strings.NewReplacer nên không bị escape lặp lại ký tự vừa chèn. Escaper nay nằm ở
		// utils.ContainsLikePattern (QA vòng 2 G5) để mọi ô tìm kiếm dùng chung một chỗ.
		// QA3-08: thêm bỏ dấu tiếng Việt ("nguyen" ra "Nguyễn") — xem utils.ApplyAccentInsensitiveKeywordSearch.
		query = utils.ApplyAccentInsensitiveKeywordSearch(query, filter.Keyword, "email", "user_name", "full_name")
	}
	// Q11-ADMIN-03: "locked" = admin khoá (không có dấu tự xoá); "self_deleted" = chính chủ xoá (vẫn is_active=false).
	// Hai nhóm rời nhau nên total/phân trang chính xác, không cần tách phía client.
	switch filter.Status {
	case "active":
		query = query.Where("is_active = ?", true)
	case "locked":
		query = query.Where("is_active = ? AND self_deleted_at IS NULL", false)
	case "self_deleted":
		query = query.Where("is_active = ? AND self_deleted_at IS NOT NULL", false)
	}
	if filter.Role != "" {
		query = query.Where(
			"id IN (?)",
			r.db.WithContext(ctx).
				Table("user_system_roles usr").
				Select("usr.user_id").
				Joins("JOIN system_roles sr ON sr.id = usr.system_role_id").
				Where("sr.name = ? AND usr.status = ?", filter.Role, model.UserSystemRoleStatusActive),
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []model.User
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// ActiveSystemRoleNamesByUserIDs xem UserRepositoryInterface.
func (r *UserRepository) ActiveSystemRoleNamesByUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	result := make(map[uuid.UUID][]string, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	type row struct {
		UserID uuid.UUID
		Name   string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("user_system_roles usr").
		Select("usr.user_id, sr.name").
		Joins("JOIN system_roles sr ON sr.id = usr.system_role_id").
		Where("usr.user_id IN ? AND usr.status = ?", userIDs, model.UserSystemRoleStatusActive).
		Order("usr.granted_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], row.Name)
	}
	return result, nil
}

// LockOrUnlockUser xem UserRepositoryInterface.
func (r *UserRepository) LockOrUnlockUser(
	ctx context.Context,
	targetUserID uuid.UUID,
	isActive bool,
	reason *string,
	actorID uuid.UUID,
	systemAdminRoleID uuid.UUID,
) (*model.User, error) {
	var updated model.User

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", targetUserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdminUserNotFound
			}
			return err
		}

		if !isActive {
			// Bất biến: không được khoá SYSTEM_ADMIN active cuối cùng đang hoạt động của hệ
			// thống. SELECT ... FOR UPDATE (không kèm COUNT/aggregate — Postgres từ chối cú
			// pháp FOR UPDATE + aggregate) toàn bộ dòng active của role này để tuần tự hoá với
			// RevokeSystemRoleFromUser (user_system_role_repository.go) đang thao tác trên
			// CÙNG tập dòng — 2 giao dịch đồng thời sẽ tự xếp hàng thay vì cùng đọc "còn 2" rồi
			// cùng tiến hành.
			var adminAssignments []model.UserSystemRole
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("system_role_id = ? AND status = ?", systemAdminRoleID, model.UserSystemRoleStatusActive).
				Find(&adminAssignments).Error; err != nil {
				return err
			}

			isTargetAdmin := false
			adminUserIDs := make([]uuid.UUID, 0, len(adminAssignments))
			for _, a := range adminAssignments {
				adminUserIDs = append(adminUserIDs, a.UserID)
				if a.UserID == targetUserID {
					isTargetAdmin = true
				}
			}

			if isTargetAdmin {
				// Dùng CHUNG fetchIsActiveByUserIDs (user_admin_guards.go) với nhánh gỡ vai trò
				// hệ thống (user_system_role_repository.go::RevokeActiveAssignment) — một công
				// thức duy nhất để đọc is_active thật, tránh 2 nơi tính bất biến này khác nhau
				// (review-260928-users-pr72-pr28.md finding #1). targetUserID vẫn nằm trong tập
				// truy vấn — user vừa SELECT ... FOR UPDATE ở trên nên không có race đọc lại.
				activeStatus, err := fetchIsActiveByUserIDs(tx, adminUserIDs)
				if err != nil {
					return err
				}
				if err := evaluateLastSystemAdminGuard(adminUserIDs, activeStatus, targetUserID); err != nil {
					return err
				}
			}

			user.IsActive = false
			user.LockedReason = reason
			now := time.Now()
			user.LockedAt = &now
			user.LockedBy = &actorID
			// Admin khoá SAU khi chính chủ tự xoá: hành động mới nhất thắng -> đọc là "Đã khoá" (đăng nhập báo bị khoá),
			// không còn dấu tự xoá cũ (#159 L1).
			user.SelfDeletedAt = nil
		} else {
			user.IsActive = true
			user.LockedReason = nil
			user.LockedAt = nil
			user.LockedBy = nil
			user.SelfDeletedAt = nil // mở khoá/kích hoạt lại cũng xoá dấu tự xoá (Q9-PG-05)
		}

		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		updated = user
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newPasswordHash string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"password_hash":       newPasswordHash,
			"password_changed_at": &now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}
