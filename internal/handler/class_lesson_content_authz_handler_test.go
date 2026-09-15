package handler

// Test cho V3-7 (issue #58), tang handler cua nhom /lesson-contents/:id/classes: loi UY QUYEN tu
// service phai thanh 403 (khong phai 400/500), danh tinh nguoi goi phai lay tu access token
// (extractUserID) chu khong tu body/URL, va co nhanh 401 khi chua dang nhap.
//
// mountWithCaller/doJSON duoc dung lai tu livestream_authz_handler_test.go (cung package).

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// errKhongPhaiUyQuyen: loi nghiep vu binh thuong — dung de chot rang cong 403 khong bien MOI loi
// thanh 403.
var errKhongPhaiUyQuyen = errors.New("content not found")

// stubCLCService: ghi lai tham so ma handler truyen xuong, va tra ve loi uy quyen khi duoc yeu
// cau — nho vay test do duoc ca 2 dau: (1) handler phan loai dung loi thanh 403, (2) handler lay
// dung danh tinh nguoi goi.
type stubCLCService struct {
	err       error
	called    int
	gotUserID uuid.UUID
	gotAdmin  bool
	gotID     uuid.UUID
	gotID2    uuid.UUID
}

func (s *stubCLCService) record(userID uuid.UUID, isAdmin bool) {
	s.called++
	s.gotUserID = userID
	s.gotAdmin = isAdmin
}

func (s *stubCLCService) AssignClassToContent(ctx context.Context, contentID, userID uuid.UUID, isAdmin bool, req dto.AssignClassToContentDTO) (*dto.ClassLessonContentResponseDTO, error) {
	s.record(userID, isAdmin)
	s.gotID = contentID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.ClassLessonContentResponseDTO{ID: uuid.New()}, nil
}

func (s *stubCLCService) UpdateClassContentSchedule(ctx context.Context, contentID, classID, userID uuid.UUID, isAdmin bool, req dto.UpdateClassContentScheduleDTO) (*dto.ClassLessonContentResponseDTO, error) {
	s.record(userID, isAdmin)
	s.gotID, s.gotID2 = contentID, classID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.ClassLessonContentResponseDTO{ID: uuid.New()}, nil
}

func (s *stubCLCService) RemoveClassFromContent(ctx context.Context, contentID, classID, userID uuid.UUID, isAdmin bool) error {
	s.record(userID, isAdmin)
	s.gotID, s.gotID2 = contentID, classID
	return s.err
}

func (s *stubCLCService) GetClassesForContent(ctx context.Context, contentID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error) {
	s.record(userID, isAdmin)
	s.gotID = contentID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.ClassLessonContentListResponseDTO{Page: page, PageSize: pageSize}, nil
}

func (s *stubCLCService) GetContentScheduleForClass(ctx context.Context, classID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error) {
	s.record(userID, isAdmin)
	s.gotID = classID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.ClassLessonContentListResponseDTO{Page: page, PageSize: pageSize}, nil
}

func (s *stubCLCService) BulkAssignClassesToContent(ctx context.Context, contentID, userID uuid.UUID, isAdmin bool, req dto.BulkAssignClassesToContentDTO) ([]dto.ClassLessonContentResponseDTO, error) {
	s.record(userID, isAdmin)
	s.gotID = contentID
	if s.err != nil {
		return nil, s.err
	}
	return []dto.ClassLessonContentResponseDTO{}, nil
}

// mountCLC gan 1 route voi caller gia lap (hoac khong co caller nao neu caller == uuid.Nil, mo
// phong request chua dang nhap).
func mountCLC(method, path string, caller *uuid.UUID, h fiber.Handler) *fiber.App {
	app := fiber.New()
	if caller == nil {
		app.Add(method, path, h)
		return app
	}
	return mountWithCaller(method, path, *caller, h)
}

// TestCLCHandler_NguoiLa_Bi403: voi MOI handler trong nhom, mot loi uy quyen tu service phai ra
// 403 — day la khang dinh "khong duoc nuot loi uy quyen thanh 400/500".
func TestCLCHandler_NguoiLa_Bi403(t *testing.T) {
	caller := uuid.New()
	contentID := uuid.New()
	classID := uuid.New()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		route  string
		get    func(*ClassLessonContentHandler) fiber.Handler
	}{
		{"AssignClassToContent", "POST", "/lesson-contents/:id/classes", `{"class_id":"` + classID.String() + `"}`, "/lesson-contents/" + contentID.String() + "/classes", func(h *ClassLessonContentHandler) fiber.Handler { return h.AssignClassToContent }},
		{"UpdateClassContentSchedule", "PUT", "/lesson-contents/:id/classes/:class_id", `{}`, "/lesson-contents/" + contentID.String() + "/classes/" + classID.String(), func(h *ClassLessonContentHandler) fiber.Handler { return h.UpdateClassContentSchedule }},
		{"RemoveClassFromContent", "DELETE", "/lesson-contents/:id/classes/:class_id", "", "/lesson-contents/" + contentID.String() + "/classes/" + classID.String(), func(h *ClassLessonContentHandler) fiber.Handler { return h.RemoveClassFromContent }},
		{"GetClassesForContent", "GET", "/lesson-contents/:id/classes", "", "/lesson-contents/" + contentID.String() + "/classes", func(h *ClassLessonContentHandler) fiber.Handler { return h.GetClassesForContent }},
		{"GetContentScheduleForClass", "GET", "/classes/:id/contents", "", "/classes/" + classID.String() + "/contents", func(h *ClassLessonContentHandler) fiber.Handler { return h.GetContentScheduleForClass }},
		{"BulkAssignClassesToContent", "POST", "/lesson-contents/:id/classes/bulk", `{"class_ids":[]}`, "/lesson-contents/" + contentID.String() + "/classes/bulk", func(h *ClassLessonContentHandler) fiber.Handler { return h.BulkAssignClassesToContent }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubCLCService{err: service.ErrNotClassMember}
			h := NewClassLessonContentHandler(svc, nil)

			// mountCLC dang ky theo PATTERN (tc.path, vd "/lesson-contents/:id/classes") de
			// c.Params("id")/(":class_id") parse duoc; request thuc di den PATH cu the (tc.route).
			app := mountCLC(tc.method, tc.path, &caller, tc.get(h))
			status := doJSON(t, app, tc.method, tc.route, tc.body)

			if status != fiber.StatusForbidden {
				t.Fatalf("status = %d, muon 403 (loi uy quyen tu service)", status)
			}
			if svc.gotUserID != caller {
				t.Errorf("userID truyen xuong service = %s, muon nguoi dang dang nhap = %s", svc.gotUserID, caller)
			}
		})
	}
}

// TestCLCHandler_ThanhVienHopLe_2xx: cung 6 handler, khi service khong tra loi uy quyen thi phai
// di het duong (khong bi chan nham) — tuc la cong kiem quyen khong pha nguoi dung hop le.
func TestCLCHandler_ThanhVienHopLe_2xx(t *testing.T) {
	caller := uuid.New()
	contentID := uuid.New()
	classID := uuid.New()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		route  string
		get    func(*ClassLessonContentHandler) fiber.Handler
		want   int
	}{
		{"AssignClassToContent", "POST", "/lesson-contents/:id/classes", `{"class_id":"` + classID.String() + `"}`, "/lesson-contents/" + contentID.String() + "/classes", func(h *ClassLessonContentHandler) fiber.Handler { return h.AssignClassToContent }, fiber.StatusCreated},
		{"UpdateClassContentSchedule", "PUT", "/lesson-contents/:id/classes/:class_id", `{}`, "/lesson-contents/" + contentID.String() + "/classes/" + classID.String(), func(h *ClassLessonContentHandler) fiber.Handler { return h.UpdateClassContentSchedule }, fiber.StatusOK},
		{"RemoveClassFromContent", "DELETE", "/lesson-contents/:id/classes/:class_id", "", "/lesson-contents/" + contentID.String() + "/classes/" + classID.String(), func(h *ClassLessonContentHandler) fiber.Handler { return h.RemoveClassFromContent }, fiber.StatusOK},
		{"GetClassesForContent", "GET", "/lesson-contents/:id/classes", "", "/lesson-contents/" + contentID.String() + "/classes", func(h *ClassLessonContentHandler) fiber.Handler { return h.GetClassesForContent }, fiber.StatusOK},
		{"GetContentScheduleForClass", "GET", "/classes/:id/contents", "", "/classes/" + classID.String() + "/contents", func(h *ClassLessonContentHandler) fiber.Handler { return h.GetContentScheduleForClass }, fiber.StatusOK},
		{"BulkAssignClassesToContent", "POST", "/lesson-contents/:id/classes/bulk", `{"class_ids":[]}`, "/lesson-contents/" + contentID.String() + "/classes/bulk", func(h *ClassLessonContentHandler) fiber.Handler { return h.BulkAssignClassesToContent }, fiber.StatusCreated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubCLCService{}
			h := NewClassLessonContentHandler(svc, nil)

			app := mountCLC(tc.method, tc.path, &caller, tc.get(h))
			status := doJSON(t, app, tc.method, tc.route, tc.body)

			if status != tc.want {
				t.Fatalf("status = %d, muon %d", status, tc.want)
			}
			if svc.called != 1 {
				t.Errorf("service duoc goi %d lan, muon 1", svc.called)
			}
			if svc.gotAdmin {
				t.Error("gotAdmin = true du permChecker = nil (khong truyen) — isAdminActor phai fail-closed")
			}
		})
	}
}

// TestCLCHandler_ChuaDangNhap_Bi401: khong co user_id trong Locals thi phai 401 TRUOC khi cham
// service. Truoc day 2 handler (AssignClassToContent, BulkAssignClassesToContent) dung
// `c.Locals("user_id").(uuid.UUID)` khong kiem tra — panic (500) khi local la string.
func TestCLCHandler_ChuaDangNhap_Bi401(t *testing.T) {
	contentID := uuid.New()

	cases := []struct {
		name  string
		path  string // route PATTERN dang ky (can ":id" de handler parse duoc content/class ID)
		route string // path CU THE dung de goi request
		body  string
		get   func(*ClassLessonContentHandler) fiber.Handler
	}{
		{"AssignClassToContent", "/lesson-contents/:id/classes", "/lesson-contents/" + contentID.String() + "/classes", `{"class_id":"` + uuid.New().String() + `"}`, func(h *ClassLessonContentHandler) fiber.Handler { return h.AssignClassToContent }},
		{"BulkAssignClassesToContent", "/lesson-contents/:id/classes/bulk", "/lesson-contents/" + contentID.String() + "/classes/bulk", `{"class_ids":[]}`, func(h *ClassLessonContentHandler) fiber.Handler { return h.BulkAssignClassesToContent }},
		{"GetContentScheduleForClass", "/classes/:id/contents", "/classes/" + contentID.String() + "/contents", "", func(h *ClassLessonContentHandler) fiber.Handler { return h.GetContentScheduleForClass }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubCLCService{}
			h := NewClassLessonContentHandler(svc, nil)

			app := mountCLC("POST", tc.path, nil, tc.get(h))
			req := httptest.NewRequest("POST", tc.route, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test loi: %v", err)
			}
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, muon 401", resp.StatusCode)
			}
			if svc.called != 0 {
				t.Error("service bi goi du chua xac thuc")
			}
		})
	}
}

// TestCLCHandler_LoiKhacUyQuyen_VanLa400/500: mot loi KHONG phai loi uy quyen (vd khong tim thay
// ban ghi) phai giu nguyen ma cu — neu khong, cong 403 se bien moi loi thanh "khong co quyen" va
// lam mat thong tin chan doan.
func TestCLCHandler_LoiKhacUyQuyen_VanLa400(t *testing.T) {
	caller := uuid.New()
	contentID := uuid.New()

	svc := &stubCLCService{err: errKhongPhaiUyQuyen}
	h := NewClassLessonContentHandler(svc, nil)

	app := mountCLC("GET", "/lesson-contents/:id/classes", &caller, h.GetClassesForContent)
	status := doJSON(t, app, "GET", "/lesson-contents/"+contentID.String()+"/classes", "")

	if status == fiber.StatusForbidden {
		t.Fatal("loi khong phai uy quyen bi doi thanh 403 — mat thong tin chan doan")
	}
	if status != fiber.StatusInternalServerError {
		t.Errorf("status = %d, muon 500 (giu nguyen hanh vi cu)", status)
	}
}

// TestCLCHandler_UpdateKhongCoClassID_Bi400: thieu tham so duong dan van la 400 nhu truoc.
func TestCLCHandler_UpdateKhongCoClassID_Bi400(t *testing.T) {
	caller := uuid.New()
	svc := &stubCLCService{}
	h := NewClassLessonContentHandler(svc, nil)

	app := mountCLC("PUT", "/lesson-contents/:id/classes/:class_id", &caller, h.UpdateClassContentSchedule)
	status := doJSON(t, app, "PUT", "/lesson-contents/"+uuid.New().String()+"/classes/khong-phai-uuid", `{}`)

	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muon 400", status)
	}
	if svc.called != 0 {
		t.Error("service bi goi du class_id khong hop le")
	}
}
