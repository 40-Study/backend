package router

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
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/utils"
)

func TestOpenDesignRoutesAuthAndConfiguration(t *testing.T) {
	cfg := &config.Config{JWTSecret: "design-test-only", JWTAccessExpiration: time.Minute, JWTRefreshExpiration: time.Hour, AllowedOrigins: "http://localhost:3000"}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	userID := uuid.New()
	_ = rdb.Set(context.Background(), constants.KeyUserVersion(userID.String()), 1, 0).Err()
	token, _, err := utils.GenerateTokens(cfg, userID, uuid.New(), "TEACHER", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	studentToken, _, err := utils.GenerateTokens(cfg, userID, uuid.New(), "STUDENT", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	SetupOpenDesignRoutes(app.Group("/api"), cfg, rdb)
	for _, tc := range []struct {
		method, path, body, auth, origin string
		status                           int
	}{
		{"GET", "/api/open-design/status", "", "", "", 401},
		{"POST", "/api/open-design/generate", "{}", "", "", 401},
		{"POST", "/api/open-design/cancel", "{}", "", "", 401},
		{"GET", "/api/open-design/status", "", token, "", 200},
		{"GET", "/api/open-design/status", "", studentToken, "", 403},
		{"POST", "/api/open-design/generate", "{}", studentToken, "http://localhost:3000", 403},
		{"POST", "/api/open-design/generate", `{"request_id":"not-uuid"}`, token, "http://localhost:3000", 400},
		{"POST", "/api/open-design/generate", `{"request_id":"` + uuid.NewString() + `","output":"slides","count":1,"prompt":"Create"}`, token, "http://localhost:3000", 503},
		{"POST", "/api/open-design/generate", `{}`, token, "https://evil.example", 403},
		{"POST", "/api/open-design/cancel", `{"request_id":"` + uuid.NewString() + `"}`, token, "http://localhost:3000", 200},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		if tc.auth != "" {
			req.Header.Set("Cookie", "accessToken="+tc.auth)
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Errorf("%s %s got %d want %d: %s", tc.method, tc.path, res.StatusCode, tc.status, raw)
		}
		if tc.method == "GET" && tc.status == 200 {
			var body struct {
				Data struct {
					Configured bool   `json:"configured"`
					Provider   string `json:"provider"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Data.Configured || body.Data.Provider != "Viettel AI" {
				t.Errorf("incorrect configuration response: %s", raw)
			}
			if strings.Contains(string(raw), cfg.JWTSecret) {
				t.Fatal("secret exposed")
			}
		}
	}
}
