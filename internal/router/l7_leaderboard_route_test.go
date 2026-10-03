package router

// Lane L7 đợt 2 (MINOR 3 của review): route công khai GET /leaderboard dùng OptionalAuth thật + handler thật +
// PermissionChecker thật (kho quyền là fake), chỉ stub service để bắt viewer mà handler dựng ra.
//
//   - khách (không token)        → viewer = nil
//   - người đăng nhập thường     → viewer{UserID, IsAdmin=false}
//   - admin (SYSTEM_SETTINGS_MANAGE) → viewer{UserID, IsAdmin=true}
//   - token sai/hết hạn          → 401, service KHÔNG được gọi (web dựa vào 401 để refresh, không hạ xuống khách)
//   - PermissionChecker = nil    → fail-closed: IsAdmin=false kể cả khi là người đăng nhập
// Gỡ OptionalAuth khỏi route thì viewer luôn nil (ĐỎ ở ca người thường/admin); đổi OptionalAuth thành "bỏ qua
// token sai" thì ca 401 ĐỎ; bỏ kiểm quyền admin trong handler thì ca admin ĐỎ.

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type viewerCapturingLeaderboardSvc struct {
	calls  int
	viewer *service.LeaderboardViewer
}

func (s *viewerCapturingLeaderboardSvc) GetLeaderboard(_ context.Context, periodType string, _ int, v *service.LeaderboardViewer, _ *uuid.UUID) (*dto.LeaderboardResponse, error) {
	s.calls++
	s.viewer = v
	return &dto.LeaderboardResponse{PeriodType: periodType}, nil
}

func (s *viewerCapturingLeaderboardSvc) GetMyRank(context.Context, uuid.UUID, string) (*dto.MyRankResponse, error) {
	return &dto.MyRankResponse{}, nil
}

func TestLeaderboardRoute_OptionalAuthDungVoiMoiLoaiNguoiXem(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "leaderboard-route-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	newRole := func(name string) *model.SystemRole {
		r := &model.SystemRole{Name: name}
		r.ID = uuid.New()
		return r
	}
	student, admin := newRole("STUDENT"), newRole("SYSTEM_ADMIN")
	userRoles := &apvUserSystemRoleRepo{roles: map[uuid.UUID][]*model.SystemRole{}}
	roleRepo := &apvSystemRoleRepo{perms: map[uuid.UUID][]string{
		student.ID: {"COURSES_VIEW"},
		admin.ID:   {"SYSTEM_SETTINGS_MANAGE"},
	}}
	token := func(role *model.SystemRole) (uuid.UUID, string) {
		id := uuid.New()
		userRoles.roles[id] = []*model.SystemRole{role}
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		access, _, err := utils.GenerateTokens(cfg, id, uuid.New(), role.Name, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		return id, access
	}
	studentID, studentTok := token(student)
	adminID, adminTok := token(admin)

	call := func(t *testing.T, pc *middleware.PermissionChecker, bearer string) (int, *viewerCapturingLeaderboardSvc) {
		t.Helper()
		svc := &viewerCapturingLeaderboardSvc{}
		app := fiber.New()
		SetupLeaderboardRoutes(app.Group("/api"), cfg, handler.NewLeaderboardHandler(svc, pc), rdb)
		req := httptest.NewRequest("GET", "/api/leaderboard?period=weekly", nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, svc
	}
	pc := middleware.NewPermissionChecker(userRoles, roleRepo, nil, nil)

	t.Run("khách: viewer nil", func(t *testing.T) {
		status, svc := call(t, pc, "")
		if status != 200 || svc.calls != 1 || svc.viewer != nil {
			t.Fatalf("status=%d calls=%d viewer=%+v, muốn 200/1/nil", status, svc.calls, svc.viewer)
		}
	})
	t.Run("người đăng nhập thường: viewer có id, không admin", func(t *testing.T) {
		status, svc := call(t, pc, studentTok)
		if status != 200 || svc.viewer == nil || svc.viewer.UserID != studentID || svc.viewer.IsAdmin {
			t.Fatalf("status=%d viewer=%+v, muốn viewer{%s, IsAdmin=false}", status, svc.viewer, studentID)
		}
	})
	t.Run("admin SYSTEM_SETTINGS_MANAGE: IsAdmin", func(t *testing.T) {
		status, svc := call(t, pc, adminTok)
		if status != 200 || svc.viewer == nil || svc.viewer.UserID != adminID || !svc.viewer.IsAdmin {
			t.Fatalf("status=%d viewer=%+v, muốn viewer{%s, IsAdmin=true}", status, svc.viewer, adminID)
		}
	})
	t.Run("token sai: 401, không hạ xuống khách", func(t *testing.T) {
		status, svc := call(t, pc, "khong-phai-jwt")
		if status != 401 || svc.calls != 0 {
			t.Fatalf("status=%d calls=%d, muốn 401 và service không được gọi", status, svc.calls)
		}
	})
	t.Run("PermissionChecker nil: fail-closed, admin thật cũng không IsAdmin", func(t *testing.T) {
		status, svc := call(t, nil, adminTok)
		if status != 200 || svc.viewer == nil || svc.viewer.UserID != adminID || svc.viewer.IsAdmin {
			t.Fatalf("status=%d viewer=%+v, muốn viewer{%s, IsAdmin=false}", status, svc.viewer, adminID)
		}
	})
}
