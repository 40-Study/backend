package handler

// Review vòng 2 (plans/reports/review-260928-round2-integration.md), mục C — MAJOR:
// cart_handler.go/review_handler.go trước đây trả nguyên văn err.Error() của tầng
// service/repository ra JSON response (kể cả lỗi hạ tầng không xác định: DB, GORM, ...). Test
// này khoá lại RespondServiceError: lỗi nghiệp vụ ĐÃ BIẾT (*apperr.KnownError) -> đúng
// Status + Message nguyên văn; lỗi KHÔNG xác định -> luôn 500, message chung, KHÔNG BAO GIỜ
// chứa nội dung err.Error() gốc trong response.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

func postJSONBody(t *testing.T, app *fiber.App, path string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("POST", path, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body loi: %v", err)
	}
	return resp.StatusCode, body
}

func TestRespondServiceError_KnownError_DungStatusVaMessageNguyenVan(t *testing.T) {
	app := fiber.New()
	app.Post("/x", func(c *fiber.Ctx) error {
		return RespondServiceError(c, apperr.Conflict("course already in cart"), "generic message khong duoc dung")
	})

	status, body := postJSONBody(t, app, "/x")
	if status != fiber.StatusConflict {
		t.Fatalf("status = %d, muon 409 (Conflict) cho KnownError", status)
	}
	if body["message"] != "course already in cart" {
		t.Fatalf("message = %v, muon dung nguyen van message cua KnownError", body["message"])
	}
	if _, hasErrorField := body["error"]; hasErrorField {
		t.Fatalf("response KHONG duoc co field 'error' rieng, body: %v", body)
	}
}

func TestRespondServiceError_LoiKhongXacDinh_Luon500VaKhongLoNoiDung(t *testing.T) {
	// Mo phong dung mot loi ha tang that: GORM/driver thuong nhung ten cot/constraint/duong dan
	// vao err.Error() -- day la thu KHONG duoc phep lot ra JSON response.
	dbErr := errors.New(`ERROR: duplicate key value violates unique constraint "cart_items_user_id_course_id_key" (SQLSTATE 23505) at /app/internal/repository/cart_repo.go:42`)

	app := fiber.New()
	app.Post("/x", func(c *fiber.Ctx) error {
		return RespondServiceError(c, dbErr, "Failed to add to cart")
	})

	status, body := postJSONBody(t, app, "/x")
	if status != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, muon 500 cho loi khong xac dinh", status)
	}
	if body["message"] != "Failed to add to cart" {
		t.Fatalf("message = %v, muon message chung da khai bao, khong phai chi tiet loi that", body["message"])
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "cart_items_user_id_course_id_key") ||
		strings.Contains(string(raw), "SQLSTATE") ||
		strings.Contains(string(raw), "/app/internal/repository") {
		t.Fatalf("response LO chi tiet loi ha tang: %s", raw)
	}
}

// --- Kiem qua dung 2 handler that (cart, review), khong chi ham RespondServiceError don le ---

type stubCartServiceForErrTest struct {
	service.CartServiceInterface
	err error
}

func (s *stubCartServiceForErrTest) AddToCart(ctx context.Context, userID uuid.UUID, req dto.AddToCartDTO) (*dto.CartItemResponseDTO, error) {
	return nil, s.err
}

func TestCartHandler_AddToCart_LoiNghiepVuDaBiet_TraDungStatusKhongLoChiTiet(t *testing.T) {
	h := NewCartHandler(&stubCartServiceForErrTest{err: apperr.Conflict("course already in cart")})
	app := mountWithCaller("POST", "/cart", uuid.New(), h.AddToCart)

	req := httptest.NewRequest("POST", "/cart", strings.NewReader(`{"course_id":"`+uuid.NewString()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, muon 409", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["message"] != "course already in cart" {
		t.Fatalf("message = %v", body["message"])
	}
	if _, has := body["error"]; has {
		t.Fatalf("response khong duoc co field 'error', body: %v", body)
	}
}

func TestCartHandler_AddToCart_LoiKhongXacDinh_500KhongLoRepoError(t *testing.T) {
	repoErr := errors.New(`pq: duplicate key value violates unique constraint "idx_cart_unique"`)
	h := NewCartHandler(&stubCartServiceForErrTest{err: repoErr})
	app := mountWithCaller("POST", "/cart", uuid.New(), h.AddToCart)

	req := httptest.NewRequest("POST", "/cart", strings.NewReader(`{"course_id":"`+uuid.NewString()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, muon 500", resp.StatusCode)
	}
	raw, _ := json.Marshal(func() map[string]interface{} {
		var b map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&b)
		return b
	}())
	if strings.Contains(string(raw), "idx_cart_unique") || strings.Contains(string(raw), "duplicate key") {
		t.Fatalf("response lo chi tiet loi repository: %s", raw)
	}
}

type stubReviewServiceForErrTest struct {
	service.ReviewServiceInterface
	err error
}

func (s *stubReviewServiceForErrTest) DeleteReview(ctx context.Context, reviewID, userID uuid.UUID) error {
	return s.err
}

func TestReviewHandler_DeleteReview_KhongPhaiChuSoHuu_403KhongLoChiTiet(t *testing.T) {
	h := NewReviewHandler(&stubReviewServiceForErrTest{err: apperr.Forbidden("forbidden: not the owner")})
	app := mountWithCaller("DELETE", "/reviews/:id", uuid.New(), h.DeleteReview)

	resp, err := app.Test(httptest.NewRequest("DELETE", "/reviews/"+uuid.NewString(), nil))
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["message"] != "forbidden: not the owner" {
		t.Fatalf("message = %v", body["message"])
	}
	if _, has := body["error"]; has {
		t.Fatalf("response khong duoc co field 'error', body: %v", body)
	}
}
