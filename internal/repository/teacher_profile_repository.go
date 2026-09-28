package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type TeacherProfileRepositoryInterface interface {
	Create(ctx context.Context, profile *model.TeacherProfile) error
	GetAll(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.TeacherProfile, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.TeacherProfile, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error)
	// UpdateContentFields ghi ĐÚNG các cột nội dung được truyền (review N1) — không bao giờ ghi cột
	// duyệt, để không hoàn tác Approve/Reject/Resubmit commit xen giữa lúc đọc và lúc ghi.
	UpdateContentFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error
	Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error
	// HasActiveSystemRole (Phase 3) — cài đặt ở teacher_application_repository.go.
	HasActiveSystemRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error)
	// GetByUserIDIncludingDeleted + Restore (review PR #73, MAJOR #2): tạo lại hồ sơ sau khi
	// xoá mềm phải khôi phục đúng dòng cũ (giữ resubmission_count), không tạo dòng mới.
	GetByUserIDIncludingDeleted(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error)
	Restore(ctx context.Context, profile *model.TeacherProfile) error
}

type TeacherProfileRepository struct {
	db *gorm.DB
}

func NewTeacherProfileRepository(db *gorm.DB) *TeacherProfileRepository {
	return &TeacherProfileRepository{db: db}
}

func (r *TeacherProfileRepository) Create(ctx context.Context, profile *model.TeacherProfile) error {
	return r.db.WithContext(ctx).Create(profile).Error
}

func (r *TeacherProfileRepository) GetAll(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.TeacherProfile, int64, error) {
	var profiles []model.TeacherProfile
	var total int64

	// GetAll chỉ phục vụ route CÔNG KHAI GET /teacher-profiles — review PR #73 (MAJOR #1): chỉ hồ
	// sơ đã duyệt; đơn đang chờ/bị từ chối xem qua route admin.
	query := r.db.WithContext(ctx).Model(&model.TeacherProfile{}).
		Joins("JOIN users ON users.id = teacher_profiles.user_id").
		Where("teacher_profiles.approval_status = ?", model.TeacherApprovalApproved)
	query = utils.ApplySoftDeleteStatus(query, status)
	query = utils.ApplyKeywordSearch(query, keyword, "users.user_name", "users.email", "teacher_profiles.specialization", "teacher_profiles.department")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Select("teacher_profiles.*").
		Order("teacher_profiles.created_at DESC").
		Find(&profiles).Error; err != nil {
		return nil, 0, err
	}

	return profiles, total, nil
}

func (r *TeacherProfileRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *TeacherProfileRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// ErrTeacherProfileColumnNotEditable: caller cố ghi một cột ngoài danh sách nội dung (vd
// approval_status) qua UpdateContentFields — lỗi lập trình, không phải lỗi người dùng.
var ErrTeacherProfileColumnNotEditable = errors.New("teacher profile column is not editable")

// teacherProfileContentColumns — cột chủ hồ sơ được tự sửa. Cột duyệt (approval_status,
// resubmission_count, rejection_reason, reviewed_*) CHỈ đổi trong transaction có FOR UPDATE ở
// teacher_application_repository.go.
var teacherProfileContentColumns = map[string]struct{}{
	"specialization": {}, "education": {}, "experience_years": {}, "certificate_info": {}, "department": {},
	"bank_name": {}, "bank_account_number": {}, "bank_account_name": {},
}

// UpdateContentFields — review N1 (review-260928-phase3-pr73-pr30.md): trước đây Update = Save cả
// dòng đã đọc từ trước, không khoá. Approve/Reject/Resubmit commit trong khoảng đọc-ghi bị ghi đè
// bằng giá trị cũ (hồ sơ quay về pending khi user đã giữ TEACHER, lời từ chối bị hoàn tác, lượt
// nộp lại biến mất). UPDATE chỉ các cột nội dung thì 2 bên không còn giẫm cột của nhau; bản thân
// câu UPDATE vẫn chờ row lock của transaction duyệt nếu đang chạy.
func (r *TeacherProfileRepository) UpdateContentFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	for column := range fields {
		if _, ok := teacherProfileContentColumns[column]; !ok {
			return fmt.Errorf("%w: %s", ErrTeacherProfileColumnNotEditable, column)
		}
	}
	result := r.db.WithContext(ctx).Model(&model.TeacherProfile{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// Hồ sơ bị xoá mềm giữa lúc đọc và lúc ghi.
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *TeacherProfileRepository) Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Unscoped().Delete(&model.TeacherProfile{}, "id = ?", id).Error
	}
	return r.db.WithContext(ctx).Delete(&model.TeacherProfile{}, "id = ?", id).Error
}
