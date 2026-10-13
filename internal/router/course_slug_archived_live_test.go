package router

// Smoke QA2 (13/10): route THẬT GET /courses/slug/:slug cho khoá archived. Trang /learn của web gọi route này theo slug;
// OptionalAuth phải đưa người dùng đăng nhập vào handler thì học viên đã ghi danh mới học tiếp được, còn khách và người
// lạ vẫn 404. Bỏ OptionalAuth khỏi route thì cả học viên cũng 404 (test đỏ).

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/database"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

func TestCourseSlugRoute_Archived_HocVienDaGhiDanhVao_NguoiLaVaKhach404(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "slug-archived-route-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	tokens := map[string]string{}
	mkUser := func(name string) model.User {
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: name + uuid.NewString()[:6], IsActive: true}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), "STUDENT", nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		tokens[name] = tok
		return u
	}
	teacher, student := mkUser("teacher"), mkUser("student")
	mkUser("stranger")
	course := model.Course{InstructorID: teacher.ID, Title: "Archived", Slug: "archived-" + uuid.NewString()[:8], Price: decimal.Zero, Status: model.CourseStatusPublished}
	if err := db.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Enrollment{UserID: student.ID, CourseID: course.ID, EnrolledAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}

	courseRepo := repository.NewCourseRepository(db)
	svc := service.NewCourseService(courseRepo, repository.NewCategoryRepository(db), repository.NewTagRepository(db), repository.NewEnrollmentRepository(db))
	app := fiber.New()
	SetupCourseRoutes(app.Group("/api"), cfg, handler.NewCourseHandler(svc, nil), nil, nil, nil, nil, nil, nil, nil, rdb)

	get := func(who string) int {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/courses/slug/"+course.Slug, nil)
		if who != "" {
			req.Header.Set("Authorization", "Bearer "+tokens[who])
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	// Đối chứng khi còn published: ai cũng 200.
	for _, who := range []string{"", "stranger", "student"} {
		if code := get(who); code != fiber.StatusOK {
			t.Fatalf("published, %q: %d, muon 200", who, code)
		}
	}
	if err := db.Exec("UPDATE courses SET status = 'archived' WHERE id = ?", course.ID).Error; err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"": 404, "stranger": 404, "teacher": 200, "student": 200}
	for who, code := range want {
		if got := get(who); got != code {
			t.Errorf("archived, %q: %d, muon %d", who, got, code)
		}
	}
}
