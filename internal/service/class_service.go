package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ClassServiceInterface interface {
	Create(ctx context.Context, req dto.CreateClassDTO) (*dto.ClassResponseDTO, error)
	GetAll(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.ClassListResponseDTO, error)
	GetByID(ctx context.Context, id uuid.UUID) (*dto.ClassResponseDTO, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateClassDTO) (*dto.ClassResponseDTO, error)
	Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error

	AssignTeacher(ctx context.Context, classID uuid.UUID, req dto.AssignTeacherDTO) (*dto.TeacherClassResponseDTO, error)
	AssignTeachers(ctx context.Context, classID uuid.UUID, req dto.AssignTeachersDTO) ([]dto.TeacherClassResponseDTO, error)
	RemoveTeacher(ctx context.Context, classID, teacherID uuid.UUID) error
	GetTeachers(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.TeacherClassListResponseDTO, error)

	EnrollStudent(ctx context.Context, classID uuid.UUID, req dto.EnrollStudentDTO) (*dto.StudentClassResponseDTO, error)
	RemoveStudent(ctx context.Context, classID, studentID uuid.UUID) error
	GetStudents(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.StudentClassListResponseDTO, error)
}

type ClassService struct {
	classRepo   repository.ClassRepositoryInterface
	courseRepo  repository.CourseRepositoryInterface
	teacherRepo repository.TeacherRepositoryInterface
	studentRepo repository.StudentRepositoryInterface
}

func NewClassService(
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	teacherRepo repository.TeacherRepositoryInterface,
	studentRepo repository.StudentRepositoryInterface,
) *ClassService {
	return &ClassService{
		classRepo:   classRepo,
		courseRepo:  courseRepo,
		teacherRepo: teacherRepo,
		studentRepo: studentRepo,
	}
}

func (s *ClassService) Create(ctx context.Context, req dto.CreateClassDTO) (*dto.ClassResponseDTO, error) {
	// Validate CourseID exists if provided
	if req.CourseID != nil {
		exists, err := s.courseRepo.Exists(ctx, *req.CourseID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("course not found")
		}
	}

	// Class luôn bắt đầu với status "draft".
	// Để chuyển sang "active" (lớp thật), cần gọi API Update với status = "active"
	// khi lớp đã sẵn sàng hoạt động (có đủ giáo viên, lịch học, etc.)
	class := &model.Class{
		Name:        req.Name,
		Description: req.Description,
		CourseID:    req.CourseID,
		Status:      "draft",
		MaxStudents: req.MaxStudents,
	}

	if req.StartDate != nil {
		t, err := time.Parse("2006-01-02", *req.StartDate)
		if err != nil {
			return nil, errors.New("invalid start_date format, expected YYYY-MM-DD")
		}
		class.StartDate = &t
	}
	if req.EndDate != nil {
		t, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			return nil, errors.New("invalid end_date format, expected YYYY-MM-DD")
		}
		class.EndDate = &t
	}

	if err := s.classRepo.Create(ctx, class); err != nil {
		return nil, err
	}

	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) GetAll(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.ClassListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	classes, total, err := s.classRepo.GetAll(ctx, page, pageSize, keyword, status)
	if err != nil {
		return nil, err
	}

	classDTOs := make([]dto.ClassResponseDTO, len(classes))
	for i, c := range classes {
		classDTOs[i] = *s.toClassResponseDTO(ctx, &c)
	}

	return &dto.ClassListResponseDTO{
		Classes:  classDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassService) GetByID(ctx context.Context, id uuid.UUID) (*dto.ClassResponseDTO, error) {
	class, err := s.classRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}
	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateClassDTO) (*dto.ClassResponseDTO, error) {
	class, err := s.classRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Validate CourseID exists if being updated
	if req.CourseID != nil {
		exists, err := s.courseRepo.Exists(ctx, *req.CourseID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("course not found")
		}
		class.CourseID = req.CourseID
	}

	if req.Name != nil {
		class.Name = *req.Name
	}
	if req.Description != nil {
		class.Description = req.Description
	}
	if req.Status != nil {
		class.Status = *req.Status
	}
	if req.MaxStudents != nil {
		class.MaxStudents = req.MaxStudents
	}
	if req.StartDate != nil {
		t, err := time.Parse("2006-01-02", *req.StartDate)
		if err != nil {
			return nil, errors.New("invalid start_date format, expected YYYY-MM-DD")
		}
		class.StartDate = &t
	}
	if req.EndDate != nil {
		t, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			return nil, errors.New("invalid end_date format, expected YYYY-MM-DD")
		}
		class.EndDate = &t
	}

	if err := s.classRepo.Update(ctx, class); err != nil {
		return nil, err
	}

	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	class, err := s.classRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if class == nil {
		return errors.New("class not found")
	}
	return s.classRepo.Delete(ctx, id, hardDelete)
}

// Teacher-Class

// AssignTeacher assigns a single teacher to a class.
// Role can be:
// - "primary": Giáo viên chính, chịu trách nhiệm chính cho lớp học
// - "assistant": Trợ giảng, hỗ trợ giáo viên chính
func (s *ClassService) AssignTeacher(ctx context.Context, classID uuid.UUID, req dto.AssignTeacherDTO) (*dto.TeacherClassResponseDTO, error) {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Check teacher exists
	exists, err := s.teacherRepo.Exists(ctx, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("teacher not found")
	}

	// Check if teacher is already assigned to this class
	alreadyAssigned, err := s.classRepo.TeacherClassExists(ctx, classID, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if alreadyAssigned {
		return nil, errors.New("teacher is already assigned to this class")
	}

	role := req.Role
	if role == "" {
		role = "primary"
	}

	tc := &model.TeacherClass{
		ID:        uuid.New(),
		TeacherID: req.TeacherID,
		ClassID:   classID,
		Role:      role,
	}

	if err := s.classRepo.AssignTeacher(ctx, tc); err != nil {
		return nil, err
	}

	return &dto.TeacherClassResponseDTO{
		ID:         tc.ID,
		TeacherID:  tc.TeacherID,
		ClassID:    tc.ClassID,
		Role:       tc.Role,
		AssignedAt: tc.AssignedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// AssignTeachers assigns multiple teachers to a class at once
func (s *ClassService) AssignTeachers(ctx context.Context, classID uuid.UUID, req dto.AssignTeachersDTO) ([]dto.TeacherClassResponseDTO, error) {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Validate all teachers and check for duplicates
	tcs := make([]*model.TeacherClass, 0, len(req.Teachers))
	for _, t := range req.Teachers {
		// Check teacher exists
		exists, err := s.teacherRepo.Exists(ctx, t.TeacherID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("teacher not found: " + t.TeacherID.String())
		}

		// Check if teacher is already assigned
		alreadyAssigned, err := s.classRepo.TeacherClassExists(ctx, classID, t.TeacherID)
		if err != nil {
			return nil, err
		}
		if alreadyAssigned {
			return nil, errors.New("teacher is already assigned to this class: " + t.TeacherID.String())
		}

		role := t.Role
		if role == "" {
			role = "primary"
		}

		tcs = append(tcs, &model.TeacherClass{
			ID:        uuid.New(),
			TeacherID: t.TeacherID,
			ClassID:   classID,
			Role:      role,
		})
	}

	if err := s.classRepo.AssignTeachers(ctx, tcs); err != nil {
		return nil, err
	}

	result := make([]dto.TeacherClassResponseDTO, len(tcs))
	for i, tc := range tcs {
		result[i] = dto.TeacherClassResponseDTO{
			ID:         tc.ID,
			TeacherID:  tc.TeacherID,
			ClassID:    tc.ClassID,
			Role:       tc.Role,
			AssignedAt: tc.AssignedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return result, nil
}

func (s *ClassService) RemoveTeacher(ctx context.Context, classID, teacherID uuid.UUID) error {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return err
	}
	if class == nil {
		return errors.New("class not found")
	}

	// Check teacher-class assignment exists
	exists, err := s.classRepo.TeacherClassExists(ctx, classID, teacherID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("teacher is not assigned to this class")
	}

	return s.classRepo.RemoveTeacher(ctx, classID, teacherID)
}

func (s *ClassService) GetTeachers(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.TeacherClassListResponseDTO, error) {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	teachers, total, err := s.classRepo.GetTeachers(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]dto.TeacherClassResponseDTO, len(teachers))
	for i, tc := range teachers {
		result[i] = dto.TeacherClassResponseDTO{
			ID:         tc.ID,
			TeacherID:  tc.TeacherID,
			ClassID:    tc.ClassID,
			Role:       tc.Role,
			AssignedAt: tc.AssignedAt.Format("2006-01-02T15:04:05Z"),
			Teacher: &dto.TeacherResponseDTO{
				ID:       tc.Teacher.ID,
				Email:    tc.Teacher.Email,
				UserName: tc.Teacher.UserName,
				FullName: tc.Teacher.FullName,
			},
		}
	}

	return &dto.TeacherClassListResponseDTO{
		Teachers: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Student-Class

func (s *ClassService) EnrollStudent(ctx context.Context, classID uuid.UUID, req dto.EnrollStudentDTO) (*dto.StudentClassResponseDTO, error) {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Check student exists
	exists, err := s.studentRepo.Exists(ctx, req.StudentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("student not found")
	}

	// Check if student is already enrolled
	alreadyEnrolled, err := s.classRepo.StudentClassExists(ctx, classID, req.StudentID)
	if err != nil {
		return nil, err
	}
	if alreadyEnrolled {
		return nil, errors.New("student is already enrolled in this class")
	}

	sc := &model.StudentClass{
		ID:        uuid.New(),
		StudentID: req.StudentID,
		ClassID:   classID,
		Status:    "active",
	}

	// Use EnrollStudentWithLock to prevent race condition when checking MaxStudents
	if err := s.classRepo.EnrollStudentWithLock(ctx, sc, class.MaxStudents); err != nil {
		return nil, err
	}

	return &dto.StudentClassResponseDTO{
		ID:         sc.ID,
		StudentID:  sc.StudentID,
		ClassID:    sc.ClassID,
		EnrolledAt: sc.EnrolledAt.Format("2006-01-02T15:04:05Z"),
		Status:     sc.Status,
	}, nil
}

func (s *ClassService) RemoveStudent(ctx context.Context, classID, studentID uuid.UUID) error {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return err
	}
	if class == nil {
		return errors.New("class not found")
	}

	// Check student-class enrollment exists
	exists, err := s.classRepo.StudentClassExists(ctx, classID, studentID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("student is not enrolled in this class")
	}

	return s.classRepo.RemoveStudent(ctx, classID, studentID)
}

func (s *ClassService) GetStudents(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.StudentClassListResponseDTO, error) {
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	students, total, err := s.classRepo.GetStudents(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]dto.StudentClassResponseDTO, len(students))
	for i, sc := range students {
		result[i] = dto.StudentClassResponseDTO{
			ID:         sc.ID,
			StudentID:  sc.StudentID,
			ClassID:    sc.ClassID,
			EnrolledAt: sc.EnrolledAt.Format("2006-01-02T15:04:05Z"),
			Status:     sc.Status,
			UserName:   sc.Student.UserName,
			Email:      sc.Student.Email,
			FullName:   sc.Student.FullName,
			AvatarURL:  sc.Student.AvatarURL,
		}
	}

	return &dto.StudentClassListResponseDTO{
		Students: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassService) toClassResponseDTO(ctx context.Context, class *model.Class) *dto.ClassResponseDTO {
	teacherCount, _ := s.classRepo.GetTeacherCount(ctx, class.ID)
	studentCount, _ := s.classRepo.GetStudentCount(ctx, class.ID)

	return &dto.ClassResponseDTO{
		ID:           class.ID,
		Name:         class.Name,
		Description:  class.Description,
		CourseID:     class.CourseID,
		Status:       class.Status,
		MaxStudents:  class.MaxStudents,
		StartDate:    class.StartDate,
		EndDate:      class.EndDate,
		TeacherCount: teacherCount,
		StudentCount: studentCount,
		CreatedAt:    class.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    class.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
