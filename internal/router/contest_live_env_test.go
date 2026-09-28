package router

// MVP "Cuộc thi" — môi trường test route SỐNG trên Postgres THẬT (pgtest.IsolatedSchema +
// database.Migrate): Fiber route thật (SetupContestRoutes) → AuthMiddleware/OptionalAuth/
// PermissionChecker thật → ContestHandler/ContestService/ContestRepository thật → Postgres.
// Chỉ fake: kho quyền (như approval_live_env_test.go), engine chấm bài và issuer phát thưởng của
// lane B2 (contest_fakes_test.go) — fake ghi/đọc quiz_attempts, user_vouchers bằng CHÍNH tx được
// truyền vào nên rollback/khóa dòng được kiểm thật.

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

type ctEnv struct {
	t      *testing.T
	app    *fiber.App
	db     *gorm.DB
	svc    *service.ContestService
	issuer *fakeIssuer
	ids    map[string]uuid.UUID // admin, teacherA, teacherB, student1..student5, parent
	toks   map[string]string
}

var ctRoles = map[string]string{
	"admin": "SYSTEM_ADMIN", "teacherA": "TEACHER", "teacherB": "TEACHER", "parent": "PARENT",
	"student1": "STUDENT", "student2": "STUDENT", "student3": "STUDENT", "student4": "STUDENT", "student5": "STUDENT",
}

func newCtEnv(t *testing.T) *ctEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "contest-live-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	roles := map[string]*model.SystemRole{}
	perms := map[uuid.UUID][]string{}
	for _, name := range []string{"SYSTEM_ADMIN", "TEACHER", "STUDENT", "PARENT"} {
		r := &model.SystemRole{Name: name}
		r.ID = uuid.New()
		roles[name] = r
	}
	perms[roles["SYSTEM_ADMIN"].ID] = []string{"*"}
	perms[roles["TEACHER"].ID] = []string{"CONTESTS_MANAGE_OWN", "COURSES_CREATE"}

	e := &ctEnv{t: t, db: db, ids: map[string]uuid.UUID{}, toks: map[string]string{}}
	usr := &apvUserSystemRoleRepo{roles: map[uuid.UUID][]*model.SystemRole{}}
	ctx := context.Background()
	device := uuid.New()
	for key, role := range ctRoles {
		u := model.User{Email: key + "@contest.test", UserName: key, PasswordHash: "x", FullName: strPtr("QA-contest " + key)}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tao user %s: %v", key, err)
		}
		e.ids[key] = u.ID
		usr.roles[u.ID] = []*model.SystemRole{roles[role]}
		if err := rdb.Set(ctx, constants.KeyUserVersion(u.ID.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		access, _, err := utils.GenerateTokens(cfg, u.ID, device, role, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		e.toks[key] = access
	}

	pc := middleware.NewPermissionChecker(usr, &apvSystemRoleRepo{perms: perms}, nil, nil)
	e.issuer = &fakeIssuer{}
	e.svc = service.NewContestService(repository.NewContestRepository(db), &fakeEngine{db: db}, e.issuer,
		repository.NewEnrollmentRepository(db))
	app := fiber.New()
	SetupContestRoutes(app.Group("/api"), cfg, handler.NewContestHandler(e.svc, pc), rdb, pc)
	e.app = app
	return e
}

func strPtr(s string) *string { return &s }

type ctResp struct {
	status int
	raw    string
	body   map[string]interface{}
}

func (r ctResp) data() map[string]interface{} { d, _ := r.body["data"].(map[string]interface{}); return d }

func (r ctResp) items() []interface{} { it, _ := r.data()["items"].([]interface{}); return it }

func (e *ctEnv) do(method, path, who, body string) ctResp {
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
	out := ctResp{status: res.StatusCode, raw: string(raw), body: map[string]interface{}{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (e *ctEnv) must(what string, r ctResp, status int, code ...string) ctResp {
	e.t.Helper()
	if r.status != status {
		e.t.Fatalf("%s: status=%d muon %d, body=%s", what, r.status, status, r.raw)
	}
	if len(code) > 0 && r.body["code"] != code[0] {
		e.t.Fatalf("%s: code=%v muon %s, body=%s", what, r.body["code"], code[0], r.raw)
	}
	return r
}

// ── Dữ liệu ─────────────────────────────────────────────────────────────────

// newQuiz tạo quiz standalone của owner gồm 2 câu single_choice (1 điểm/câu). Trả quiz id và
// đáp án ĐÚNG của từng câu (để test nộp bài đúng/sai).
func (e *ctEnv) newQuiz(owner string) (uuid.UUID, map[uuid.UUID]uuid.UUID) {
	e.t.Helper()
	q := model.Quiz{Title: "QA-contest quiz " + uuid.NewString()[:6], TriggerType: "manual"}
	if err := e.db.Create(&q).Error; err != nil {
		e.t.Fatal(err)
	}
	if err := e.db.Exec("UPDATE quizzes SET created_by = ? WHERE id = ?", e.ids[owner], q.ID).Error; err != nil {
		e.t.Fatal(err)
	}
	correct := map[uuid.UUID]uuid.UUID{}
	for i := 1; i <= 2; i++ {
		qq := model.Question{QuizID: q.ID, QuestionText: "Cau " + string(rune('0'+i)), QuestionType: "single_choice",
			Points: decimal.NewFromInt(1), DisplayOrder: i, Explanation: strPtr("giai thich bi mat")}
		if err := e.db.Create(&qq).Error; err != nil {
			e.t.Fatal(err)
		}
		for j, ok := range []bool{true, false} {
			a := model.QuestionAnswer{QuestionID: qq.ID, AnswerText: "dap an", IsCorrect: ok, DisplayOrder: j + 1}
			if err := e.db.Create(&a).Error; err != nil {
				e.t.Fatal(err)
			}
			if ok {
				correct[qq.ID] = a.ID
			}
		}
	}
	return q.ID, correct
}

func (e *ctEnv) newVoucher(active bool) uuid.UUID {
	e.t.Helper()
	v := model.Voucher{Code: "QACONTEST" + strings.ToUpper(uuid.NewString()[:6]), Name: "QA-contest voucher",
		DiscountUnit: "MONEY", DiscountMethod: "FIXED", IsActive: true}
	if err := e.db.Create(&v).Error; err != nil {
		e.t.Fatal(err)
	}
	if !active {
		e.db.Exec("UPDATE vouchers SET is_active = false WHERE id = ?", v.ID)
	}
	return v.ID
}

func contestBody(quizID uuid.UUID, extra string) string {
	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)
	b := `{"title":"QA-contest thi thu","quiz_id":"` + quizID.String() + `","start_time":"` + start +
		`","end_time":"` + end + `","duration_minutes":30,"max_participants":0,"is_public":true,"prizes":[]`
	if extra != "" {
		b += "," + extra
	}
	return b + "}"
}

// publishedContest: teacherA tạo → gửi duyệt → admin duyệt; trả id + slug.
func (e *ctEnv) publishedContest(extra string) (uuid.UUID, string, map[uuid.UUID]uuid.UUID) {
	e.t.Helper()
	quizID, correct := e.newQuiz("teacherA")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, extra)), 201)
	id := r.data()["id"].(string)
	e.must("gui duyet", e.do("POST", "/api/contests/"+id+"/submit-review", "teacherA", ""), 200)
	e.must("duyet", e.do("POST", "/api/admin/contests/"+id+"/approve", "admin", ""), 200)
	return uuid.MustParse(id), r.data()["slug"].(string), correct
}

// setWindow dời lịch cuộc thi (tương đối với hiện tại) — cách DUY NHẤT để đi qua các phase mà
// không chờ thật; API không cho đặt giờ bắt đầu trong quá khứ.
func (e *ctEnv) setWindow(id uuid.UUID, startAgo, endIn time.Duration) {
	e.t.Helper()
	now := time.Now()
	if err := e.db.Exec("UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?",
		now.Add(-startAgo), now.Add(endIn), id).Error; err != nil {
		e.t.Fatal(err)
	}
}

func answersBody(attemptID string, correct map[uuid.UUID]uuid.UUID, nRight int) string {
	parts := []string{}
	i := 0
	for q, a := range correct {
		sel := a.String()
		if i >= nRight {
			sel = uuid.NewString() // đáp án sai
		}
		parts = append(parts, `{"question_id":"`+q.String()+`","selected_answer_ids":["`+sel+`"]}`)
		i++
	}
	return `{"attempt_id":"` + attemptID + `","answers":[` + strings.Join(parts, ",") + `]}`
}
