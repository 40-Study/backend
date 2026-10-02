package handler

// L1: voucher holders_only và discount_price ở tầng HTTP. Người không giữ voucher dành riêng nhận
// 404 y hệt mã không tồn tại (cùng status + body), ở cả tra mã công khai và tự lưu; discount_price
// sai trả 400 DISCOUNT_PRICE_INVALID cho cả tạo lẫn sửa khoá học.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type holdersVoucherStub struct {
	service.VoucherServiceInterface
	// holders: mã -> user giữ voucher. Mã không có trong map = không tồn tại.
	holders    map[string]uuid.UUID
	seenViewer uuid.UUID
}

func (s *holdersVoucherStub) GetVoucherByCodeForViewer(_ context.Context, code string, viewer uuid.UUID) (*model.Voucher, error) {
	s.seenViewer = viewer
	holder, ok := s.holders[code]
	if !ok || holder != viewer {
		return nil, repository.ErrVoucherNotFound
	}
	return &model.Voucher{Code: code, HoldersOnly: true}, nil
}

func (s *holdersVoucherStub) GetVoucherByID(context.Context, uuid.UUID) (*model.Voucher, error) {
	return &model.Voucher{Code: "HIDDEN"}, nil
}

func (s *holdersVoucherStub) SaveVoucher(_ context.Context, user uuid.UUID, req *dto.SaveVoucherRequest) (*model.UserVoucher, error) {
	if holder, ok := s.holders[req.VoucherCode]; !ok || holder != user {
		return nil, repository.ErrVoucherNotFound
	}
	return &model.UserVoucher{UserID: user}, nil
}

func doVoucher(t *testing.T, h *VoucherHandler, user *uuid.UUID, method, path string) (int, string) {
	t.Helper()
	app := fiber.New()
	set := func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user_id", *user)
		}
		return c.Next()
	}
	app.Get("/vouchers/code/:code", set, h.GetVoucherByCode)
	app.Post("/vouchers/:id/save", set, h.SaveVoucher)
	resp, err := app.Test(httptest.NewRequest(method, path, strings.NewReader("{}")))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestGetVoucherByCode_HoldersOnlyIsIndistinguishableFromMissing(t *testing.T) {
	holder := uuid.New()
	stranger := uuid.New()
	stub := &holdersVoucherStub{holders: map[string]uuid.UUID{"VIP": holder}}
	h := NewVoucherHandler(stub)

	missingStatus, missingBody := doVoucher(t, h, nil, "GET", "/vouchers/code/NOPE")
	for name, user := range map[string]*uuid.UUID{"khách": nil, "người ngoài": &stranger} {
		status, body := doVoucher(t, h, user, "GET", "/vouchers/code/VIP")
		if status != missingStatus || body != missingBody {
			t.Errorf("%s tra voucher dành riêng: %d %s, muốn y hệt mã không tồn tại: %d %s", name, status, body, missingStatus, missingBody)
		}
	}
	if missingStatus != fiber.StatusNotFound {
		t.Fatalf("mã không tồn tại: status %d, muốn 404", missingStatus)
	}

	status, _ := doVoucher(t, h, &holder, "GET", "/vouchers/code/VIP")
	if status != fiber.StatusOK || stub.seenViewer != holder {
		t.Fatalf("người giữ: status=%d viewer=%s, muốn 200 và handler truyền đúng user_id", status, stub.seenViewer)
	}
	if _, _ = doVoucher(t, h, nil, "GET", "/vouchers/code/VIP"); stub.seenViewer != uuid.Nil {
		t.Fatalf("khách phải truyền uuid.Nil, nhận %s", stub.seenViewer)
	}
}

func TestSaveVoucher_HoldersOnlyStrangerGets404NotBadRequest(t *testing.T) {
	stranger := uuid.New()
	stub := &holdersVoucherStub{holders: map[string]uuid.UUID{"HIDDEN": uuid.New()}}
	status, body := doVoucher(t, NewVoucherHandler(stub), &stranger, "POST", "/vouchers/"+uuid.NewString()+"/save")
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d (%s), muốn 404 như voucher không tồn tại", status, body)
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(body), &out)
	if out["code"] != "ERR_NOT_FOUND" || out["message"] != "Voucher not found" {
		t.Fatalf("body = %s, muốn ERR_NOT_FOUND / Voucher not found", body)
	}
}

// ── discount_price ─────────────────────────────────────────────────────────

type discountCourseStub struct {
	service.CourseServiceInterface
	err error
}

func (s *discountCourseStub) UpdateCourse(context.Context, uuid.UUID, uuid.UUID, bool, dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error) {
	return nil, s.err
}

func TestUpdateCourse_InvalidDiscountPriceReturns400WithCode(t *testing.T) {
	h := NewCourseHandler(&discountCourseStub{err: service.ErrDiscountPriceInvalid}, nil)
	app := fiber.New()
	app.Put("/courses/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.UpdateCourse(c)
	})
	req := httptest.NewRequest("PUT", "/courses/"+uuid.NewString(), strings.NewReader(`{"discount_price":600000}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != fiber.StatusBadRequest || out["code"] != DiscountPriceInvalidCode {
		t.Fatalf("status=%d body=%v, muốn 400 + code %s", resp.StatusCode, out, DiscountPriceInvalidCode)
	}
}

func TestWriteDiscountPriceInvalid_OnlyMatchesItsOwnError(t *testing.T) {
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error {
		if writeDiscountPriceInvalid(c, service.ErrNotCourseOwner) {
			t.Errorf("lỗi khác bị nuốt")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil || resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("err=%v status=%d", err, resp.StatusCode)
	}
}
