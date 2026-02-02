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

type TeacherServiceInterface interface {
	GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.TeacherListResponseDTO, error)
	GetTeacherByID(ctx context.Context, id uuid.UUID) (*dto.TeacherResponseDTO, error)
	UpdateTeacher(ctx context.Context, id uuid.UUID, req dto.UpdateTeacherDTO) (*dto.TeacherResponseDTO, error)
	DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type TeacherService struct {
	repo repository.TeacherRepositoryInterface
}

func NewTeacherService(repo repository.TeacherRepositoryInterface) *TeacherService {
	return &TeacherService{repo: repo}
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

func (s *TeacherService) UpdateTeacher(ctx context.Context, id uuid.UUID, req dto.UpdateTeacherDTO) (*dto.TeacherResponseDTO, error) {
	teacher, err := s.repo.GetTeacherByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if teacher == nil {
		return nil, errors.New("teacher not found")
	}

	if req.UserName != nil {
		teacher.UserName = *req.UserName
	}
	if req.FullName != nil {
		teacher.FullName = req.FullName
	}
	if req.Phone != nil {
		teacher.Phone = req.Phone
	}
	if req.Bio != nil {
		teacher.Bio = req.Bio
	}
	if req.DateOfBirth != nil {
		dob, err := time.Parse("2006-01-02", *req.DateOfBirth)
		if err != nil {
			return nil, errors.New("invalid date_of_birth format, expected YYYY-MM-DD")
		}
		teacher.DateOfBirth = &dob
	}

	if err := s.repo.UpdateTeacher(ctx, teacher); err != nil {
		return nil, err
	}

	return toTeacherResponseDTO(teacher), nil
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
		Email:       user.Email,
		UserName:    user.UserName,
		FullName:    user.FullName,
		AvatarURL:   user.AvatarURL,
		Phone:       user.Phone,
		Bio:         user.Bio,
		DateOfBirth: user.DateOfBirth,
		IsVerified:  user.IsVerified,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
