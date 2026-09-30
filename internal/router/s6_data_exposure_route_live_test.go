package router

// Lane S6: hai route công khai/đăng nhập đi bằng HTTP qua route THẬT (AuthMiddleware/OptionalAuth ->
// handler -> service -> Postgres schema tạm), từng vai.
//  - GET /users/:id/public-profile theo cài đặt riêng tư: khách, người khác, chính chủ, admin.
//  - POST /courses/:courseId/reviews: chỉ học viên ghi danh; mỗi người một đánh giá mỗi khoá.
// Bỏ OptionalAuth khỏi route hồ sơ thì chính chủ cũng chỉ thấy bản rút gọn; bỏ kiểm ghi danh thì học viên lạ 201.

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
	"study.com/v1/internal/database"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type s6ExposureEnv struct {
	app     *fiber.App
	tok     map[string]string
	ids     map[string]uuid.UUID
	course  model.Course
	private model.User
	hidden  model.User
	// friendsOnly đặt hồ sơ chế độ `friends` (plan 260930 phase 05); learner là bạn ACCEPTED của họ,
	// stranger có dòng ACCEPTED nhưng bị chặn (dữ liệu không nhất quán: chặn phải thắng).
	friendsOnly model.User
}

func newS6ExposureEnv(t *testing.T) *s6ExposureEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "s6-exposure-route-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	e := &s6ExposureEnv{tok: map[string]string{}, ids: map[string]uuid.UUID{}}
	mkUser := func(name string) model.User {
		full := "Ho Ten " + name
		bio := "bio " + name
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: name + uuid.NewString()[:6], FullName: &full, Bio: &bio, IsActive: true}
		must(db.Create(&u).Error)
		must(rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err())
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), "STUDENT", nil, 1)
		must(err)
		e.tok[name] = tok
		e.ids[name] = u.ID
		return u
	}
	e.private, e.hidden, e.friendsOnly = mkUser("private"), mkUser("hidden"), mkUser("friendsonly")
	for u, vis := range map[uuid.UUID]string{e.private.ID: "private", e.hidden.ID: "hidden", e.friendsOnly.ID: "friends"} {
		must(db.Create(&model.UserPreference{UserID: u, ProfileVisibility: vis}).Error)
	}
	viewer, learner, stranger, teacher, admin := mkUser("viewer"), mkUser("learner"), mkUser("stranger"), mkUser("teacher"), mkUser("admin")
	_ = viewer
	now := time.Now()
	for _, friendID := range []uuid.UUID{learner.ID, stranger.ID} {
		must(db.Create(&model.Friendship{RequesterID: e.friendsOnly.ID, AddresseeID: friendID,
			Status: model.FriendshipStatusAccepted, RequestedAt: now, RespondedAt: &now}).Error)
	}
	must(db.Create(&model.UserBlock{BlockerID: e.friendsOnly.ID, BlockedID: stranger.ID}).Error)

	e.course = model.Course{InstructorID: teacher.ID, Title: "S6 course", Slug: "s6-" + uuid.NewString()[:8], Price: decimal.Zero, Status: "published"}
	must(db.Create(&e.course).Error)
	must(db.Create(&model.Enrollment{UserID: learner.ID, CourseID: e.course.ID, EnrolledAt: time.Now()}).Error)

	// Admin hệ thống thật (SYSTEM_SETTINGS_MANAGE qua system role) để isAdminActor đúng.
	perm := model.Permission{Name: "SYSTEM_SETTINGS_MANAGE"}
	must(db.Create(&perm).Error)
	sys := model.SystemRole{Name: "SYSTEM_ADMIN", Status: "active"}
	must(db.Create(&sys).Error)
	must(db.Create(&model.SystemRolePermission{SystemRoleID: sys.ID, PermissionID: perm.ID}).Error)
	must(db.Create(&model.UserSystemRole{UserID: admin.ID, SystemRoleID: sys.ID, Status: model.UserSystemRoleStatusActive}).Error)

	pc := middleware.NewPermissionChecker(repository.NewUserSystemRoleRepository(db), repository.NewSystemRoleRepository(db),
		repository.NewUserOrganizationRoleRepository(db), repository.NewRoleRepository(db))
	statsSvc := service.NewUserStatsService(repository.NewUserStatsRepository(db), repository.NewUserPreferenceRepository(db))
	statsSvc.SetFriendshipChecker(service.NewFriendshipService(repository.NewFriendshipRepository(db), repository.NewUserBlockRepository(db)))
	reviewSvc := service.NewReviewService(repository.NewReviewRepository(db), repository.NewCourseRepository(db), repository.NewEnrollmentRepository(db), nil)

	app := fiber.New()
	api := app.Group("/api")
	SetupUserStatsRoutes(api, cfg, handler.NewUserStatsHandler(statsSvc, pc), rdb)
	SetupReviewRoutes(api, cfg, handler.NewReviewHandler(reviewSvc), rdb)
	e.app = app
	return e
}

func (e *s6ExposureEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if who != "" {
		req.Header.Set("Authorization", "Bearer "+e.tok[who])
	}
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestS6_PublicProfileRoute_TheoCaiDatRiengTuVaTungVai(t *testing.T) {
	e := newS6ExposureEnv(t)
	path := func(u model.User) string { return "/api/users/" + u.ID.String() + "/public-profile" }

	isPrivate := func(raw string) (bool, string) {
		var out struct {
			Data struct {
				IsPrivate bool    `json:"is_private"`
				Bio       *string `json:"bio"`
				UserName  string  `json:"user_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("body không phải JSON: %s", raw)
		}
		bio := ""
		if out.Data.Bio != nil {
			bio = *out.Data.Bio
		}
		return out.Data.IsPrivate, bio
	}

	// Hồ sơ riêng tư: khách và người khác chỉ thấy tên + avatar, không có bio.
	for _, who := range []string{"", "viewer"} {
		status, raw := e.do(t, who, "GET", path(e.private), "")
		priv, bio := isPrivate(raw)
		if status != fiber.StatusOK || !priv || bio != "" || strings.Contains(raw, "bio private") {
			t.Errorf("%q xem hồ sơ riêng tư: %d %s, muốn 200 + is_private + không có bio", who, status, raw)
		}
	}
	// Chính chủ (nhờ OptionalAuth) và admin thấy đầy đủ.
	for _, who := range []string{"private", "admin"} {
		status, raw := e.do(t, who, "GET", path(e.private), "")
		priv, bio := isPrivate(raw)
		if status != fiber.StatusOK || priv || !strings.HasPrefix(bio, "bio private") {
			t.Errorf("%q xem hồ sơ riêng tư của private: %d %s, muốn hồ sơ đầy đủ", who, status, raw)
		}
	}
	// Chế độ `friends` (Q12, đảo kỳ vọng cũ "friends = private"): chỉ bạn ACCEPTED, chính chủ và admin thấy
	// đầy đủ. Khách, người lạ, giáo viên (mọi người không phải bạn) chỉ thấy tên + avatar; người đã bị chặn
	// thì dù còn dòng ACCEPTED cũng không được mở.
	for _, who := range []string{"learner", "friendsonly", "admin"} {
		status, raw := e.do(t, who, "GET", path(e.friendsOnly), "")
		priv, bio := isPrivate(raw)
		if status != fiber.StatusOK || priv || !strings.HasPrefix(bio, "bio friendsonly") {
			t.Errorf("%q xem hồ sơ chế độ friends: %d %s, muốn hồ sơ đầy đủ", who, status, raw)
		}
	}
	for _, who := range []string{"", "viewer", "teacher", "stranger"} {
		status, raw := e.do(t, who, "GET", path(e.friendsOnly), "")
		priv, bio := isPrivate(raw)
		if status != fiber.StatusOK || !priv || bio != "" || strings.Contains(raw, "bio friendsonly") {
			t.Errorf("%q (không phải bạn hợp lệ) xem hồ sơ chế độ friends: %d %s, muốn chỉ tên + avatar", who, status, raw)
		}
	}
	// Ẩn hẳn: khách và người khác 404, chính chủ và admin vẫn xem được.
	for _, who := range []string{"", "viewer"} {
		if status, raw := e.do(t, who, "GET", path(e.hidden), ""); status != fiber.StatusNotFound {
			t.Errorf("%q xem hồ sơ ẩn hẳn: %d %s, muốn 404", who, status, raw)
		}
	}
	for _, who := range []string{"hidden", "admin"} {
		if status, raw := e.do(t, who, "GET", path(e.hidden), ""); status != fiber.StatusOK {
			t.Errorf("%q xem hồ sơ ẩn hẳn: %d %s, muốn 200", who, status, raw)
		}
	}
	// Token sai vẫn 401 (không lặng lẽ hạ xuống khách), giữ hợp đồng OptionalAuth.
	req := httptest.NewRequest("GET", path(e.private), nil)
	req.Header.Set("Authorization", "Bearer khong-hop-le")
	if resp, err := e.app.Test(req, -1); err != nil || resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("token sai: %v %v, muốn 401", resp, err)
	}
}

func TestS6_ReviewRoute_ChiHocVienGhiDanhTungVai(t *testing.T) {
	e := newS6ExposureEnv(t)
	path := "/api/courses/" + e.course.ID.String() + "/reviews"
	body := `{"rating":5,"comment":"Rat hay"}`

	for _, who := range []string{"stranger", "teacher"} {
		status, raw := e.do(t, who, "POST", path, body)
		if status != fiber.StatusForbidden || !strings.Contains(raw, "đăng ký khoá học") {
			t.Errorf("%s đánh giá khoá chưa ghi danh: %d %s, muốn 403 kèm thông báo tiếng Việt", who, status, raw)
		}
	}
	if status, raw := e.do(t, "", "POST", path, body); status != fiber.StatusUnauthorized {
		t.Errorf("khách đánh giá: %d %s, muốn 401", status, raw)
	}
	if status, raw := e.do(t, "learner", "POST", path, body); status != fiber.StatusCreated {
		t.Errorf("học viên ghi danh đánh giá: %d %s, muốn 201", status, raw)
	}
	if status, raw := e.do(t, "learner", "POST", path, body); status != fiber.StatusConflict {
		t.Errorf("đánh giá lần 2: %d %s, muốn 409", status, raw)
	}
	// Danh sách đánh giá vẫn công khai.
	status, raw := e.do(t, "", "GET", path, "")
	if status != fiber.StatusOK || strings.Count(raw, `"rating":5`) != 1 {
		t.Errorf("danh sách đánh giá công khai: %d %s, muốn đúng 1 đánh giá", status, raw)
	}
}
