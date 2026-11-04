package handler

// QA3-01: GET /api/orders/admin?q= phải tới repository filter (đã cắt khoảng trắng); vắng q thì không lọc.

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type listOrdersSpy struct {
	service.AdminOrderServiceInterface
	got []repository.AdminOrderFilter
}

func (s *listOrdersSpy) ListOrders(_ context.Context, f repository.AdminOrderFilter) (*dto.AdminOrderListResponse, error) {
	s.got = append(s.got, f)
	return &dto.AdminOrderListResponse{}, nil
}

func TestAdminListOrders_QueryParamQ_ReachesFilter(t *testing.T) {
	spy := &listOrdersSpy{}
	h := NewAdminOrderHandler(nil, spy, nil)
	app := fiber.New()
	app.Get("/orders/admin", h.ListOrders)

	for _, tc := range []struct{ query, want string }{
		{"q=" + url.QueryEscape("  FX-000226 "), "FX-000226"},
		{"q=" + url.QueryEscape("Nguyễn văn"), "Nguyễn văn"},
		{"q=%20%20", ""},
		{"status=completed", ""},
	} {
		resp, err := app.Test(httptest.NewRequest("GET", "/orders/admin?"+tc.query, nil))
		if err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s: status=%v err=%v", tc.query, resp, err)
		}
		if got := spy.got[len(spy.got)-1].Keyword; got != tc.want {
			t.Errorf("%s: Keyword = %q, muốn %q", tc.query, got, tc.want)
		}
	}
}
