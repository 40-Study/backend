package handler_test

// QA8 (C18-C24, org name) ở tầng HTTP: handler THẬT, service thay bằng stub.
// Mỗi ca đỏ khi bỏ phần sửa tương ứng: bỏ bankInfoErrors/normalizeBankInfo (C20), bỏ respondIfKnownError ở handler auth/org/role (C19,
// C22, C23, org), bỏ sessionListOptions (C21), đổi withContestID về INVALID_ID (C24).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

// ── C20: tài khoản nhận tiền ─────────────────────────────────────────────────────────────

type qa8WalletSvc struct {
	service.WalletServiceInterface
	saved []dto.UpdateBankInfoRequest
	err   error
}

func (s *qa8WalletSvc) UpdateBankInfo(_ context.Context, _ uuid.UUID, req dto.UpdateBankInfoRequest) error {
	s.saved = append(s.saved, req)
	return s.err
}

func qa8WalletApp(svc *qa8WalletSvc) *fiber.App {
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Put("/bank", handler.NewWalletHandler(svc).UpdateBankInfo)
	return app
}

func TestQA8_C20_BankInfo_Validation(t *testing.T) {
	long := strings.Repeat("a", 300)
	for _, tc := range []struct {
		name, body, field string
	}{
		{"số tài khoản có ký tự lạ", `{"bank_name":"VCB","bank_account_number":"abc!!","bank_account_name":"NGUYEN VAN A"}`, "bank_account_number"},
		{"số tài khoản quá ngắn", `{"bank_name":"VCB","bank_account_number":"12345","bank_account_name":"NGUYEN VAN A"}`, "bank_account_number"},
		{"số tài khoản quá dài", `{"bank_name":"VCB","bank_account_number":"` + strings.Repeat("1", 21) + `","bank_account_name":"NGUYEN VAN A"}`, "bank_account_number"},
		{"số tài khoản 300 ký tự", `{"bank_name":"VCB","bank_account_number":"` + long + `","bank_account_name":"NGUYEN VAN A"}`, "bank_account_number"},
		{"tên chủ chỉ có khoảng trắng", `{"bank_name":"VCB","bank_account_number":"0123456789","bank_account_name":"   "}`, "bank_account_name"},
		{"tên chủ quá dài", `{"bank_name":"VCB","bank_account_number":"0123456789","bank_account_name":"` + strings.Repeat("ê", 256) + `"}`, "bank_account_name"},
		{"thiếu tên ngân hàng", `{"bank_name":" ","bank_account_number":"0123456789","bank_account_name":"NGUYEN VAN A"}`, "bank_name"},
		{"tên ngân hàng quá dài", `{"bank_name":"` + strings.Repeat("b", 101) + `","bank_account_number":"0123456789","bank_account_name":"NGUYEN VAN A"}`, "bank_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &qa8WalletSvc{}
			code, out, raw := qa7Do(t, qa8WalletApp(svc), "PUT", "/bank", tc.body)
			errs, _ := out["errors"].([]any)
			if code != 400 || out["code"] != "ERR_VALIDATION" || len(errs) == 0 || errs[0].(map[string]any)["field"] != tc.field {
				t.Fatalf("status=%d body=%s, muốn 400 ERR_VALIDATION nêu field %s", code, raw, tc.field)
			}
			if msg, _ := out["message"].(string); strings.Contains(msg, "_") || !strings.HasSuffix(msg, ".") {
				t.Fatalf("message phải là tiếng Việt, không lộ tên cột: %q", msg)
			}
			if len(svc.saved) != 0 {
				t.Fatal("service bị gọi dù dữ liệu sai")
			}
		})
	}
}

func TestQA8_C20_BankInfo_TrimsAndSaves_AndDoesNotLeakDbError(t *testing.T) {
	svc := &qa8WalletSvc{}
	code, _, raw := qa7Do(t, qa8WalletApp(svc), "PUT", "/bank", `{"bank_name":"  Vietcombank ","bank_account_number":" 0123456789 ","bank_account_name":" NGUYEN VAN A  "}`)
	if code != 200 || len(svc.saved) != 1 {
		t.Fatalf("hợp lệ: status=%d body=%s saved=%d", code, raw, len(svc.saved))
	}
	if got := svc.saved[0]; got.BankName != "Vietcombank" || got.BankAccountNumber != "0123456789" || got.BankAccountName != "NGUYEN VAN A" {
		t.Fatalf("giá trị lưu phải đã cắt khoảng trắng: %+v", got)
	}

	svc = &qa8WalletSvc{err: errors.New("value too long for type character varying(50) (SQLSTATE 22001)")}
	code, out, raw := qa7Do(t, qa8WalletApp(svc), "PUT", "/bank", `{"bank_name":"VCB","bank_account_number":"0123456789","bank_account_name":"A"}`)
	if code != 500 || out["code"] != "ERR_INTERNAL" || strings.Contains(raw, "SQLSTATE") {
		t.Fatalf("lỗi DB: status=%d body=%s, muốn 500 ERR_INTERNAL không lộ lỗi SQL", code, raw)
	}
}

// ── C19, C22: lỗi nghiệp vụ của auth có code ổn định ─────────────────────────────────────

type qa8AuthSvc struct {
	service.AuthServiceInterface
	err error
}

func (s *qa8AuthSvc) RequestRegister(context.Context, dto.RegisterRequestDto) error { return s.err }
func (s *qa8AuthSvc) Register(context.Context, dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error) {
	return nil, s.err
}
func (s *qa8AuthSvc) UpdateMe(context.Context, uuid.UUID, dto.UpdateMeRequestDto) (*dto.UserResponseDto, error) {
	return nil, s.err
}
func (s *qa8AuthSvc) DeleteAccount(context.Context, uuid.UUID, dto.DeleteAccountDto) error {
	return s.err
}

func TestQA8_C19_C22_AuthHandlers_KnownErrorsKeepStatusAndCode(t *testing.T) {
	const reg = `{"email":"a@b.com","password":"Demo@12345","confirm_password":"Demo@12345","user_name":"someone"}`
	for _, tc := range []struct {
		name, method, path, body string
		err                      error
		status                   int
		code                     string
	}{
		{"đăng ký tên đã dùng", "POST", "/register/request", reg, service.ErrUserNameTaken, 409, "ERR_USERNAME_EXISTS"},
		{"đăng ký tên dành riêng", "POST", "/register/request", reg, service.ErrUserNameReserved, 409, "ERR_USERNAME_EXISTS"},
		{"xác thực OTP, tên bị lấy", "POST", "/register", `{"email":"a@b.com","otp":"123456"}`, service.ErrUserNameTaken, 409, "ERR_USERNAME_EXISTS"},
		{"PUT /auth/me tên đã dùng", "PUT", "/me", `{"username":"admin"}`, service.ErrUserNameReserved, 409, "ERR_USERNAME_EXISTS"},
		{"xoá tài khoản còn tiền", "DELETE", "/me", `{"password":"Demo@12345"}`, service.ErrAccountHasBalance, 409, "ERR_ACCOUNT_HAS_BALANCE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewAuthHandler(&qa8AuthSvc{err: tc.err})
			app := fiber.New()
			app.Use(qa7AsUser(uuid.New()))
			app.Post("/register/request", h.RequestRegister)
			app.Post("/register", h.Register)
			app.Put("/me", h.UpdateMe)
			app.Delete("/me", h.DeleteAccount)
			code, out, raw := qa7Do(t, app, tc.method, tc.path, tc.body)
			msg, _ := out["message"].(string)
			if code != tc.status || out["code"] != tc.code || msg == "" || msg == "Failed" {
				t.Fatalf("status=%d body=%s, muốn %d %s kèm message tiếng Việt", code, raw, tc.status, tc.code)
			}
		})
	}
	// Lỗi chưa biết vẫn theo đường cũ (400 kèm message chung), không bị nuốt thành 409.
	h := handler.NewAuthHandler(&qa8AuthSvc{err: errors.New("email already registered")})
	app := fiber.New()
	app.Post("/register/request", h.RequestRegister)
	if code, out, raw := qa7Do(t, app, "POST", "/register/request", reg); code != 400 || out["message"] != "Request register failed" {
		t.Fatalf("lỗi thường: status=%d body=%s", code, raw)
	}
}

// ── C21: tham số danh sách buổi học ──────────────────────────────────────────────────────

type qa8ScheduleSvc struct {
	service.ScheduleServiceInterface
	opts  []repository.SessionListOptions
	pages []int
}

func (s *qa8ScheduleSvc) GetSessionsByClass(_ context.Context, _, _ uuid.UUID, _ bool, page, _ int, opts repository.SessionListOptions) (*dto.ClassSessionListDTO, error) {
	s.opts, s.pages = append(s.opts, opts), append(s.pages, page)
	return &dto.ClassSessionListDTO{Data: []dto.ClassSessionResponseDTO{}}, nil
}

func TestQA8_C21_SessionsList_OrderAndAroundQuery(t *testing.T) {
	svc := &qa8ScheduleSvc{}
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Get("/classes/:classId/sessions", handler.NewScheduleHandler(svc, nil).GetSessionsByClass)
	base := "/classes/" + uuid.NewString() + "/sessions"

	for q, want := range map[string]repository.SessionListOptions{
		"":                         {},
		"?order=asc":               {},
		"?order=desc":              {Desc: true},
		"?around=today":            {AroundToday: true},
		"?order=desc&around=today": {Desc: true, AroundToday: true},
	} {
		if code, _, raw := qa7Do(t, app, "GET", base+q, ""); code != 200 {
			t.Fatalf("%q: status=%d %s", q, code, raw)
		}
		if got := svc.opts[len(svc.opts)-1]; got != want {
			t.Errorf("%q: opts=%+v, muốn %+v", q, got, want)
		}
	}
	for _, q := range []string{"?order=sideways", "?around=tomorrow", "?order=DESC"} {
		n := len(svc.opts)
		code, out, raw := qa7Do(t, app, "GET", base+q, "")
		if code != 400 || out["code"] != "ERR_VALIDATION" || len(svc.opts) != n {
			t.Errorf("%q: status=%d body=%s, muốn 400 ERR_VALIDATION và không gọi service", q, code, raw)
		}
	}
}

// ── C23: vai theo tổ chức ────────────────────────────────────────────────────────────────

type qa8UserRoleSvc struct {
	service.UserSystemRoleServiceInterface
	err error
}

func (s *qa8UserRoleSvc) AssignSystemRolesToUser(context.Context, uuid.UUID, dto.AssignSystemRolesToUserDTO, uuid.UUID) ([]dto.UserSystemRoleResponseDTO, error) {
	return nil, s.err
}

func TestQA8_C23_AssignSystemRoles_OrgScopedRole_400WithCode(t *testing.T) {
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Post("/users/:user_id/system-roles", handler.NewUserSystemRoleHandler(&qa8UserRoleSvc{err: service.ErrSystemRoleNotAssignable}).AssignSystemRolesToUser)
	body := `{"system_role_ids":["` + uuid.NewString() + `"]}`
	code, out, raw := qa7Do(t, app, "POST", "/users/"+uuid.NewString()+"/system-roles", body)
	if code != 400 || out["code"] != "ERR_ROLE_NOT_ASSIGNABLE" || out["message"] == "" {
		t.Fatalf("status=%d body=%s, muốn 400 ERR_ROLE_NOT_ASSIGNABLE", code, raw)
	}
}

// ── Tên tổ chức ──────────────────────────────────────────────────────────────────────────

type qa8OrgSvc struct {
	service.OrganizationServiceInterface
}

func (qa8OrgSvc) CreateOrganization(context.Context, uuid.UUID, dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	return nil, service.ErrOrgNameExists
}
func (qa8OrgSvc) UpdateOrganization(context.Context, uuid.UUID, dto.UpdateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	return nil, service.ErrOrgNameExists
}

func TestQA8_OrgName_Duplicate_409WithCode(t *testing.T) {
	h := handler.NewOrganizationHandler(qa8OrgSvc{})
	app := fiber.New()
	app.Use(qa7AsUser(uuid.New()))
	app.Post("/organizations", h.CreateOrganization)
	app.Put("/organizations/:id", h.UpdateOrganization)
	for _, r := range [][3]string{
		{"POST", "/organizations", `{"name":"Trung tâm A"}`},
		{"PUT", "/organizations/" + uuid.NewString(), `{"name":"trung tâm a"}`},
	} {
		code, out, raw := qa7Do(t, app, r[0], r[1], r[2])
		if code != 409 || out["code"] != "ERR_ORG_NAME_EXISTS" || out["message"] == "" {
			t.Errorf("%s: status=%d body=%s, muốn 409 ERR_ORG_NAME_EXISTS", r[0], code, raw)
		}
	}
}

// ── C24: id cuộc thi sai định dạng ───────────────────────────────────────────────────────

func TestQA8_C24_ContestMalformedId_400ErrInvalidId(t *testing.T) {
	h := handler.NewContestHandler(nil, nil)
	app := fiber.New()
	app.Get("/contests/manage/:id", h.GetManage)
	app.Get("/contests/manage/:id/participants", h.ListParticipants)
	for _, p := range []string{"/contests/manage/not-a-uuid", "/contests/manage/not-a-uuid/participants"} {
		code, out, raw := qa7Do(t, app, "GET", p, "")
		msg, _ := out["message"].(string)
		if code != 400 || out["code"] != "ERR_INVALID_ID" || !strings.Contains(msg, "cuộc thi") {
			t.Errorf("%s: status=%d body=%s, muốn 400 ERR_INVALID_ID tiếng Việt", p, code, raw)
		}
	}
}
