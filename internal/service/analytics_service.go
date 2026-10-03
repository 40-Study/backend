package service

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

// P1 QA 260927 teacher: trước bản vá này 3 endpoint analytics chỉ có AuthMiddleware (bất kỳ ai
// đã đăng nhập — kể cả học sinh, kể cả giáo viên khác) và KHÔNG kiểm actor có liên quan gì tới
// buổi live/bài tập không, miễn biết (hoặc đoán) đúng sessionId/assignmentId là xem được số liệu
// (số người xem, danh sách participant, tỉ lệ chấp nhận bài nộp). Dùng lại
// ensureClassManage (class_access.go — NGUỒN SỰ THẬT DUY NHẤT cho câu hỏi "user này có quản lý
// lớp này không") thay vì viết lại phép kiểm riêng.
var ErrNotAnalyticsOwner = ErrNotClassTeacher

type AnalyticsServiceInterface interface {
	GetLivestreamAnalytics(ctx context.Context, sessionID, actorUserID uuid.UUID, isAdmin bool) (*dto.AnalyticsResponseDTO, error)
	GetAssignmentAnalytics(ctx context.Context, assignmentID, actorUserID uuid.UUID, isAdmin bool) (*dto.AssignmentAnalyticsDTO, error)
	GetParticipantAnalytics(ctx context.Context, sessionID, actorUserID uuid.UUID, isAdmin bool) (*dto.ParticipantAnalyticsDTO, error)
}

type AnalyticsService struct {
	analyticsRepo   repository.AnalyticsRepositoryInterface
	participantRepo repository.ParticipantRepositoryInterface
	submissionRepo  repository.SubmissionRepositoryInterface
	assignmentRepo  repository.AssignmentRepositoryInterface
	livestreamRepo  repository.LivestreamRepositoryInterface
	classRepo       repository.ClassRepositoryInterface
	courseRepo      repository.CourseRepositoryInterface
}

func NewAnalyticsService(
	analyticsRepo repository.AnalyticsRepositoryInterface,
	participantRepo repository.ParticipantRepositoryInterface,
	submissionRepo repository.SubmissionRepositoryInterface,
	assignmentRepo repository.AssignmentRepositoryInterface,
	livestreamRepo repository.LivestreamRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
) *AnalyticsService {
	return &AnalyticsService{
		analyticsRepo:   analyticsRepo,
		participantRepo: participantRepo,
		submissionRepo:  submissionRepo,
		assignmentRepo:  assignmentRepo,
		livestreamRepo:  livestreamRepo,
		classRepo:       classRepo,
		courseRepo:      courseRepo,
	}
}

// ensureSessionAnalyticsAccess (P1 QA 260927 teacher): host của phiên luôn qua; còn lại đi qua
// ensureClassManage bằng ClassID của phiên (session.ClassID NOT NULL theo model — nhánh
// CourseID-only chỉ là phòng hờ, giống hệt resolveJoinRole ở livestream_service.go).
func (s *AnalyticsService) ensureSessionAnalyticsAccess(ctx context.Context, sessionID, actorUserID uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	session, err := s.livestreamRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return ErrSessionNotFound
	}
	if session.HostID == actorUserID {
		return nil
	}
	if session.ClassID != uuid.Nil {
		return ensureClassManage(ctx, s.classRepo, s.courseRepo, actorUserID, session.ClassID, isAdmin)
	}
	if session.CourseID != nil {
		course, err := s.courseRepo.GetByID(ctx, *session.CourseID)
		if err != nil {
			return err
		}
		if course != nil && course.InstructorID == actorUserID {
			return nil
		}
	}
	return ErrNotAnalyticsOwner
}

func (s *AnalyticsService) GetLivestreamAnalytics(ctx context.Context, sessionID, actorUserID uuid.UUID, isAdmin bool) (*dto.AnalyticsResponseDTO, error) {
	if err := s.ensureSessionAnalyticsAccess(ctx, sessionID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	analytics, err := s.analyticsRepo.GetBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if analytics == nil {
		return &dto.AnalyticsResponseDTO{
			SessionID: sessionID,
		}, nil
	}

	return &dto.AnalyticsResponseDTO{
		SessionID:        analytics.SessionID,
		PeakViewers:      analytics.PeakViewers,
		TotalViewers:     analytics.TotalViewers,
		TotalMessages:    analytics.TotalMessages,
		AvgWatchTimeSecs: analytics.AvgWatchTimeSecs,
	}, nil
}

func (s *AnalyticsService) GetAssignmentAnalytics(ctx context.Context, assignmentID, actorUserID uuid.UUID, isAdmin bool) (*dto.AssignmentAnalyticsDTO, error) {
	// GetByIDWithSession (thay vì GetByID trần): cần Session.ClassID khi bài tập là loại
	// live_coding (ClassID trực tiếp trên Assignment là nil, chỉ SessionID có giá trị).
	assignment, err := s.assignmentRepo.GetByIDWithSession(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		// S4: id không tồn tại trả 404, không phải 200 với data null (trước đây 200 null cho id không có và
		// 403 cho id có thật, nên dò được id nào tồn tại).
		return nil, ErrAssignmentNotFound
	}

	// S4 (SSOT): số liệu assignment cần đúng quyền quản lý assignment (CanManage: host phiên, giảng
	// viên lớp, chủ khoá) — không tự suy lớp rồi kiểm lại theo một định nghĩa khác.
	if !isAdmin {
		manage, err := s.assignmentRepo.CanManage(ctx, assignmentID, actorUserID)
		if err != nil {
			return nil, err
		}
		if !manage {
			return nil, ErrNotAnalyticsOwner
		}
	}

	stats, err := s.analyticsRepo.GetSubmissionStats(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	totalSubmissions := int64(0)
	acceptedCount := int64(0)
	acceptanceRate := 0.0

	if v, ok := stats["total_submissions"].(int64); ok {
		totalSubmissions = v
	}
	if v, ok := stats["accepted_count"].(int64); ok {
		acceptedCount = v
	}
	if v, ok := stats["acceptance_rate"].(float64); ok {
		acceptanceRate = v
	}

	return &dto.AssignmentAnalyticsDTO{
		AssignmentID:     assignmentID,
		TotalSubmissions: int(totalSubmissions),
		AcceptedCount:    int(acceptedCount),
		AcceptanceRate:   acceptanceRate,
		Difficulty:       string(assignment.Difficulty),
	}, nil
}

func (s *AnalyticsService) GetParticipantAnalytics(ctx context.Context, sessionID, actorUserID uuid.UUID, isAdmin bool) (*dto.ParticipantAnalyticsDTO, error) {
	if err := s.ensureSessionAnalyticsAccess(ctx, sessionID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	totalJoined, err := s.participantRepo.CountBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	activeCount, err := s.participantRepo.CountActiveBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	participants, err := s.participantRepo.GetActiveBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	byRole := make(map[string]int)
	for _, p := range participants {
		byRole[string(p.Role)]++
	}

	return &dto.ParticipantAnalyticsDTO{
		SessionID:   sessionID,
		ActiveCount: int(activeCount),
		TotalJoined: int(totalJoined),
		ByRole:      byRole,
	}, nil
}
