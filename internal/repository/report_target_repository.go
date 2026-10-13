package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// ReportTargetRow: một đối tượng bị báo cáo đã tra cứu, đã chuẩn hoá về cùng một hình dạng. Tìm theo type+id;
// không thấy (không tồn tại hoặc đã xoá mềm) thì không có trong map kết quả.
type ReportTargetRow struct {
	ID         uuid.UUID
	Name       *string // course: title | user: full_name (nil nếu trống) | discussion: title | review: comment
	Text       *string // discussion: content (dự phòng tên) | user: user_name (dự phòng tên)
	Status     string  // course.status | user: active/locked | discussion: visible/hidden | review: ""
	Slug       *string
	CourseID   *uuid.UUID
	CourseSlug *string
	OwnerID    *uuid.UUID
}

// ReportTargetRepositoryInterface tra cứu hàng loạt (một truy vấn mỗi loại) các đối tượng bị báo cáo để danh
// sách report không phải N+1.
type ReportTargetRepositoryInterface interface {
	// FindTargets trả map[reported_type][reported_id]→row cho các (type,id) được hỏi.
	FindTargets(ctx context.Context, idsByType map[string][]uuid.UUID) (map[string]map[uuid.UUID]ReportTargetRow, error)
}

type ReportTargetRepository struct {
	db *gorm.DB
}

func NewReportTargetRepository(db *gorm.DB) *ReportTargetRepository {
	return &ReportTargetRepository{db: db}
}

func (r *ReportTargetRepository) FindTargets(ctx context.Context, idsByType map[string][]uuid.UUID) (map[string]map[uuid.UUID]ReportTargetRow, error) {
	out := map[string]map[uuid.UUID]ReportTargetRow{}
	finders := map[string]func(context.Context, []uuid.UUID) ([]ReportTargetRow, error){
		"course": r.findCourses, "user": r.findUsers, "discussion": r.findDiscussions, "review": r.findReviews,
	}
	for typ, ids := range idsByType {
		find, ok := finders[typ]
		if !ok || len(ids) == 0 {
			continue
		}
		rows, err := find(ctx, ids)
		if err != nil {
			return nil, err
		}
		byID := make(map[uuid.UUID]ReportTargetRow, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}
		out[typ] = byID
	}
	return out, nil
}

func (r *ReportTargetRepository) findCourses(ctx context.Context, ids []uuid.UUID) ([]ReportTargetRow, error) {
	var rows []ReportTargetRow
	err := r.db.WithContext(ctx).Model(&model.Course{}).
		Select("id, title AS name, status, slug, instructor_id AS owner_id, id AS course_id, slug AS course_slug").
		Where("id IN ?", ids).Scan(&rows).Error
	return rows, err
}

func (r *ReportTargetRepository) findUsers(ctx context.Context, ids []uuid.UUID) ([]ReportTargetRow, error) {
	var rows []ReportTargetRow
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Select(`id, NULLIF(full_name, '') AS name, user_name AS text, id AS owner_id,
			CASE WHEN is_active THEN 'active' ELSE 'locked' END AS status`).
		Where("id IN ?", ids).Scan(&rows).Error
	return rows, err
}

func (r *ReportTargetRepository) findDiscussions(ctx context.Context, ids []uuid.UUID) ([]ReportTargetRow, error) {
	var rows []ReportTargetRow
	err := r.db.WithContext(ctx).Table("discussions d").
		Select(`d.id, d.title AS name, d.content AS text, d.slug, d.user_id AS owner_id,
			CASE WHEN d.is_hidden THEN 'hidden' ELSE 'visible' END AS status,
			c.id AS course_id, c.slug AS course_slug`).
		Joins("LEFT JOIN lessons l ON l.id = d.lesson_id").
		Joins("LEFT JOIN sections s ON s.id = l.section_id AND s.deleted_at IS NULL").
		Joins("LEFT JOIN courses c ON c.id = s.course_id AND c.deleted_at IS NULL").
		Where("d.id IN ? AND d.deleted_at IS NULL", ids).Scan(&rows).Error
	return rows, err
}

func (r *ReportTargetRepository) findReviews(ctx context.Context, ids []uuid.UUID) ([]ReportTargetRow, error) {
	var rows []ReportTargetRow
	err := r.db.WithContext(ctx).Table("reviews rv").
		Select("rv.id, rv.comment AS name, rv.user_id AS owner_id, c.id AS course_id, c.slug AS course_slug").
		Joins("LEFT JOIN courses c ON c.id = rv.course_id AND c.deleted_at IS NULL").
		Where("rv.id IN ? AND rv.deleted_at IS NULL", ids).Scan(&rows).Error
	return rows, err
}
