package model

import "github.com/google/uuid"

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

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (TeacherProfile) TableName() string {
	return "teacher_profiles"
}
