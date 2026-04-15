package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// CERTIFICATE
// ============================================================================

type CertificateResponseDTO struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"user_id"`
	UserName          string    `json:"user_name"`
	CourseID          uuid.UUID `json:"course_id"`
	CourseName        string    `json:"course_name"`
	EnrollmentID      uuid.UUID `json:"enrollment_id"`
	CertificateNumber string    `json:"certificate_number"`
	CertificateURL    *string   `json:"certificate_url,omitempty"`
	IssuedAt          time.Time `json:"issued_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type CertificateListDTO struct {
	Data     []CertificateResponseDTO `json:"data"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

type VerifyCertificateResponseDTO struct {
	Valid             bool      `json:"valid"`
	CertificateNumber string    `json:"certificate_number"`
	UserName          string    `json:"user_name,omitempty"`
	CourseName        string    `json:"course_name,omitempty"`
	IssuedAt          time.Time `json:"issued_at,omitempty"`
}
