package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// ContestRepository — truy cập dữ liệu cuộc thi (MVP "Cuộc thi", contract §1–§5). Mọi chuyển
// trạng thái đi qua UpdateStatusTx (UPDATE ... WHERE status IN (...) + RowsAffected), không
// đọc-rồi-ghi. Phần tham gia/xếp hạng/giải ở contest_participation_repository.go.
type ContestRepository struct {
	db *gorm.DB
}

func NewContestRepository(db *gorm.DB) *ContestRepository {
	return &ContestRepository{db: db}
}

// Transaction chạy fn trong 1 transaction; tx truyền cho engine quiz (B2) dùng chung.
func (r *ContestRepository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *ContestRepository) withDetails(q *gorm.DB) *gorm.DB {
	return q.Preload("Creator").Preload("Course").Preload("Quiz").
		Preload("Prizes", func(db *gorm.DB) *gorm.DB { return db.Order("rank_from ASC") }).
		Preload("Prizes.Voucher", func(db *gorm.DB) *gorm.DB { return db.Unscoped() })
}

func firstOrNil(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

// GetByID trả (nil, nil) khi không có.
func (r *ContestRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Contest, error) {
	var c model.Contest
	if err := r.withDetails(r.db.WithContext(ctx)).First(&c, "contests.id = ?", id).Error; err != nil {
		return nil, firstOrNil(err)
	}
	return &c, nil
}

func (r *ContestRepository) GetBySlug(ctx context.Context, slug string) (*model.Contest, error) {
	var c model.Contest
	if err := r.withDetails(r.db.WithContext(ctx)).First(&c, "contests.slug = ?", slug).Error; err != nil {
		return nil, firstOrNil(err)
	}
	return &c, nil
}

// LockByIDTx: SELECT ... FOR UPDATE (chốt kết quả, contract §5 bước 1).
func (r *ContestRepository) LockByIDTx(tx *gorm.DB, id uuid.UUID) (*model.Contest, error) {
	var c model.Contest
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, "id = ?", id).Error; err != nil {
		return nil, firstOrNil(err)
	}
	return &c, nil
}

// FindByQuizID — cuộc thi đang gắn quiz (kể cả CANCELLED); nil khi quiz tự do.
func (r *ContestRepository) FindByQuizID(ctx context.Context, quizID uuid.UUID) (*model.Contest, error) {
	var c model.Contest
	if err := r.db.WithContext(ctx).Select("id, status, created_by").First(&c, "quiz_id = ?", quizID).Error; err != nil {
		return nil, firstOrNil(err)
	}
	return &c, nil
}

func (r *ContestRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Contest{}).Unscoped().Where("slug = ?", slug).Count(&n).Error
	return n > 0, err
}

func (r *ContestRepository) CreateTx(tx *gorm.DB, c *model.Contest, prizes []model.ContestPrize) error {
	if err := tx.Create(c).Error; err != nil {
		return err
	}
	return r.ReplacePrizesTx(tx, c.ID, prizes)
}

// ReplacePrizesTx thay TOÀN BỘ giải (contract #9, #20, #23).
func (r *ContestRepository) ReplacePrizesTx(tx *gorm.DB, contestID uuid.UUID, prizes []model.ContestPrize) error {
	if err := tx.Where("contest_id = ?", contestID).Delete(&model.ContestPrize{}).Error; err != nil {
		return err
	}
	if len(prizes) == 0 {
		return nil
	}
	for i := range prizes {
		prizes[i].ContestID = contestID
	}
	return tx.Create(&prizes).Error
}

// UpdateStatusTx cập nhật `values` chỉ khi status hiện tại thuộc `from` (và thoả `extra` nếu có);
// trả số dòng bị đổi để service phân biệt "sai trạng thái" với "thành công".
func (r *ContestRepository) UpdateStatusTx(tx *gorm.DB, id uuid.UUID, from []string, extra string, values map[string]interface{}) (int64, error) {
	q := tx.Model(&model.Contest{}).Where("id = ? AND status IN ?", id, from)
	if extra != "" {
		q = q.Where(extra)
	}
	res := q.Updates(values)
	return res.RowsAffected, res.Error
}

// HardDelete — xoá hẳn (giải phóng quiz_id), chỉ khi status thuộc `from`.
func (r *ContestRepository) HardDelete(ctx context.Context, id uuid.UUID, from []string) (int64, error) {
	res := r.db.WithContext(ctx).Unscoped().Where("id = ? AND status IN ?", id, from).Delete(&model.Contest{})
	return res.RowsAffected, res.Error
}

// ContestListFilter — một bộ lọc dùng cho 4 danh sách (#1 công khai, #2 của tôi, #3 quản lý, #19 admin).
type ContestListFilter struct {
	PublicOnly     bool       // #1: PUBLISHED + is_public
	CreatedBy      *uuid.UUID // #3
	JoinedBy       *uuid.UUID // #2
	AdminViewer    *uuid.UUID // #19: ẩn DRAFT của người khác
	Status         string
	Phase          string
	CourseID       *uuid.UUID
	Q              string
	Now            time.Time
	Page, Limit    int
	OrderStartDesc bool
}

// phaseCondition dịch phase (tính lúc đọc) sang SQL theo đúng quy tắc model.ContestPhaseAt.
func phaseCondition(q *gorm.DB, phase string, now time.Time) *gorm.DB {
	pub := "contests.status = 'PUBLISHED' AND contests.finalized_at IS NULL"
	switch phase {
	case model.ContestPhaseUpcoming:
		return q.Where(pub+" AND contests.start_time > ?", now)
	case model.ContestPhaseActive:
		return q.Where(pub+" AND contests.start_time <= ? AND contests.end_time > ?", now, now)
	case model.ContestPhaseEnded:
		return q.Where(pub+" AND contests.end_time <= ?", now)
	case model.ContestPhaseFinalized:
		return q.Where("contests.status = 'PUBLISHED' AND contests.finalized_at IS NOT NULL")
	case "":
		return q
	default: // DRAFT/PENDING_REVIEW/REJECTED/CANCELLED: phase = status
		return q.Where("contests.status = ?", phase)
	}
}

func (r *ContestRepository) List(ctx context.Context, f ContestListFilter) ([]model.Contest, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Contest{})
	if f.PublicOnly {
		q = q.Where("contests.status = ? AND contests.is_public = true", model.ContestStatusPublished)
	}
	if f.CreatedBy != nil {
		q = q.Where("contests.created_by = ?", *f.CreatedBy)
	}
	if f.JoinedBy != nil {
		q = q.Where("EXISTS (SELECT 1 FROM contest_participants cp WHERE cp.contest_id = contests.id AND cp.user_id = ?)", *f.JoinedBy)
	}
	if f.AdminViewer != nil {
		q = q.Where("NOT (contests.status = ? AND contests.created_by <> ?)", model.ContestStatusDraft, *f.AdminViewer)
	}
	if f.Status != "" {
		q = q.Where("contests.status = ?", f.Status)
	}
	if f.CourseID != nil {
		q = q.Where("contests.course_id = ?", *f.CourseID)
	}
	if s := strings.TrimSpace(f.Q); s != "" {
		q = q.Where("contests.title ILIKE ?", "%"+escapeLike(s)+"%")
	}
	q = phaseCondition(q, f.Phase, f.Now)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order := "contests.created_at DESC"
	if f.OrderStartDesc {
		order = "contests.start_time DESC"
	}
	var out []model.Contest
	err := r.withDetails(q).Order(order).Order("contests.id").
		Limit(f.Limit).Offset((f.Page - 1) * f.Limit).Find(&out).Error
	return out, total, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// QuizStat — số câu và tổng điểm của một quiz (không lưu trùng ở contests).
type QuizStat struct {
	QuizID        uuid.UUID
	QuestionCount int
	TotalPoints   decimal.Decimal
}

func (r *ContestRepository) QuizStats(ctx context.Context, quizIDs []uuid.UUID) (map[uuid.UUID]QuizStat, error) {
	out := map[uuid.UUID]QuizStat{}
	if len(quizIDs) == 0 {
		return out, nil
	}
	var rows []QuizStat
	err := r.db.WithContext(ctx).Raw(`
		SELECT quiz_id, COUNT(*) AS question_count, COALESCE(SUM(points), 0) AS total_points
		FROM questions WHERE quiz_id IN ? AND deleted_at IS NULL GROUP BY quiz_id`, quizIDs).Scan(&rows).Error
	for _, s := range rows {
		out[s.QuizID] = s
	}
	return out, err
}
