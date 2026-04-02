package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type ClassRepositoryInterface interface {
	Create(ctx context.Context, class *model.Class) error
	GetAll(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.Class, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Class, error)
	Update(ctx context.Context, class *model.Class) error
	Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	GetByCourseID(ctx context.Context, courseID uuid.UUID) ([]model.Class, error)

	// Class relationship checks
	TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error)
	StudentClassExists(ctx context.Context, classID, studentID uuid.UUID) (bool, error)

	// Teacher-Class
	AssignTeacher(ctx context.Context, tc *model.TeacherClass) error
	AssignTeachers(ctx context.Context, tcs []*model.TeacherClass) error
	RemoveTeacher(ctx context.Context, classID, teacherID uuid.UUID) error
	GetTeachers(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.TeacherClass, int64, error)
	GetTeacherCount(ctx context.Context, classID uuid.UUID) (int64, error)

	// Student-Class
	EnrollStudent(ctx context.Context, sc *model.StudentClass) error
	EnrollStudentWithLock(ctx context.Context, sc *model.StudentClass, maxStudents *int) error
	RemoveStudent(ctx context.Context, classID, studentID uuid.UUID) error
	GetStudents(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.StudentClass, int64, error)
	GetStudentCount(ctx context.Context, classID uuid.UUID) (int64, error)
}

type ClassRepository struct {
	db *gorm.DB
}

func NewClassRepository(db *gorm.DB) *ClassRepository {
	return &ClassRepository{db: db}
}

func (r *ClassRepository) Create(ctx context.Context, class *model.Class) error {
	return r.db.WithContext(ctx).Create(class).Error
}

func (r *ClassRepository) GetAll(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.Class, int64, error) {
	var classes []model.Class
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Class{})
	query = utils.ApplySoftDeleteStatus(query, status)
	query = utils.ApplyKeywordSearch(query, keyword, "classes.name", "classes.description")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Order("classes.created_at DESC").
		Find(&classes).Error; err != nil {
		return nil, 0, err
	}

	return classes, total, nil
}

func (r *ClassRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Class, error) {
	var class model.Class
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&class).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &class, nil
}

func (r *ClassRepository) Update(ctx context.Context, class *model.Class) error {
	return r.db.WithContext(ctx).Save(class).Error
}

func (r *ClassRepository) Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("class_id = ?", id).Delete(&model.TeacherClass{}).Error; err != nil {
				return err
			}
			if err := tx.Where("class_id = ?", id).Delete(&model.StudentClass{}).Error; err != nil {
				return err
			}
			if err := tx.Where("class_id = ?", id).Delete(&model.ClassLessonContent{}).Error; err != nil {
				return err
			}
			if err := tx.Where("class_id = ?", id).Delete(&model.Attendance{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&model.Class{}, "id = ?", id).Error
		})
	}
	return r.db.WithContext(ctx).Delete(&model.Class{}, "id = ?", id).Error
}

func (r *ClassRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Class{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *ClassRepository) GetByCourseID(ctx context.Context, courseID uuid.UUID) ([]model.Class, error) {
	var classes []model.Class
	err := r.db.WithContext(ctx).
		Where("course_id = ?", courseID).
		Order("created_at DESC").
		Find(&classes).Error
	return classes, err
}

// Class relationship checks

func (r *ClassRepository) TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.TeacherClass{}).
		Where("class_id = ? AND teacher_id = ?", classID, teacherID).
		Count(&count).Error
	return count > 0, err
}

func (r *ClassRepository) StudentClassExists(ctx context.Context, classID, studentID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.StudentClass{}).
		Where("class_id = ? AND student_id = ?", classID, studentID).
		Count(&count).Error
	return count > 0, err
}

// Teacher-Class

func (r *ClassRepository) AssignTeacher(ctx context.Context, tc *model.TeacherClass) error {
	return r.db.WithContext(ctx).Create(tc).Error
}

func (r *ClassRepository) AssignTeachers(ctx context.Context, tcs []*model.TeacherClass) error {
	if len(tcs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&tcs).Error
}

func (r *ClassRepository) RemoveTeacher(ctx context.Context, classID, teacherID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("class_id = ? AND teacher_id = ?", classID, teacherID).
		Delete(&model.TeacherClass{}).Error
}

func (r *ClassRepository) GetTeachers(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.TeacherClass, int64, error) {
	var teachers []model.TeacherClass
	var total int64

	query := r.db.WithContext(ctx).Model(&model.TeacherClass{}).Where("class_id = ?", classID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Teacher").
		Where("class_id = ?", classID).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Order("assigned_at DESC").
		Find(&teachers).Error

	return teachers, total, err
}

func (r *ClassRepository) GetTeacherCount(ctx context.Context, classID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TeacherClass{}).Where("class_id = ?", classID).Count(&count).Error
	return count, err
}

// Student-Class

func (r *ClassRepository) EnrollStudent(ctx context.Context, sc *model.StudentClass) error {
	return r.db.WithContext(ctx).Create(sc).Error
}

// EnrollStudentWithLock enrolls a student with a transaction lock to prevent race conditions
// when checking MaxStudents limit. This ensures that concurrent enrollments don't exceed the limit.
func (r *ClassRepository) EnrollStudentWithLock(ctx context.Context, sc *model.StudentClass, maxStudents *int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the class row to prevent concurrent enrollments
		var class model.Class
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", sc.ClassID).First(&class).Error; err != nil {
			return err
		}

		// Check current student count
		if maxStudents != nil {
			var count int64
			if err := tx.Model(&model.StudentClass{}).Where("class_id = ?", sc.ClassID).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(*maxStudents) {
				return errors.New("class is full")
			}
		}

		// Enroll the student
		return tx.Create(sc).Error
	})
}

func (r *ClassRepository) RemoveStudent(ctx context.Context, classID, studentID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("class_id = ? AND student_id = ?", classID, studentID).
		Delete(&model.StudentClass{}).Error
}

func (r *ClassRepository) GetStudents(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.StudentClass, int64, error) {
	var students []model.StudentClass
	var total int64

	query := r.db.WithContext(ctx).Model(&model.StudentClass{}).Where("class_id = ?", classID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Student").
		Where("class_id = ?", classID).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Order("enrolled_at DESC").
		Find(&students).Error

	return students, total, err
}

func (r *ClassRepository) GetStudentCount(ctx context.Context, classID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.StudentClass{}).Where("class_id = ?", classID).Count(&count).Error
	return count, err
}
