package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type StudentRepositoryInterface interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
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

// GetTeacherStudents (P1 QA 260927 teacher): chuyển sang EnrollmentRepository.GetByInstructor
// — xem enrollment_repository.go. Truy vấn cũ chỉ đếm học viên đã được xếp vào MỘT LỚP
// (student_classes × teacher_classes), luôn trả rỗng cho giáo viên chưa tạo lớp nào dù khoá học
// của họ đã bán được rất nhiều đơn (enrollment không đi qua lớp).
