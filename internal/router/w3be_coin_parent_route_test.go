package router

// Lane W3-BE: quyết định cũ "phụ huynh không dùng ví xu" được ghim ở ROUTER thật (SetupCoinRoutes: AuthMiddleware thật +
// DenyActiveRole), không chỉ ở test riêng của middleware. Gỡ dòng authed.Use(DenyActiveRole(...)) khỏi coin_routes.go thì
// mọi ca PARENT dưới đây ĐỎ (rơi về handler, không còn 403); chặn theo mọi vai người dùng giữ thì ca STUDENT ĐỎ.

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// walletSpy: chỉ GetWallet có thân; mọi phương thức khác panic (nil interface nhúng), nên một route bị
// rơi lọt qua DenyActiveRole sẽ thành 500 (recover) thay vì 403 và test báo đỏ.
type walletSpy struct {
	service.CoinServiceInterface
	wallet int
}

func (s *walletSpy) GetWallet(context.Context, uuid.UUID) (*dto.CoinWalletResponse, error) {
	s.wallet++
	return &dto.CoinWalletResponse{}, nil
}

func TestW3BE_CoinRoutes_PhuHuynhBi403_QuaRouterThat(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "w3be-coin-route-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	spy := &walletSpy{}
	app := fiber.New()
	app.Use(fiberrecover.New())
	SetupCoinRoutes(app.Group("/api"), cfg, handler.NewCoinHandler(spy), rdb, middleware.NewPermissionChecker(nil, nil, nil, nil))

	token := func(role string) string {
		uid := uuid.New()
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(uid.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		tok, _, err := utils.GenerateTokens(cfg, uid, uuid.New(), role, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	do := func(tok, method, path string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	id := uuid.NewString()
	authed := [][2]string{
		{"GET", "/api/coins/wallet"}, {"GET", "/api/coins/wallet/transactions"},
		{"POST", "/api/coins/purchases"}, {"GET", "/api/coins/purchases"}, {"GET", "/api/coins/purchases/" + id},
		{"POST", "/api/coins/purchases/" + id + "/verify"}, {"POST", "/api/coins/gift"},
	}
	parent := token("PARENT")
	for _, r := range authed {
		status, raw := do(parent, r[0], r[1])
		if status != fiber.StatusForbidden || !strings.Contains(raw, `"code":"COIN_WALLET_NOT_FOR_PARENT"`) {
			t.Errorf("PARENT %s %s: %d %s, muốn 403 kèm code COIN_WALLET_NOT_FOR_PARENT", r[0], r[1], status, raw)
		}
	}
	if spy.wallet != 0 {
		t.Errorf("handler ví vẫn bị gọi %d lần cho phụ huynh", spy.wallet)
	}

	// Vai đang chọn quyết định, không phải mọi vai người dùng giữ: học viên vào được ví; không token thì 401.
	if status, raw := do(token("STUDENT"), "GET", "/api/coins/wallet"); status != fiber.StatusOK {
		t.Errorf("STUDENT GET /coins/wallet: %d %s, muốn 200", status, raw)
	}
	if status, _ := do("", "GET", "/api/coins/wallet"); status != fiber.StatusUnauthorized {
		t.Errorf("không token: %d, muốn 401", status)
	}
}
