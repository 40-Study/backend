package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrNotTeacherProfileOwner: C-05 (audit 260909) — trước đây UpdateTeacherProfile/
// DeleteTeacherProfile không kiểm tra ai gọi, ẩn danh cũng sửa/xóa được hồ sơ giáo viên bất kỳ.
var ErrNotTeacherProfileOwner = errors.New("forbidden: not the owner")

type TeacherProfileServiceInterface interface {
	CreateTeacherProfile(ctx context.Context, req dto.CreateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error)
	GetAllTeacherProfiles(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.TeacherProfileListResponseDTO, error)
	GetTeacherProfileByID(ctx context.Context, id uuid.UUID) (*dto.TeacherProfileResponseDTO, error)
	UpdateTeacherProfile(ctx context.Context, id, actorUserID uuid.UUID, req dto.UpdateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error)
	DeleteTeacherProfile(ctx context.Context, id, actorUserID uuid.UUID, hardDelete bool) error
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

	// Phase 3: hồ sơ mới = đơn đăng ký chờ duyệt (pending), TRỪ khi user đã là TEACHER (giáo
	// viên cũ tạo hồ sơ muộn) — không đẩy họ vào hàng chờ duyệt.
	isTeacher, err := s.repo.HasActiveSystemRole(ctx, req.UserID, "TEACHER")
	if err != nil {
		return nil, err
	}
	approvalStatus := model.TeacherApprovalPending
	if isTeacher {
		approvalStatus = model.TeacherApprovalApproved
	}

	profile := &model.TeacherProfile{
		UserID:          req.UserID,
		Specialization:  req.Specialization,
		Education:       req.Education,
		ExperienceYears: req.ExperienceYears,
		CertificateInfo: req.CertificateInfo,
		Department:      req.Department,
		ApprovalStatus:  approvalStatus,
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

func (s *TeacherProfileService) UpdateTeacherProfile(ctx context.Context, id, actorUserID uuid.UUID, req dto.UpdateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, errors.New("teacher profile not found")
	}
	if profile.UserID != actorUserID {
		return nil, ErrNotTeacherProfileOwner
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

func (s *TeacherProfileService) DeleteTeacherProfile(ctx context.Context, id, actorUserID uuid.UUID, hardDelete bool) error {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return errors.New("teacher profile not found")
	}
	if profile.UserID != actorUserID {
		return ErrNotTeacherProfileOwner
	}
	return s.repo.Delete(ctx, id, hardDelete)
}

func toTeacherProfileResponseDTO(p *model.TeacherProfile) *dto.TeacherProfileResponseDTO {
	var reviewedAt *string
	if p.ReviewedAt != nil {
		s := p.ReviewedAt.Format("2006-01-02T15:04:05Z07:00")
		reviewedAt = &s
	}
	return &dto.TeacherProfileResponseDTO{
		ApprovalStatus:    p.ApprovalStatus,
		RejectionReason:   p.RejectionReason,
		ReviewedAt:        reviewedAt,
		ResubmissionCount: p.ResubmissionCount,
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
