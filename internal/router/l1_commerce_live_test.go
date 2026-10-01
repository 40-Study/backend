package router

// L1 commerce — test route SỐNG trên Postgres THẬT (pgtest.IsolatedSchema + database.Migrate):
// Fiber route thật → AuthMiddleware/OptionalAuth/PermissionChecker thật → handler/service/repository
// thật. Chỉ fake kho quyền (dùng lại apvSystemRoleRepo/apvUserSystemRoleRepo của approval_live_env_test).
//
//   - Voucher holders_only: người ngoài (khách, người đăng nhập chưa được cấp) nhận Y HỆT 404 của mã
//     không tồn tại ở tra mã công khai và ở tự lưu; người giữ thấy được; danh sách công khai không lộ.
//   - discount_price: 400 DISCOUNT_PRICE_INVALID khi <= 0 hoặc >= price; null vẫn xoá được.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/database"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type l1Env struct {
	t    *testing.T
	app  *fiber.App
	db   *gorm.DB
	ids  map[string]uuid.UUID
	toks map[string]string
}

var l1Roles = map[string]string{"admin": "SYSTEM_ADMIN", "teacher": "TEACHER", "holder": "STUDENT", "stranger": "STUDENT"}

func newL1Env(t *testing.T) *l1Env {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "l1-live-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	roles := map[string]*model.SystemRole{}
	for _, name := range []string{"SYSTEM_ADMIN", "TEACHER", "STUDENT"} {
		r := &model.SystemRole{Name: name}
		r.ID = uuid.New()
		roles[name] = r
	}
	perms := map[uuid.UUID][]string{
		roles["SYSTEM_ADMIN"].ID: {"*"},
		roles["TEACHER"].ID:      {"COURSES_CREATE", "COURSES_UPDATE_OWN"},
	}

	e := &l1Env{t: t, db: db, ids: map[string]uuid.UUID{}, toks: map[string]string{}}
	usr := &apvUserSystemRoleRepo{roles: map[uuid.UUID][]*model.SystemRole{}}
	device := uuid.New()
	for key, role := range l1Roles {
		u := model.User{Email: key + "@l1.test", UserName: key, PasswordHash: "x", FullName: strPtr("QA-L1 " + key)}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", key, err)
		}
		e.ids[key] = u.ID
		usr.roles[u.ID] = []*model.SystemRole{roles[role]}
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		access, _, err := utils.GenerateTokens(cfg, u.ID, device, role, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		e.toks[key] = access
	}

	pc := middleware.NewPermissionChecker(usr, &apvSystemRoleRepo{perms: perms}, nil, nil)
	// Production (cmd/api) serialize decimal thành số JSON; test phải thấy đúng shape đó.
	prevQuotes := decimal.MarshalJSONWithoutQuotes
	decimal.MarshalJSONWithoutQuotes = true
	t.Cleanup(func() { decimal.MarshalJSONWithoutQuotes = prevQuotes })

	app := fiber.New()
	api := app.Group("/api")
	voucherSvc := service.NewVoucherService(repository.NewVoucherRepository(db), repository.NewUserRepository(db))
	SetupVoucherRoutes(api, cfg, handler.NewVoucherHandler(voucherSvc), rdb, pc)
	courseSvc := service.NewCourseService(repository.NewCourseRepository(db), nil, nil, nil)
	SetupCourseRoutes(api, cfg, handler.NewCourseHandler(courseSvc, pc), nil, nil, nil, nil, nil, nil, nil, rdb)
	e.app = app
	return e
}

type l1Resp struct {
	status int
	raw    string
	body   map[string]interface{}
}

func (e *l1Env) do(method, path, who, body string) l1Resp {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if who != "" {
		req.Header.Set("Authorization", "Bearer "+e.toks[who])
	}
	res, err := e.app.Test(req, -1)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(res.Body)
	out := l1Resp{status: res.StatusCode, raw: string(raw), body: map[string]interface{}{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (e *l1Env) createVoucher(code string, holdersOnly bool) string {
	e.t.Helper()
	body := `{"code":"` + code + `","name":"Voucher ` + code + `","discount_unit":"MONEY","discount_method":"FIXED",` +
		`"discount_amount_money":50000,"accept_all_payment_methods":true,"holders_only":` + map[bool]string{true: "true", false: "false"}[holdersOnly] + `}`
	r := e.do("POST", "/api/vouchers", "admin", body)
	if r.status != fiber.StatusCreated {
		e.t.Fatalf("tạo voucher %s: %d %s", code, r.status, r.raw)
	}
	if got, _ := r.body["holders_only"].(bool); got != holdersOnly {
		e.t.Fatalf("tạo voucher %s: holders_only=%v trong response, muốn %v (%s)", code, got, holdersOnly, r.raw)
	}
	id, _ := r.body["id"].(string)
	return id
}

func (e *l1Env) grant(user string, voucherID string) {
	e.t.Helper()
	uv := model.UserVoucher{UserID: e.ids[user], VoucherID: uuid.MustParse(voucherID), Source: "admin_grant", SavedAt: time.Now()}
	if err := e.db.Create(&uv).Error; err != nil {
		e.t.Fatalf("cấp voucher: %v", err)
	}
}

func TestL1Live_HoldersOnlyVoucher(t *testing.T) {
	e := newL1Env(t)
	hiddenID := e.createVoucher("L1VIP", true)
	e.createVoucher("L1OPEN", false)
	e.grant("holder", hiddenID)

	// Tra mã công khai: khách và người ngoài nhận Y HỆT 404 của mã không tồn tại; người giữ thấy được.
	missing := e.do("GET", "/api/vouchers/code/L1NOPE", "", "")
	if missing.status != fiber.StatusNotFound {
		t.Fatalf("mã không tồn tại: %d %s, muốn 404", missing.status, missing.raw)
	}
	for name, who := range map[string]string{"khách": "", "người ngoài": "stranger"} {
		r := e.do("GET", "/api/vouchers/code/L1VIP", who, "")
		if r.status != missing.status || r.raw != missing.raw {
			t.Errorf("%s tra voucher dành riêng: %d %s, muốn y hệt mã không tồn tại: %d %s", name, r.status, r.raw, missing.status, missing.raw)
		}
	}
	if r := e.do("GET", "/api/vouchers/code/L1VIP", "holder", ""); r.status != fiber.StatusOK || r.body["holders_only"] != true {
		t.Errorf("người giữ tra voucher dành riêng: %d %s, muốn 200 holders_only=true", r.status, r.raw)
	}
	if r := e.do("GET", "/api/vouchers/code/L1OPEN", "", ""); r.status != fiber.StatusOK {
		t.Errorf("khách tra voucher công khai: %d %s, muốn 200", r.status, r.raw)
	}

	// Danh sách công khai không lộ voucher dành riêng.
	pub := e.do("GET", "/api/vouchers/public", "", "")
	if pub.status != fiber.StatusOK || strings.Contains(pub.raw, "L1VIP") || !strings.Contains(pub.raw, "L1OPEN") {
		t.Errorf("/vouchers/public: %d %s, muốn có L1OPEN và KHÔNG có L1VIP", pub.status, pub.raw)
	}

	// Tự lưu: người ngoài nhận Y HỆT 404 của voucher không tồn tại và không có user_vouchers nào; người
	// giữ lưu lại được.
	absent := e.do("POST", "/api/vouchers/"+uuid.NewString()+"/save", "stranger", `{}`)
	hiddenSave := e.do("POST", "/api/vouchers/"+hiddenID+"/save", "stranger", `{}`)
	if absent.status != fiber.StatusNotFound || hiddenSave.status != absent.status || hiddenSave.raw != absent.raw {
		t.Errorf("người ngoài tự lưu: %d %s, muốn y hệt voucher không tồn tại: %d %s", hiddenSave.status, hiddenSave.raw, absent.status, absent.raw)
	}
	var n int64
	e.db.Model(&model.UserVoucher{}).Where("user_id = ?", e.ids["stranger"]).Count(&n)
	if n != 0 {
		t.Errorf("người ngoài tự lưu bị chặn mà vẫn có %d user_vouchers", n)
	}
	if r := e.do("POST", "/api/vouchers/"+hiddenID+"/save", "holder", `{}`); r.status != fiber.StatusCreated {
		t.Errorf("người giữ lưu lại: %d %s, muốn 201", r.status, r.raw)
	}

	// Ví của người giữ có voucher dành riêng; ví người ngoài rỗng.
	if r := e.do("GET", "/api/vouchers/me", "holder", ""); !strings.Contains(r.raw, "L1VIP") {
		t.Errorf("/vouchers/me của người giữ thiếu L1VIP: %s", r.raw)
	}
	if r := e.do("GET", "/api/vouchers/me", "stranger", ""); strings.Contains(r.raw, "L1VIP") {
		t.Errorf("/vouchers/me của người ngoài lộ L1VIP: %s", r.raw)
	}
}

// Admin đổi holders_only qua PUT; người không phải admin không tạo/sửa được voucher.
func TestL1Live_HoldersOnlyAdminUpdate(t *testing.T) {
	e := newL1Env(t)
	id := e.createVoucher("L1TOGGLE", false)

	if r := e.do("PUT", "/api/vouchers/"+id, "admin", `{"holders_only":true}`); r.status != fiber.StatusOK || r.body["holders_only"] != true {
		t.Fatalf("bật holders_only: %d %s", r.status, r.raw)
	}
	if r := e.do("GET", "/api/vouchers/code/L1TOGGLE", "stranger", ""); r.status != fiber.StatusNotFound {
		t.Fatalf("sau khi bật, người ngoài tra mã: %d, muốn 404", r.status)
	}
	if r := e.do("PUT", "/api/vouchers/"+id, "admin", `{"name":"Đổi tên khác"}`); r.body["holders_only"] != true {
		t.Fatalf("sửa không gửi holders_only phải giữ nguyên true: %s", r.raw)
	}
	if r := e.do("PUT", "/api/vouchers/"+id, "admin", `{"holders_only":false}`); r.body["holders_only"] != false {
		t.Fatalf("tắt holders_only: %s", r.raw)
	}
	if r := e.do("GET", "/api/vouchers/code/L1TOGGLE", "stranger", ""); r.status != fiber.StatusOK {
		t.Fatalf("sau khi tắt, người ngoài tra mã: %d, muốn 200", r.status)
	}
	if r := e.do("POST", "/api/vouchers", "stranger", `{"code":"L1HACK","name":"Hack voucher","discount_unit":"MONEY","discount_method":"FIXED","discount_amount_money":1,"holders_only":false}`); r.status != fiber.StatusForbidden {
		t.Fatalf("học viên tạo voucher: %d %s, muốn 403", r.status, r.raw)
	}
}

func TestL1Live_DiscountPriceValidation(t *testing.T) {
	e := newL1Env(t)
	create := func(discount string) l1Resp {
		return e.do("POST", "/api/courses", "teacher", `{"title":"Khoa hoc L1","price":500000,"discount_price":`+discount+`}`)
	}
	assertInvalid := func(what string, r l1Resp) {
		t.Helper()
		if r.status != fiber.StatusBadRequest || r.body["code"] != "DISCOUNT_PRICE_INVALID" {
			t.Errorf("%s: %d %s, muốn 400 DISCOUNT_PRICE_INVALID", what, r.status, r.raw)
		}
	}
	assertInvalid("tạo với khuyến mãi bằng giá", create("500000"))
	assertInvalid("tạo với khuyến mãi lớn hơn giá", create("600000"))
	assertInvalid("tạo với khuyến mãi 0", create("0"))
	var courses int64
	e.db.Model(&model.Course{}).Where("title = ?", "Khoa hoc L1").Count(&courses)
	if courses != 0 {
		t.Fatalf("tạo bị từ chối mà vẫn có %d khoá trong DB", courses)
	}

	ok := create("400000")
	if ok.status != fiber.StatusCreated {
		t.Fatalf("tạo với khuyến mãi hợp lệ: %d %s, muốn 201", ok.status, ok.raw)
	}
	data, _ := ok.body["data"].(map[string]interface{})
	id, _ := data["id"].(string)
	put := func(body string) l1Resp { return e.do("PUT", "/api/courses/"+id, "teacher", body) }

	assertInvalid("sửa khuyến mãi bằng giá", put(`{"discount_price":500000}`))
	assertInvalid("sửa khuyến mãi 0 (xoá phải gửi null)", put(`{"discount_price":0}`))
	assertInvalid("hạ giá xuống dưới khuyến mãi đang có", put(`{"price":300000}`))
	if r := put(`{"discount_price":450000}`); r.status != fiber.StatusOK {
		t.Fatalf("sửa khuyến mãi hợp lệ: %d %s", r.status, r.raw)
	}
	if r := put(`{"discount_price":null}`); r.status != fiber.StatusOK {
		t.Fatalf("xoá khuyến mãi bằng null: %d %s, muốn 200", r.status, r.raw)
	}
	var c model.Course
	e.db.First(&c, "id = ?", id)
	if c.DiscountPrice != nil {
		t.Fatalf("null phải xoá khuyến mãi, DB còn %v", c.DiscountPrice)
	}
}
