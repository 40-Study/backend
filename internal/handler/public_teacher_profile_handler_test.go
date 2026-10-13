package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type fakePublicTeacherService struct {
	page *dto.PublicTeacherPageDTO
	err  error
}

func (f fakePublicTeacherService) Get(context.Context, uuid.UUID) (*dto.PublicTeacherPageDTO, error) {
	return f.page, f.err
}

func publicTeacherApp(f fakePublicTeacherService) *fiber.App {
	app := fiber.New()
	app.Get("/teachers/:id/public-profile", NewPublicTeacherProfileHandler(f).GetPublicProfile)
	return app
}

func TestPublicTeacherProfileHandler_MaTrangThai(t *testing.T) {
	id := uuid.New()
	cases := []struct {
		name string
		path string
		svc  fakePublicTeacherService
		want int
	}{
		{"ok", "/teachers/" + id.String() + "/public-profile", fakePublicTeacherService{page: &dto.PublicTeacherPageDTO{ID: id, Courses: []dto.PublicTeacherCourseDTO{}}}, http.StatusOK},
		{"khong tim thay", "/teachers/" + id.String() + "/public-profile", fakePublicTeacherService{err: service.ErrTeacherPublicProfileNotFound}, http.StatusNotFound},
		{"uuid hong", "/teachers/abc/public-profile", fakePublicTeacherService{}, http.StatusBadRequest},
		{"loi ha tang khong lo chi tiet", "/teachers/" + id.String() + "/public-profile", fakePublicTeacherService{err: errors.New("pq: connection refused")}, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := doWishlist(t, publicTeacherApp(tc.svc), "GET", tc.path, "")
			if code != tc.want {
				t.Fatalf("status = %d, muon %d (%v)", code, tc.want, out)
			}
			if msg, _ := out["message"].(string); tc.want == http.StatusInternalServerError && msg != "Failed to retrieve teacher profile" {
				t.Errorf("500 lo chi tiet noi bo: %q", msg)
			}
		})
	}
}
