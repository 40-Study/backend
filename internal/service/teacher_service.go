package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type TeacherServiceInterface interface {
	GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.TeacherListResponseDTO, error)
	GetTeacherByID(ctx context.Context, id uuid.UUID) (*dto.TeacherResponseDTO, error)
	GetMyStudents(ctx context.Context, teacherID uuid.UUID, page, pageSize int) (*dto.TeacherStudentListResponseDTO, error)
	DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type TeacherService struct {
	repo              repository.TeacherRepositoryInterface
	enrollmentRepo    repository.EnrollmentRepositoryInterface
	parentStudentRepo repository.ParentStudentRepositoryInterface
}

func NewTeacherService(repo repository.TeacherRepositoryInterface, enrollmentRepo repository.EnrollmentRepositoryInterface, parentStudentRepo repository.ParentStudentRepositoryInterface) TeacherServiceInterface {
	return &TeacherService{repo: repo, enrollmentRepo: enrollmentRepo, parentStudentRepo: parentStudentRepo}
}

func (s *TeacherService) GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.TeacherListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	teachers, total, err := s.repo.GetAllTeachers(ctx, page, pageSize, keyword, status)
	if err != nil {
		return nil, err
	}

	teacherDTOs := make([]dto.TeacherResponseDTO, len(teachers))
	for i, t := range teachers {
		teacherDTOs[i] = *toTeacherResponseDTO(&t)
	}

	return &dto.TeacherListResponseDTO{
		Teachers: teacherDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *TeacherService) GetTeacherByID(ctx context.Context, id uuid.UUID) (*dto.TeacherResponseDTO, error) {
	teacher, err := s.repo.GetTeacherByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if teacher == nil {
		return nil, errors.New("teacher not found")
	}

	return toTeacherResponseDTO(teacher), nil
}

// GetMyStudents (P1 QA 260927 teacher): liệt kê học viên theo ĐƠN GHI DANH khoá học thật của
// giáo viên (enrollments), không còn chỉ đếm học viên đã được xếp vào một lớp — trước bản vá
// này "Quản lý học viên" luôn trống nếu giáo viên chưa tạo lớp, dù khoá đã có hàng nghìn học
// viên mua (xem EnrollmentRepository.GetByInstructor).
func (s *TeacherService) GetMyStudents(ctx context.Context, teacherID uuid.UUID, page, pageSize int) (*dto.TeacherStudentListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	rows, total, err := s.enrollmentRepo.GetByInstructor(ctx, teacherID, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]dto.TeacherStudentDTO, len(rows))
	for i, row := range rows {
		var name string
		if row.FullName != nil && *row.FullName != "" {
			name = *row.FullName
		} else {
			name = row.UserName
		}

		var parentName *string
		var parentPhone *string
		parentRelation, err := s.parentStudentRepo.GetPrimaryParentByStudentID(ctx, row.StudentID)
		if err != nil {
			return nil, err
		}
		if parentRelation != nil && parentRelation.Parent != nil {
			parentName = parentRelation.Parent.FullName
			parentPhone = parentRelation.Parent.Phone
		}

		// Không có cột trạng thái trực tiếp trên enrollments — suy ra "graduated" khi đã hoàn
		// thành (completed_at khác nil), còn lại coi là "active" (đang học).
		status := "active"
		if row.CompletedAt != nil {
			status = "graduated"
		}

		progress, _ := row.ProgressPercent.Float64()
		courseID := row.CourseID
		courseTitle := row.CourseTitle

		result[i] = dto.TeacherStudentDTO{
			ID:          row.StudentID,
			Name:        name,
			Avatar:      row.AvatarURL,
			ParentName:  parentName,
			ParentPhone: parentPhone,
			ClassID:     row.ClassID,
			ClassName:   row.ClassName,
			CourseID:    &courseID,
			CourseName:  &courseTitle,
			Status:      status,
			Progress:    &progress,
			EnrolledAt:  row.EnrolledAt,
		}

		if row.UserName != "" {
			result[i].StudentID = &row.UserName
		}
	}

	return &dto.TeacherStudentListResponseDTO{
		Students: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *TeacherService) DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	teacher, err := s.repo.GetTeacherByID(ctx, id)
	if err != nil {
		return err
	}
	if teacher == nil {
		return errors.New("teacher not found")
	}

	return s.repo.DeleteTeacher(ctx, id, hardDelete)
}

func toTeacherResponseDTO(user *model.User) *dto.TeacherResponseDTO {
	return &dto.TeacherResponseDTO{
		ID:          user.ID,
		UserName:    user.UserName,
		FullName:    user.FullName,
		AvatarURL:   user.AvatarURL,
		Bio:         user.Bio,
		IsVerified:  user.IsVerified,
		IsActive:    user.IsActive,
		CreatedAt:   utils.FormatTimestamp(user.CreatedAt),
		UpdatedAt:   utils.FormatTimestamp(user.UpdatedAt),
	}
}
