package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// ErrWithdrawalRecordNotFound — không tìm thấy yêu cầu rút / hồ sơ giáo viên.
var ErrWithdrawalRecordNotFound = errors.New("withdrawal record not found")

// WithdrawalRepository — tầng dữ liệu cho yêu cầu rút tiền giảng viên (bảng instructor_payouts,
// Phase 4). Quy tắc nghiệp vụ nằm ở service.WithdrawalService; repo chỉ cung cấp thao tác
// nguyên tử + khoá dòng.
type WithdrawalRepository struct {
	db *gorm.DB
}

func NewWithdrawalRepository(db *gorm.DB) *WithdrawalRepository {
	return &WithdrawalRepository{db: db}
}

// Transaction chạy fn trong 1 transaction; txRepo/txWallet dùng CHUNG transaction đó.
func (r *WithdrawalRepository) Transaction(ctx context.Context, fn func(txRepo *WithdrawalRepository, txWallet *WalletRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewWithdrawalRepository(tx), NewWalletRepository(tx))
	})
}

// LockTeacherProfile — SELECT ... FOR UPDATE đúng 1 dòng teacher_profiles của giảng viên.
//
// Đây là "khoá" tuần tự hoá mọi yêu cầu rút của CÙNG 1 giảng viên: request thứ 2 chờ ở đây tới
// khi request thứ 1 commit, rồi mới đọc số dư/yêu cầu đang mở — nên đọc được yêu cầu vừa tạo.
// Không khoá thẳng trên instructor_payouts vì Postgres cấm FOR UPDATE trong câu có SUM(), và khi
// chưa có yêu cầu nào thì cũng không có dòng nào để khoá.
func (r *WithdrawalRepository) LockTeacherProfile(ctx context.Context, teacherID uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", teacherID).
		First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWithdrawalRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// FindOpenByTeacher trả yêu cầu đang xử lý (pending/approved) mới nhất của giảng viên, nil nếu không có.
func (r *WithdrawalRepository) FindOpenByTeacher(ctx context.Context, teacherID uuid.UUID) (*model.InstructorPayout, error) {
	var p model.InstructorPayout
	err := r.db.WithContext(ctx).
		Where("instructor_id = ? AND status IN ?", teacherID, model.PayoutOpenStatuses).
		Order("created_at DESC").
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *WithdrawalRepository) Create(ctx context.Context, p *model.InstructorPayout) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// LockByID — SELECT ... FOR UPDATE 1 yêu cầu rút, để 2 admin thao tác cùng lúc trên cùng yêu cầu
// được tuần tự hoá (người sau đọc trạng thái MỚI và nhận 409 thay vì ghi đè).
func (r *WithdrawalRepository) LockByID(ctx context.Context, id uuid.UUID) (*model.InstructorPayout, error) {
	var p model.InstructorPayout
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWithdrawalRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateFields cập nhật các cột cho 1 yêu cầu (caller đã khoá dòng + kiểm tra transition).
func (r *WithdrawalRepository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	fields["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&model.InstructorPayout{}).Where("id = ?", id).Updates(fields).Error
}

// WithdrawalListFilter — lọc danh sách. TeacherID != nil giới hạn trong 1 giảng viên (bắt buộc với
// API của giáo viên: không có đường nào để giáo viên xem yêu cầu của người khác).
type WithdrawalListFilter struct {
	TeacherID *uuid.UUID
	Status    string
	Page      int
	Limit     int
}

// List trả trang yêu cầu rút (mới nhất trước) kèm Instructor đã preload.
func (r *WithdrawalRepository) List(ctx context.Context, f WithdrawalListFilter) ([]model.InstructorPayout, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.InstructorPayout{})
	if f.TeacherID != nil {
		q = q.Where("instructor_id = ?", *f.TeacherID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []model.InstructorPayout
	err := q.Preload("Instructor").
		Order("created_at DESC").
		Offset((f.Page - 1) * f.Limit).Limit(f.Limit).
		Find(&items).Error
	return items, total, err
}

// FindUsersByIDs — tên/email giảng viên cho cảnh báo số dư âm.
func (r *WithdrawalRepository) FindUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]model.User, error) {
	var users []model.User
	if len(ids) == 0 {
		return users, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error
	return users, err
}
