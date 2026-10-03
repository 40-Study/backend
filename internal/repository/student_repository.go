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
	// SearchEnrollable (B-12): học viên (role STUDENT, đang hoạt động) chưa là thành viên active của lớp, khớp từ khoá theo
	// TÊN (không theo email: route này phục vụ ô chọn học viên, không được dùng để dò email). Tối đa limit dòng.
	SearchEnrollable(ctx context.Context, classID uuid.UUID, keyword string, limit int) ([]model.User, error)
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

func (r *StudentRepository) SearchEnrollable(ctx context.Context, classID uuid.UUID, keyword string, limit int) ([]model.User, error) {
	var students []model.User
	query := r.studentQuery(ctx).
		Where("users.is_active = ?", true).
		Where("users.id NOT IN (SELECT student_id FROM student_classes WHERE class_id = ? AND "+StudentClassActiveCondition+")", classID)
	query = utils.ApplyKeywordSearch(query, keyword, "users.user_name", "users.full_name")
	err := query.Select("users.*").Order("users.full_name ASC, users.user_name ASC").Limit(limit).Find(&students).Error
	return students, err
}

// GetTeacherStudents (P1 QA 260927 teacher): chuyển sang EnrollmentRepository.GetByInstructor
// — xem enrollment_repository.go. Truy vấn cũ chỉ đếm học viên đã được xếp vào MỘT LỚP
// (student_classes × teacher_classes), luôn trả rỗng cho giáo viên chưa tạo lớp nào dù khoá học
// của họ đã bán được rất nhiều đơn (enrollment không đi qua lớp).
