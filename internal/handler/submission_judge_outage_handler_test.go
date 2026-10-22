package handler

// Gap #5 (QA 261021): with Judge0 down, POST /submissions stored verdict=runtime_error and
// POST /submissions/run, /run-custom answered 200 {"error":"Post \"http://localhost:2358/...\": ..."},
// leaking the judge URL. Now run and run-custom answer the same 503 JUDGE_UNAVAILABLE as the lesson
// routes, and POST /submissions (still asynchronous: 201 pending) ends as system_error, never a
// verdict on the code. No response echoes the URL. Real SubmissionService + AssignmentService on a
// temporary Postgres schema; the judge URL points at a port nothing listens on.

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/config"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
)

func closedJudgeURL(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(nil)
	url := server.URL
	server.Close()
	return url
}

func TestLiveAssignmentJudgeOutage_Run503WithoutURLSubmitEndsSystemError(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mk := func(v interface{}) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	suffix := uuid.NewString()[:8]
	owner := model.User{Email: "qa-outage-owner-" + suffix + "@40study.test", PasswordHash: "x", UserName: "qa-outage-owner-" + suffix}
	student := model.User{Email: "qa-outage-student-" + suffix + "@40study.test", PasswordHash: "x", UserName: "qa-outage-student-" + suffix}
	mk(&owner)
	mk(&student)
	course := model.Course{InstructorID: owner.ID, Title: "QA outage " + suffix, Slug: "qa-outage-" + suffix, Price: decimal.Zero, Status: "published"}
	mk(&course)
	class := model.Class{Name: "QA outage " + suffix, CourseID: &course.ID, Status: "active"}
	mk(&class)
	mk(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"})
	assignment := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "Sum", Description: "d", Language: []string{"python"}, IsPublished: true}
	mk(&assignment)
	mk(&model.TestCase{AssignmentID: assignment.ID, Input: "1", ExpectedOutput: "1"})

	judgeURL := closedJudgeURL(t)
	assignments := service.NewAssignmentService(repository.NewAssignmentRepository(db), repository.NewTestCaseRepository(db),
		repository.NewSubmissionRepository(db), repository.NewClassRepository(db), nil)
	submissions := service.NewSubmissionService(repository.NewSubmissionRepository(db), assignments, repository.NewTestCaseRepository(db),
		nil, &config.Config{Environment: "test", Judge0URL: judgeURL})

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", student.ID); return c.Next() })
	h := NewSubmissionHandler(submissions, nil)
	app.Post("/submissions", h.Submit)
	app.Post("/submissions/run", h.RunCode)
	app.Post("/submissions/run-custom", h.RunCustomCode)

	aid := assignment.ID.String()
	host := strings.TrimPrefix(judgeURL, "http://")
	post := func(path, body string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		if strings.Contains(string(raw), host) || strings.Contains(string(raw), "http://") || strings.Contains(string(raw), "dial tcp") {
			t.Errorf("%s: judge address leaked: %s", path, raw)
		}
		return res.StatusCode, raw
	}
	for path, body := range map[string]string{
		"/submissions/run":        `{"assignment_id":"` + aid + `","language":"python","code":"print(1)"}`,
		"/submissions/run-custom": `{"assignment_id":"` + aid + `","language":"python","code":"print(1)","custom_input":"1"}`,
	} {
		status, raw := post(path, body)
		if status != fiber.StatusServiceUnavailable {
			t.Errorf("%s: status=%d body=%s, want 503", path, status, raw)
		}
		var out struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(raw, &out)
		if out.Code != "JUDGE_UNAVAILABLE" {
			t.Errorf("%s: code=%q body=%s, want JUDGE_UNAVAILABLE", path, out.Code, raw)
		}
	}

	status, raw := post("/submissions", `{"assignment_id":"`+aid+`","language":"python","code":"print(1)"}`)
	if status != fiber.StatusCreated {
		t.Fatalf("/submissions: status=%d body=%s, want 201 (grading stays asynchronous)", status, raw)
	}
	var row model.Submission
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if err := db.Where("assignment_id = ?", assignment.ID).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.Verdict != model.VerdictPending {
			break
		}
	}
	if row.Verdict != service.VerdictJudgeUnavailable {
		t.Fatalf("judge outage stored verdict %q, want %q (not a verdict on the code)", row.Verdict, service.VerdictJudgeUnavailable)
	}
}
