package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
)

// F-10: pin hợp đồng HTTP của wishlist (contract-be3.md §4) với service giả - mã trạng thái, thân JSON, route tĩnh
// "/ids" và "/check/:id" không bị "/:courseID" nuốt, và user_id lấy từ Locals (không từ body).

type fakeWishlistService struct {
	gotUser   uuid.UUID
	created   bool
	addErr    error
	removed   uuid.UUID
	inWish    bool
	courseIDs []uuid.UUID
}

func (f *fakeWishlistService) Add(_ context.Context, u, c uuid.UUID) (*dto.WishlistItemResponseDTO, bool, error) {
	f.gotUser = u
	if f.addErr != nil {
		return nil, false, f.addErr
	}
	return &dto.WishlistItemResponseDTO{ID: uuid.New(), CourseID: c, UserID: u}, f.created, nil
}
func (f *fakeWishlistService) List(_ context.Context, u uuid.UUID) (*dto.WishlistListResponseDTO, error) {
	f.gotUser = u
	return &dto.WishlistListResponseDTO{Items: []dto.WishlistItemResponseDTO{}}, nil
}
func (f *fakeWishlistService) CourseIDs(_ context.Context, u uuid.UUID) ([]uuid.UUID, error) {
	f.gotUser = u
	return f.courseIDs, nil
}
func (f *fakeWishlistService) Remove(_ context.Context, _, c uuid.UUID) error {
	f.removed = c
	return nil
}
func (f *fakeWishlistService) Check(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return f.inWish, nil
}

func wishlistTestApp(f *fakeWishlistService, user *uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user_id", *user)
		}
		return c.Next()
	})
	h := NewWishlistHandler(f)
	g := app.Group("/wishlist")
	g.Get("/", h.GetWishlist)
	g.Post("/", h.AddToWishlist)
	g.Get("/ids", h.GetWishlistIDs)
	g.Get("/check/:courseID", h.CheckCourseInWishlist)
	g.Delete("/:courseID", h.RemoveFromWishlist)
	return app
}

func doWishlist(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestWishlistHandler_AddStatusTheoCreated(t *testing.T) {
	user, course := uuid.New(), uuid.New()
	f := &fakeWishlistService{created: true}
	app := wishlistTestApp(f, &user)
	body := `{"course_id":"` + course.String() + `"}`

	if code, _ := doWishlist(t, app, "POST", "/wishlist", body); code != http.StatusCreated {
		t.Errorf("lan dau: %d, muon 201", code)
	}
	if f.gotUser != user {
		t.Errorf("service nhan user %s, muon %s", f.gotUser, user)
	}
	f.created = false
	code, out := doWishlist(t, app, "POST", "/wishlist", body)
	if code != http.StatusOK {
		t.Errorf("da co: %d, muon 200", code)
	}
	if data, _ := out["data"].(map[string]any); data["course_id"] != course.String() {
		t.Errorf("data.course_id = %v", data["course_id"])
	}
}

func TestWishlistHandler_AddLoi(t *testing.T) {
	user := uuid.New()
	f := &fakeWishlistService{addErr: apperr.NotFound("course not found")}
	app := wishlistTestApp(f, &user)

	code, out := doWishlist(t, app, "POST", "/wishlist", `{"course_id":"`+uuid.NewString()+`"}`)
	if code != http.StatusNotFound || out["message"] != "course not found" {
		t.Errorf("khoa khong ton tai: %d %v", code, out)
	}
	for name, body := range map[string]string{"thieu course_id": `{}`, "uuid hong": `{"course_id":"abc"}`, "khong phai json": `not json`} {
		code, out := doWishlist(t, app, "POST", "/wishlist", body)
		if code != http.StatusBadRequest || out["code"] != "VALIDATION_FAILED" {
			t.Errorf("%s: %d %v, muon 400 VALIDATION_FAILED", name, code, out)
		}
	}
}

func TestWishlistHandler_RouteTinhKhongBiParamNuot(t *testing.T) {
	user, course := uuid.New(), uuid.New()
	f := &fakeWishlistService{courseIDs: []uuid.UUID{course}, inWish: true}
	app := wishlistTestApp(f, &user)

	_, out := doWishlist(t, app, "GET", "/wishlist/ids", "")
	ids, _ := out["data"].(map[string]any)["course_ids"].([]any)
	if len(ids) != 1 || ids[0] != course.String() {
		t.Errorf("/ids: %v", out)
	}
	_, out = doWishlist(t, app, "GET", "/wishlist/check/"+course.String(), "")
	if in, _ := out["data"].(map[string]any)["in_wishlist"].(bool); !in {
		t.Errorf("/check: %v", out)
	}
	code, _ := doWishlist(t, app, "DELETE", "/wishlist/"+course.String(), "")
	if code != http.StatusOK || f.removed != course {
		t.Errorf("DELETE: %d removed=%s", code, f.removed)
	}
	if code, out := doWishlist(t, app, "DELETE", "/wishlist/not-a-uuid", ""); code != http.StatusBadRequest {
		t.Errorf("uuid hong: %d %v", code, out)
	}
}

func TestWishlistHandler_ThieuUserTra401(t *testing.T) {
	app := wishlistTestApp(&fakeWishlistService{}, nil)
	for _, c := range [][2]string{{"GET", "/wishlist"}, {"POST", "/wishlist"}, {"GET", "/wishlist/ids"}, {"DELETE", "/wishlist/" + uuid.NewString()}, {"GET", "/wishlist/check/" + uuid.NewString()}} {
		if code, _ := doWishlist(t, app, c[0], c[1], `{}`); code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d, muon 401", c[0], c[1], code)
		}
	}
}
