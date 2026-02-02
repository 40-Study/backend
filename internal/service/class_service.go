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
	RemoveTeacher(ctx context.Context, classID, teacherID uuid.UUID) error
	GetTeachers(ctx context.Context, classID uuid.UUID) ([]dto.TeacherClassResponseDTO, error)

	EnrollStudent(ctx context.Context, classID uuid.UUID, req dto.EnrollStudentDTO) (*dto.StudentClassResponseDTO, error)
	RemoveStudent(ctx context.Context, classID, studentID uuid.UUID) error
	GetStudents(ctx context.Context, classID uuid.UUID) ([]dto.StudentClassResponseDTO, error)
}

type ClassService struct {
	repo repository.ClassRepositoryInterface
}

func NewClassService(repo repository.ClassRepositoryInterface) *ClassService {
	return &ClassService{repo: repo}
}

func (s *ClassService) Create(ctx context.Context, req dto.CreateClassDTO) (*dto.ClassResponseDTO, error) {
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

	if err := s.repo.Create(ctx, class); err != nil {
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

	classes, total, err := s.repo.GetAll(ctx, page, pageSize, keyword, status)
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
	class, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}
	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateClassDTO) (*dto.ClassResponseDTO, error) {
	class, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if req.Name != nil {
		class.Name = *req.Name
	}
	if req.Description != nil {
		class.Description = req.Description
	}
	if req.CourseID != nil {
		class.CourseID = req.CourseID
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

	if err := s.repo.Update(ctx, class); err != nil {
		return nil, err
	}

	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	class, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if class == nil {
		return errors.New("class not found")
	}
	return s.repo.Delete(ctx, id, hardDelete)
}

// Teacher-Class

func (s *ClassService) AssignTeacher(ctx context.Context, classID uuid.UUID, req dto.AssignTeacherDTO) (*dto.TeacherClassResponseDTO, error) {
	class, err := s.repo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
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

	if err := s.repo.AssignTeacher(ctx, tc); err != nil {
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

func (s *ClassService) RemoveTeacher(ctx context.Context, classID, teacherID uuid.UUID) error {
	return s.repo.RemoveTeacher(ctx, classID, teacherID)
}

func (s *ClassService) GetTeachers(ctx context.Context, classID uuid.UUID) ([]dto.TeacherClassResponseDTO, error) {
	teachers, err := s.repo.GetTeachers(ctx, classID)
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
	return result, nil
}

// Student-Class

func (s *ClassService) EnrollStudent(ctx context.Context, classID uuid.UUID, req dto.EnrollStudentDTO) (*dto.StudentClassResponseDTO, error) {
	class, err := s.repo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if class.MaxStudents != nil {
		count, err := s.repo.GetStudentCount(ctx, classID)
		if err != nil {
			return nil, err
		}
		if count >= int64(*class.MaxStudents) {
			return nil, errors.New("class is full")
		}
	}

	sc := &model.StudentClass{
		ID:        uuid.New(),
		StudentID: req.StudentID,
		ClassID:   classID,
		Status:    "active",
	}

	if err := s.repo.EnrollStudent(ctx, sc); err != nil {
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
	return s.repo.RemoveStudent(ctx, classID, studentID)
}

func (s *ClassService) GetStudents(ctx context.Context, classID uuid.UUID) ([]dto.StudentClassResponseDTO, error) {
	students, err := s.repo.GetStudents(ctx, classID)
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
	return result, nil
}

func (s *ClassService) toClassResponseDTO(ctx context.Context, class *model.Class) *dto.ClassResponseDTO {
	teacherCount, _ := s.repo.GetTeacherCount(ctx, class.ID)
	studentCount, _ := s.repo.GetStudentCount(ctx, class.ID)

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
