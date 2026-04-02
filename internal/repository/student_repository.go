package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type StudentRepositoryInterface interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	GetTeacherStudents(ctx context.Context, teacherID uuid.UUID, page, pageSize int) ([]model.StudentClass, int64, error)
}

type StudentRepository struct {
	db *gorm.DB
}

func NewStudentRepository(db *gorm.DB) *StudentRepository {
	return &StudentRepository{db: db}
}

func (r *StudentRepository) studentQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_system_roles ON user_system_roles.user_id = users.id").
		Joins("JOIN system_roles ON system_roles.id = user_system_roles.system_role_id").
		Where("system_roles.name = ?", "STUDENT").
		Where("user_system_roles.status = ?", model.UserSystemRoleStatusActive)
}

func (r *StudentRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.studentQuery(ctx).
		Where("users.id = ?", id).
		Count(&count).Error
	return count > 0, err
}

func (r *StudentRepository) GetTeacherStudents(ctx context.Context, teacherID uuid.UUID, page, pageSize int) ([]model.StudentClass, int64, error) {
	var students []model.StudentClass
	var total int64

	baseQuery := r.db.WithContext(ctx).
		Model(&model.StudentClass{}).
		Joins("JOIN teacher_classes ON teacher_classes.class_id = student_classes.class_id").
		Where("teacher_classes.teacher_id = ?", teacherID)

	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := r.db.WithContext(ctx).
		Model(&model.StudentClass{}).
		Joins("JOIN teacher_classes ON teacher_classes.class_id = student_classes.class_id").
		Where("teacher_classes.teacher_id = ?", teacherID).
		Preload("Student").
		Preload("Class").
		Preload("Class.Course")

	if err := utils.ApplyPagination(query, page, pageSize).
		Order("student_classes.enrolled_at DESC").
		Find(&students).Error; err != nil {
		return nil, 0, err
	}

	return students, total, nil
}
