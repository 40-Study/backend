package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type CertificateRepositoryInterface interface {
	CreateCertificate(ctx context.Context, cert *model.Certificate) error
	GetCertificateByID(ctx context.Context, id uuid.UUID) (*model.Certificate, error)
	GetCertificateByNumber(ctx context.Context, number string) (*model.Certificate, error)
	GetCertificatesByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Certificate, int64, error)
	GetCertificateByCourseAndUser(ctx context.Context, courseID, userID uuid.UUID) (*model.Certificate, error)
	UpdateCertificate(ctx context.Context, cert *model.Certificate) error
	DeleteCertificate(ctx context.Context, id uuid.UUID) error
}

type CertificateRepository struct {
	db *gorm.DB
}

func NewCertificateRepository(db *gorm.DB) *CertificateRepository {
	return &CertificateRepository{db: db}
}

func (r *CertificateRepository) CreateCertificate(ctx context.Context, cert *model.Certificate) error {
	return r.db.WithContext(ctx).Create(cert).Error
}

func (r *CertificateRepository) GetCertificateByID(ctx context.Context, id uuid.UUID) (*model.Certificate, error) {
	var cert model.Certificate
	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Course").
		First(&cert, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cert, nil
}

func (r *CertificateRepository) GetCertificateByNumber(ctx context.Context, number string) (*model.Certificate, error) {
	var cert model.Certificate
	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Course").
		First(&cert, "certificate_number = ?", number).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cert, nil
}

func (r *CertificateRepository) GetCertificatesByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Certificate, int64, error) {
	var certs []model.Certificate
	var total int64

	// Chứng chỉ đã thu hồi (hoàn tiền) không được liệt kê: học viên không còn thấy/tải được.
	query := r.db.WithContext(ctx).Model(&model.Certificate{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Preload("User").
		Preload("Course").
		Order("issued_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&certs).Error
	return certs, total, err
}

func (r *CertificateRepository) GetCertificateByCourseAndUser(ctx context.Context, courseID, userID uuid.UUID) (*model.Certificate, error) {
	var cert model.Certificate
	err := r.db.WithContext(ctx).
		Where("course_id = ? AND user_id = ?", courseID, userID).
		First(&cert).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cert, nil
}

func (r *CertificateRepository) UpdateCertificate(ctx context.Context, cert *model.Certificate) error {
	return r.db.WithContext(ctx).Save(cert).Error
}

// RevokeCertificates thu hồi chứng chỉ khoá học (bảng certificates) của (user, course) trên db
// truyền vào — gọi bằng *gorm.DB của transaction hoàn tiền để thu hồi nguyên tử với việc đổi
// trạng thái đơn. Chỉ đụng dòng chưa thu hồi nên gọi lại là idempotent. Trả về số chứng chỉ vừa
// thu hồi để caller xoá cache tra cứu công khai. KHÔNG đụng contest_awards (chứng nhận cuộc
// thi nằm ở bảng khác và không phụ thuộc đơn mua khoá).
func RevokeCertificates(ctx context.Context, db *gorm.DB, userID, courseID uuid.UUID, at time.Time) ([]string, error) {
	var numbers []string
	if err := db.WithContext(ctx).Model(&model.Certificate{}).
		Where("user_id = ? AND course_id = ? AND revoked_at IS NULL", userID, courseID).
		Pluck("certificate_number", &numbers).Error; err != nil {
		return nil, err
	}
	if len(numbers) == 0 {
		return nil, nil
	}
	if err := db.WithContext(ctx).Model(&model.Certificate{}).
		Where("user_id = ? AND course_id = ? AND revoked_at IS NULL", userID, courseID).
		Update("revoked_at", at).Error; err != nil {
		return nil, err
	}
	return numbers, nil
}

func (r *CertificateRepository) DeleteCertificate(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Certificate{}, "id = ?", id).Error
}
