package model

import (
	"time"

	"github.com/google/uuid"
)

type TeacherProfile struct {
	BaseModel
	UserID          uuid.UUID `gorm:"type:uuid;uniqueIndex;not null;column:user_id"`
	Specialization  *string   `gorm:"type:varchar(255);column:specialization" json:"specialization,omitempty"`
	Education       *string   `gorm:"type:varchar(255);column:education" json:"education,omitempty"`
	ExperienceYears *int      `gorm:"column:experience_years" json:"experience_years,omitempty"`
	CertificateInfo *string   `gorm:"type:text;column:certificate_info" json:"certificate_info,omitempty"`
	Department      *string   `gorm:"type:varchar(255);column:department" json:"department,omitempty"`

	// Bank info for payout
	BankName          *string `gorm:"type:varchar(100);column:bank_name" json:"bank_name,omitempty"`
	BankAccountNumber *string `gorm:"type:varchar(50);column:bank_account_number" json:"bank_account_number,omitempty"`
	BankAccountName   *string `gorm:"type:varchar(255);column:bank_account_name" json:"bank_account_name,omitempty"`

	// Phase 3 duyệt giáo viên: trạng thái hồ sơ đăng ký (SSOT TeacherApprovalStatuses,
	// course_status.go). ResubmissionCount đếm số lần NỘP LẠI sau khi bị từ chối — chặn khi đạt
	// MaxTeacherResubmissions (quyết định #5).
	ApprovalStatus    string     `gorm:"type:varchar(20);not null;default:'pending';check:approval_status IN ('pending', 'approved', 'rejected');column:approval_status" json:"approval_status"`
	RejectionReason   *string    `gorm:"type:text;column:rejection_reason" json:"rejection_reason,omitempty"`
	ReviewedBy        *uuid.UUID `gorm:"type:uuid;column:reviewed_by" json:"reviewed_by,omitempty"`
	ReviewedAt        *time.Time `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
	ResubmissionCount int        `gorm:"not null;default:0;column:resubmission_count" json:"resubmission_count"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (TeacherProfile) TableName() string {
	return "teacher_profiles"
}
