package model

import (
	"time"

	"github.com/google/uuid"
)

type Certificate struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	UserID            uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	CourseID          uuid.UUID `gorm:"type:uuid;not null;index" json:"course_id"`
	EnrollmentID      uuid.UUID `gorm:"type:uuid;not null;index" json:"enrollment_id"`
	CertificateNumber string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"certificate_number"`
	CertificateURL    *string   `gorm:"type:varchar(500);column:certificate_url" json:"certificate_url,omitempty"`
	IssuedAt          time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"issued_at"`

	// Relationships
	User       User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Course     Course     `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Enrollment Enrollment `gorm:"foreignKey:EnrollmentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Certificate) TableName() string {
	return "certificates"
}
