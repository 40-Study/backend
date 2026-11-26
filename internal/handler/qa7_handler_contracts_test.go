package handler_test

// QA7 (C12, C13, C14, C17, A03, A09) ở tầng HTTP: handler THẬT, service thay bằng stub.
// Mỗi ca đỏ khi bỏ phần sửa tương ứng: bỏ lessonDurationError (C12), courseMoneyError / RespondServiceError ở course_handler
// (C13), ErrSystemRoleNameExists ở system_role_handler (C14), RespondServiceError ở parent_dashboard_handler (C17), TrimSpace tên
// hoặc voucherActionFailed ở voucher_handler (A03/A09).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

func qa7Do(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any, string) {
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
	return res.StatusCode, out, string(raw)
}

func qa7AsUser(user uuid.UUID) fiber.Handler {
	return func(c *fiber.Ctx) error { c.Locals("user_id", user); return c.Next() }
}

// ── C12: thời lượng bài học ───────────────────────────────────────────────────────────

type qa7LessonSvc struct {
	service.LessonServiceInterface
	created, updated int
}

func (s *qa7LessonSvc) CreateLesson(context.Context, uuid.UUID, uuid.UUID, dto.CreateLessonDTO) (*dto.LessonResponseDTO, error) {
	s.created++
	return &dto.LessonResponseDTO{}, nil
}

func (s *qa7LessonSvc) UpdateLesson(context.Context, uuid.UUID, uuid.UUID, bool, dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error) {
	s.updated++
	return &dto.LessonResponseDTO{}, nil
}

func TestQA7_C12_LessonDuration_0Den1440_NguyenPhut(t *testing.T) {
	for _, tc := range []struct {
		name, dur string
		ok        bool
	}{
		{"âm", `-5`, false}, {"quá 1440", `1441`, false}, {"khổng lồ", `99999999999`, false}, {"số lẻ", `2.5`, false},
		{"chữ", `"abc"`, false}, {"tràn int64", `1e30`, false},
		{"0", `0`, true}, {"1440", `1440`, true}, {"bình thường", `45`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &qa7LessonSvc{}
			h := handler.NewLessonHandler(svc, nil)
			app := fiber.New()
			app.Use(qa7AsUser(uuid.New()))
			app.Post("/sections/:section_id/lessons", h.CreateLesson)
			app.Put("/lessons/:id", h.UpdateLesson)
			body := `{"title":"Bài QA7","duration_minutes":` + tc.dur + `}`

			code, out, raw := qa7Do(t, app, "POST", "/sections/"+uuid.NewString()+"/lessons", body)
			ucode, uout, uraw := qa7Do(t, app, "PUT", "/lessons/"+uuid.NewString(), body)
			if tc.ok {
				if code != 201 || ucode != 200 {
					t.Fatalf("hợp lệ: create=%d update=%d (%s | %s)", code, ucode, raw, uraw)
				}
				return
			}
			for _, r := range []struct {
				code int
				out  map[string]any
				raw  string
			}{{code, out, raw}, {ucode, uout, uraw}} {
				msg, _ := r.out["message"].(string)
				if r.code != 400 || r.out["code"] != "ERR_VALIDATION" || !strings.Contains(msg, "Thời lượng bài học") {
					t.Fatalf("status=%d body=%s, muốn 400 ERR_VALIDATION nêu \"Thời lượng bài học\"", r.code, r.raw)
				}
			}
			if svc.created != 0 || svc.updated != 0 {
				t.Fatalf("service bị gọi (%d,%d) dù dữ liệu sai", svc.created, svc.updated)
			}
		})
	}
}

func TestQA7_C12_LessonDuration_KhongGuiThiHopLe(t *testing.T) {
	h := handler.NewLessonHandler(&qa7LessonSvc{}, nil)
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Post("/sections/:section_id/lessons", h.CreateLesson)
	if code, _, raw := qa7Do(t, app, "POST", "/sections/"+uuid.NewString()+"/lessons", `{"title":"Bài không thời lượng"}`); code != 201 {
		t.Fatalf("không gửi duration_minutes: %d %s", code, raw)
	}
}

// ── C13: giá khoá học, không lộ err.Error() ────────────────────────────────────────────

type qa7CourseSvc struct {
	service.CourseServiceInterface
	err             error
	created, edited int
}

func (s *qa7CourseSvc) CreateCourse(context.Context, dto.CreateCourseDTO) (*dto.CourseResponseDTO, error) {
	s.created++
	return &dto.CourseResponseDTO{}, s.err
}

func (s *qa7CourseSvc) UpdateCourse(context.Context, uuid.UUID, uuid.UUID, bool, dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error) {
	s.edited++
	return &dto.CourseResponseDTO{}, s.err
}

func qa7CourseApp(svc *qa7CourseSvc) *fiber.App {
	teacher, role := uuid.New(), uuid.New()
	sysRoles := &asFakeSystemRoleRepo{perms: map[uuid.UUID][]string{role: {"COURSES_CREATE"}}}
	userRoles := &asFakeUserSystemRoleRepo{byUser: map[uuid.UUID][]model.UserSystemRole{
		teacher: {{UserID: teacher, SystemRoleID: role, Status: model.UserSystemRoleStatusActive}},
	}}
	h := handler.NewCourseHandler(svc, middleware.NewPermissionChecker(userRoles, sysRoles, nil, nil))
	app := fiber.New()
	app.Use(qa7AsUser(teacher))
	app.Post("/courses", h.CreateCourse)
	app.Put("/courses/:id", h.UpdateCourse)
	app.Get("/courses/:id", h.GetCourseByID)
	return app
}

func TestQA7_C13_CoursePrice_NguyenDongTrongKhoang(t *testing.T) {
	for _, tc := range []struct {
		name, fields, wantField string // wantField rỗng = hợp lệ
	}{
		{"tràn numeric", `"price":99999999999999`, "Giá bán"},
		{"vượt 1 tỷ", `"price":1000000001`, "Giá bán"},
		{"số lẻ nhỏ", `"price":0.5`, "Giá bán"},
		{"số lẻ", `"price":1000.55`, "Giá bán"},
		{"âm", `"price":-3`, "Giá bán"},
		{"khuyến mãi lẻ", `"price":50000,"discount_price":1000.5`, "Giá khuyến mãi"},
		{"khuyến mãi vượt", `"price":50000,"discount_price":2000000000`, "Giá khuyến mãi"},
		{"đúng 1 tỷ", `"price":1000000000`, ""},
		{"miễn phí giá 0", `"price":0,"is_free":true`, ""},
		{"giá thường", `"price":99000,"discount_price":49000`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &qa7CourseSvc{}
			app := qa7CourseApp(svc)
			body := `{"title":"Khoá QA7",` + tc.fields + `}`
			code, out, raw := qa7Do(t, app, "POST", "/courses", body)
			ucode, uout, uraw := qa7Do(t, app, "PUT", "/courses/"+uuid.NewString(), `{`+tc.fields+`}`)
			if tc.wantField == "" {
				if code != 201 || ucode != 200 {
					t.Fatalf("hợp lệ: create=%d update=%d (%s | %s)", code, ucode, raw, uraw)
				}
				return
			}
			for _, r := range []struct {
				code int
				out  map[string]any
				raw  string
			}{{code, out, raw}, {ucode, uout, uraw}} {
				msg, _ := r.out["message"].(string)
				if r.code != 400 || r.out["code"] != "ERR_VALIDATION" || !strings.Contains(msg, tc.wantField) {
					t.Fatalf("status=%d body=%s, muốn 400 ERR_VALIDATION nêu %q", r.code, r.raw, tc.wantField)
				}
			}
			if svc.created != 0 || svc.edited != 0 {
				t.Fatalf("service bị gọi (%d,%d): lỗi đầu vào phải chặn trước khi chạm DB", svc.created, svc.edited)
			}
		})
	}
}

func TestQA7_C13_LoiKhongXacDinh_500ChungKhongLoSQL(t *testing.T) {
	leak := errors.New("ERROR: numeric field overflow (SQLSTATE 22003)")
	for _, tc := range []struct{ name, method, path, body string }{
		{"tạo", "POST", "/courses", `{"title":"Khoá QA7","price":1000}`},
		{"sửa", "PUT", "/courses/" + uuid.NewString(), `{"title":"Khoá sửa"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := qa7CourseApp(&qa7CourseSvc{err: leak})
			code, out, raw := qa7Do(t, app, tc.method, tc.path, tc.body)
			if code != 500 || out["code"] != "ERR_INTERNAL" || out["message"] != "Đã có lỗi xảy ra, vui lòng thử lại" || strings.Contains(raw, "SQLSTATE") || strings.Contains(raw, "numeric") {
				t.Fatalf("status=%d body=%s, muốn 500 ERR_INTERNAL chung, không lộ lỗi SQL", code, raw)
			}
		})
	}
}

func TestQA7_C13_LoiNghiepVuDaBiet_GiuStatusVaCau(t *testing.T) {
	app := qa7CourseApp(&qa7CourseSvc{err: apperr.NotFound("Không tìm thấy khoá học.")})
	code, out, raw := qa7Do(t, app, "PUT", "/courses/"+uuid.NewString(), `{"title":"Khoá sửa"}`)
	if code != 404 || out["message"] != "Không tìm thấy khoá học." {
		t.Fatalf("status=%d body=%s, muốn 404 + câu tiếng Việt", code, raw)
	}
}

// ── C14: vai trò hệ thống trùng tên ───────────────────────────────────────────────────

type qa7SysRoleSvc struct {
	service.SystemRoleServiceInterface
	err error
}

func (s qa7SysRoleSvc) CreateSystemRole(context.Context, dto.CreateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	return &dto.SystemRoleResponseDTO{ID: uuid.New()}, s.err
}

func (s qa7SysRoleSvc) UpdateSystemRole(context.Context, uuid.UUID, dto.UpdateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	return &dto.SystemRoleResponseDTO{ID: uuid.New()}, s.err
}

func TestQA7_C14_SystemRole_TrungTen_409(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
		want string
	}{
		{"trùng tên", service.ErrSystemRoleNameExists, 409, "ERR_ROLE_NAME_EXISTS"},
		{"bọc lỗi trùng tên", errors.Join(errors.New("ctx"), service.ErrSystemRoleNameExists), 409, "ERR_ROLE_NAME_EXISTS"},
		{"lỗi DB", errors.New("duplicated key not allowed"), 500, "ERR_INTERNAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewSystemRoleHandler(qa7SysRoleSvc{err: tc.err})
			app := fiber.New()
			app.Post("/system-roles", h.CreateSystemRole)
			app.Put("/system-roles/:id", h.UpdateSystemRole)
			for _, r := range [][3]string{{"POST", "/system-roles", `{"name":"STUDENT"}`}, {"PUT", "/system-roles/" + uuid.NewString(), `{"name":"STUDENT"}`}} {
				code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
				if code != tc.code || out["code"] != tc.want || strings.Contains(raw, "duplicated key") {
					t.Fatalf("%s: status=%d body=%s, muốn %d %s và không lộ lỗi gốc", r[0], code, raw, tc.code, tc.want)
				}
			}
		})
	}
}

// ── C17: /parent/children/:id/* ───────────────────────────────────────────────────────

type qa7ParentSvc struct {
	service.ParentDashboardServiceInterface
	err error
}

func (s qa7ParentSvc) GetChildOverview(context.Context, uuid.UUID, uuid.UUID) (*dto.ChildOverviewDto, error) {
	return &dto.ChildOverviewDto{}, s.err
}

func (s qa7ParentSvc) GetChildGrades(context.Context, uuid.UUID, uuid.UUID) (*dto.ChildGradesResponseDto, error) {
	return &dto.ChildGradesResponseDto{}, s.err
}

func TestQA7_C17_ParentChildren_MaLoiTheoNguyenNhan(t *testing.T) {
	notLinked := &apperr.KnownError{Status: 403, Code: "ERR_CHILD_NOT_LINKED", Message: "Không có quyền xem thông tin học sinh này."}
	for _, tc := range []struct {
		name, id string
		err      error
		code     int
		want     string
	}{
		{"id sai", "abc", nil, 400, "ERR_INVALID_ID"},
		{"chưa liên kết", uuid.NewString(), notLinked, 403, "ERR_CHILD_NOT_LINKED"},
		{"DB lỗi", uuid.NewString(), errors.New("failed to get attendance: dial tcp 10.0.0.5:5432: connection refused"), 500, "ERR_INTERNAL"},
		{"bọc lỗi DB", uuid.NewString(), errors.New("failed to verify relation: pq: too many connections"), 500, "ERR_INTERNAL"},
		{"hợp lệ", uuid.NewString(), nil, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewParentDashboardHandler(qa7ParentSvc{err: tc.err})
			app := fiber.New()
			app.Use(qa7AsUser(uuid.New()))
			app.Get("/parent/children/:id/overview", h.GetChildOverview)
			app.Get("/parent/children/:id/grades", h.GetChildGrades)
			for _, tab := range []string{"overview", "grades"} {
				code, out, raw := qa7Do(t, app, "GET", "/parent/children/"+tc.id+"/"+tab, "")
				if code != tc.code || (tc.want != "" && out["code"] != tc.want) {
					t.Fatalf("%s: status=%d body=%s, muốn %d %s", tab, code, raw, tc.code, tc.want)
				}
				if code != 200 {
					if msg, _ := out["message"].(string); msg == "" || strings.Contains(raw, "dial tcp") || strings.Contains(raw, "pq:") || strings.Contains(raw, "failed to") {
						t.Fatalf("%s: body=%s, muốn message tiếng Việt, không lộ lỗi nội bộ", tab, raw)
					}
				}
			}
		})
	}
}

// ── A03 / A09: voucher ────────────────────────────────────────────────────────────────

type qa7VoucherSvc struct {
	service.VoucherServiceInterface
	err         error
	createdName string
	updatedName *string
	calls       int
}

func (s *qa7VoucherSvc) CreateVoucher(_ context.Context, req *dto.CreateVoucherRequest) (*model.Voucher, error) {
	s.calls++
	s.createdName = req.Name
	return &model.Voucher{ID: uuid.New()}, s.err
}

func (s *qa7VoucherSvc) UpdateVoucher(_ context.Context, _ uuid.UUID, req *dto.UpdateVoucherRequest) (*model.Voucher, error) {
	s.calls++
	s.updatedName = req.Name
	return &model.Voucher{ID: uuid.New()}, s.err
}

func (s *qa7VoucherSvc) GetVoucherByID(context.Context, uuid.UUID) (*model.Voucher, error) {
	return nil, s.err
}
func (s *qa7VoucherSvc) DeleteVoucher(context.Context, uuid.UUID) error   { return s.err }
func (s *qa7VoucherSvc) ActivateVoucher(context.Context, uuid.UUID) error { return s.err }

func qa7VoucherApp(svc *qa7VoucherSvc) *fiber.App {
	h := handler.NewVoucherHandler(svc)
	app := fiber.New()
	app.Post("/v", h.CreateVoucher)
	app.Put("/v/:id", h.UpdateVoucher)
	app.Get("/v/:id", h.GetVoucher)
	app.Delete("/v/:id", h.DeleteVoucher)
	app.Post("/v/:id/activate", h.ActivateVoucher)
	return app
}

const qa7VoucherBase = `"code":"QA7TEST","discount_unit":"MONEY","discount_method":"FIXED","discount_amount_money":1000`

func TestQA7_A03_TenVoucher_CatKhoangTrang(t *testing.T) {
	for _, name := range []string{"   ", "", "  ab  ", "\t\n"} {
		svc := &qa7VoucherSvc{}
		app := qa7VoucherApp(svc)
		nameJSON, _ := json.Marshal(name)
		code, out, raw := qa7Do(t, app, "POST", "/v", `{`+qa7VoucherBase+`,"name":`+string(nameJSON)+`}`)
		if code != 400 || out["code"] != "ERR_VALIDATION" || svc.calls != 0 {
			t.Fatalf("tạo với tên %q: status=%d body=%s calls=%d, muốn 400 ERR_VALIDATION và không gọi service", name, code, raw, svc.calls)
		}
		ucode, uout, uraw := qa7Do(t, app, "PUT", "/v/"+uuid.NewString(), `{"name":`+string(nameJSON)+`}`)
		if ucode != 400 || uout["code"] != "ERR_VALIDATION" || svc.calls != 0 {
			t.Fatalf("sửa với tên %q: status=%d body=%s calls=%d", name, ucode, uraw, svc.calls)
		}
	}

	svc := &qa7VoucherSvc{}
	app := qa7VoucherApp(svc)
	if code, _, raw := qa7Do(t, app, "POST", "/v", `{`+qa7VoucherBase+`,"name":"  Voucher hợp lệ  "}`); code != 201 || svc.createdName != "Voucher hợp lệ" {
		t.Fatalf("tạo: status=%d body=%s name=%q, muốn 201 và tên đã cắt", code, raw, svc.createdName)
	}
	if code, _, raw := qa7Do(t, app, "PUT", "/v/"+uuid.NewString(), `{"name":"  Tên mới  "}`); code != 200 || svc.updatedName == nil || *svc.updatedName != "Tên mới" {
		t.Fatalf("sửa: status=%d body=%s, muốn 200 và tên đã cắt", code, raw)
	}
}

func TestQA7_A09_Voucher_ThongBaoTiengViet_KhongLoLoiNoiBo(t *testing.T) {
	english := []string{"Invalid", "not found", "successfully", "Failed", "required"}
	check := func(label string, code int, wantCode string, wantStatus int, out map[string]any, raw string) {
		t.Helper()
		msg, _ := out["message"].(string)
		if code != wantStatus || out["code"] != wantCode || msg == "" {
			t.Fatalf("%s: status=%d body=%s, muốn %d %s", label, code, raw, wantStatus, wantCode)
		}
		for _, w := range english {
			if strings.Contains(msg, w) {
				t.Fatalf("%s: message %q còn tiếng Anh (%q)", label, msg, w)
			}
		}
		if strings.Contains(raw, "pq:") || strings.Contains(raw, "connection refused") {
			t.Fatalf("%s: body lộ lỗi nội bộ: %s", label, raw)
		}
	}

	app := qa7VoucherApp(&qa7VoucherSvc{})
	code, out, raw := qa7Do(t, app, "GET", "/v/not-a-uuid", "")
	check("get id sai", code, "ERR_INVALID_ID", 400, out, raw)
	code, out, raw = qa7Do(t, app, "DELETE", "/v/not-a-uuid", "")
	check("delete id sai", code, "ERR_INVALID_ID", 400, out, raw)

	notFound := qa7VoucherApp(&qa7VoucherSvc{err: repository.ErrVoucherNotFound})
	code, out, raw = qa7Do(t, notFound, "GET", "/v/"+uuid.NewString(), "")
	check("get không có", code, "ERR_NOT_FOUND", 404, out, raw)
	code, out, raw = qa7Do(t, notFound, "DELETE", "/v/"+uuid.NewString(), "")
	check("delete không có", code, "ERR_NOT_FOUND", 404, out, raw)
	code, out, raw = qa7Do(t, notFound, "POST", "/v/"+uuid.NewString()+"/activate", "")
	check("activate không có", code, "ERR_NOT_FOUND", 404, out, raw)

	infra := qa7VoucherApp(&qa7VoucherSvc{err: errors.New("pq: connection refused")})
	code, out, raw = qa7Do(t, infra, "DELETE", "/v/"+uuid.NewString(), "")
	check("delete lỗi DB", code, "ERR_INTERNAL", 500, out, raw)
	code, out, raw = qa7Do(t, infra, "POST", "/v", `{`+qa7VoucherBase+`,"name":"Voucher QA7"}`)
	check("create lỗi DB", code, "ERR_INTERNAL", 500, out, raw)

	// Lỗi nghiệp vụ đã biết của service (câu tiếng Việt) vẫn hiện nguyên văn.
	known := qa7VoucherApp(&qa7VoucherSvc{err: apperr.BadRequest("Ngày bắt đầu phải trước ngày kết thúc.")})
	code, out, raw = qa7Do(t, known, "PUT", "/v/"+uuid.NewString(), `{"name":"Voucher QA7"}`)
	check("update lỗi nghiệp vụ", code, "ERR_UPDATE_VOUCHER", 400, out, raw)
	if out["message"] != "Ngày bắt đầu phải trước ngày kết thúc." {
		t.Fatalf("lỗi nghiệp vụ bị che: %s", raw)
	}
}
