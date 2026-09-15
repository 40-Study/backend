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
	IsUserRelatedToClass(ctx context.Context, classID, userID uuid.UUID) (bool, error)

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
	GetClassesByTeacher(ctx context.Context, teacherID uuid.UUID) ([]model.Class, error)
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

// StudentClassActiveCondition (D6, issue #58 review vòng 3): "thành viên lớp" (dùng để vào phiên
// livestream/đọc-gửi chat) CHỈ tính học sinh đang HOẠT ĐỘNG trong lớp — status 'dropped'/
// 'completed'/'pending' (model.StudentClass) không còn là thành viên. Dữ liệu cũ trước khi cột có
// default 'active' có thể mang status rỗng/NULL — coi là active (không tự động loại học sinh cũ
// ra khỏi lớp họ đang học chỉ vì thiếu dữ liệu). Dùng chung ở StudentClassExists,
// IsUserRelatedToClass, và LivestreamRepository.GetAll (F-1) để tránh 3 nơi định nghĩa "active"
// khác nhau (xem R2-9: hai định nghĩa "thành viên phiên" đã tồn tại song song, thêm nơi thứ ba tự
// định nghĩa lại là chính cách chúng lệch nhau lặng lẽ).
const StudentClassActiveCondition = "(status = 'active' OR status = '' OR status IS NULL)"

// buildStudentClassExistsQuery (D6/R2-7, issue #58 review vòng 3): tách phần XÂY câu Count ra
// khỏi phần đọc .Error, để test DryRun (class_repository_authz_test.go) gọi được ĐÚNG hàm sản
// xuất thật thay vì hand-roll lại câu query trong test — xoá/sửa sai điều kiện lọc status ở đây
// sẽ làm test đỏ, theo đúng pattern buildRestoreAndReactivateQuery (enrollment_repository.go).
func buildStudentClassExistsQuery(db *gorm.DB, classID, studentID uuid.UUID, count *int64) *gorm.DB {
	return db.Model(&model.StudentClass{}).
		Where("class_id = ? AND student_id = ? AND "+StudentClassActiveCondition, classID, studentID).
		Count(count)
}

func (r *ClassRepository) StudentClassExists(ctx context.Context, classID, studentID uuid.UUID) (bool, error) {
	var count int64
	tx := buildStudentClassExistsQuery(r.db.WithContext(ctx), classID, studentID, &count)
	return count > 0, tx.Error
}

// buildIsUserRelatedToClassQuery (D6/R2-7, cùng lý do với buildStudentClassExistsQuery ở trên).
func buildIsUserRelatedToClassQuery(db *gorm.DB, classID, userID uuid.UUID, exists *bool) *gorm.DB {
	return db.Raw(`
		SELECT EXISTS(
			SELECT 1 FROM teacher_classes WHERE class_id = ? AND teacher_id = ?
			UNION
			SELECT 1 FROM student_classes WHERE class_id = ? AND student_id = ? AND `+StudentClassActiveCondition+`
			UNION
			SELECT 1 FROM classes c JOIN courses co ON co.id = c.course_id
				WHERE c.id = ? AND co.instructor_id = ?
		)
	`, classID, userID, classID, userID, classID, userID).Scan(exists)
}

// IsUserRelatedToClass (F-8, issue #58 review vòng 2): gộp GV lớp / học sinh lớp / instructor
// khoá chứa lớp vào MỘT truy vấn thay vì 3 lời gọi riêng (TeacherClassExists + StudentClassExists
// + course.GetByID) — dùng cho đường kiểm quyền NÓNG (EnsureSessionMember, chạy trên mỗi tin
// nhắn chat/sự kiện bảng trắng), nơi chỉ cần biết CÓ/KHÔNG, không cần phân biệt vai trò chính
// xác như resolveJoinRole.
func (r *ClassRepository) IsUserRelatedToClass(ctx context.Context, classID, userID uuid.UUID) (bool, error) {
	var exists bool
	tx := buildIsUserRelatedToClassQuery(r.db.WithContext(ctx), classID, userID, &exists)
	return exists, tx.Error
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

func (r *ClassRepository) GetClassesByTeacher(ctx context.Context, teacherID uuid.UUID) ([]model.Class, error) {
	var classes []model.Class
	if err := r.db.WithContext(ctx).
		Model(&model.Class{}).
		Joins("JOIN teacher_classes ON teacher_classes.class_id = classes.id").
		Where("teacher_classes.teacher_id = ?", teacherID).
		Order("classes.created_at DESC").
		Find(&classes).Error; err != nil {
		return nil, err
	}
	return classes, nil
}
