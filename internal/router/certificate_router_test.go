package router

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
)

// certRouteStubService: chỉ VerifyCertificate trả dữ liệu; mọi route khác không được tới service
// khi thiếu token.
type certRouteStubService struct{ reachedByID bool }

func (s *certRouteStubService) IssueCertificate(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*dto.CertificateResponseDTO, error) {
	return nil, errors.New("unused")
}

func (s *certRouteStubService) GetMyCertificates(context.Context, uuid.UUID, int, int) (*dto.CertificateListDTO, error) {
	return nil, errors.New("unused")
}

func (s *certRouteStubService) GetCertificateByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.CertificateResponseDTO, error) {
	s.reachedByID = true
	return nil, errors.New("unused")
}

func (s *certRouteStubService) VerifyCertificate(_ context.Context, number string) (*dto.VerifyCertificateResponseDTO, error) {
	return &dto.VerifyCertificateResponseDTO{Valid: true, CertificateNumber: number}, nil
}

// Tra cứu theo số chứng chỉ là CÔNG KHAI (không token vẫn 200); xem theo id thì bắt buộc đăng nhập.
func TestCertificateRoutes_VerifyIsPublic_ByIDRequiresAuth(t *testing.T) {
	svc := &certRouteStubService{}
	app := fiber.New()
	SetupCertificateRoutes(app.Group("/api"), &config.Config{}, handler.NewCertificateHandler(svc, nil), nil)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/certificates/verify/CERT-PUBLIC-1", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("verify công khai: status=%v err=%v, muốn 200 không cần token", resp.StatusCode, err)
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/api/certificates/"+uuid.New().String(), nil))
	if err != nil || resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("xem theo id không token: status=%v err=%v, muốn 401", resp.StatusCode, err)
	}
	if svc.reachedByID {
		t.Fatal("request không token không được chạm tới service GetCertificateByID")
	}
}
