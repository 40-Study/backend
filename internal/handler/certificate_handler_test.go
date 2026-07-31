package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

// ============================================================================
// Stub service — implement CertificateServiceInterface, khong can DB/redis.
// ============================================================================

type stubCertificateService struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*dto.CertificateResponseDTO, error)
	listFn    func(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CertificateListDTO, error)
	verifyFn  func(ctx context.Context, number string) (*dto.VerifyCertificateResponseDTO, error)

	// Ghi lai tham so de assert phan trang
	gotPage     int
	gotPageSize int
}

func (s *stubCertificateService) IssueCertificate(ctx context.Context, userID, courseID, enrollmentID uuid.UUID) (*dto.CertificateResponseDTO, error) {
	return nil, errors.New("not used in these tests")
}

func (s *stubCertificateService) GetMyCertificates(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CertificateListDTO, error) {
	s.gotPage, s.gotPageSize = page, pageSize
	if s.listFn != nil {
		return s.listFn(ctx, userID, page, pageSize)
	}
	return &dto.CertificateListDTO{Data: []dto.CertificateResponseDTO{}}, nil
}

func (s *stubCertificateService) GetCertificateByID(ctx context.Context, id uuid.UUID) (*dto.CertificateResponseDTO, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return nil, errors.New("certificate not found")
}

func (s *stubCertificateService) VerifyCertificate(ctx context.Context, number string) (*dto.VerifyCertificateResponseDTO, error) {
	if s.verifyFn != nil {
		return s.verifyFn(ctx, number)
	}
	return nil, errors.New("certificate not found")
}

// ============================================================================
// Helpers
// ============================================================================

func sampleCert() *dto.CertificateResponseDTO {
	return &dto.CertificateResponseDTO{
		ID:                uuid.New(),
		UserID:            uuid.New(),
		UserName:          "Nguyen Van A",
		CourseID:          uuid.New(),
		CourseName:        "Lap trinh Go",
		EnrollmentID:      uuid.New(),
		CertificateNumber: "CERT-2026-0001",
		IssuedAt:          time.Now(),
		CreatedAt:         time.Now(),
	}
}

// decodeBody doc body thanh map de kiem tra dung ten field JSON that su tra ve.
func decodeBody(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		t.Fatalf("khong parse duoc JSON response: %v", err)
	}
	return out
}

// ============================================================================
// VerifyCertificate — endpoint CONG KHAI, frontend phu thuoc truc tiep shape nay
// ============================================================================

func TestVerifyCertificate_ValidNumber(t *testing.T) {
	svc := &stubCertificateService{
		verifyFn: func(ctx context.Context, number string) (*dto.VerifyCertificateResponseDTO, error) {
			return &dto.VerifyCertificateResponseDTO{
				Valid:             true,
				CertificateNumber: number,
				UserName:          "Nguyen Van A",
				CourseName:        "Lap trinh Go",
				IssuedAt:          time.Now(),
			}, nil
		},
	}

	app := fiber.New()
	h := NewCertificateHandler(svc)
	app.Get("/certificates/verify/:number", h.VerifyCertificate)

	resp, err := app.Test(httptest.NewRequest("GET", "/certificates/verify/CERT-2026-0001", nil))
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", resp.StatusCode)
	}

	body := decodeBody(t, resp.Body)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response thieu object 'data': %v", body)
	}

	// Cac field nay la contract voi web/src/services/certificate.service.ts.
	// Doi ten o day = vo frontend im lang (TS khong bat duoc).
	if data["valid"] != true {
		t.Errorf("valid = %v, muon true", data["valid"])
	}
	if data["user_name"] != "Nguyen Van A" {
		t.Errorf("user_name = %v", data["user_name"])
	}
	if data["course_name"] != "Lap trinh Go" {
		t.Errorf("course_name = %v", data["course_name"])
	}
	if _, exists := data["holder_name"]; exists {
		t.Error("khong duoc co 'holder_name' — frontend doc 'user_name'")
	}
	if _, exists := data["course_title"]; exists {
		t.Error("khong duoc co 'course_title' — frontend doc 'course_name'")
	}
}

func TestVerifyCertificate_NotFound(t *testing.T) {
	svc := &stubCertificateService{
		verifyFn: func(ctx context.Context, number string) (*dto.VerifyCertificateResponseDTO, error) {
			return nil, errors.New("certificate not found")
		},
	}

	app := fiber.New()
	h := NewCertificateHandler(svc)
	app.Get("/certificates/verify/:number", h.VerifyCertificate)

	resp, _ := app.Test(httptest.NewRequest("GET", "/certificates/verify/SAI", nil))
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, muon 404", resp.StatusCode)
	}

	// Endpoint cong khai: ma sai KHONG duoc lo thong tin nguoi hoc nao
	body := decodeBody(t, resp.Body)
	if _, exists := body["data"]; exists {
		t.Error("response loi khong duoc kem 'data'")
	}
}

// ============================================================================
// GetMyCertificates — shape phan trang ma frontend doc
// ============================================================================

func TestGetMyCertificates_ReturnsDataArrayNotCertificates(t *testing.T) {
	svc := &stubCertificateService{
		listFn: func(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CertificateListDTO, error) {
			return &dto.CertificateListDTO{
				Data:     []dto.CertificateResponseDTO{*sampleCert()},
				Total:    1,
				Page:     page,
				PageSize: pageSize,
			}, nil
		},
	}

	app := fiber.New()
	h := NewCertificateHandler(svc)
	app.Get("/certificates", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.GetMyCertificates(c)
	})

	resp, _ := app.Test(httptest.NewRequest("GET", "/certificates?page=2&page_size=5", nil))
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", resp.StatusCode)
	}

	body := decodeBody(t, resp.Body)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("thieu 'data': %v", body)
	}

	// Mang nam o 'data', KHONG phai 'certificates' — day chinh la cho
	// frontend tung viet sai va doc ra undefined.
	if _, exists := data["data"]; !exists {
		t.Error("thieu mang 'data' trong CertificateListDTO")
	}
	if _, exists := data["certificates"]; exists {
		t.Error("khong duoc co 'certificates' — frontend doc 'data'")
	}

	if svc.gotPage != 2 || svc.gotPageSize != 5 {
		t.Errorf("phan trang khong duoc truyen xuong: page=%d page_size=%d", svc.gotPage, svc.gotPageSize)
	}
}

func TestGetMyCertificates_Unauthorized(t *testing.T) {
	app := fiber.New()
	h := NewCertificateHandler(&stubCertificateService{})
	// Khong set user_id trong Locals -> mo phong request thieu auth
	app.Get("/certificates", h.GetMyCertificates)

	resp, _ := app.Test(httptest.NewRequest("GET", "/certificates", nil))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", resp.StatusCode)
	}
}

// ============================================================================
// GetCertificateByID
// ============================================================================

func TestGetCertificateByID_InvalidUUID(t *testing.T) {
	app := fiber.New()
	h := NewCertificateHandler(&stubCertificateService{})
	app.Get("/certificates/:id", h.GetCertificateByID)

	resp, _ := app.Test(httptest.NewRequest("GET", "/certificates/khong-phai-uuid", nil))
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muon 400", resp.StatusCode)
	}
}

func TestGetCertificateByID_FlatFields(t *testing.T) {
	cert := sampleCert()
	svc := &stubCertificateService{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*dto.CertificateResponseDTO, error) {
			return cert, nil
		},
	}

	app := fiber.New()
	h := NewCertificateHandler(svc)
	app.Get("/certificates/:id", h.GetCertificateByID)

	resp, _ := app.Test(httptest.NewRequest("GET", "/certificates/"+uuid.New().String(), nil))
	body := decodeBody(t, resp.Body)
	data := body["data"].(map[string]any)

	// Field phang, khong long object — khop web/src/services/certificate.service.ts
	if data["course_name"] != "Lap trinh Go" {
		t.Errorf("course_name = %v", data["course_name"])
	}
	if _, exists := data["course"]; exists {
		t.Error("khong duoc long object 'course' — frontend doc 'course_name'")
	}
	if _, exists := data["pdf_url"]; exists {
		t.Error("khong duoc co 'pdf_url' — ten that la 'certificate_url'")
	}
}

// ============================================================================
// TODO(bao-mat): GetCertificateByID hien KHONG kiem tra quyen so huu.
// Bat ky tai khoan da dang nhap nao biet UUID deu doc duoc chung chi cua
// nguoi khac (IDOR). Khi backend them check, bo sung test:
//   - user A goi GET /certificates/{id-cua-user-B} -> 403/404
// ============================================================================
