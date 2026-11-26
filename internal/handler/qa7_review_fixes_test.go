package handler_test

// QA7 review #155 (M1, M2, M5, L2, L6) ở tầng HTTP: handler THẬT, service thay bằng stub.
// Mỗi ca đỏ khi bỏ phần sửa tương ứng: bỏ utils.DecimalExponentWithinBounds ở courseMoneyError (M1), thay RespondServiceError /
// invalidIDResponse bằng err.Error() ở lesson_handler, system_role_handler, assignment_handler, assignmentWriteFail (M2),
// voucherActionFailed trả 400 cho lỗi lạ (M5), apperr.NotFound không mang code (L2), voucherLookupFailed (L6).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

// qa7rLeak: mô phỏng lỗi tầng DB; mọi mảnh của nó là thứ KHÔNG được xuất hiện trong body.
var qa7rLeak = errors.New(`pq: relation "lessons" does not exist (SQLSTATE 42P01) invalid UUID length: 14`)

func qa7rNoLeak(t *testing.T, label, raw string) {
	t.Helper()
	for _, bad := range []string{"SQLSTATE", "pq:", "relation", "invalid UUID", "does not exist", "json:", "unmarshal", "cannot unmarshal"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("%s: body lộ chi tiết nội bộ %q: %s", label, bad, raw)
		}
	}
}

// ── M1: số mũ khổng lồ không được đốt CPU ─────────────────────────────────────────────

func TestQA7R_M1_CoursePrice_SoMuKhongLo_400NhanhKhongChamService(t *testing.T) {
	for _, tc := range []struct{ name, fields string }{
		{"price mũ dương lớn", `"price":1e20000000`},
		{"price mũ âm lớn", `"price":1e-20000000`},
		{"price 0 với mũ lớn", `"price":0e20000000`},
		{"discount mũ dương lớn", `"price":50000,"discount_price":1e20000000`},
		{"discount mũ âm lớn", `"price":50000,"discount_price":1e-20000000`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &qa7CourseSvc{}
			app := qa7CourseApp(svc)
			start := time.Now()
			code, out, raw := qa7Do(t, app, "POST", "/courses", `{"title":"Khoá QA7",`+tc.fields+`}`)
			ucode, uout, uraw := qa7Do(t, app, "PUT", "/courses/"+uuid.NewString(), `{`+tc.fields+`}`)
			// Chưa vá: mỗi request tốn ~2 s CPU (1e20000000), nên trần 1 s cho CẢ HAI request đủ để đỏ.
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("2 request mất %v, muốn chặn ngay trước khi so sánh số lớn", elapsed)
			}
			for _, r := range []struct {
				code int
				out  map[string]any
				raw  string
			}{{code, out, raw}, {ucode, uout, uraw}} {
				if r.code != 400 || r.out["code"] != "ERR_VALIDATION" {
					t.Fatalf("status=%d body=%s, muốn 400 ERR_VALIDATION", r.code, r.raw)
				}
			}
			if svc.created != 0 || svc.edited != 0 {
				t.Fatalf("service bị gọi (%d,%d)", svc.created, svc.edited)
			}
		})
	}
}

// ── M2: lesson_handler không đưa err.Error() ra body ──────────────────────────────────

type qa7rLessonSvc struct {
	service.LessonServiceInterface
	err error
}

func (s qa7rLessonSvc) GetAllLessons(context.Context, uuid.UUID, uuid.UUID, bool) ([]dto.LessonResponseDTO, error) {
	return nil, s.err
}

func (s qa7rLessonSvc) GetLessonByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.LessonResponseDTO, error) {
	return nil, s.err
}

func (s qa7rLessonSvc) ReorderLessons(context.Context, uuid.UUID, uuid.UUID, bool, dto.ReorderDTO) error {
	return s.err
}

func qa7rLessonApp(err error) *fiber.App {
	h := handler.NewLessonHandler(qa7rLessonSvc{err: err}, nil)
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Get("/sections/:section_id/lessons", h.GetAllLessons)
	app.Put("/sections/:section_id/lessons/reorder", h.ReorderLessons)
	app.Get("/lessons/:id", h.GetLessonByID)
	app.Put("/lessons/:id", h.UpdateLesson)
	app.Delete("/lessons/:id", h.DeleteLesson)
	return app
}

func TestQA7R_M2_Lesson_LoiKhongXacDinh_500Chung(t *testing.T) {
	app := qa7rLessonApp(qa7rLeak)
	sec, les := uuid.NewString(), uuid.NewString()
	for _, r := range [][3]string{
		{"GET", "/sections/" + sec + "/lessons", ""},
		{"GET", "/lessons/" + les, ""},
		{"PUT", "/sections/" + sec + "/lessons/reorder", `{"items":[{"id":"` + les + `","display_order":1}]}`},
	} {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 500 || out["code"] != "ERR_INTERNAL" || out["message"] != "Đã có lỗi xảy ra, vui lòng thử lại" {
			t.Fatalf("%s %s: status=%d body=%s, muốn 500 ERR_INTERNAL chung", r[0], r[1], code, raw)
		}
		qa7rNoLeak(t, r[0]+" "+r[1], raw)
	}
}

func TestQA7R_M2_Lesson_IdSaiVaBodyHong_KhongLoLoiParse(t *testing.T) {
	app := qa7rLessonApp(nil)
	for _, r := range [][3]string{
		{"GET", "/sections/xyz/lessons", ""},
		{"GET", "/lessons/xyz", ""},
		{"PUT", "/lessons/xyz", `{}`},
		{"DELETE", "/lessons/xyz", ""},
		{"PUT", "/sections/xyz/lessons/reorder", `{}`},
	} {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 400 || out["code"] != "ERR_INVALID_ID" || out["message"] == "" {
			t.Fatalf("%s %s: status=%d body=%s, muốn 400 ERR_INVALID_ID", r[0], r[1], code, raw)
		}
		qa7rNoLeak(t, r[0]+" "+r[1], raw)
	}
	code, out, raw := qa7Do(t, app, "PUT", "/sections/"+uuid.NewString()+"/lessons/reorder", `{"items": "không phải mảng"}`)
	if code != 400 || out["code"] != "ERR_INVALID_REQUEST" {
		t.Fatalf("body sai kiểu: status=%d body=%s, muốn 400 ERR_INVALID_REQUEST", code, raw)
	}
	qa7rNoLeak(t, "reorder body", raw)
}

func TestQA7R_M2_L2_Lesson_KhongTimThay_404CoMa_LoiLaKhongGiaLam404(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"service báo không có", apperr.NotFound("Không tìm thấy bài học.")},
		{"khoá ẩn", service.ErrCourseHidden},
		{"bài không thuộc khoá", service.ErrLessonNotInCourse},
	} {
		code, out, raw := qa7Do(t, qa7rLessonApp(tc.err), "GET", "/lessons/"+uuid.NewString(), "")
		if code != 404 || out["code"] != "ERR_NOT_FOUND" || out["message"] == "" {
			t.Fatalf("%s: status=%d body=%s, muốn 404 ERR_NOT_FOUND", tc.name, code, raw)
		}
	}
	code, out, raw := qa7Do(t, qa7rLessonApp(service.ErrCourseHidden), "GET", "/sections/"+uuid.NewString()+"/lessons", "")
	if code != 404 || out["code"] != "ERR_NOT_FOUND" {
		t.Fatalf("danh sách bài của khoá ẩn: status=%d body=%s, muốn 404 ERR_NOT_FOUND", code, raw)
	}
	// Lỗi DB KHÔNG được giả làm 404 (trước đây GetLessonByID trả 404 cho mọi lỗi).
	if code, _, raw := qa7Do(t, qa7rLessonApp(qa7rLeak), "GET", "/lessons/"+uuid.NewString(), ""); code != 500 {
		t.Fatalf("lỗi DB: status=%d body=%s, muốn 500", code, raw)
	}
	// Reorder: bài không thuộc chương là lỗi nghiệp vụ đã biết (400), giữ nguyên câu tiếng Việt.
	known := apperr.BadRequest("Một hoặc nhiều bài học không thuộc chương này.")
	code, out, raw = qa7Do(t, qa7rLessonApp(known), "PUT", "/sections/"+uuid.NewString()+"/lessons/reorder",
		`{"items":[{"id":"`+uuid.NewString()+`","display_order":1}]}`)
	if code != 400 || out["message"] != known.Message {
		t.Fatalf("reorder lỗi nghiệp vụ: status=%d body=%s", code, raw)
	}
}

// ── M2: system_role_handler ───────────────────────────────────────────────────────────

type qa7rRoleSvc struct {
	service.SystemRoleServiceInterface
	err error
}

func (s qa7rRoleSvc) GetSystemRoleByID(context.Context, uuid.UUID) (*dto.SystemRoleDetailResponseDTO, error) {
	return nil, s.err
}

func (s qa7rRoleSvc) GetAllSystemRoles(context.Context, int, int, string, string) (*dto.SystemRoleListResponseDTO, error) {
	return nil, s.err
}
func (s qa7rRoleSvc) DeleteSystemRole(context.Context, uuid.UUID, bool) error { return s.err }
func (s qa7rRoleSvc) RestoreSystemRole(context.Context, uuid.UUID) error      { return s.err }
func (s qa7rRoleSvc) AddPermissionsToSystemRole(context.Context, uuid.UUID, dto.AddPermissionsToSystemRoleDTO) error {
	return s.err
}

func (s qa7rRoleSvc) RemovePermissionsFromSystemRole(context.Context, uuid.UUID, dto.RemovePermissionsFromSystemRoleDTO) error {
	return s.err
}

func (s qa7rRoleSvc) SetSystemRolePermissions(context.Context, uuid.UUID, dto.AddPermissionsToSystemRoleDTO) error {
	return s.err
}

func (s qa7rRoleSvc) GetSystemRolePermissions(context.Context, uuid.UUID) ([]dto.PermissionResponseDTO, error) {
	return nil, s.err
}

func qa7rRoleApp(err error) *fiber.App {
	h := handler.NewSystemRoleHandler(qa7rRoleSvc{err: err})
	app := fiber.New()
	app.Get("/system-roles", h.GetAllSystemRoles)
	app.Get("/system-roles/:id", h.GetSystemRole)
	app.Put("/system-roles/:id", h.UpdateSystemRole)
	app.Delete("/system-roles/:id", h.DeleteSystemRole)
	app.Post("/system-roles/:id/restore", h.RestoreSystemRole)
	app.Post("/system-roles/:id/permissions", h.AddPermissionsToSystemRole)
	app.Delete("/system-roles/:id/permissions", h.RemovePermissionsFromSystemRole)
	app.Put("/system-roles/:id/permissions", h.SetSystemRolePermissions)
	app.Get("/system-roles/:id/permissions", h.GetSystemRolePermissions)
	return app
}

func qa7rRoleRoutes() [][3]string {
	id := uuid.NewString()
	perms := `{"permission_ids":["` + uuid.NewString() + `"]}`
	return [][3]string{
		{"GET", "/system-roles", ""},
		{"GET", "/system-roles/" + id, ""},
		{"DELETE", "/system-roles/" + id, ""},
		{"POST", "/system-roles/" + id + "/restore", ""},
		{"POST", "/system-roles/" + id + "/permissions", perms},
		{"DELETE", "/system-roles/" + id + "/permissions", perms},
		{"PUT", "/system-roles/" + id + "/permissions", perms},
		{"GET", "/system-roles/" + id + "/permissions", ""},
	}
}

func TestQA7R_M2_SystemRole_LoiKhongXacDinh_500Chung(t *testing.T) {
	app := qa7rRoleApp(qa7rLeak)
	for _, r := range qa7rRoleRoutes() {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 500 || out["code"] != "ERR_INTERNAL" {
			t.Fatalf("%s %s: status=%d body=%s, muốn 500 ERR_INTERNAL", r[0], r[1], code, raw)
		}
		qa7rNoLeak(t, r[0]+" "+r[1], raw)
	}
}

func TestQA7R_M2_SystemRole_LoiDaBiet_GiuMaVaCauTiengViet(t *testing.T) {
	app := qa7rRoleApp(service.ErrSystemRoleNotFound)
	for _, r := range qa7rRoleRoutes()[1:] {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 404 || out["code"] != "ERR_NOT_FOUND" || out["message"] != "Không tìm thấy vai trò." {
			t.Fatalf("%s %s: status=%d body=%s, muốn 404 ERR_NOT_FOUND + câu tiếng Việt", r[0], r[1], code, raw)
		}
	}
	code, out, raw := qa7Do(t, qa7rRoleApp(service.ErrSystemRoleProtected), "DELETE", "/system-roles/"+uuid.NewString(), "")
	if code != 409 || out["code"] != "SYSTEM_ROLE_PROTECTED" || strings.Contains(raw, "built-in system role cannot be deleted") {
		t.Fatalf("xoá vai dựng sẵn: status=%d body=%s, muốn 409 SYSTEM_ROLE_PROTECTED không kèm lỗi gốc", code, raw)
	}
	code, out, raw = qa7Do(t, qa7rRoleApp(service.ErrSystemRolePermissionsInvalid), "POST",
		"/system-roles/"+uuid.NewString()+"/permissions", `{"permission_ids":["`+uuid.NewString()+`"]}`)
	if code != 400 || out["message"] != "Một hoặc nhiều quyền không tồn tại." {
		t.Fatalf("quyền không tồn tại: status=%d body=%s", code, raw)
	}
	// id sai + body hỏng: câu tiếng Việt, không lộ lỗi parse.
	app = qa7rRoleApp(nil)
	code, out, raw = qa7Do(t, app, "GET", "/system-roles/xyz", "")
	if code != 400 || out["code"] != "ERR_INVALID_ID" {
		t.Fatalf("id sai: status=%d body=%s", code, raw)
	}
	qa7rNoLeak(t, "id sai", raw)
	code, out, raw = qa7Do(t, app, "POST", "/system-roles/"+uuid.NewString()+"/permissions", `{"permission_ids": 5}`)
	if code != 400 || out["code"] != "ERR_INVALID_REQUEST" {
		t.Fatalf("body sai kiểu: status=%d body=%s", code, raw)
	}
	qa7rNoLeak(t, "body sai kiểu", raw)
}

// ── M2: assignment_handler (và assignmentWriteFail) ───────────────────────────────────

type qa7rAssignSvc struct {
	service.AssignmentServiceInterface
	err       error
	canManage bool
}

func (s qa7rAssignSvc) CanManage(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return s.canManage, nil
}

func (s qa7rAssignSvc) CanView(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return true, nil
}

func (s qa7rAssignSvc) GetByID(context.Context, uuid.UUID, bool) (*model.Assignment, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) GetTestCases(context.Context, uuid.UUID, bool) ([]model.TestCase, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) GetSandbox(context.Context, uuid.UUID, uuid.UUID) (*dto.SandboxResponseDTO, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) GetBySession(context.Context, uuid.UUID, bool, uuid.UUID, int, int) (*dto.AssignmentListDTO, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) GetByClass(context.Context, uuid.UUID, bool, uuid.UUID, int, int) (*dto.AssignmentListDTO, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) Create(context.Context, uuid.UUID, bool, dto.CreateAssignmentDTO) (*model.Assignment, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) Update(context.Context, uuid.UUID, dto.UpdateAssignmentDTO) (*model.Assignment, error) {
	return nil, s.err
}
func (s qa7rAssignSvc) Delete(context.Context, uuid.UUID) error { return s.err }
func (s qa7rAssignSvc) Publish(context.Context, uuid.UUID, service.LivekitServiceInterface) (*model.Assignment, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) Unpublish(context.Context, uuid.UUID) (*model.Assignment, error) {
	return nil, s.err
}

func (s qa7rAssignSvc) AddTestCase(context.Context, uuid.UUID, dto.CreateTestCaseDTO) (*model.TestCase, error) {
	return nil, s.err
}
func (s qa7rAssignSvc) DeleteTestCase(context.Context, uuid.UUID, uuid.UUID) error { return s.err }
func (s qa7rAssignSvc) ImportTestCases(context.Context, uuid.UUID, dto.ImportTestCasesDTO) ([]model.TestCase, error) {
	return nil, s.err
}

func qa7rAssignApp(svc qa7rAssignSvc) *fiber.App {
	h := handler.NewAssignmentHandler(svc, nil, nil)
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Post("/assignments", h.Create)
	app.Get("/assignments", h.GetBySession)
	app.Get("/assignments/:id", h.GetByID)
	app.Put("/assignments/:id", h.Update)
	app.Delete("/assignments/:id", h.Delete)
	app.Post("/assignments/:id/publish", h.Publish)
	app.Post("/assignments/:id/unpublish", h.Unpublish)
	app.Post("/assignments/:id/test-cases", h.AddTestCase)
	app.Post("/assignments/:id/test-cases/import", h.ImportTestCases)
	app.Get("/assignments/:id/test-cases", h.GetTestCases)
	app.Get("/assignments/:id/sandbox", h.GetSandbox)
	app.Get("/classes/:classId/assignments", h.GetByClass)
	return app
}

func TestQA7R_M2_Assignment_LoiKhongXacDinh_500Chung(t *testing.T) {
	app := qa7rAssignApp(qa7rAssignSvc{err: qa7rLeak, canManage: true})
	id := uuid.NewString()
	for _, r := range [][3]string{
		{"POST", "/assignments", `{"title":"Bài QA7","session_id":"` + uuid.NewString() + `"}`},
		{"GET", "/assignments?session_id=" + uuid.NewString(), ""},
		{"GET", "/classes/" + uuid.NewString() + "/assignments", ""},
		{"GET", "/assignments/" + id, ""},
		{"PUT", "/assignments/" + id, `{"title":"Sửa"}`},
		{"DELETE", "/assignments/" + id, ""},
		{"POST", "/assignments/" + id + "/publish", ""},
		{"POST", "/assignments/" + id + "/unpublish", ""},
		{"POST", "/assignments/" + id + "/test-cases", `{"input":"1","expected_output":"1"}`},
		{"POST", "/assignments/" + id + "/test-cases/import", `{"test_cases":[]}`},
		{"GET", "/assignments/" + id + "/test-cases", ""},
		{"GET", "/assignments/" + id + "/sandbox", ""},
	} {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 500 || out["code"] != "ERR_INTERNAL" {
			t.Fatalf("%s %s: status=%d body=%s, muốn 500 ERR_INTERNAL", r[0], r[1], code, raw)
		}
		qa7rNoLeak(t, r[0]+" "+r[1], raw)
	}
}

func TestQA7R_M2_Assignment_BodyHong_400KhongLoLoiParse(t *testing.T) {
	app := qa7rAssignApp(qa7rAssignSvc{canManage: true})
	id := uuid.NewString()
	for _, r := range [][3]string{
		{"POST", "/assignments", `{"title": 5`},
		{"PUT", "/assignments/" + id, `{"title": ["x"]}`},
		{"POST", "/assignments/" + id + "/test-cases", `{"input": 3}`},
		{"POST", "/assignments/" + id + "/test-cases/import", `not json`},
	} {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 400 || out["code"] != "ERR_INVALID_REQUEST" {
			t.Fatalf("%s %s: status=%d body=%s, muốn 400 ERR_INVALID_REQUEST", r[0], r[1], code, raw)
		}
		qa7rNoLeak(t, r[0]+" "+r[1], raw)
	}
}

func TestQA7R_M2_Assignment_LoiDaBiet_GiuMaVaStatus(t *testing.T) {
	id := uuid.NewString()
	for _, tc := range []struct {
		name string
		err  error
		code int
		want string
	}{
		{"lớp đã lưu trữ", service.ErrClassArchived, 409, "CLASS_ARCHIVED"},
		{"đã có bài nộp", service.ErrAssignmentHasSubmissions, 409, "ERR_ASSIGNMENT_HAS_SUBMISSIONS"},
		{"hạn nộp quá khứ", service.ErrDueDateInPast, 400, "ERR_DUE_DATE_IN_PAST"},
	} {
		code, out, raw := qa7Do(t, qa7rAssignApp(qa7rAssignSvc{err: tc.err, canManage: true}), "PUT", "/assignments/"+id, `{"title":"Sửa"}`)
		if code != tc.code || out["code"] != tc.want {
			t.Fatalf("%s: status=%d body=%s, muốn %d %s", tc.name, code, raw, tc.code, tc.want)
		}
	}
	// Id sai định dạng: web nhận ra qua body cũ {"error":"invalid id"} hoặc ERR_INVALID_ID, nên giữ nguyên chuỗi đó.
	code, out, raw := qa7Do(t, qa7rAssignApp(qa7rAssignSvc{canManage: true}), "GET", "/assignments/xyz", "")
	if code != 400 || out["error"] != "invalid id" {
		t.Fatalf("id sai: status=%d body=%s, muốn 400 {error: invalid id} như trước", code, raw)
	}
}

// ── M5 / L6: voucher ──────────────────────────────────────────────────────────────────

func TestQA7R_M5_Voucher_LoiLa_500ERR_INTERNAL_LoiDaBiet_GiuMaRieng(t *testing.T) {
	infra := qa7VoucherApp(&qa7VoucherSvc{err: errors.New("pq: connection refused")})
	for _, r := range [][3]string{
		{"DELETE", "/v/" + uuid.NewString(), ""},
		{"POST", "/v/" + uuid.NewString() + "/activate", ""},
		{"PUT", "/v/" + uuid.NewString(), `{"name":"Voucher QA7"}`},
		{"POST", "/v", `{` + qa7VoucherBase + `,"name":"Voucher QA7"}`},
	} {
		code, out, raw := qa7Do(t, infra, r[0], r[1], r[2])
		if code != 500 || out["code"] != "ERR_INTERNAL" || out["message"] == "" {
			t.Fatalf("%s %s: status=%d body=%s, muốn 500 ERR_INTERNAL", r[0], r[1], code, raw)
		}
		qa7rNoLeak(t, r[0]+" "+r[1], raw)
	}

	// Lỗi nghiệp vụ đã biết GIỮ code riêng của nó (không bị ghi đè bằng code của thao tác); không có code thì dùng code thao tác.
	withCode := qa7VoucherApp(&qa7VoucherSvc{err: &apperr.KnownError{Status: 409, Code: "ERR_VOUCHER_SOMETHING", Message: "Voucher đang được dùng."}})
	code, out, raw := qa7Do(t, withCode, "DELETE", "/v/"+uuid.NewString(), "")
	if code != 409 || out["code"] != "ERR_VOUCHER_SOMETHING" || out["message"] != "Voucher đang được dùng." {
		t.Fatalf("lỗi đã biết có code: status=%d body=%s", code, raw)
	}
	noCode := qa7VoucherApp(&qa7VoucherSvc{err: apperr.BadRequest("Ngày bắt đầu phải trước ngày kết thúc.")})
	code, out, raw = qa7Do(t, noCode, "PUT", "/v/"+uuid.NewString(), `{"name":"Voucher QA7"}`)
	if code != 400 || out["code"] != "ERR_UPDATE_VOUCHER" {
		t.Fatalf("lỗi đã biết không code: status=%d body=%s, muốn 400 ERR_UPDATE_VOUCHER", code, raw)
	}
}

func TestQA7R_L6_Voucher_ChiKhongCoMoiLa404(t *testing.T) {
	// GET theo id: lỗi DB không được giả làm 404.
	code, out, raw := qa7Do(t, qa7VoucherApp(&qa7VoucherSvc{err: errors.New("pq: connection refused")}), "GET", "/v/"+uuid.NewString(), "")
	if code != 500 || out["code"] != "ERR_INTERNAL" {
		t.Fatalf("GET lỗi DB: status=%d body=%s, muốn 500 ERR_INTERNAL", code, raw)
	}
	qa7rNoLeak(t, "GET lỗi DB", raw)
	code, out, raw = qa7Do(t, qa7VoucherApp(&qa7VoucherSvc{err: repository.ErrVoucherNotFound}), "GET", "/v/"+uuid.NewString(), "")
	if code != 404 || out["code"] != "ERR_NOT_FOUND" {
		t.Fatalf("GET không có: status=%d body=%s, muốn 404 ERR_NOT_FOUND", code, raw)
	}
}
