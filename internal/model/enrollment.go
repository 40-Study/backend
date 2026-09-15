package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PlayedRange la MOT khoang [Start, End) giay da Phat THAT trong trinh phat.
//
// Y nghia: client gom 10s/lan roi gui len danh sach cac khoang vua phat. Doan bi TUA QUA
// khong nam trong bat ky khoang nao, nen "da xem bao nhieu" tinh tu day KHONG the bi thoi
// phong bang cach keo thanh tua (xem MergePlayedRanges, internal/service/played_ranges.go).
type PlayedRange struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// PlayedRanges luu duoi dang JSONB (cot lesson_progress.played_ranges).
//
// implements driver.Valuer + sql.Scanner de GORM ghi/doc duoc slice struct nay ma khong
// can them thu vien kieu nao (khong dung gorm.io/datatypes chi vi mot cot).
type PlayedRanges []PlayedRange

// Value — driver.Valuer: luon ghi mot JSON array, ke ca khi rong (NULL trong cot JSONB se
// khien lan doc sau tra ve nil thay vi slice rong, va moi noi goi phai tu doan "nil nghia la
// gi"; ghi thang "[]" giu mot bieu dien duy nhat).
func (p PlayedRanges) Value() (driver.Value, error) {
	if p == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]PlayedRange(p))
	if err != nil {
		return nil, fmt.Errorf("marshal played_ranges: %w", err)
	}
	return string(b), nil
}

// Scan — sql.Scanner: doc ca []byte (pgx/lib-pq) lan string, chiu duoc NULL va chuoi rong.
func (p *PlayedRanges) Scan(value interface{}) error {
	if value == nil {
		*p = PlayedRanges{}
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("scan played_ranges: kieu khong ho tro %T", value)
	}
	if len(raw) == 0 {
		*p = PlayedRanges{}
		return nil
	}
	var out []PlayedRange
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("unmarshal played_ranges: %w", err)
	}
	*p = out
	return nil
}

type Enrollment struct {
	BaseModel
	UserID          uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course" json:"user_id"`
	CourseID        uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course" json:"course_id"`
	EnrolledAt      time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"enrolled_at"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	ProgressPercent decimal.Decimal `gorm:"type:decimal(5,2);default:0;column:progress_percentage" json:"progress_percentage"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	LastAccessedAt  *time.Time      `json:"last_accessed_at,omitempty"`
	CertificateID   *uuid.UUID      `gorm:"type:uuid;index" json:"certificate_id,omitempty"`

	// Relationships
	User           User             `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Course         Course           `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Certificate    *Certificate     `gorm:"foreignKey:CertificateID" json:"-"`
	LessonProgress []LessonProgress `gorm:"foreignKey:EnrollmentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Enrollment) TableName() string {
	return "enrollments"
}

type LessonProgress struct {
	BaseModel
	UserID           uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_lesson" json:"user_id"`
	LessonID         uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_lesson" json:"lesson_id"`
	EnrollmentID     uuid.UUID       `gorm:"type:uuid;not null;index" json:"enrollment_id"`
	Status           string          `gorm:"type:varchar(20);default:'not_started';check:status IN ('not_started', 'in_progress', 'completed');index" json:"status"`
	ProgressPercent  decimal.Decimal `gorm:"type:decimal(5,2);default:0;column:progress_percentage" json:"progress_percentage"`
	VideoWatchedSecs int             `gorm:"default:0;column:video_watched_seconds" json:"video_watched_seconds"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	LastAccessedAt   time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"last_accessed_at"`

	// Enhanced time tracking
	TimeSpentSeconds    int `gorm:"default:0;column:time_spent_seconds" json:"time_spent_seconds"`       // Tong thoi gian hoc
	LastPositionSeconds int `gorm:"default:0;column:last_position_seconds" json:"last_position_seconds"` // Vi tri video cuoi
	ViewsCount          int `gorm:"default:0" json:"views_count"`                                        // So lan xem

	// PlayedRanges (Phase 1 §1, chong tua): hop cua moi khoang video da Phat THAT, da merge.
	// watched_seconds duoc SUY RA tu day (tong do dai sau merge) chu khong cong don tu client,
	// nen khong co duong nao de "xem 1 lan tua het bai" ma tinh la da xem het.
	PlayedRanges PlayedRanges `gorm:"type:jsonb;default:'[]';column:played_ranges" json:"played_ranges"`
	// WatchedPct: watched_seconds / duration * 100, lam tron 1 chu so thap phan. Cot nay la
	// gia tri DAN XUAT duoc luu san CO CHU DICH: no la dieu kien so sanh voi course.min_video_pct
	// o MOI request tien do, va cung la gia tri curriculum tra ve cho tung bai — tinh lai tu
	// played_ranges + duration o moi noi doc se khong cho ra cung ket qua khi duration thay doi.
	WatchedPct decimal.Decimal `gorm:"type:decimal(5,2);default:0;column:watched_pct" json:"watched_pct"`

	// Relationships
	User       User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Lesson     Lesson     `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Enrollment Enrollment `gorm:"foreignKey:EnrollmentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LessonProgress) TableName() string {
	return "lesson_progress"
}

type UserNote struct {
	BaseModel
	UserID             uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	LessonID           uuid.UUID `gorm:"type:uuid;not null;index" json:"lesson_id"`
	Content            string    `gorm:"type:text;not null" json:"content"`
	VideoTimestampSecs *int      `gorm:"column:video_timestamp_seconds" json:"video_timestamp_seconds,omitempty"`
	IsBookmarked       bool      `gorm:"default:false" json:"is_bookmarked"`

	// Relationships
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Lesson Lesson `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserNote) TableName() string {
	return "user_notes"
}
