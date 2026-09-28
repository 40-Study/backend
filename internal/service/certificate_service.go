package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/repository"
)

type CertificateServiceInterface interface {
	IssueCertificate(ctx context.Context, userID, courseID, enrollmentID uuid.UUID) (*dto.CertificateResponseDTO, error)
	GetMyCertificates(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CertificateListDTO, error)
	GetCertificateByID(ctx context.Context, id uuid.UUID) (*dto.CertificateResponseDTO, error)
	VerifyCertificate(ctx context.Context, number string) (*dto.VerifyCertificateResponseDTO, error)
}

type CertificateEnrollmentRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	// GetByUserID (A4, QA vòng 2): liệt kê ghi danh của học viên để cấp bù chứng chỉ cho khoá đã
	// hoàn thành — xem issueMissingCertificates.
	GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error)
}

type CertificateService struct {
	repo           repository.CertificateRepositoryInterface
	enrollmentRepo CertificateEnrollmentRepository
	redis          *redis.Client
	rabbitMQ       *rabbitmq_queue.RabbitMQService
}

func NewCertificateService(
	repo repository.CertificateRepositoryInterface,
	enrollmentRepo CertificateEnrollmentRepository,
	redis *redis.Client,
	rabbitMQ *rabbitmq_queue.RabbitMQService,
) *CertificateService {
	return &CertificateService{
		repo:           repo,
		enrollmentRepo: enrollmentRepo,
		redis:          redis,
		rabbitMQ:       rabbitMQ,
	}
}

const (
	certVerifyCachePrefix = "cert_verify:"
	certCacheTTL          = 30 * time.Minute
)

// CertificateGenerationMessage is pushed to RabbitMQ for PDF generation
type CertificateGenerationMessage struct {
	CertificateID     uuid.UUID `json:"certificate_id"`
	UserID            uuid.UUID `json:"user_id"`
	CourseID          uuid.UUID `json:"course_id"`
	CertificateNumber string    `json:"certificate_number"`
	UserName          string    `json:"user_name"`
	CourseName        string    `json:"course_name"`
	IssuedAt          time.Time `json:"issued_at"`
}

func (s *CertificateService) IssueCertificate(ctx context.Context, userID, courseID, enrollmentID uuid.UUID) (*dto.CertificateResponseDTO, error) {
	if s.enrollmentRepo == nil {
		return nil, errors.New("enrollment repository unavailable")
	}

	enrollment, err := s.enrollmentRepo.GetByID(ctx, enrollmentID)
	if err != nil {
		return nil, fmt.Errorf("get enrollment: %w", err)
	}
	if enrollment == nil || enrollment.UserID != userID || enrollment.CourseID != courseID {
		return nil, errors.New("enrollment does not match the authenticated user and course")
	}
	if enrollment.CompletedAt == nil {
		return nil, errors.New("course must be completed before issuing a certificate")
	}

	// The database also enforces this invariant to close concurrent request races.
	existing, err := s.repo.GetCertificateByCourseAndUser(ctx, courseID, userID)
	if err != nil {
		return nil, fmt.Errorf("check existing certificate: %w", err)
	}
	if existing != nil {
		return nil, errors.New("certificate already issued for this course")
	}

	cert, err := s.createCertificate(ctx, userID, courseID, enrollmentID)
	if err != nil {
		return nil, err
	}
	certNumber := cert.CertificateNumber
	now := cert.IssuedAt

	loaded, _ := s.repo.GetCertificateByID(ctx, cert.ID)
	if loaded != nil {
		return s.mapCertToDTO(loaded), nil
	}
	return &dto.CertificateResponseDTO{
		ID:                cert.ID,
		UserID:            userID,
		CourseID:          courseID,
		EnrollmentID:      enrollmentID,
		CertificateNumber: certNumber,
		IssuedAt:          now,
		CreatedAt:         cert.CreatedAt,
	}, nil
}

// createCertificate ghi một chứng chỉ mới rồi đẩy việc tạo PDF sang RabbitMQ. Nơi gọi chịu trách
// nhiệm kiểm tra ghi danh đã hoàn thành và chưa có chứng chỉ.
func (s *CertificateService) createCertificate(ctx context.Context, userID, courseID, enrollmentID uuid.UUID) (*model.Certificate, error) {
	certNumber := fmt.Sprintf("CERT-%s-%s", time.Now().Format("20060102"), uuid.New().String()[:8])
	now := time.Now()

	cert := &model.Certificate{
		UserID:            userID,
		CourseID:          courseID,
		EnrollmentID:      enrollmentID,
		CertificateNumber: certNumber,
		IssuedAt:          now,
	}

	if err := s.repo.CreateCertificate(ctx, cert); err != nil {
		return nil, err
	}

	// Push PDF generation to RabbitMQ (long-running task)
	if s.rabbitMQ != nil {
		loaded, _ := s.repo.GetCertificateByID(ctx, cert.ID)
		if loaded != nil {
			msg := CertificateGenerationMessage{
				CertificateID:     cert.ID,
				UserID:            userID,
				CourseID:          courseID,
				CertificateNumber: certNumber,
				IssuedAt:          now,
			}
			if loaded.User.FullName != nil {
				msg.UserName = *loaded.User.FullName
			} else {
				msg.UserName = loaded.User.UserName
			}
			msg.CourseName = loaded.Course.Title

			data, _ := json.Marshal(msg)
			if err := s.rabbitMQ.PublishMessage(ctx, "certificate", "certificate.generate", data); err != nil {
				log.Printf("[certificate] Failed to publish generation task: %v", err)
			}
		}
	}
	return cert, nil
}

// certificateBackfillPageSize: số ghi danh đọc mỗi trang khi quét cấp bù chứng chỉ.
const certificateBackfillPageSize = 100

// issueMissingCertificates (A4, QA vòng 2, N14): trước đây chứng chỉ KHÔNG BAO GIỜ tự cấp — chỉ có
// POST /certificates mà web không gọi ở đâu, nên học viên học xong 100% vẫn thấy "chưa có chứng
// chỉ". Hàm này cấp chứng chỉ cho mọi ghi danh đã hoàn thành (completed_at do recalculateProgress
// đặt khi tiến độ chạm 100%) mà chưa có chứng chỉ.
//
// Vì sao cấp ở đây (lúc đọc danh sách) thay vì ngay trong UpdateLessonProgress: nối
// EnrollmentService với CertificateService phải sửa app/services.go, file ngoài phạm vi lane A.
// Cấp lúc đọc vẫn đúng với người dùng (danh sách luôn đủ khi họ mở ra xem), idempotent, và cấp bù
// luôn cho người đã hoàn thành TRƯỚC bản vá này.
//
// Idempotent: kiểm tra chứng chỉ có sẵn trước khi tạo; hai request song song cùng tạo thì unique
// index (user_id, course_id) chặn bản thứ hai — lỗi đó được coi là "đã có", đọc lại để xác nhận.
func (s *CertificateService) issueMissingCertificates(ctx context.Context, userID uuid.UUID) error {
	if s.enrollmentRepo == nil {
		return nil
	}
	for page := 1; ; page++ {
		enrollments, total, err := s.enrollmentRepo.GetByUserID(ctx, userID, page, certificateBackfillPageSize)
		if err != nil {
			return fmt.Errorf("list enrollments: %w", err)
		}
		for i := range enrollments {
			e := &enrollments[i]
			if e.CompletedAt == nil {
				continue
			}
			if err := s.ensureCertificate(ctx, e); err != nil {
				return err
			}
		}
		if len(enrollments) == 0 || int64(page*certificateBackfillPageSize) >= total {
			return nil
		}
	}
}

// ensureCertificate tạo chứng chỉ cho một ghi danh đã hoàn thành nếu chưa có. Không lỗi khi đã có.
func (s *CertificateService) ensureCertificate(ctx context.Context, e *model.Enrollment) error {
	existing, err := s.repo.GetCertificateByCourseAndUser(ctx, e.CourseID, e.UserID)
	if err != nil {
		return fmt.Errorf("check existing certificate: %w", err)
	}
	if existing != nil {
		return nil
	}
	if _, err := s.createCertificate(ctx, e.UserID, e.CourseID, e.ID); err != nil {
		// Thua race với request song song: unique index đã có bản ghi -> coi như đã cấp.
		again, lookupErr := s.repo.GetCertificateByCourseAndUser(ctx, e.CourseID, e.UserID)
		if lookupErr == nil && again != nil {
			return nil
		}
		return fmt.Errorf("issue certificate for enrollment %s: %w", e.ID, err)
	}
	return nil
}

func (s *CertificateService) GetMyCertificates(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CertificateListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	if err := s.issueMissingCertificates(ctx, userID); err != nil {
		return nil, err
	}

	certs, total, err := s.repo.GetCertificatesByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.CertificateResponseDTO, len(certs))
	for i, c := range certs {
		data[i] = *s.mapCertToDTO(&c)
	}

	return &dto.CertificateListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *CertificateService) GetCertificateByID(ctx context.Context, id uuid.UUID) (*dto.CertificateResponseDTO, error) {
	cert, err := s.repo.GetCertificateByID(ctx, id)
	if err != nil || cert == nil {
		return nil, errors.New("certificate not found")
	}
	return s.mapCertToDTO(cert), nil
}

func (s *CertificateService) VerifyCertificate(ctx context.Context, number string) (*dto.VerifyCertificateResponseDTO, error) {
	// Check cache
	if s.redis != nil {
		cacheKey := certVerifyCachePrefix + number
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result dto.VerifyCertificateResponseDTO
			if json.Unmarshal([]byte(cached), &result) == nil {
				return &result, nil
			}
		}
	}

	cert, err := s.repo.GetCertificateByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		result := &dto.VerifyCertificateResponseDTO{
			Valid:             false,
			CertificateNumber: number,
		}
		return result, nil
	}

	userName := cert.User.UserName
	if cert.User.FullName != nil {
		userName = *cert.User.FullName
	}

	result := &dto.VerifyCertificateResponseDTO{
		Valid:             true,
		CertificateNumber: cert.CertificateNumber,
		UserName:          userName,
		CourseName:        cert.Course.Title,
		IssuedAt:          cert.IssuedAt,
	}

	// Cache
	if s.redis != nil {
		if data, err := json.Marshal(result); err == nil {
			s.redis.Set(ctx, certVerifyCachePrefix+number, data, certCacheTTL)
		}
	}

	return result, nil
}

// SetupCertificateQueues sets up RabbitMQ queues for certificate generation
func (s *CertificateService) SetupCertificateQueues() error {
	if s.rabbitMQ == nil {
		return fmt.Errorf("RabbitMQ not available")
	}

	if err := s.rabbitMQ.SetupQueue("certificate", "certificate.generate", "certificate.generate"); err != nil {
		return fmt.Errorf("setup certificate queue: %w", err)
	}
	return nil
}

func (s *CertificateService) mapCertToDTO(c *model.Certificate) *dto.CertificateResponseDTO {
	userName := c.User.UserName
	if c.User.FullName != nil {
		userName = *c.User.FullName
	}

	return &dto.CertificateResponseDTO{
		ID:                c.ID,
		UserID:            c.UserID,
		UserName:          userName,
		CourseID:          c.CourseID,
		CourseName:        c.Course.Title,
		EnrollmentID:      c.EnrollmentID,
		CertificateNumber: c.CertificateNumber,
		CertificateURL:    c.CertificateURL,
		IssuedAt:          c.IssuedAt,
		CreatedAt:         c.CreatedAt,
	}
}
