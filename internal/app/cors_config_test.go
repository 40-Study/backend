package app

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// M2: preflight của trình duyệt cho request mang Idempotency-Key phải được chấp nhận, và Idempotent-Replay phải đọc được.
func TestBuildCORSConfig_AllowsIdempotencyKeyAndExposesReplay(t *testing.T) {
	app := fiber.New()
	app.Use(cors.New(BuildCORSConfig("http://localhost:3000")))
	app.Post("/x", func(c *fiber.Ctx) error { c.Set("Idempotent-Replay", "true"); return c.SendStatus(fiber.StatusCreated) })

	pre := httptest.NewRequest("OPTIONS", "/x", nil)
	pre.Header.Set("Origin", "http://localhost:3000")
	pre.Header.Set("Access-Control-Request-Method", "POST")
	pre.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
	resp, err := app.Test(pre, -1)
	if err != nil {
		t.Fatal(err)
	}
	if allow := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(allow), "idempotency-key") {
		t.Fatalf("preflight Access-Control-Allow-Headers = %q, thiếu Idempotency-Key", allow)
	}

	post := httptest.NewRequest("POST", "/x", nil)
	post.Header.Set("Origin", "http://localhost:3000")
	resp, err = app.Test(post, -1)
	if err != nil {
		t.Fatal(err)
	}
	if exposed := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(exposed, "Idempotent-Replay") {
		t.Fatalf("Access-Control-Expose-Headers = %q, thiếu Idempotent-Replay", exposed)
	}
}
