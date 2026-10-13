package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// Review QA2 H1: GET /discussions/:slug chỉ bật "xem bài đã ẩn" cho người kiểm duyệt đã đăng nhập; khách, người thường và
// handler chưa cấu hình checker đều không được. "post not found" của service vẫn là 404.

type hiddenViewSpy struct {
	service.DiscussionServiceInterface
	sawModerator bool
	err          error
}

func (s *hiddenViewSpy) GetPostBySlug(ctx context.Context, _ string, _ *uuid.UUID) (*dto.ForumPostDetailResponseDTO, error) {
	s.sawModerator = service.IsModeratorView(ctx)
	if s.err != nil {
		return nil, s.err
	}
	return &dto.ForumPostDetailResponseDTO{}, nil
}

func hiddenViewApp(h *DiscussionHandler, user *uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user_id", *user)
		}
		return c.Next()
	})
	app.Get("/discussions/:slug", h.GetPostBySlug)
	return app
}

func TestDiscussionHandler_BaiDaAn_ChiNguoiKiemDuyetDuocXem(t *testing.T) {
	mod, normal := uuid.New(), uuid.New()
	checker := func(_ *fiber.Ctx, id uuid.UUID) bool { return id == mod }
	cases := []struct {
		name    string
		user    *uuid.UUID
		checker func(*fiber.Ctx, uuid.UUID) bool
		want    bool
	}{
		{"kiem duyet", &mod, checker, true},
		{"nguoi thuong", &normal, checker, false},
		{"khach", nil, checker, false},
		{"chua cau hinh checker", &mod, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &hiddenViewSpy{}
			h := NewDiscussionHandler(spy)
			h.isModerator = tc.checker
			if code, _ := doWishlist(t, hiddenViewApp(h, tc.user), "GET", "/discussions/abc", ""); code != 200 {
				t.Fatalf("status %d", code)
			}
			if spy.sawModerator != tc.want {
				t.Errorf("moderator view = %v, muon %v", spy.sawModerator, tc.want)
			}
		})
	}
}

func TestDiscussionHandler_BaiDaAn_Tra404(t *testing.T) {
	h := NewDiscussionHandler(&hiddenViewSpy{err: errors.New("post not found")})
	if code, _ := doWishlist(t, hiddenViewApp(h, nil), "GET", "/discussions/abc", ""); code != 404 {
		t.Fatalf("status %d, muon 404", code)
	}
}
