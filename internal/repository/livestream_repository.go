package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type LivestreamRepositoryInterface interface {
	Create(ctx context.Context, session *model.LivestreamSession) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error)
	GetByRoomName(ctx context.Context, roomName string) (*model.LivestreamSession, error)
	// GetAll: lessonContentID (N10, review vòng 2 — "nếu rẻ") lọc phiên theo lesson_content_id,
	// nil = không lọc. userID/isAdmin (F-1, issue #58 review vòng 2): non-admin chỉ thấy phiên
	// mình là host/GV lớp/instructor khoá/học sinh lớp — không được liệt kê toàn hệ thống.
	GetAll(ctx context.Context, userID uuid.UUID, isAdmin bool, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) ([]model.LivestreamSession, int64, error)
	Update(ctx context.Context, session *model.LivestreamSession) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.LivestreamSessionStatus) error
	StartSession(ctx context.Context, id uuid.UUID) error
	EndSession(ctx context.Context, id uuid.UUID) error
	WithTx(tx *gorm.DB) LivestreamRepositoryInterface
}

type LivestreamRepository struct {
	db *gorm.DB
}

func NewLivestreamRepository(db *gorm.DB) *LivestreamRepository {
	return &LivestreamRepository{db: db}
}

func (r *LivestreamRepository) WithTx(tx *gorm.DB) LivestreamRepositoryInterface {
	return &LivestreamRepository{db: tx}
}
func (r *LivestreamRepository) Create(ctx context.Context, session *model.LivestreamSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *LivestreamRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	var session model.LivestreamSession
	err := r.db.WithContext(ctx).Preload("Participants").First(&session, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *LivestreamRepository) GetByRoomName(ctx context.Context, roomName string) (*model.LivestreamSession, error) {
	var session model.LivestreamSession
	err := r.db.WithContext(ctx).Where("room_name = ?", roomName).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// buildGetAllQuery (F-1/D6/R2-7, issue #58 review vòng 3): tách phần XÂY các điều kiện `WHERE`
// (lọc theo status/host/lesson_content_id, và lọc theo người gọi khi không phải admin) ra khỏi
// phần đếm/phân trang, để test DryRun (livestream_repository_authz_test.go) gọi được ĐÚNG hàm
// sản xuất thật thay vì hand-roll lại câu query — theo đúng pattern buildStudentClassExistsQuery
// (class_repository.go)/buildRestoreAndReactivateQuery (enrollment_repository.go).
func buildGetAllQuery(db *gorm.DB, userID uuid.UUID, isAdmin bool, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) *gorm.DB {
	query := db.Model(&model.LivestreamSession{})
	if status != "" {
		// Handle comma-separated status values (e.g., "live,scheduled")
		statuses := utils.SplitAndTrim(status, ",")
		if len(statuses) == 1 {
			query = query.Where("status = ?", statuses[0])
		} else if len(statuses) > 1 {
			query = query.Where("status IN ?", statuses)
		}
	}
	if hostID != nil {
		query = query.Where("host_id = ?", hostID)
	}
	if lessonContentID != nil {
		query = query.Where("lesson_content_id = ?", lessonContentID)
	}

	// F-1 (issue #58 review vòng 2): trước đây GetAll không lọc theo người gọi — bất kỳ user
	// đăng nhập nào cũng liệt kê được TOÀN BỘ phiên của hệ thống. Non-admin chỉ thấy phiên mình
	// là host, hoặc thuộc lớp mình dạy/học, hoặc thuộc khoá mình là instructor. Admin (isAdmin)
	// giữ nguyên hành vi cũ (thấy tất cả).
	if !isAdmin {
		// D6/R2-7 (issue #58 review vòng 3): nhánh "học sinh lớp" phải lọc theo
		// StudentClassActiveCondition (status active/rỗng/NULL) — trước đây liệt kê cả lớp học
		// sinh đã nghỉ (dropped/completed), cùng lớp lỗi với D1 vừa đóng ở resolveJoinRole.
		query = query.Where(
			"host_id = ? OR class_id IN (SELECT class_id FROM teacher_classes WHERE teacher_id = ?) OR "+
				"class_id IN (SELECT class_id FROM student_classes WHERE student_id = ? AND "+StudentClassActiveCondition+") OR "+
				"class_id IN (SELECT id FROM classes WHERE course_id IN (SELECT id FROM courses WHERE instructor_id = ?))",
			userID, userID, userID, userID,
		)
	}
	return query
}

func (r *LivestreamRepository) GetAll(ctx context.Context, userID uuid.UUID, isAdmin bool, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) ([]model.LivestreamSession, int64, error) {
	var sessions []model.LivestreamSession
	var total int64

	query := buildGetAllQuery(r.db.WithContext(ctx), userID, isAdmin, status, hostID, lessonContentID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Order("created_at DESC").
		Find(&sessions).Error

	return sessions, total, err
}

func (r *LivestreamRepository) Update(ctx context.Context, session *model.LivestreamSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *LivestreamRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.LivestreamSession{}, "id = ?", id).Error
}

func (r *LivestreamRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status model.LivestreamSessionStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamSession{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (r *LivestreamRepository) StartSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamSession{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     model.LivestreamStatusLive,
			"started_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

func (r *LivestreamRepository) EndSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamSession{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":   model.LivestreamStatusEnded,
			"ended_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}
