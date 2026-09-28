package router

// Phase 4 rút tiền giảng viên — test route SỐNG: Fiber route THẬT (SetupWithdrawalRoutes) →
// AuthMiddleware THẬT (miniredis) → PermissionChecker THẬT → WithdrawalHandler THẬT. Chỉ service
// (biên nghiệp vụ, đã có test Postgres riêng) và repo quyền được fake.

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
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type wdFakeSystemRoleRepo struct {
	repository.SystemRoleRepositoryInterface
	perms map[uuid.UUID][]string
}

func (f *wdFakeSystemRoleRepo) GetPermissionsBySystemRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	var out []model.Permission
	for _, n := range f.perms[roleID] {
		out = append(out, model.Permission{Name: n})
	}
	return out, nil
}

type wdFakeUserSystemRoleRepo struct {
	repository.UserSystemRoleRepositoryInterface
	byUser map[uuid.UUID][]model.UserSystemRole
}

func (f *wdFakeUserSystemRoleRepo) FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return f.byUser[userID], nil
}

// wdFakeService ghi lại tham số để khẳng định handler lấy teacherID từ TOKEN.
type wdFakeService struct {
	service.WithdrawalServiceInterface
	listMineFor uuid.UUID
	createdFor  uuid.UUID
	approved    uuid.UUID
	payoutErr   error // nếu khác nil: Approve/MarkCompleted trả lỗi này
	cancelFor   uuid.UUID
	cancelID    uuid.UUID
	cancelErr   error
}

func (f *wdFakeService) Cancel(ctx context.Context, teacherID, id uuid.UUID) (*dto.WithdrawalStatusResponse, error) {
	f.cancelFor, f.cancelID = teacherID, id
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	return &dto.WithdrawalStatusResponse{ID: id, Status: "cancelled"}, nil
}

func (f *wdFakeService) Create(ctx context.Context, teacherID uuid.UUID, amount decimal.Decimal) (*dto.WithdrawalItem, error) {
	f.createdFor = teacherID
	return &dto.WithdrawalItem{ID: uuid.New(), Amount: amount, Status: "pending"}, nil
}

func (f *wdFakeService) ListMine(ctx context.Context, teacherID uuid.UUID, status string, page, limit int) (*dto.WithdrawalListResponse, error) {
	f.listMineFor = teacherID
	return &dto.WithdrawalListResponse{Items: []dto.WithdrawalItem{}, Page: page, Limit: limit}, nil
}

func (f *wdFakeService) AdminList(ctx context.Context, teacherID *uuid.UUID, status string, page, limit int) (*dto.AdminWithdrawalListResponse, error) {
	return &dto.AdminWithdrawalListResponse{Items: []dto.AdminWithdrawalItem{}, Page: page, Limit: limit}, nil
}

func (f *wdFakeService) NegativeBalances(ctx context.Context) (*dto.NegativeBalanceListResponse, error) {
	return &dto.NegativeBalanceListResponse{Items: []dto.NegativeBalanceTeacher{}}, nil
}

func (f *wdFakeService) Approve(ctx context.Context, actorID, id uuid.UUID) (*dto.WithdrawalStatusResponse, error) {
	f.approved = id
	if f.payoutErr != nil {
		return nil, f.payoutErr
	}
	return &dto.WithdrawalStatusResponse{ID: id, Status: "approved"}, nil
}

func (f *wdFakeService) Reject(ctx context.Context, actorID, id uuid.UUID, reason string) (*dto.WithdrawalStatusResponse, error) {
	return &dto.WithdrawalStatusResponse{ID: id, Status: "rejected"}, nil
}

func (f *wdFakeService) MarkCompleted(ctx context.Context, actorID, id uuid.UUID, txID string) (*dto.WithdrawalStatusResponse, error) {
	if f.payoutErr != nil {
		return nil, f.payoutErr
	}
	return &dto.WithdrawalStatusResponse{ID: id, Status: "completed"}, nil
}

type wdEnv struct {
	app                  *fiber.App
	svc                  *wdFakeService
	teacherID            uuid.UUID
	adminTok, teacherTok string
}

func newWithdrawalRouteEnv(t *testing.T) *wdEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "withdrawal-route-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	adminID, teacherID := uuid.New(), uuid.New()
	adminRole, teacherRole := uuid.New(), uuid.New()
	sysRoles := &wdFakeSystemRoleRepo{perms: map[uuid.UUID][]string{
		adminRole:   {"WALLET_WITHDRAWALS_MANAGE"},
		teacherRole: {"COURSES_CREATE", "COURSES_UPDATE_OWN", "LESSONS_MANAGE"}, // quyền thật của TEACHER
	}}
	userRoles := &wdFakeUserSystemRoleRepo{byUser: map[uuid.UUID][]model.UserSystemRole{
		adminID:   {{UserID: adminID, SystemRoleID: adminRole, Status: model.UserSystemRoleStatusActive}},
		teacherID: {{UserID: teacherID, SystemRoleID: teacherRole, Status: model.UserSystemRoleStatusActive}},
	}}
	pc := middleware.NewPermissionChecker(userRoles, sysRoles, nil, nil)

	deviceID := uuid.New()
	tok := func(id uuid.UUID, role string) string {
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), int64(1), 0).Err(); err != nil {
			t.Fatalf("seed user_version: %v", err)
		}
		s, _, err := utils.GenerateTokens(cfg, id, deviceID, role, nil, 1)
		if err != nil {
			t.Fatalf("GenerateTokens: %v", err)
		}
		return s
	}

	svc := &wdFakeService{}
	app := fiber.New()
	SetupWithdrawalRoutes(app.Group("/api"), cfg, handler.NewWithdrawalHandler(svc), rdb, pc)
	return &wdEnv{app: app, svc: svc, teacherID: teacherID, adminTok: tok(adminID, "SYSTEM_ADMIN"), teacherTok: tok(teacherID, "TEACHER")}
}

func (e *wdEnv) do(t *testing.T, method, path, token, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// Giáo viên (không có WALLET_WITHDRAWALS_MANAGE) bị 403 ở MỌI route admin; admin có quyền thì qua.
func TestWithdrawalAdminRoutes_RequirePermission(t *testing.T) {
	e := newWithdrawalRouteEnv(t)
	id := uuid.NewString()
	routes := []struct{ method, path, body string }{
		{"GET", "/api/admin/withdrawals", ""},
		{"GET", "/api/admin/withdrawals/negative-balances", ""},
		{"POST", "/api/admin/withdrawals/" + id + "/approve", ""},
		{"POST", "/api/admin/withdrawals/" + id + "/reject", `{"reason":"QA"}`},
		{"POST", "/api/admin/withdrawals/" + id + "/mark-completed", `{"transaction_id":"QA-FT"}`},
	}
	for _, r := range routes {
		if code, body := e.do(t, r.method, r.path, e.teacherTok, r.body); code != fiber.StatusForbidden {
			t.Fatalf("giáo viên %s %s = %d %v, muốn 403", r.method, r.path, code, body)
		}
		if code, body := e.do(t, r.method, r.path, e.adminTok, r.body); code != fiber.StatusOK {
			t.Fatalf("admin %s %s = %d %v, muốn 200", r.method, r.path, code, body)
		}
	}
	if code, _ := e.do(t, "GET", "/api/admin/withdrawals", "", ""); code != fiber.StatusUnauthorized {
		t.Fatalf("không token = %d, muốn 401", code)
	}
	if e.svc.approved.String() != id {
		t.Fatalf("approve nhận id %s, muốn %s", e.svc.approved, id)
	}
}

// Review Phase 4, B-1: số dư GV âm lúc duyệt/đánh dấu đã chuyển -> 409 negative_balance, kèm số dư
// và số tiền để trang admin giải thích được.
func TestWithdrawalAdminPayout_NegativeBalanceIs409(t *testing.T) {
	e := newWithdrawalRouteEnv(t)
	e.svc.payoutErr = &service.WithdrawalRuleError{
		Kind: service.ErrWithdrawalPayoutNegativeBalance,
		Data: map[string]interface{}{"available_balance": decimal.NewFromInt(-200000), "amount": decimal.NewFromInt(400000)},
	}
	id := uuid.NewString()
	for _, r := range []struct{ path, body string }{
		{"/api/admin/withdrawals/" + id + "/approve", ""},
		{"/api/admin/withdrawals/" + id + "/mark-completed", `{"transaction_id":"QA-FT"}`},
	} {
		code, body := e.do(t, "POST", r.path, e.adminTok, r.body)
		if code != fiber.StatusConflict || body["error"] != "negative_balance" {
			t.Fatalf("%s = %d %v, muốn 409 negative_balance", r.path, code, body)
		}
		data, _ := body["data"].(map[string]interface{})
		if data["available_balance"] == nil || data["amount"] == nil {
			t.Fatalf("%s thiếu data.available_balance/amount: %v", r.path, body)
		}
	}
}

// Route giáo viên luôn dùng user_id trong TOKEN — query ?teacher_id= của người khác bị bỏ qua.
func TestWithdrawalTeacherRoutes_UseTokenIdentity(t *testing.T) {
	e := newWithdrawalRouteEnv(t)
	other := uuid.NewString()
	if code, _ := e.do(t, "GET", "/api/wallet/teacher/withdrawals?teacher_id="+other, e.teacherTok, ""); code != fiber.StatusOK {
		t.Fatalf("list = %d", code)
	}
	if e.svc.listMineFor != e.teacherID {
		t.Fatalf("ListMine gọi cho %s, muốn chính giáo viên %s", e.svc.listMineFor, e.teacherID)
	}
	code, body := e.do(t, "POST", "/api/wallet/teacher/withdrawals", e.teacherTok, `{"amount":"150000","teacher_id":"`+other+`"}`)
	if code != fiber.StatusCreated || e.svc.createdFor != e.teacherID {
		t.Fatalf("create = %d %v, createdFor = %s", code, body, e.svc.createdFor)
	}
	if code, _ := e.do(t, "GET", "/api/wallet/teacher/withdrawals?status=processing", e.teacherTok, ""); code != fiber.StatusBadRequest {
		t.Fatalf("status ngoài enum = %d, muốn 400", code)
	}
}

// Q2 (QA vòng 2): POST /wallet/teacher/withdrawals/:id/cancel — giảng viên lấy từ TOKEN, map lỗi
// đúng contract: không phải của mình/không tồn tại -> 404, không còn pending -> 409, id sai -> 400.
func TestWithdrawalTeacherCancel_Route(t *testing.T) {
	e := newWithdrawalRouteEnv(t)
	id := uuid.NewString()
	code, body := e.do(t, "POST", "/api/wallet/teacher/withdrawals/"+id+"/cancel", e.teacherTok, `{"teacher_id":"`+uuid.NewString()+`"}`)
	if code != fiber.StatusOK || e.svc.cancelFor != e.teacherID || e.svc.cancelID.String() != id {
		t.Fatalf("cancel = %d %v, cancelFor=%s cancelID=%s", code, body, e.svc.cancelFor, e.svc.cancelID)
	}
	if data, _ := body["data"].(map[string]interface{}); data["status"] != "cancelled" {
		t.Fatalf("data.status = %v", body)
	}
	if code, _ := e.do(t, "POST", "/api/wallet/teacher/withdrawals/khong-phai-uuid/cancel", e.teacherTok, ""); code != fiber.StatusBadRequest {
		t.Fatalf("id sai = %d, muốn 400", code)
	}
	if code, _ := e.do(t, "POST", "/api/wallet/teacher/withdrawals/"+id+"/cancel", "", ""); code != fiber.StatusUnauthorized {
		t.Fatalf("không token = %d, muốn 401", code)
	}

	e.svc.cancelErr = service.ErrWithdrawalNotFound
	if code, body := e.do(t, "POST", "/api/wallet/teacher/withdrawals/"+id+"/cancel", e.teacherTok, ""); code != fiber.StatusNotFound || body["error"] != "withdrawal_not_found" {
		t.Fatalf("không phải của mình = %d %v, muốn 404 withdrawal_not_found", code, body)
	}
	e.svc.cancelErr = &service.WithdrawalRuleError{Kind: service.ErrWithdrawalInvalidTransition, Data: map[string]interface{}{"current_status": "approved"}}
	code, body = e.do(t, "POST", "/api/wallet/teacher/withdrawals/"+id+"/cancel", e.teacherTok, "")
	if code != fiber.StatusConflict || body["error"] != "invalid_status_transition" {
		t.Fatalf("đã duyệt = %d %v, muốn 409 invalid_status_transition", code, body)
	}
	if data, _ := body["data"].(map[string]interface{}); data["current_status"] != "approved" {
		t.Fatalf("thiếu data.current_status: %v", body)
	}
	if code, _ := e.do(t, "GET", "/api/wallet/teacher/withdrawals?status=cancelled", e.teacherTok, ""); code != fiber.StatusOK {
		t.Fatalf("lọc status=cancelled = %d, muốn 200", code)
	}
}