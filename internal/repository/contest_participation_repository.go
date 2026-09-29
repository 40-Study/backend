package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// ParticipationRow — contest_participants JOIN quiz_attempts JOIN users. Điểm/giờ nộp luôn lấy
// từ quiz_attempts (contract §1.3), không từ cột total_score/finished_at cũ.
type ParticipationRow struct {
	ParticipantID    uuid.UUID
	ContestID        uuid.UUID
	UserID           uuid.UUID
	JoinedAt         time.Time
	AttemptID        *uuid.UUID
	Rank             *int
	StartedAt        *time.Time
	CompletedAt      *time.Time
	Score            *decimal.Decimal
	TotalPoints      *decimal.Decimal
	Percentage       *decimal.Decimal
	TimeSpentSeconds *int
	UserName         string
	AvatarURL        *string
}

const participationSelect = `
	SELECT cp.id AS participant_id, cp.contest_id, cp.user_id, cp.created_at AS joined_at, cp.attempt_id, cp.rank,
	       qa.started_at, qa.completed_at, qa.score, qa.total_points, qa.percentage, qa.time_spent_seconds,
	       COALESCE(u.full_name, u.user_name) AS user_name, u.avatar_url
	FROM contest_participants cp
	LEFT JOIN quiz_attempts qa ON qa.id = cp.attempt_id
	JOIN users u ON u.id = cp.user_id`

// ParticipationsOfUser — lượt tham gia của 1 người ở nhiều cuộc thi (map theo contest_id).
func (r *ContestRepository) ParticipationsOfUser(ctx context.Context, userID uuid.UUID, contestIDs []uuid.UUID) (map[uuid.UUID]ParticipationRow, error) {
	out := map[uuid.UUID]ParticipationRow{}
	if len(contestIDs) == 0 {
		return out, nil
	}
	var rows []ParticipationRow
	err := r.db.WithContext(ctx).Raw(participationSelect+` WHERE cp.user_id = ? AND cp.contest_id IN ?`,
		userID, contestIDs).Scan(&rows).Error
	for _, p := range rows {
		out[p.ContestID] = p
	}
	return out, err
}

func (r *ContestRepository) ListParticipants(ctx context.Context, contestID uuid.UUID, page, limit int) ([]ParticipationRow, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.ContestParticipant{}).Where("contest_id = ?", contestID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ParticipationRow
	err := r.db.WithContext(ctx).Raw(participationSelect+` WHERE cp.contest_id = ?
		ORDER BY cp.rank ASC NULLS LAST, cp.created_at ASC, cp.id ASC LIMIT ? OFFSET ?`,
		contestID, limit, (page-1)*limit).Scan(&rows).Error
	return rows, total, err
}

// AwardRow — award kèm voucher (nếu có) của chính người nhận.
type AwardRow struct {
	ContestID         uuid.UUID
	UserID            uuid.UUID
	Rank              *int
	CertificateNumber *string
	UserVoucherID     *uuid.UUID
	IssuedAt          time.Time
	VoucherID         *uuid.UUID
	VoucherCode       *string
	VoucherName       *string
}

func (r *ContestRepository) AwardsOfUser(ctx context.Context, userID uuid.UUID, contestIDs []uuid.UUID) (map[uuid.UUID]AwardRow, error) {
	out := map[uuid.UUID]AwardRow{}
	if len(contestIDs) == 0 {
		return out, nil
	}
	var rows []AwardRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT ca.contest_id, ca.user_id, ca.rank, ca.certificate_number, ca.user_voucher_id, ca.issued_at,
		       v.id AS voucher_id, v.code AS voucher_code, v.name AS voucher_name
		FROM contest_awards ca
		LEFT JOIN user_vouchers uv ON uv.id = ca.user_voucher_id
		LEFT JOIN vouchers v ON v.id = uv.voucher_id
		WHERE ca.user_id = ? AND ca.contest_id IN ?`, userID, contestIDs).Scan(&rows).Error
	for _, a := range rows {
		out[a.ContestID] = a
	}
	return out, err
}

// ── Join / start (transaction) ──────────────────────────────────────────────

// InsertParticipantTx: ON CONFLICT DO NOTHING — 0 dòng = đã tham gia (contract §3.4).
func (r *ContestRepository) InsertParticipantTx(tx *gorm.DB, contestID, userID uuid.UUID) (int64, error) {
	p := model.ContestParticipant{ID: uuid.New(), ContestID: contestID, UserID: userID}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p)
	return res.RowsAffected, res.Error
}

// IncrementParticipantCountTx giữ chỗ nguyên tử: 0 dòng = hết chỗ HOẶC cuộc thi vừa đổi trạng thái
// (huỷ/hết giờ) giữa lúc kiểm tra và lúc ghi — điều kiện trạng thái kiểm lại ngay trong UPDATE.
func (r *ContestRepository) IncrementParticipantCountTx(tx *gorm.DB, contestID uuid.UUID, now time.Time) (int64, error) {
	res := tx.Exec(`UPDATE contests SET participant_count = participant_count + 1
		WHERE id = ? AND status = ? AND end_time > ?
		  AND (max_participants = 0 OR participant_count < max_participants)`,
		contestID, model.ContestStatusPublished, now)
	return res.RowsAffected, res.Error
}

// LockParticipantTx: SELECT ... FOR UPDATE — tuần tự hoá 2 request start đồng thời (contract §4.1).
func (r *ContestRepository) LockParticipantTx(tx *gorm.DB, contestID, userID uuid.UUID) (*model.ContestParticipant, error) {
	var p model.ContestParticipant
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&p, "contest_id = ? AND user_id = ?", contestID, userID).Error
	if err != nil {
		return nil, firstOrNil(err)
	}
	return &p, nil
}

func (r *ContestRepository) SetParticipantAttemptTx(tx *gorm.DB, participantID, attemptID uuid.UUID) (int64, error) {
	res := tx.Model(&model.ContestParticipant{}).Where("id = ? AND attempt_id IS NULL", participantID).
		Update("attempt_id", attemptID)
	return res.RowsAffected, res.Error
}

func (r *ContestRepository) GetParticipant(ctx context.Context, contestID, userID uuid.UUID) (*model.ContestParticipant, error) {
	var p model.ContestParticipant
	if err := r.db.WithContext(ctx).First(&p, "contest_id = ? AND user_id = ?", contestID, userID).Error; err != nil {
		return nil, firstOrNil(err)
	}
	return &p, nil
}

func (r *ContestRepository) GetAttemptTx(tx *gorm.DB, attemptID uuid.UUID) (*model.QuizAttempt, error) {
	var a model.QuizAttempt
	if err := tx.First(&a, "id = ?", attemptID).Error; err != nil {
		return nil, firstOrNil(err)
	}
	return &a, nil
}

func (r *ContestRepository) DB(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

// ── Xếp hạng (contract §4.4) ────────────────────────────────────────────────

type RankedRow struct {
	UserID           uuid.UUID
	Rank             int
	Score            decimal.Decimal
	TotalPoints      decimal.Decimal
	Percentage       decimal.Decimal
	TimeSpentSeconds int
	CompletedAt      time.Time
	UserName         string
	AvatarURL        *string
}

// liveRankSQL: chỉ attempt đã nộp; điểm cao trước, hoà thì ai NỘP TRƯỚC xếp trên, rồi thời gian
// làm ít hơn, cuối cùng user_id cho tất định. ROW_NUMBER — không đồng hạng.
const liveRankSQL = `
	WITH ranked AS (
		SELECT cp.user_id, COALESCE(qa.score, 0) AS score, COALESCE(qa.total_points, 0) AS total_points,
		       COALESCE(qa.percentage, 0) AS percentage, COALESCE(qa.time_spent_seconds, 0) AS time_spent_seconds,
		       qa.completed_at,
		       ROW_NUMBER() OVER (ORDER BY COALESCE(qa.score, 0) DESC, qa.completed_at ASC,
		                          COALESCE(qa.time_spent_seconds, 0) ASC, cp.user_id ASC) AS rank
		FROM contest_participants cp JOIN quiz_attempts qa ON qa.id = cp.attempt_id
		WHERE cp.contest_id = ? AND qa.completed_at IS NOT NULL
	)
	SELECT r.*, COALESCE(u.full_name, u.user_name) AS user_name, u.avatar_url
	FROM ranked r JOIN users u ON u.id = r.user_id ORDER BY r.rank`

// finalRankSQL: sau khi chốt, BXH đọc contest_participants.rank đã ghi.
const finalRankSQL = `
	SELECT cp.user_id, cp.rank, COALESCE(qa.score, 0) AS score, COALESCE(qa.total_points, 0) AS total_points,
	       COALESCE(qa.percentage, 0) AS percentage, COALESCE(qa.time_spent_seconds, 0) AS time_spent_seconds,
	       qa.completed_at, COALESCE(u.full_name, u.user_name) AS user_name, u.avatar_url
	FROM contest_participants cp JOIN quiz_attempts qa ON qa.id = cp.attempt_id JOIN users u ON u.id = cp.user_id
	WHERE cp.contest_id = ? AND cp.rank IS NOT NULL ORDER BY cp.rank`

// RankedRows — limit <= 0 lấy hết (dùng khi chốt, trong tx).
func (r *ContestRepository) RankedRows(db *gorm.DB, contestID uuid.UUID, finalized bool, page, limit int) ([]RankedRow, int64, error) {
	base := liveRankSQL
	if finalized {
		base = finalRankSQL
	}
	var total int64
	if err := db.Raw(`SELECT COUNT(*) FROM (`+base+`) x`, contestID).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []RankedRow
	var err error
	if limit > 0 {
		err = db.Raw(base+` LIMIT ? OFFSET ?`, contestID, limit, (page-1)*limit).Scan(&rows).Error
	} else {
		err = db.Raw(base, contestID).Scan(&rows).Error
	}
	return rows, total, err
}

// ── Chốt kết quả (contract §5) ──────────────────────────────────────────────

func (r *ContestRepository) SetRankTx(tx *gorm.DB, contestID, userID uuid.UUID, rank int) error {
	return tx.Model(&model.ContestParticipant{}).Where("contest_id = ? AND user_id = ?", contestID, userID).
		Update("rank", rank).Error
}

func (r *ContestRepository) PrizesTx(tx *gorm.DB, contestID uuid.UUID) ([]model.ContestPrize, error) {
	var out []model.ContestPrize
	err := tx.Where("contest_id = ?", contestID).Order("rank_from ASC").Find(&out).Error
	return out, err
}

func (r *ContestRepository) InsertAwardTx(tx *gorm.DB, a *model.ContestAward) error {
	return tx.Create(a).Error
}

// AwardCounts — số liệu cho lần bấm chốt lặp lại (already_finalized=true).
type AwardCounts struct {
	RankedCount, AwardCount, CertificateCount, VoucherCount int
}

func (r *ContestRepository) AwardCountsTx(tx *gorm.DB, contestID uuid.UUID) (AwardCounts, error) {
	var c AwardCounts
	err := tx.Raw(`
		SELECT (SELECT COUNT(*) FROM contest_participants WHERE contest_id = ? AND rank IS NOT NULL) AS ranked_count,
		       COUNT(*) AS award_count, COUNT(certificate_number) AS certificate_count,
		       COUNT(user_voucher_id) AS voucher_count
		FROM contest_awards WHERE contest_id = ?`, contestID, contestID).Scan(&c).Error
	return c, err
}
