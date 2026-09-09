package router

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

// dummyID1/dummyID2 — UUID hợp lệ dùng làm giá trị :id/:problemId/:userId cho các route cần
// tham số path — không quan trọng giá trị THẬT vì mọi route trong bảng dưới đây (trừ đúng 1-2
// route mutation-target) đều bị auth middleware/parseUserID chặn TRƯỚC KHI parse UUID.
const (
	dummyID1 = "11111111-1111-1111-1111-111111111111"
	dummyID2 = "22222222-2222-2222-2222-222222222222"
)

// routeAuthCase (I6-01, review vòng 6→7) — 1 dòng bảng: request KHÔNG kèm token tới path này có
// PHẢI 401 hay không.
type routeAuthCase struct {
	method   string
	path     string
	wantAuth bool // true = PHẢI 401 khi thiếu token; false = KHÔNG được 401 (route công khai)
}

// runRouteAuthCases gửi request THẬT (không token) cho từng case, assert đúng 401/không-401 —
// dùng chung cho contest/coin/group router (cả 3 đều từng dùng pattern group+Use() gây C-02).
func runRouteAuthCases(t *testing.T, app *fiber.App, cases []routeAuthCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test lỗi: %v", err)
			}
			got401 := resp.StatusCode == fiber.StatusUnauthorized
			if tc.wantAuth && !got401 {
				t.Errorf("kỳ vọng 401 (route riêng tư, thiếu token), nhận %d — mất auth", resp.StatusCode)
			}
			if !tc.wantAuth && got401 {
				t.Errorf("kỳ vọng KHÔNG 401 (route công khai), nhận 401 — route công khai bị chặn nhầm")
			}
		})
	}
}

// TestContestRoutes_AllRoutesAuthCorrect (I6-01, review vòng 6→7) — mở rộng
// TestContestRoutes_SlugPublicMeRequiresAuth (chỉ 2 route) ra TOÀN BỘ 15 route đăng ký trong
// contest_routes.go. Review vòng 6 tự chạy MUT-2b (bỏ `auth` khỏi `POST /contests`) và thấy suite
// vẫn XANH vì KHÔNG có test nào pin route đó — bảng này đóng khoảng trống.
func TestContestRoutes_AllRoutesAuthCorrect(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	h := handler.NewContestHandler(&fakeContestServiceForRouteTest{})
	SetupContestRoutes(api, &config.Config{}, h, nil)

	cases := []routeAuthCase{
		{"GET", "/api/contests/", false},
		{"GET", "/api/contests/some-slug", false},
		{"GET", "/api/contests/me", true},
		{"POST", "/api/contests/", true},
		{"PUT", "/api/contests/" + dummyID1, true},
		{"DELETE", "/api/contests/" + dummyID1, true},
		{"POST", "/api/contests/" + dummyID1 + "/publish", true},
		{"GET", "/api/contests/" + dummyID1 + "/problems", true},
		{"POST", "/api/contests/" + dummyID1 + "/problems", true},
		{"PUT", "/api/contests/" + dummyID1 + "/problems/" + dummyID2, true},
		{"DELETE", "/api/contests/" + dummyID1 + "/problems/" + dummyID2, true},
		{"POST", "/api/contests/" + dummyID1 + "/join", true},
		{"GET", "/api/contests/" + dummyID1 + "/leaderboard", true},
		{"POST", "/api/contests/" + dummyID1 + "/problems/" + dummyID2 + "/submit", true},
		{"GET", "/api/contests/" + dummyID1 + "/submissions/me", true},
	}
	runRouteAuthCases(t, app, cases)
}

// ─── coin_routes.go ─────────────────────────────────────────────────────────

// fakeCoinServiceForRouteTest — chỉ implement 2 method public (ListPackages/GetPackage) thật sự
// bị gọi trong test này; mọi route "authed"/"admin" khác đều có defense-in-depth ở tầng handler
// (parseUserID hoặc PermissionChecker.RequirePermissions tự kiểm userID) nên KHÔNG BAO GIỜ chạm
// tới service khi request thiếu token — nil-embed an toàn cho các method còn lại.
type fakeCoinServiceForRouteTest struct {
	service.CoinServiceInterface
}

func (f *fakeCoinServiceForRouteTest) ListPackages(ctx context.Context) ([]dto.CoinPackageResponse, error) {
	return []dto.CoinPackageResponse{}, nil
}

func (f *fakeCoinServiceForRouteTest) GetPackage(ctx context.Context, id uuid.UUID) (*dto.CoinPackageResponse, error) {
	return &dto.CoinPackageResponse{}, nil
}

// TestCoinRoutes_AllRoutesAuthCorrect (I6-01, review vòng 6→7) — bảng tương tự cho coin_routes.go
// (L6-02: vẫn dùng pattern `authed := coins.Group(""); authed.Use(auth)` gây ra C-02, hiện ĐÚNG
// chỉ vì mọi route public đăng ký TRƯỚC dòng Use()). Ghi chú riêng: KHÔNG có route "authed" nào
// trong file này chỉ dựa vào auth middleware — mọi handler wallet/purchases/gift đều tự gọi
// parseUserID, mọi handler admin đều qua PermissionChecker.RequirePermissions (tự kiểm userID) —
// nghĩa là mutation "bỏ 1 dòng auth middleware" KHÔNG làm bảng này đỏ (xem mutation thật ở báo
// cáo "Vòng 7": mutation THẬT có ý nghĩa cho file này là ĐẢO THỨ TỰ route public ra SAU Use() —
// đúng rủi ro L6-02 nêu, không phải "xoá auth").
func TestCoinRoutes_AllRoutesAuthCorrect(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	h := handler.NewCoinHandler(&fakeCoinServiceForRouteTest{})
	permChecker := middleware.NewPermissionChecker(nil, nil, nil, nil)
	SetupCoinRoutes(api, &config.Config{}, h, nil, permChecker)

	cases := []routeAuthCase{
		{"GET", "/api/coins/packages", false},
		{"GET", "/api/coins/packages/" + dummyID1, false},
		{"GET", "/api/coins/wallet", true},
		{"GET", "/api/coins/wallet/transactions", true},
		{"POST", "/api/coins/purchases", true},
		{"GET", "/api/coins/purchases", true},
		{"GET", "/api/coins/purchases/" + dummyID1, true},
		{"POST", "/api/coins/purchases/" + dummyID1 + "/verify", true},
		{"POST", "/api/coins/gift", true},
		{"POST", "/api/coins/admin/packages", true},
		{"PUT", "/api/coins/admin/packages/" + dummyID1, true},
		{"DELETE", "/api/coins/admin/packages/" + dummyID1, true},
		{"POST", "/api/coins/admin/adjust", true},
	}
	runRouteAuthCases(t, app, cases)
}

// ─── group_routes.go ────────────────────────────────────────────────────────

// fakeGroupServiceForRouteTest — implement 2 method public (ListGroups/GetGroupBySlug) VÀ
// ListMembers thật (khác coin ở trên: ListMembers KHÔNG có parseUserID phòng thủ ở tầng handler
// — xem mutation "Vòng 7" bên dưới — cần implement thật để không panic khi mutation chạy tới).
type fakeGroupServiceForRouteTest struct {
	service.GroupServiceInterface
}

func (f *fakeGroupServiceForRouteTest) ListGroups(ctx context.Context, keyword, privacy string, page, pageSize int) (*dto.GroupListResponse, error) {
	return &dto.GroupListResponse{}, nil
}

func (f *fakeGroupServiceForRouteTest) GetGroupBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.GroupResponse, error) {
	return &dto.GroupResponse{Slug: slug}, nil
}

func (f *fakeGroupServiceForRouteTest) ListMembers(ctx context.Context, groupID uuid.UUID, page, pageSize int) (*dto.GroupMemberListResponse, error) {
	return &dto.GroupMemberListResponse{}, nil
}

// TestGroupRoutes_AllRoutesAuthCorrect (I6-01, review vòng 6→7) — bảng cho group_routes.go.
// Khác coin_routes.go: `ListMembers` (GET /:id/members) KHÔNG gọi parseUserID ở tầng handler —
// CHỈ được bảo vệ bởi `authed.Use(auth)` — đây là route thật sự mutation-nhạy cảm trong file này
// (xem mutation "Vòng 7" bên dưới: xoá `authed.Use(auth)` -> ListMembers mất auth, các route
// "authed" khác vẫn 401 nhờ parseUserID riêng).
func TestGroupRoutes_AllRoutesAuthCorrect(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	h := handler.NewGroupHandler(&fakeGroupServiceForRouteTest{})
	SetupGroupRoutes(api, &config.Config{}, h, nil)

	cases := []routeAuthCase{
		{"GET", "/api/groups/", false},
		{"GET", "/api/groups/some-slug", false},
		{"GET", "/api/groups/me/joined", true},
		{"GET", "/api/groups/me/owned", true},
		{"POST", "/api/groups/", true},
		{"PUT", "/api/groups/" + dummyID1, true},
		{"DELETE", "/api/groups/" + dummyID1, true},
		{"POST", "/api/groups/" + dummyID1 + "/join", true},
		{"POST", "/api/groups/" + dummyID1 + "/leave", true},
		{"GET", "/api/groups/" + dummyID1 + "/members", true},
		{"POST", "/api/groups/" + dummyID1 + "/members/invite", true},
		{"PUT", "/api/groups/" + dummyID1 + "/members/" + dummyID2 + "/role", true},
		{"DELETE", "/api/groups/" + dummyID1 + "/members/" + dummyID2, true},
		{"POST", "/api/groups/" + dummyID1 + "/members/" + dummyID2 + "/ban", true},
		{"POST", "/api/groups/" + dummyID1 + "/members/" + dummyID2 + "/unban", true},
		{"GET", "/api/groups/" + dummyID1 + "/requests", true},
		{"POST", "/api/groups/" + dummyID1 + "/requests/" + dummyID2 + "/approve", true},
		{"POST", "/api/groups/" + dummyID1 + "/requests/" + dummyID2 + "/reject", true},
	}
	runRouteAuthCases(t, app, cases)
}
