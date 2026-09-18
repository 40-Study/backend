package router

// Test cho hai route bo sung ngay 2026-09-15:
//   GET  /api/lessons/:lessonId/quizzes  (quiz_router.go       -> QuizHandler.GetQuizzesByLesson)
//   POST /api/progress                  (enrollment_router.go  -> EnrollmentHandler.TrackProgressBeacon)
//
// Truoc do ca hai deu khong ton tai: quiz khong bao gio tai duoc trong trinh phat
// (moi bai hoc nhan 404) va beacon luc dong tab mat tien do am tham.
//
// LY DO TEST NAM O DAY chu khong o package handler: Smart App Control tren may
// phat trien nay chan binary test cua internal/handler (do van con chan 8/8 lan),
// nhung binary cua internal/router thi chay duoc. Package router import duoc ca
// handler va service nen dung du de kiem ca hai route.
//
// Hai test dau kiem DANG KY route (path + method dung nhu web goi). Hai test sau
// kiem HANH VI handler qua request that, trong do quan trong nhat la
// TestTrackProgressBeacon_ParsesTextPlainBody: sendBeacon gui
// Content-Type "text/plain;charset=UTF-8" va KHONG cho doi header, nen neu handler
// dung c.BodyParser thi Fiber tra 422 va tien do bi mat — dung lai loi cu duoi mot
// hinh thuc khac. Test do la thu duy nhat chan hoi quy ay.

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
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
)

// ---------------------------------------------------------------------------
// Fake service: nhung interface that roi chi override method can dung. Method
// nao khong override ma bi goi se panic voi nil pointer — dung y muon, vi no to
// ra ngay test dang cham vao thu ngoai pham vi thay vi im lang tra zero value.
// ---------------------------------------------------------------------------

type fakeQuizService struct {
	service.QuizServiceInterface
	gotLessonID *uuid.UUID
	ret         *dto.QuizListDTO
}

func (f *fakeQuizService) GetAllQuizzes(
	_ context.Context, lessonID, _, _ *uuid.UUID, _ uuid.UUID, _ bool, _, _ int,
) (*dto.QuizListDTO, error) {
	f.gotLessonID = lessonID
	return f.ret, nil
}

type fakeEnrollmentService struct {
	service.EnrollmentServiceInterface
	gotUserID   uuid.UUID
	gotLessonID uuid.UUID
	gotReq      dto.UpdateLessonProgressDTO
	called      int
	// ret: phan hoi ma fake tra ve. Phase 1 §1 doi kieu tra ve sang LessonProgressStateDTO
	// (contract {lesson_id, status, watched_seconds, watched_pct, last_position_seconds,
	// completed_at, next_lesson_unlocked}) nen test phai khang dinh duoc DUNG shape do.
	ret *dto.LessonProgressStateDTO
}

func (f *fakeEnrollmentService) UpdateLessonProgress(
	_ context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO, _ bool,
) (*dto.LessonProgressStateDTO, error) {
	f.called++
	f.gotUserID = userID
	f.gotLessonID = lessonID
	f.gotReq = req
	if f.ret != nil {
		return f.ret, nil
	}
	return &dto.LessonProgressStateDTO{LessonID: lessonID}, nil
}

// routePaths tra ve tap "METHOD path" ma app that dang phuc vu.
func routePaths(app *fiber.App) map[string]bool {
	out := map[string]bool{}
	for _, stack := range app.Stack() {
		for _, r := range stack {
			out[r.Method+" "+r.Path] = true
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 1. Dang ky route
// ---------------------------------------------------------------------------

func TestLessonQuizzesRoute_IsRegistered(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")

	// cfg/redis nil an toan o day: AuthMiddleware chi dung closure luc setup,
	// khong doc cfg/redis cho toi khi co request di qua middleware.
	SetupQuizRoutes(api, nil, handler.NewQuizHandler(&fakeQuizService{}, nil), nil)

	want := "GET /api/lessons/:lessonId/quizzes"
	if got := routePaths(app); !got[want] {
		t.Fatalf("thieu route %q — web goi dung duong dan nay (services/quiz.service.ts getByLesson, lib/server-fetchers/curriculum.ts)", want)
	}
}

func TestProgressBeaconRoute_IsRegistered(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")

	SetupEnrollmentRoutes(api, nil, handler.NewEnrollmentHandler(&fakeEnrollmentService{}, nil), nil)

	want := "POST /api/progress"
	if got := routePaths(app); !got[want] {
		t.Fatalf("thieu route %q — player-client.tsx goi navigator.sendBeacon(\"/api/progress\")", want)
	}
}

// ---------------------------------------------------------------------------
// 2. Hanh vi handler
// ---------------------------------------------------------------------------

// TestGetQuizzesByLesson_ReturnsFlatArray: web doc `data` nhu MOT MANG
// (serverApi unwrap mot lop: `data.data ?? data`; client axios doc r.data.data).
// Neu handler tra thang envelope phan trang cua GetAllQuizzes thi web nhan
// object {data,total,page,page_size} va cho goi .length vo — day la test chan viec do.
func TestGetQuizzesByLesson_ReturnsFlatArray(t *testing.T) {
	lessonID := uuid.New()
	fake := &fakeQuizService{
		ret: &dto.QuizListDTO{
			Data:     []dto.QuizResponseDTO{{ID: uuid.New(), Title: "Quiz A"}},
			Total:    1,
			Page:     1,
			PageSize: 50,
		},
	}

	app := fiber.New()
	// Mount truc tiep, khong qua AuthMiddleware that: middleware can Redis that va
	// khong phai doi tuong cua test nay. Middleware gia dat user_id giong AuthMiddleware that lam
	// (c.Locals) — SEC-1 (vá lộ nội dung quiz) doi hoi handler doc duoc user_id de goi
	// GetAllQuizzes voi userID/isAdmin.
	app.Get("/api/lessons/:lessonId/quizzes", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, handler.NewQuizHandler(fake, nil).GetQuizzesByLesson)

	res, err := app.Test(httptest.NewRequest("GET", "/api/lessons/"+lessonID.String()+"/quizzes", nil))
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)

	// Giai ma vao struct co `data` la MANG. Neu handler tra envelope thi buoc
	// nay that bai — chinh la hoi quy can bat.
	var parsed struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("`data` khong phai mang (co the handler tra envelope phan trang): %v — body: %s", err, body)
	}
	if len(parsed.Data) != 1 || parsed.Data[0].Title != "Quiz A" {
		t.Fatalf("data = %+v, muon 1 quiz \"Quiz A\" — body: %s", parsed.Data, body)
	}

	// lessonId phai duoc chuyen xuong service, khong bi bo qua.
	if fake.gotLessonID == nil || *fake.gotLessonID != lessonID {
		t.Fatalf("service nhan lessonID = %v, muon %v", fake.gotLessonID, lessonID)
	}
}

// TestGetQuizzesByLesson_EmptyIsArrayNotNull: bai hoc khong co quiz phai ra
// `"data":[]`, khong phai `"data":null` — web lam `res || []` nhung client axios
// doc r.data.data roi goi .length, nen null se vo.
func TestGetQuizzesByLesson_EmptyIsArrayNotNull(t *testing.T) {
	fake := &fakeQuizService{ret: &dto.QuizListDTO{Data: nil, Total: 0, Page: 1, PageSize: 50}}

	app := fiber.New()
	// SEC-1: xem chu thich tai TestGetQuizzesByLesson_ReturnsFlatArray.
	app.Get("/api/lessons/:lessonId/quizzes", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, handler.NewQuizHandler(fake, nil).GetQuizzesByLesson)

	res, err := app.Test(httptest.NewRequest("GET", "/api/lessons/"+uuid.New().String()+"/quizzes", nil))
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 (bai khong co quiz KHONG duoc tra 404 — web se khong phan biet duoc \"khong co quiz\" voi \"route sai\")", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), `"data":[]`) {
		t.Fatalf("muon `\"data\":[]`, nhan: %s", body)
	}
}

// TestTrackProgressBeacon_ParsesTextPlainBody — test quan trong nhat trong file.
// navigator.sendBeacon LUON gui Content-Type "text/plain;charset=UTF-8" va khong
// cho doi. c.BodyParser cua Fiber tu choi content-type do (422), nen handler phai
// json.Unmarshal(c.Body()) truc tiep. Doi handler ve BodyParser se lam test nay do.
func TestTrackProgressBeacon_ParsesTextPlainBody(t *testing.T) {
	userID := uuid.New()
	lessonID := uuid.New()
	fake := &fakeEnrollmentService{}

	app := fiber.New()
	// Middleware gia dat user_id giong AuthMiddleware that lam (c.Locals).
	app.Post("/api/progress", func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		return c.Next()
	}, handler.NewEnrollmentHandler(fake, nil).TrackProgressBeacon)

	// Body camelCase y nguyen nhu player-client.tsx gui.
	body := `{"lessonId":"` + lessonID.String() + `","status":"in_progress","videoWatchedSeconds":137}`
	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		got, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, muon 200 (422 = handler dang dung BodyParser, khong doc duoc text/plain cua sendBeacon) — body: %s", res.StatusCode, got)
	}

	if fake.called != 1 {
		t.Fatalf("service duoc goi %d lan, muon 1", fake.called)
	}
	if fake.gotUserID != userID {
		t.Fatalf("userID = %v, muon %v", fake.gotUserID, userID)
	}
	if fake.gotLessonID != lessonID {
		t.Fatalf("lessonID = %v, muon %v (lessonId nam trong BODY vi sendBeacon chi nhan URL co dinh)", fake.gotLessonID, lessonID)
	}
	if fake.gotReq.VideoWatchedSecs == nil || *fake.gotReq.VideoWatchedSecs != 137 {
		t.Fatalf("videoWatchedSeconds = %v, muon 137 — field camelCase phai map dung", fake.gotReq.VideoWatchedSecs)
	}
	if fake.gotReq.Status == nil || *fake.gotReq.Status != "in_progress" {
		t.Fatalf("status = %v, muon \"in_progress\"", fake.gotReq.Status)
	}
}

// TestTrackProgressBeacon_RejectsNegativeSeconds: giu lai rang buoc min=0 da
// them sau su co 11/09 (client gui -999999 luu thang vao DB lam tong thoi gian
// hoc am). Duong beacon la duong ghi THU HAI nen phai chan cung mot thu.
func TestTrackProgressBeacon_RejectsNegativeSeconds(t *testing.T) {
	fake := &fakeEnrollmentService{}

	app := fiber.New()
	app.Post("/api/progress", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, handler.NewEnrollmentHandler(fake, nil).TrackProgressBeacon)

	body := `{"lessonId":"` + uuid.New().String() + `","status":"in_progress","videoWatchedSeconds":-999999}`
	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muon 400 cho so giay am", res.StatusCode)
	}
	if fake.called != 0 {
		t.Fatalf("service bi goi %d lan voi gia tri am — phai bi chan truoc khi ghi", fake.called)
	}
}

// TestTrackProgressBeacon_RejectsMissingLessonID: lessonId nam trong body nen
// khong co path param nao bat thay khi client bo sot no.
func TestTrackProgressBeacon_RejectsMissingLessonID(t *testing.T) {
	fake := &fakeEnrollmentService{}

	app := fiber.New()
	app.Post("/api/progress", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, handler.NewEnrollmentHandler(fake, nil).TrackProgressBeacon)

	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(`{"status":"in_progress","videoWatchedSeconds":10}`))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muon 400 khi thieu lessonId", res.StatusCode)
	}
	if fake.called != 0 {
		t.Fatalf("service bi goi %d lan du thieu lessonId", fake.called)
	}
}
