package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type TeacherProfileServiceInterface interface {
	CreateTeacherProfile(ctx context.Context, req dto.CreateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error)
	GetAllTeacherProfiles(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.TeacherProfileListResponseDTO, error)
	GetTeacherProfileByID(ctx context.Context, id uuid.UUID) (*dto.TeacherProfileResponseDTO, error)
	UpdateTeacherProfile(ctx context.Context, id uuid.UUID, req dto.UpdateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error)
	DeleteTeacherProfile(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type TeacherProfileService struct {
	repo repository.TeacherProfileRepositoryInterface
}

func NewTeacherProfileService(repo repository.TeacherProfileRepositoryInterface) *TeacherProfileService {
	return &TeacherProfileService{repo: repo}
}

func (s *TeacherProfileService) CreateTeacherProfile(ctx context.Context, req dto.CreateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error) {
	existing, err := s.repo.GetByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("teacher profile already exists for this user")
	}

	profile := &model.TeacherProfile{
		UserID:          req.UserID,
		Specialization:  req.Specialization,
		Education:       req.Education,
		ExperienceYears: req.ExperienceYears,
		CertificateInfo: req.CertificateInfo,
		Department:      req.Department,
	}

	if err := s.repo.Create(ctx, profile); err != nil {
		return nil, err
	}

	return toTeacherProfileResponseDTO(profile), nil
}

func (s *TeacherProfileService) GetAllTeacherProfiles(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.TeacherProfileListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	profiles, total, err := s.repo.GetAll(ctx, page, pageSize, keyword, status)
	if err != nil {
		return nil, err
	}

	profileDTOs := make([]dto.TeacherProfileResponseDTO, len(profiles))
	for i, p := range profiles {
		profileDTOs[i] = *toTeacherProfileResponseDTO(&p)
	}

	return &dto.TeacherProfileListResponseDTO{
		TeacherProfiles: profileDTOs,
		Total:           total,
		Page:            page,
		PageSize:        pageSize,
	}, nil
}

func (s *TeacherProfileService) GetTeacherProfileByID(ctx context.Context, id uuid.UUID) (*dto.TeacherProfileResponseDTO, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, errors.New("teacher profile not found")
	}
	return toTeacherProfileResponseDTO(profile), nil
}

func (s *TeacherProfileService) UpdateTeacherProfile(ctx context.Context, id uuid.UUID, req dto.UpdateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, errors.New("teacher profile not found")
	}

	if req.Specialization != nil {
		profile.Specialization = req.Specialization
	}
	if req.Education != nil {
		profile.Education = req.Education
	}
	if req.ExperienceYears != nil {
		profile.ExperienceYears = req.ExperienceYears
	}
	if req.CertificateInfo != nil {
		profile.CertificateInfo = req.CertificateInfo
	}
	if req.Department != nil {
		profile.Department = req.Department
	}

	if err := s.repo.Update(ctx, profile); err != nil {
		return nil, err
	}

	return toTeacherProfileResponseDTO(profile), nil
}

func (s *TeacherProfileService) DeleteTeacherProfile(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return errors.New("teacher profile not found")
	}
	return s.repo.Delete(ctx, id, hardDelete)
}

func toTeacherProfileResponseDTO(p *model.TeacherProfile) *dto.TeacherProfileResponseDTO {
	return &dto.TeacherProfileResponseDTO{
		ID:              p.ID,
		UserID:          p.UserID,
		Specialization:  p.Specialization,
		Education:       p.Education,
		ExperienceYears: p.ExperienceYears,
		CertificateInfo: p.CertificateInfo,
		Department:      p.Department,
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
