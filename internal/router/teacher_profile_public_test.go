package router

// Review PR #73 (MAJOR #1): GET /teacher-profiles và /teacher-profiles/:id là route CÔNG KHAI
// (không AuthMiddleware). Gọi KHÔNG token: hồ sơ pending/rejected không được lộ, và JSON không có
// approval_status / rejection_reason / resubmission_count / reviewed_at. Việc lọc approved ở tầng
// SQL được pin riêng bằng Postgres thật (repository/approval_pg_test.go).

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type publicProfileRepo struct {
	repository.TeacherProfileRepositoryInterface
	byID map[uuid.UUID]*model.TeacherProfile
}

func (f *publicProfileRepo) GetByID(_ context.Context, id uuid.UUID) (*model.TeacherProfile, error) {
	return f.byID[id], nil
}

// GetAll mô phỏng repo thật: chỉ trả approved. Dòng approved này vẫn còn rejection_reason cũ (từng
// bị từ chối rồi mới được duyệt) — DTO công khai không được mang nó ra.
func (f *publicProfileRepo) GetAll(context.Context, int, int, string, string) ([]model.TeacherProfile, int64, error) {
	var out []model.TeacherProfile
	for _, p := range f.byID {
		if p.ApprovalStatus == model.TeacherApprovalApproved {
			out = append(out, *p)
		}
	}
	return out, int64(len(out)), nil
}

func TestTeacherProfilesPublic_NoReviewDataWithoutToken(t *testing.T) {
	reason := "Bang cap khong hop le"
	mk := func(status string) *model.TeacherProfile {
		p := &model.TeacherProfile{UserID: uuid.New(), ApprovalStatus: status, RejectionReason: &reason, ResubmissionCount: 2}
		p.ID = uuid.New()
		return p
	}
	approved, pending, rejected := mk(model.TeacherApprovalApproved), mk(model.TeacherApprovalPending), mk(model.TeacherApprovalRejected)
	repo := &publicProfileRepo{byID: map[uuid.UUID]*model.TeacherProfile{approved.ID: approved, pending.ID: pending, rejected.ID: rejected}}

	app := fiber.New()
	SetupTeacherProfileRoutes(app.Group("/api"), &config.Config{JWTSecret: "x"},
		handler.NewTeacherProfileHandler(service.NewTeacherProfileService(repo)), nil)

	get := func(path string) (int, string) {
		res, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	forbidden := []string{"rejection_reason", "approval_status", "resubmission_count", "reviewed_at", reason}

	code, body := get("/api/teacher-profiles")
	if code != 200 || !strings.Contains(body, approved.ID.String()) {
		t.Fatalf("danh sach cong khai: %d %s", code, body)
	}
	for _, id := range []uuid.UUID{pending.ID, rejected.ID} {
		if strings.Contains(body, id.String()) {
			t.Errorf("danh sach cong khai lo ho so chua duyet %s", id)
		}
	}
	code, detail := get("/api/teacher-profiles/" + approved.ID.String())
	if code != 200 {
		t.Fatalf("chi tiet ho so da duyet: %d %s", code, detail)
	}
	for _, f := range forbidden {
		if strings.Contains(body, f) || strings.Contains(detail, f) {
			t.Errorf("route cong khai lo %q", f)
		}
	}
	for _, p := range []*model.TeacherProfile{pending, rejected} {
		if code, b := get("/api/teacher-profiles/" + p.ID.String()); code != 404 || strings.Contains(b, reason) {
			t.Errorf("chi tiet ho so %s phai 404 khong lo ly do: %d %s", p.ApprovalStatus, code, b)
		}
	}
}
