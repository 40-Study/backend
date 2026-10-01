package service

// GET /certificates/:id: chỉ chủ chứng chỉ, admin hệ thống và phụ huynh có liên kết ACTIVE với học
// viên được xem. Người khác nhận ErrCertificateNotFound (404) y như id không tồn tại.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type certParentLinkStub struct {
	parentID, studentID uuid.UUID
	err                 error
}

func (s *certParentLinkStub) HasActiveParent(_ context.Context, parentID, studentID uuid.UUID) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return parentID == s.parentID && studentID == s.studentID, nil
}

func newCertAccessService(cert *model.Certificate) *CertificateService {
	return NewCertificateService(&certificateRepoStub{byID: cert}, &certificateEnrollmentRepoStub{}, nil, nil)
}

func sampleOwnedCertificate() *model.Certificate {
	return &model.Certificate{ID: uuid.New(), UserID: uuid.New(), CourseID: uuid.New(), CertificateNumber: "CERT-ACCESS-1", IssuedAt: time.Now()}
}

func TestGetCertificateByID_OwnerCanView(t *testing.T) {
	cert := sampleOwnedCertificate()
	got, err := newCertAccessService(cert).GetCertificateByID(context.Background(), cert.ID, cert.UserID, false)
	if err != nil || got == nil || got.ID != cert.ID {
		t.Fatalf("chủ chứng chỉ xem: got=%+v err=%v, muốn xem được", got, err)
	}
}

func TestGetCertificateByID_OtherUserGetsNotFound(t *testing.T) {
	cert := sampleOwnedCertificate()
	got, err := newCertAccessService(cert).GetCertificateByID(context.Background(), cert.ID, uuid.New(), false)
	if got != nil || !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("người khác xem: got=%+v err=%v, muốn ErrCertificateNotFound", got, err)
	}
}

func TestGetCertificateByID_AdminCanView(t *testing.T) {
	cert := sampleOwnedCertificate()
	got, err := newCertAccessService(cert).GetCertificateByID(context.Background(), cert.ID, uuid.New(), true)
	if err != nil || got == nil || got.ID != cert.ID {
		t.Fatalf("admin xem: got=%+v err=%v, muốn xem được", got, err)
	}
}

func TestGetCertificateByID_LinkedParentCanView(t *testing.T) {
	cert := sampleOwnedCertificate()
	parent := uuid.New()
	svc := newCertAccessService(cert)
	svc.SetParentLinkChecker(&certParentLinkStub{parentID: parent, studentID: cert.UserID})

	got, err := svc.GetCertificateByID(context.Background(), cert.ID, parent, false)
	if err != nil || got == nil || got.ID != cert.ID {
		t.Fatalf("phụ huynh liên kết active xem: got=%+v err=%v, muốn xem được", got, err)
	}
	// Phụ huynh của học viên KHÁC thì không.
	if _, err := svc.GetCertificateByID(context.Background(), cert.ID, uuid.New(), false); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("người không liên kết: err=%v, muốn ErrCertificateNotFound", err)
	}
}

// Lỗi tra liên kết phụ huynh = không được xem (fail-closed), không lộ lỗi hạ tầng.
func TestGetCertificateByID_ParentLookupErrorFailsClosed(t *testing.T) {
	cert := sampleOwnedCertificate()
	svc := newCertAccessService(cert)
	svc.SetParentLinkChecker(&certParentLinkStub{err: errors.New("db down")})

	if _, err := svc.GetCertificateByID(context.Background(), cert.ID, uuid.New(), false); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("lỗi tra liên kết: err=%v, muốn ErrCertificateNotFound", err)
	}
}

// Chưa nối parent checker (nil) thì không phụ huynh nào xem được.
func TestGetCertificateByID_NoParentCheckerDeniesNonOwner(t *testing.T) {
	cert := sampleOwnedCertificate()
	if _, err := newCertAccessService(cert).GetCertificateByID(context.Background(), cert.ID, uuid.New(), false); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("err=%v, muốn ErrCertificateNotFound", err)
	}
}

// Không tồn tại và đã thu hồi trả CÙNG lỗi với "không được xem" — kể cả với admin/chủ.
func TestGetCertificateByID_MissingOrRevokedIsNotFound(t *testing.T) {
	if _, err := newCertAccessService(nil).GetCertificateByID(context.Background(), uuid.New(), uuid.New(), true); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("không tồn tại: err=%v, muốn ErrCertificateNotFound", err)
	}
	cert := sampleOwnedCertificate()
	revokedAt := time.Now()
	cert.RevokedAt = &revokedAt
	if _, err := newCertAccessService(cert).GetCertificateByID(context.Background(), cert.ID, cert.UserID, false); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("đã thu hồi: err=%v, muốn ErrCertificateNotFound", err)
	}
}

// Lỗi DB không bị nuốt thành 404: trả lỗi gốc để handler trả 500 chung (RespondServiceError).
func TestGetCertificateByID_RepoErrorIsNotMaskedAsNotFound(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := NewCertificateService(&certificateRepoStub{byIDErr: dbErr}, &certificateEnrollmentRepoStub{}, nil, nil)
	_, err := svc.GetCertificateByID(context.Background(), uuid.New(), uuid.New(), false)
	if !errors.Is(err, dbErr) || errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("err=%v, muốn bọc lỗi DB, không phải ErrCertificateNotFound", err)
	}
}
