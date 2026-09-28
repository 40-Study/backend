package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Phase 3 duyệt giáo viên đăng ký (2026-09-28) — quyết định #4 (hàng chờ TEACHER_APPLICANT),
// #5 (từ chối bắt lý do, nộp lại tối đa 3 lần), #6 (duyệt có hiệu lực ngay).

var ErrTeacherProfileNotFound = errors.New("teacher profile not found")

// RoleChangeNotifier — phần của AuthService mà luồng duyệt cần (tách interface để test thay thế).
type RoleChangeNotifier interface {
	MarkRoleChanged(ctx context.Context, userID uuid.UUID) error
}

type TeacherApplicationServiceInterface interface {
	List(ctx context.Context, status, keyword string, page, limit int) (*dto.TeacherApplicationListDTO, error)
	Approve(ctx context.Context, userID, adminID uuid.UUID) (*dto.TeacherApplicationResultDTO, error)
	Reject(ctx context.Context, userID, adminID uuid.UUID, rawReason string) (*dto.TeacherApplicationResultDTO, error)
	GetMine(ctx context.Context, userID uuid.UUID) (*dto.MyTeacherApplicationDTO, error)
	Resubmit(ctx context.Context, userID uuid.UUID) (*dto.MyTeacherApplicationDTO, error)
}

type TeacherApplicationService struct {
	repo        repository.TeacherApplicationRepositoryInterface
	profileRepo repository.TeacherProfileRepositoryInterface
	notifier    RoleChangeNotifier
}

func NewTeacherApplicationService(
	repo repository.TeacherApplicationRepositoryInterface,
	profileRepo repository.TeacherProfileRepositoryInterface,
	notifier RoleChangeNotifier,
) *TeacherApplicationService {
	return &TeacherApplicationService{repo: repo, profileRepo: profileRepo, notifier: notifier}
}

func formatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func (s *TeacherApplicationService) List(ctx context.Context, status, keyword string, page, limit int) (*dto.TeacherApplicationListDTO, error) {
	if status == "" {
		status = model.TeacherApprovalPending
	}
	if !isOneOf(status, model.TeacherApprovalStatuses) {
		return nil, ErrInvalidReviewFilter
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, total, err := s.repo.List(ctx, repository.TeacherApplicationFilter{
		Status: status, Keyword: keyword, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]dto.TeacherApplicationItemDTO, len(rows))
	for i, r := range rows {
		items[i] = dto.TeacherApplicationItemDTO{
			UserID: r.UserID, ProfileID: r.ID, Email: r.Email, FullName: r.FullName,
			Specialization: r.Specialization, Education: r.Education, ExperienceYears: r.ExperienceYears,
			CertificateInfo: r.CertificateInfo, Department: r.Department,
			ApprovalStatus: r.ApprovalStatus, RejectionReason: r.RejectionReason,
			ResubmissionCount: r.ResubmissionCount,
			CreatedAt:         r.CreatedAt.Format(time.RFC3339),
			UpdatedAt:         r.UpdatedAt.Format(time.RFC3339),
			ReviewedAt:        formatOptionalTime(r.ReviewedAt),
		}
	}
	totalPages := int((total + int64(limit) - 1) / int64(limit))
	return &dto.TeacherApplicationListDTO{
		Items: items, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages,
	}, nil
}

// Approve: transaction đổi role (repository) rồi MarkRoleChanged. Nếu bước Redis lỗi, role đã
// được gán trong DB nhưng token cũ của user chưa bị vô hiệu — trả lỗi rõ ràng (không nuốt) để
// admin biết; quyền thật vẫn đúng ngay vì PermissionChecker đọc role từ DB mỗi request.
func (s *TeacherApplicationService) Approve(ctx context.Context, userID, adminID uuid.UUID) (*dto.TeacherApplicationResultDTO, error) {
	profile, err := s.repo.Approve(ctx, userID, adminID)
	if err != nil {
		return nil, err
	}
	if err := s.notifier.MarkRoleChanged(ctx, userID); err != nil {
		return nil, fmt.Errorf("teacher approved but failed to refresh user sessions: %w", err)
	}
	return &dto.TeacherApplicationResultDTO{UserID: profile.UserID, ApprovalStatus: profile.ApprovalStatus}, nil
}

func (s *TeacherApplicationService) Reject(ctx context.Context, userID, adminID uuid.UUID, rawReason string) (*dto.TeacherApplicationResultDTO, error) {
	reason, err := NormalizeReviewReason(rawReason)
	if err != nil {
		return nil, err
	}
	profile, err := s.repo.Reject(ctx, userID, adminID, reason)
	if err != nil {
		return nil, err
	}
	return &dto.TeacherApplicationResultDTO{UserID: profile.UserID, ApprovalStatus: profile.ApprovalStatus}, nil
}

func toMyApplication(p *model.TeacherProfile) *dto.MyTeacherApplicationDTO {
	return &dto.MyTeacherApplicationDTO{
		TeacherProfileResponseDTO: *toTeacherProfileResponseDTO(p),
		MaxResubmissions:          model.MaxTeacherResubmissions,
		CanResubmit:               repository.EvaluateTeacherResubmission(p.ApprovalStatus, p.ResubmissionCount) == nil,
	}
}

func (s *TeacherApplicationService) GetMine(ctx context.Context, userID uuid.UUID) (*dto.MyTeacherApplicationDTO, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrTeacherProfileNotFound
	}
	return toMyApplication(profile), nil
}

func (s *TeacherApplicationService) Resubmit(ctx context.Context, userID uuid.UUID) (*dto.MyTeacherApplicationDTO, error) {
	profile, err := s.repo.Resubmit(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toMyApplication(profile), nil
}
