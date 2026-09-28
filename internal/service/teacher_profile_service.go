package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	GetTeacherProfileByID(ctx context.Context, id uuid.UUID) (*dto.PublicTeacherProfileDTO, error)
	UpdateTeacherProfile(ctx context.Context, id, actorUserID uuid.UUID, req dto.UpdateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error)
	DeleteTeacherProfile(ctx context.Context, id, actorUserID uuid.UUID, hardDelete bool) error
}

type TeacherProfileService struct {
	repo repository.TeacherProfileRepositoryInterface
}

func NewTeacherProfileService(repo repository.TeacherProfileRepositoryInterface) *TeacherProfileService {
	return &TeacherProfileService{repo: repo}
}

// Review PR #73 (MAJOR #2): 2 lỗi cho chủ hồ sơ tự xoá — xem DeleteTeacherProfile.
var (
	ErrTeacherProfileHardDeleteForbidden = errors.New("forbidden: teacher profile cannot be permanently deleted")
	ErrTeacherProfileUnderReview         = errors.New("teacher profile is pending or rejected and cannot be deleted")
	ErrTeacherProfileAlreadyExists       = errors.New("teacher profile already exists for this user")
)

func (s *TeacherProfileService) CreateTeacherProfile(ctx context.Context, req dto.CreateTeacherProfileDTO) (*dto.TeacherProfileResponseDTO, error) {
	// Tìm CẢ dòng đã xoá mềm: bộ đếm nộp lại (quyết định #5) gắn với user, không được reset bằng
	// cách xoá rồi tạo lại (review PR #73, MAJOR #2) — dòng cũ được KHÔI PHỤC thay vì tạo mới.
	existing, err := s.repo.GetByUserIDIncludingDeleted(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if existing != nil && !existing.DeletedAt.Valid {
		return nil, ErrTeacherProfileAlreadyExists
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

	if existing != nil {
		// Chỉ hồ sơ approved mới xoá được (DeleteTeacherProfile), nên dòng khôi phục vốn approved:
		// còn giữ TEACHER -> vẫn approved; đã mất TEACHER -> thành đơn mới (pending) nhưng GIỮ
		// resubmission_count cũ.
		existing.Specialization = req.Specialization
		existing.Education = req.Education
		existing.ExperienceYears = req.ExperienceYears
		existing.CertificateInfo = req.CertificateInfo
		existing.Department = req.Department
		existing.ApprovalStatus = approvalStatus
		// Review N3: về pending = đơn MỚI chờ xét; giữ reviewed_at/by của lần duyệt cũ thì admin
		// thấy đơn chờ mang dấu "đã xét". Còn approved (vẫn giữ TEACHER) thì lần duyệt cũ vẫn đúng.
		if approvalStatus == model.TeacherApprovalPending {
			existing.ReviewedAt = nil
			existing.ReviewedBy = nil
		}
		if err := s.repo.Restore(ctx, existing); err != nil {
			if errors.Is(err, repository.ErrTeacherProfileNotDeleted) {
				return nil, ErrTeacherProfileAlreadyExists
			}
			return nil, err
		}
		return toTeacherProfileResponseDTO(existing), nil
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

	// Route công khai: repo GetAll chỉ trả hồ sơ approved; DTO công khai bỏ trường duyệt.
	profileDTOs := make([]dto.PublicTeacherProfileDTO, len(profiles))
	for i, p := range profiles {
		profileDTOs[i] = *toPublicTeacherProfileDTO(&p)
	}

	return &dto.TeacherProfileListResponseDTO{
		TeacherProfiles: profileDTOs,
		Total:           total,
		Page:            page,
		PageSize:        pageSize,
	}, nil
}

// GetTeacherProfileByID — route CÔNG KHAI: hồ sơ chưa duyệt (pending/rejected) coi như không tồn
// tại, để người lạ không dò được ai đang ứng tuyển (review PR #73, MAJOR #1).
func (s *TeacherProfileService) GetTeacherProfileByID(ctx context.Context, id uuid.UUID) (*dto.PublicTeacherProfileDTO, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.ApprovalStatus != model.TeacherApprovalApproved {
		return nil, errors.New("teacher profile not found")
	}
	return toPublicTeacherProfileDTO(profile), nil
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

	// Review N1: chỉ ghi các cột nội dung người dùng gửi lên, KHÔNG Save cả dòng vừa đọc — dòng đó
	// có thể đã cũ nếu admin duyệt/từ chối hoặc chính user nộp lại xen giữa (xem UpdateContentFields).
	fields := map[string]interface{}{}
	if req.Specialization != nil {
		fields["specialization"] = req.Specialization
	}
	if req.Education != nil {
		fields["education"] = req.Education
	}
	if req.ExperienceYears != nil {
		fields["experience_years"] = req.ExperienceYears
	}
	if req.CertificateInfo != nil {
		fields["certificate_info"] = req.CertificateInfo
	}
	if req.Department != nil {
		fields["department"] = req.Department
	}
	if err := s.repo.UpdateContentFields(ctx, id, fields); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("teacher profile not found")
		}
		return nil, err
	}

	// Đọc lại để response phản ánh trạng thái duyệt HIỆN TẠI, không phải bản đọc trước khi ghi.
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, errors.New("teacher profile not found")
	}
	return toTeacherProfileResponseDTO(updated), nil
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
	// Review PR #73 (MAJOR #2): trước đây chủ hồ sơ hard_delete rồi POST lại là được hồ sơ mới
	// pending với resubmission_count=0 — lách giới hạn 3 lần nộp lại (quyết định #5).
	//  - Cấm xoá vĩnh viễn: mất dòng = mất bộ đếm, không còn gì để khôi phục.
	//  - Cấm xoá khi đang pending/rejected: đơn đang được xét hoặc đang tính lượt nộp lại.
	// Xoá mềm hồ sơ approved vẫn được; tạo lại sẽ khôi phục đúng dòng cũ (CreateTeacherProfile).
	if hardDelete {
		return ErrTeacherProfileHardDeleteForbidden
	}
	if profile.ApprovalStatus == model.TeacherApprovalPending || profile.ApprovalStatus == model.TeacherApprovalRejected {
		return ErrTeacherProfileUnderReview
	}
	return s.repo.Delete(ctx, id, false)
}

func toPublicTeacherProfileDTO(p *model.TeacherProfile) *dto.PublicTeacherProfileDTO {
	return &dto.PublicTeacherProfileDTO{
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
