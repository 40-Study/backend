package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type certificateRepoStub struct {
	created   bool
	existing  *model.Certificate
	lookupErr error
	createErr error
}

func (s *certificateRepoStub) CreateCertificate(_ context.Context, _ *model.Certificate) error {
	s.created = true
	return s.createErr
}

func (s *certificateRepoStub) GetCertificateByID(context.Context, uuid.UUID) (*model.Certificate, error) {
	return nil, nil
}

func (s *certificateRepoStub) GetCertificateByNumber(context.Context, string) (*model.Certificate, error) {
	return nil, nil
}

func (s *certificateRepoStub) GetCertificatesByUserID(context.Context, uuid.UUID, int, int) ([]model.Certificate, int64, error) {
	return nil, 0, nil
}

func (s *certificateRepoStub) GetCertificateByCourseAndUser(context.Context, uuid.UUID, uuid.UUID) (*model.Certificate, error) {
	return s.existing, s.lookupErr
}

func (s *certificateRepoStub) UpdateCertificate(context.Context, *model.Certificate) error {
	return nil
}

func (s *certificateRepoStub) DeleteCertificate(context.Context, uuid.UUID) error {
	return nil
}

type certificateEnrollmentRepoStub struct {
	enrollment *model.Enrollment
}

func (s *certificateEnrollmentRepoStub) GetByID(context.Context, uuid.UUID) (*model.Enrollment, error) {
	return s.enrollment, nil
}

func TestIssueCertificateRejectsEnrollmentFromAnotherUser(t *testing.T) {
	userID := uuid.New()
	courseID := uuid.New()
	completedAt := time.Now()
	certRepo := &certificateRepoStub{}
	svc := NewCertificateService(
		certRepo,
		&certificateEnrollmentRepoStub{enrollment: &model.Enrollment{
			UserID:      uuid.New(),
			CourseID:    courseID,
			CompletedAt: &completedAt,
		}},
		nil,
		nil,
	)

	if _, err := svc.IssueCertificate(context.Background(), userID, courseID, uuid.New()); err == nil {
		t.Fatal("muon loi khi enrollment thuoc user khac")
	}
	if certRepo.created {
		t.Fatal("khong duoc tao certificate khi ownership sai")
	}
}

func TestIssueCertificateRejectsEnrollmentForAnotherCourse(t *testing.T) {
	userID := uuid.New()
	completedAt := time.Now()
	certRepo := &certificateRepoStub{}
	svc := NewCertificateService(
		certRepo,
		&certificateEnrollmentRepoStub{enrollment: &model.Enrollment{
			UserID:      userID,
			CourseID:    uuid.New(),
			CompletedAt: &completedAt,
		}},
		nil,
		nil,
	)

	if _, err := svc.IssueCertificate(context.Background(), userID, uuid.New(), uuid.New()); err == nil {
		t.Fatal("muon loi khi enrollment khong thuoc course duoc yeu cau")
	}
	if certRepo.created {
		t.Fatal("khong duoc tao certificate khi course sai")
	}
}

func TestIssueCertificateRejectsIncompleteEnrollment(t *testing.T) {
	userID := uuid.New()
	courseID := uuid.New()
	certRepo := &certificateRepoStub{}
	svc := NewCertificateService(
		certRepo,
		&certificateEnrollmentRepoStub{enrollment: &model.Enrollment{
			UserID:   userID,
			CourseID: courseID,
		}},
		nil,
		nil,
	)

	if _, err := svc.IssueCertificate(context.Background(), userID, courseID, uuid.New()); err == nil {
		t.Fatal("muon loi khi khoa hoc chua hoan thanh")
	}
	if certRepo.created {
		t.Fatal("khong duoc tao certificate truoc khi hoan thanh khoa hoc")
	}
}

func TestIssueCertificateAllowsMatchingCompletedEnrollment(t *testing.T) {
	userID := uuid.New()
	courseID := uuid.New()
	enrollmentID := uuid.New()
	completedAt := time.Now()
	certRepo := &certificateRepoStub{}
	svc := NewCertificateService(
		certRepo,
		&certificateEnrollmentRepoStub{enrollment: &model.Enrollment{
			BaseModel:   model.BaseModel{ID: enrollmentID},
			UserID:      userID,
			CourseID:    courseID,
			CompletedAt: &completedAt,
		}},
		nil,
		nil,
	)

	if _, err := svc.IssueCertificate(context.Background(), userID, courseID, enrollmentID); err != nil {
		t.Fatalf("enrollment hop le bi tu choi: %v", err)
	}
	if !certRepo.created {
		t.Fatal("certificate hop le phai duoc tao")
	}
}

func TestIssueCertificateStopsWhenDuplicateLookupFails(t *testing.T) {
	userID := uuid.New()
	courseID := uuid.New()
	completedAt := time.Now()
	certRepo := &certificateRepoStub{lookupErr: errors.New("database unavailable")}
	svc := NewCertificateService(
		certRepo,
		&certificateEnrollmentRepoStub{enrollment: &model.Enrollment{
			UserID:      userID,
			CourseID:    courseID,
			CompletedAt: &completedAt,
		}},
		nil,
		nil,
	)

	if _, err := svc.IssueCertificate(context.Background(), userID, courseID, uuid.New()); err == nil {
		t.Fatal("muon loi khi khong kiem tra duoc certificate da ton tai")
	}
	if certRepo.created {
		t.Fatal("khong duoc tao certificate khi duplicate lookup bi loi")
	}
}
