package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ADM-N4 hành động kiểm duyệt trên đối tượng bị báo cáo - contract: plans/261013-qa2-role-findings/contract-be3.md §1b.

var (
	// ErrReportActionNotApplicable: action không khớp reported_type của report (vd hide_discussion trên báo cáo khoá học).
	ErrReportActionNotApplicable = errors.New("action does not apply to this report's target type")
	// ErrReportActionReasonRequired: unpublish_course và hide_discussion cần lý do (>= minReportActionReasonRunes ký tự).
	ErrReportActionReasonRequired = errors.New("reason is required for this action")
	// ErrReportTargetNotFound / ErrReportTargetUnchanged: lấy nguyên từ repository để handler map về mã lỗi.
	ErrReportTargetNotFound  = repository.ErrReportTargetNotFound
	ErrReportTargetUnchanged = repository.ErrReportTargetUnchanged
)

const minReportActionReasonRunes = 3

// reportActionTargetType: action -> reported_type duy nhất nó áp dụng được.
var reportActionTargetType = map[string]string{
	repository.ReportActionUnpublishCourse:  "course",
	repository.ReportActionHideDiscussion:   "discussion",
	repository.ReportActionUnhideDiscussion: "discussion",
}

// reportActionNeedsReason: hành động gỡ/ẩn nội dung phải có lý do (lưu vào admin_notes, báo cho giáo viên).
var reportActionNeedsReason = map[string]bool{
	repository.ReportActionUnpublishCourse: true,
	repository.ReportActionHideDiscussion:  true,
}

// ReportActionNotifier: phần của NotificationService mà moderation dùng để báo cho giáo viên (best-effort).
type ReportActionNotifier interface {
	SendNotification(req dto.CreateNotificationDTO) error
}

type ReportModerationServiceInterface interface {
	Apply(ctx context.Context, reportID, adminID uuid.UUID, req dto.ReportActionRequestDTO) (*dto.ReportActionResponseDTO, error)
}

type ReportModerationService struct {
	reports  repository.ReportRepositoryInterface
	moderate repository.ReportModerationRepositoryInterface
	targets  ReportTargetResolverInterface
	notifier ReportActionNotifier // nil = không gửi thông báo
	now      func() time.Time
}

func NewReportModerationService(
	reports repository.ReportRepositoryInterface,
	moderate repository.ReportModerationRepositoryInterface,
	targets ReportTargetResolverInterface,
	notifier ReportActionNotifier,
) *ReportModerationService {
	return &ReportModerationService{reports: reports, moderate: moderate, targets: targets, notifier: notifier, now: time.Now}
}

func (s *ReportModerationService) Apply(ctx context.Context, reportID, adminID uuid.UUID, req dto.ReportActionRequestDTO) (*dto.ReportActionResponseDTO, error) {
	report, err := s.reports.GetReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, ErrReportNotFound
	}
	wantType, known := reportActionTargetType[req.Action]
	if !known || report.ReportedType != wantType {
		return nil, ErrReportActionNotApplicable
	}
	reason := strings.TrimSpace(req.Reason)
	if reportActionNeedsReason[req.Action] && utf8.RuneCountInString(reason) < minReportActionReasonRunes {
		return nil, ErrReportActionReasonRequired
	}

	target, err := s.resolveTarget(ctx, report)
	if err != nil {
		return nil, err
	}
	if !target.Exists {
		return nil, ErrReportTargetNotFound
	}
	if err := checkReportActionAllowed(req.Action, target); err != nil {
		return nil, err
	}

	resolve := req.Resolve == nil || *req.Resolve
	outcome, err := s.moderate.Apply(ctx, repository.ReportModerationInput{
		ReportID: report.ID, AdminID: adminID, Action: req.Action, TargetID: report.ReportedID,
		Notes: reason, Resolve: resolve, OccurredAt: s.now(),
	})
	if err != nil {
		return nil, err
	}
	s.notifyCourseOwner(req.Action, reason, outcome)

	refreshed, err := s.reports.GetReportByID(ctx, report.ID)
	if err != nil {
		return nil, err
	}
	if refreshed == nil {
		return nil, ErrReportNotFound
	}
	resp := reportToDTO(refreshed)
	if resp.Target, err = s.resolveTarget(ctx, refreshed); err != nil {
		return nil, err
	}
	return &dto.ReportActionResponseDTO{Action: req.Action, Report: resp}, nil
}

func (s *ReportModerationService) resolveTarget(ctx context.Context, report *model.Report) (*dto.ReportTargetDTO, error) {
	targets, err := s.targets.Resolve(ctx, []model.Report{*report})
	if err != nil {
		return nil, err
	}
	return targets[report.ID], nil
}

// checkReportActionAllowed: khoá chỉ gỡ được khi đang published (dùng lại luật chuyển trạng thái thủ công published->archived);
// thảo luận chỉ đổi khi trạng thái ẩn/hiện thực sự đổi.
func checkReportActionAllowed(action string, target *dto.ReportTargetDTO) error {
	status := ""
	if target.Status != nil {
		status = *target.Status
	}
	switch action {
	case repository.ReportActionUnpublishCourse:
		if status == model.CourseStatusArchived || ValidateManualCourseStatusChange(status, model.CourseStatusArchived) != nil {
			return ErrReportTargetUnchanged
		}
	case repository.ReportActionHideDiscussion:
		if status != "visible" {
			return ErrReportTargetUnchanged
		}
	case repository.ReportActionUnhideDiscussion:
		if status != "hidden" {
			return ErrReportTargetUnchanged
		}
	}
	return nil
}

// notifyCourseOwner báo giáo viên khoá bị gỡ. Best-effort: hành động đã commit, lỗi gửi chỉ được log (không hoàn tác).
func (s *ReportModerationService) notifyCourseOwner(action, reason string, out repository.ReportModerationOutcome) {
	if s.notifier == nil || action != repository.ReportActionUnpublishCourse || out.CourseOwnerID == nil {
		return
	}
	refType := "course"
	err := s.notifier.SendNotification(dto.CreateNotificationDTO{
		Title:            "Khoá học của bạn đã bị gỡ khỏi danh mục",
		Content:          "Khoá học \"" + out.CourseTitle + "\" đã bị quản trị viên gỡ sau khi có báo cáo. Lý do: " + reason + ". Học viên đã ghi danh vẫn học được; liên hệ quản trị viên nếu bạn muốn khiếu nại.",
		NotificationType: "course_update",
		ReferenceType:    &refType,
		ReferenceID:      out.CourseID,
		UserIDs:          []uuid.UUID{*out.CourseOwnerID},
	})
	if err != nil {
		log.Printf("[report-moderation] không gửi được thông báo gỡ khoá %v cho giáo viên %s: %v", out.CourseID, out.CourseOwnerID, err)
	}
}
