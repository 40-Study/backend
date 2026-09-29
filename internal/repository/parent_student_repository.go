package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ParentStudentRepositoryInterface interface {
	// GetChildrenByParentID lấy danh sách con của phụ huynh với pagination
	GetChildrenByParentID(ctx context.Context, parentID uuid.UUID, page, pageSize int) ([]model.ParentStudentRelation, int64, error)
	// GetParentsByStudentID lấy danh sách phụ huynh của học sinh
	GetParentsByStudentID(ctx context.Context, studentID uuid.UUID) ([]model.ParentStudentRelation, error)
	GetPrimaryParentByStudentID(ctx context.Context, studentID uuid.UUID) (*model.ParentStudentRelation, error)
	// CreateRelation tạo quan hệ phụ huynh - học sinh mới
	CreateRelation(ctx context.Context, relation *model.ParentStudentRelation) error
	// UpdateStatus cập nhật trạng thái quan hệ
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	// SaveRelation ghi đè toàn bộ dòng quan hệ đã có (kích hoạt lại với quyền đặt lại từ đầu)
	SaveRelation(ctx context.Context, relation *model.ParentStudentRelation) error
	// RunLocked chạy fn trong một transaction đã giữ khoá tư vấn `lockKey`; fn nhận repo gắn với
	// transaction đó. Dùng để hai luồng tạo quan hệ (lời mời cũ và yêu cầu mới) không đua nhau.
	RunLocked(ctx context.Context, lockKey string, fn func(tx ParentStudentRepositoryInterface) error) error
	// FindByID tìm quan hệ theo ID
	FindByID(ctx context.Context, id uuid.UUID) (*model.ParentStudentRelation, error)
	// FindByParentAndStudent tìm quan hệ theo parent và student
	FindByParentAndStudent(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentStudentRelation, error)
}

type ParentStudentRepository struct {
	db *gorm.DB
}

func NewParentStudentRepository(db *gorm.DB) *ParentStudentRepository {
	return &ParentStudentRepository{db: db}
}

// GetChildrenByParentID lấy danh sách con của phụ huynh với pagination và preload Student
func (r *ParentStudentRepository) GetChildrenByParentID(
	ctx context.Context,
	parentID uuid.UUID,
	page, pageSize int,
) ([]model.ParentStudentRelation, int64, error) {
	var relations []model.ParentStudentRelation
	var total int64

	offset := (page - 1) * pageSize

	baseQuery := r.db.WithContext(ctx).
		Model(&model.ParentStudentRelation{}).
		Where("parent_user_id = ? AND status = ?", parentID, model.ParentStudentStatusActive)

	// Count total
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results with Student preloaded
	err := r.db.WithContext(ctx).
		Where("parent_user_id = ? AND status = ?", parentID, model.ParentStudentStatusActive).
		Preload("Student").
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&relations).Error

	return relations, total, err
}

// GetParentsByStudentID lấy danh sách phụ huynh của học sinh
func (r *ParentStudentRepository) GetParentsByStudentID(ctx context.Context, studentID uuid.UUID) ([]model.ParentStudentRelation, error) {
	var relations []model.ParentStudentRelation
	err := r.db.WithContext(ctx).
		Preload("Parent").
		Where("student_user_id = ? AND status = ?", studentID, model.ParentStudentStatusActive).
		Order("created_at DESC").
		Find(&relations).Error
	return relations, err
}

func (r *ParentStudentRepository) GetPrimaryParentByStudentID(ctx context.Context, studentID uuid.UUID) (*model.ParentStudentRelation, error) {
	var relation model.ParentStudentRelation
	err := r.db.WithContext(ctx).
		Preload("Parent").
		Where("student_user_id = ? AND status = ?", studentID, model.ParentStudentStatusActive).
		Order("created_at ASC").
		First(&relation).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &relation, err
}

// CreateRelation tạo quan hệ phụ huynh - học sinh mới
func (r *ParentStudentRepository) CreateRelation(ctx context.Context, relation *model.ParentStudentRelation) error {
	return r.db.WithContext(ctx).Create(relation).Error
}

// RunLocked — review PR #81 vòng 2, B-2: khoá tư vấn cùng chuỗi khoá với
// ParentLinkRequestRepository.LockKey, nên luồng lời mời cũ và luồng yêu cầu mới tuần tự hoá
// theo cặp phụ huynh–con (khoá tự nhả khi COMMIT/ROLLBACK).
func (r *ParentStudentRepository) RunLocked(ctx context.Context, lockKey string, fn func(tx ParentStudentRepositoryInterface) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey).Error; err != nil {
			return err
		}
		return fn(&ParentStudentRepository{db: tx})
	})
}

// SaveRelation ghi đè toàn bộ dòng quan hệ đã có (review PR #81, MINOR-6).
func (r *ParentStudentRepository) SaveRelation(ctx context.Context, relation *model.ParentStudentRelation) error {
	return r.db.WithContext(ctx).Save(relation).Error
}

// UpdateStatus cập nhật trạng thái quan hệ
func (r *ParentStudentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.ParentStudentRelation{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// FindByID tìm quan hệ theo ID
func (r *ParentStudentRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.ParentStudentRelation, error) {
	var relation model.ParentStudentRelation
	err := r.db.WithContext(ctx).
		Preload("Parent").
		Preload("Student").
		Where("id = ?", id).
		First(&relation).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &relation, err
}

// FindByParentAndStudent tìm quan hệ theo parent và student
func (r *ParentStudentRepository) FindByParentAndStudent(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentStudentRelation, error) {
	var relation model.ParentStudentRelation
	err := r.db.WithContext(ctx).
		Where("parent_user_id = ? AND student_user_id = ?", parentID, studentID).
		First(&relation).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &relation, err
}

// HasActiveParent (S2): parentID là phụ huynh có liên kết ACTIVE của studentID (quyền xem bài làm
// quiz của con). Trạng thái khác active (pending/revoked...) không được tính.
func (r *ParentStudentRepository) HasActiveParent(ctx context.Context, parentID, studentID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ParentStudentRelation{}).
		Where("parent_user_id = ? AND student_user_id = ? AND status = ?", parentID, studentID, model.ParentStudentRelationStatusActive).
		Count(&n).Error
	return n > 0, err
}