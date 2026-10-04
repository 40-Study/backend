package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type TeacherRepositoryInterface interface {
	GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.User, int64, error)
	GetTeacherByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	// SearchAssignable (W2-A): ô chọn giảng viên để gán vào lớp. orgID != nil thì chỉ giảng viên là thành viên
	// ACTIVE của tổ chức đó (cùng điều kiện ClassRepository.ActiveOrgMemberExists); nil thì mọi giảng viên.
	SearchAssignable(ctx context.Context, orgID *uuid.UUID, keyword string, limit int) ([]model.User, error)
}

type TeacherRepository struct {
	db *gorm.DB
}

func NewTeacherRepository(db *gorm.DB) *TeacherRepository {
	return &TeacherRepository{db: db}
}

func (r *TeacherRepository) teacherQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_system_roles ON user_system_roles.user_id = users.id").
		Joins("JOIN system_roles ON system_roles.id = user_system_roles.system_role_id").
		Where("system_roles.name = ?", "TEACHER").
		Where("user_system_roles.status = ?", model.UserSystemRoleStatusActive)
}

func (r *TeacherRepository) GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.User, int64, error) {
	var teachers []model.User
	var total int64

	query := r.teacherQuery(ctx)
	query = utils.ApplySoftDeleteStatus(query, status)
	// S6: KHÔNG tìm theo users.email. Route công khai (GET /teachers) trước đây khớp keyword với email nên
	// gõ "ten@gmail.com" (hoặc dò từng tiền tố) là dựng lại được email giảng viên, vô hiệu hoá việc S3 đã
	// bỏ email khỏi DTO. Chỉ tìm theo tên.
	query = utils.ApplyKeywordSearch(query, keyword, "users.user_name", "users.full_name")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Select("users.*").
		Order("users.created_at DESC").
		Find(&teachers).Error; err != nil {
		return nil, 0, err
	}

	return teachers, total, nil
}

func (r *TeacherRepository) SearchAssignable(ctx context.Context, orgID *uuid.UUID, keyword string, limit int) ([]model.User, error) {
	var teachers []model.User
	query := r.teacherQuery(ctx).Where("users.is_active = ?", true)
	if orgID != nil {
		query = query.Where(`users.id IN (SELECT uor.user_id FROM user_organization_roles uor
			JOIN organizations o ON o.id = uor.organization_id AND o.deleted_at IS NULL
			WHERE uor.organization_id = ? AND uor.status = ?)`, *orgID, model.UserOrgRoleStatusActive)
	}
	query = utils.ApplyKeywordSearch(query, keyword, "users.user_name", "users.full_name")
	err := query.Select("users.*").Order("users.full_name ASC, users.user_name ASC").Limit(limit).Find(&teachers).Error
	return teachers, err
}

func (r *TeacherRepository) GetTeacherByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var teacher model.User
	err := r.teacherQuery(ctx).
		Select("users.*").
		Where("users.id = ?", id).
		First(&teacher).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &teacher, nil
}

func (r *TeacherRepository) DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_id = ?", id).Delete(&model.UserSystemRole{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&model.User{}, "id = ?", id).Error
		})
	}
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		Update("is_active", false).Error
}

func (r *TeacherRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.teacherQuery(ctx).
		Where("users.id = ?", id).
		Count(&count).Error
	return count > 0, err
}
