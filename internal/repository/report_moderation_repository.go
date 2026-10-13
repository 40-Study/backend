package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// Hành động kiểm duyệt từ một report (ADM-N4). SSOT cho cả service, handler và bảng available_actions.
const (
	ReportActionUnpublishCourse  = "unpublish_course"
	ReportActionHideDiscussion   = "hide_discussion"
	ReportActionUnhideDiscussion = "unhide_discussion"
)

var (
	// ErrReportTargetNotFound: đối tượng bị báo cáo không còn tồn tại.
	ErrReportTargetNotFound = errors.New("report target not found")
	// ErrReportTargetUnchanged: đối tượng không ở trạng thái cho phép hành động này (hoặc đã ở trạng thái đích).
	ErrReportTargetUnchanged = errors.New("report target is not in an actionable state")
)

// ReportModerationInput: Notes rỗng = giữ nguyên admin_notes; Resolve=true chốt report thành resolved.
type ReportModerationInput struct {
	ReportID   uuid.UUID
	AdminID    uuid.UUID
	Action     string
	TargetID   uuid.UUID
	Notes      string
	Resolve    bool
	OccurredAt time.Time
}

// ReportModerationOutcome: thông tin để service báo cho chủ đối tượng.
type ReportModerationOutcome struct {
	CourseOwnerID *uuid.UUID
	CourseTitle   string
	CourseID      *uuid.UUID
}

type ReportModerationRepositoryInterface interface {
	// Apply áp hành động lên đối tượng VÀ cập nhật report trong cùng một giao dịch: lỗi ở bước nào thì không bước nào được ghi.
	Apply(ctx context.Context, in ReportModerationInput) (ReportModerationOutcome, error)
}

type ReportModerationRepository struct {
	db *gorm.DB
}

func NewReportModerationRepository(db *gorm.DB) *ReportModerationRepository {
	return &ReportModerationRepository{db: db}
}

func (r *ReportModerationRepository) Apply(ctx context.Context, in ReportModerationInput) (ReportModerationOutcome, error) {
	var out ReportModerationOutcome
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		switch in.Action {
		case ReportActionUnpublishCourse:
			out, err = unpublishCourse(tx, in.TargetID)
		case ReportActionHideDiscussion:
			err = setDiscussionHidden(tx, in.TargetID, true)
		case ReportActionUnhideDiscussion:
			err = setDiscussionHidden(tx, in.TargetID, false)
		default:
			err = errors.New("unsupported report action: " + in.Action)
		}
		if err != nil {
			return err
		}
		return updateReportAfterAction(tx, in)
	})
	return out, err
}

// unpublishCourse: published -> archived (cùng chuyển đổi thủ công được phép ở ValidateManualCourseStatusChange). UPDATE có
// điều kiện status='published' nên hai admin bấm cùng lúc chỉ một người thắng.
func unpublishCourse(tx *gorm.DB, courseID uuid.UUID) (ReportModerationOutcome, error) {
	var course model.Course
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "title", "instructor_id", "status").
		First(&course, "id = ?", courseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ReportModerationOutcome{}, ErrReportTargetNotFound
		}
		return ReportModerationOutcome{}, err
	}
	res := tx.Model(&model.Course{}).
		Where("id = ? AND status = ?", courseID, model.CourseStatusPublished).
		Update("status", model.CourseStatusArchived)
	if res.Error != nil {
		return ReportModerationOutcome{}, res.Error
	}
	if res.RowsAffected == 0 {
		return ReportModerationOutcome{}, ErrReportTargetUnchanged
	}
	return ReportModerationOutcome{CourseOwnerID: &course.InstructorID, CourseTitle: course.Title, CourseID: &course.ID}, nil
}

func setDiscussionHidden(tx *gorm.DB, id uuid.UUID, hidden bool) error {
	res := tx.Model(&model.Discussion{}).Where("id = ? AND is_hidden <> ?", id, hidden).Update("is_hidden", hidden)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	var n int64
	if err := tx.Model(&model.Discussion{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrReportTargetNotFound
	}
	return ErrReportTargetUnchanged
}

func updateReportAfterAction(tx *gorm.DB, in ReportModerationInput) error {
	updates := map[string]any{}
	if in.Notes != "" {
		updates["admin_notes"] = in.Notes
	}
	if in.Resolve {
		updates["status"] = "resolved"
		updates["resolved_by"] = in.AdminID
		updates["resolved_at"] = in.OccurredAt
	}
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&model.Report{}).Where("id = ?", in.ReportID).Updates(updates).Error
}
