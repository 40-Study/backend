package router

// Phase 3 duyệt khoá học + duyệt giáo viên (2026-09-28) — môi trường test "route SỐNG", cùng
// khuôn user_admin_live_test.go: Fiber route THẬT (SetupApprovalRoutes/SetupAuthRoutes/
// SetupCourseRoutes) → AuthMiddleware THẬT → PermissionChecker THẬT → handler/service THẬT →
// Redis THẬT (miniredis). Chỉ repository được fake (biên DB); các fake gọi CHÍNH hàm thuần
// quyết định transition của repository thật (EvaluateCourseReviewTransition,
// EvaluateTeacherReview, EvaluateTeacherResubmission) để không chép lại luật. Transaction/khoá
// dòng Postgres thật được pin riêng ở internal/repository/approval_pg_test.go.

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

const approvalLiveSecret = "approval-live-test-secret"

// ── Fake quyền / vai trò ────────────────────────────────────────────────────

type apvSystemRoleRepo struct {
	repository.SystemRoleRepositoryInterface
	perms map[uuid.UUID][]string
}

func (f *apvSystemRoleRepo) GetPermissionsBySystemRoleID(_ context.Context, id uuid.UUID) ([]model.Permission, error) {
	var out []model.Permission
	for _, p := range f.perms[id] {
		out = append(out, model.Permission{Name: p})
	}
	return out, nil
}

type apvUserSystemRoleRepo struct {
	repository.UserSystemRoleRepositoryInterface
	roles map[uuid.UUID][]*model.SystemRole // user -> các role ĐANG active
}

func (f *apvUserSystemRoleRepo) rows(userID uuid.UUID) []model.UserSystemRole {
	var out []model.UserSystemRole
	for _, r := range f.roles[userID] {
		out = append(out, model.UserSystemRole{UserID: userID, SystemRoleID: r.ID, SystemRole: r, Status: model.UserSystemRoleStatusActive})
	}
	return out
}

func (f *apvUserSystemRoleRepo) FindByUserID(_ context.Context, userID uuid.UUID, _ string) ([]model.UserSystemRole, error) {
	return f.rows(userID), nil
}

func (f *apvUserSystemRoleRepo) FindByUserIDWithDetails(_ context.Context, userID uuid.UUID, _ string) ([]model.UserSystemRole, error) {
	return f.rows(userID), nil
}

func (f *apvUserSystemRoleRepo) hasRole(userID uuid.UUID, name string) bool {
	for _, r := range f.roles[userID] {
		if r.Name == name {
			return true
		}
	}
	return false
}

// ── Fake khoá học ───────────────────────────────────────────────────────────

type apvCourseReviewRepo struct {
	courses map[uuid.UUID]*model.Course
}

func (f *apvCourseReviewRepo) ApplyReviewAction(_ context.Context, courseID uuid.UUID, action repository.CourseReviewAction,
	ownerID, reviewerID *uuid.UUID, reason *string) (*model.Course, error) {
	c, ok := f.courses[courseID]
	if !ok {
		return nil, repository.ErrCourseReviewNotFound
	}
	if ownerID != nil && c.InstructorID != *ownerID {
		return nil, repository.ErrCourseReviewNotOwner
	}
	next, err := repository.EvaluateCourseReviewTransition(c.Status, action)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	c.Status = next
	switch action {
	case repository.CourseActionSubmit:
		c.SubmittedAt = &now
	case repository.CourseActionApprove:
		c.PublishedAt, c.ReviewedBy, c.ReviewedAt, c.RejectionReason = &now, reviewerID, &now, nil
	case repository.CourseActionReject:
		c.ReviewedBy, c.ReviewedAt, c.RejectionReason = reviewerID, &now, reason
	}
	return c, nil
}

func (f *apvCourseReviewRepo) ListForReview(_ context.Context, filter repository.AdminCourseReviewFilter) ([]model.Course, int64, error) {
	var out []model.Course
	for _, c := range f.courses {
		if c.Status == filter.Status {
			out = append(out, *c)
		}
	}
	return out, int64(len(out)), nil
}

type apvCourseRepo struct {
	repository.CourseRepositoryInterface
	created []*model.Course
}

func (f *apvCourseRepo) SlugExists(context.Context, string) (bool, error) { return false, nil }

func (f *apvCourseRepo) Create(_ context.Context, c *model.Course) error {
	c.ID = uuid.New()
	f.created = append(f.created, c)
	return nil
}

// ── Fake hồ sơ giáo viên ────────────────────────────────────────────────────

type apvTeacherAppRepo struct {
	profiles  map[uuid.UUID]*model.TeacherProfile // theo user_id
	usr       *apvUserSystemRoleRepo
	teacher   *model.SystemRole
	applicant *model.SystemRole
}

func (f *apvTeacherAppRepo) profile(userID uuid.UUID) (*model.TeacherProfile, error) {
	p, ok := f.profiles[userID]
	if !ok {
		return nil, repository.ErrTeacherApplicationNotFound
	}
	return p, nil
}

func (f *apvTeacherAppRepo) List(_ context.Context, filter repository.TeacherApplicationFilter) ([]repository.TeacherApplicationRow, int64, error) {
	var out []repository.TeacherApplicationRow
	for _, p := range f.profiles {
		if p.ApprovalStatus == filter.Status {
			out = append(out, repository.TeacherApplicationRow{TeacherProfile: *p, Email: "applicant@demo.com"})
		}
	}
	return out, int64(len(out)), nil
}

// Approve mô phỏng đúng thứ tự của repo thật: gán TEACHER rồi gỡ TEACHER_APPLICANT.
func (f *apvTeacherAppRepo) Approve(_ context.Context, userID, reviewerID uuid.UUID) (*model.TeacherProfile, error) {
	p, err := f.profile(userID)
	if err != nil {
		return nil, err
	}
	if err := repository.EvaluateTeacherReview(p.ApprovalStatus); err != nil {
		return nil, err
	}
	var kept []*model.SystemRole
	for _, r := range f.usr.roles[userID] {
		if r.Name != f.applicant.Name {
			kept = append(kept, r)
		}
	}
	f.usr.roles[userID] = append(kept, f.teacher)
	now := time.Now()
	p.ApprovalStatus, p.ReviewedBy, p.ReviewedAt, p.RejectionReason = model.TeacherApprovalApproved, &reviewerID, &now, nil
	return p, nil
}

func (f *apvTeacherAppRepo) Reject(_ context.Context, userID, reviewerID uuid.UUID, reason string) (*model.TeacherProfile, error) {
	p, err := f.profile(userID)
	if err != nil {
		return nil, err
	}
	if err := repository.EvaluateTeacherReview(p.ApprovalStatus); err != nil {
		return nil, err
	}
	p.ApprovalStatus, p.RejectionReason, p.ReviewedBy = model.TeacherApprovalRejected, &reason, &reviewerID
	return p, nil
}

func (f *apvTeacherAppRepo) Resubmit(_ context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	p, err := f.profile(userID)
	if err != nil {
		return nil, err
	}
	if err := repository.EvaluateTeacherResubmission(p.ApprovalStatus, p.ResubmissionCount); err != nil {
		return nil, err
	}
	p.ApprovalStatus = model.TeacherApprovalPending
	p.ResubmissionCount++
	return p, nil
}

type apvTeacherProfileRepo struct {
	repository.TeacherProfileRepositoryInterface
	app *apvTeacherAppRepo
}

func (f *apvTeacherProfileRepo) GetByUserID(_ context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	return f.app.profiles[userID], nil
}

// ── Môi trường ──────────────────────────────────────────────────────────────

type apvEnv struct {
	app                                        *fiber.App
	cfg                                        *config.Config
	rdb                                        *redis.Client
	usr                                        *apvUserSystemRoleRepo
	courses                                    *apvCourseReviewRepo
	teacherApp                                 *apvTeacherAppRepo
	courseRepo                                 *apvCourseRepo
	adminID, teacherID, otherTeacherID         uuid.UUID
	applicantID, studentID, deviceID           uuid.UUID
	adminTok, teacherTok, otherTeacherTok      string
	applicantTok, applicantRefresh, studentTok string
	draftCourseID                              uuid.UUID
}

func newApvEnv(t *testing.T) *apvEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: approvalLiveSecret, JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 24 * time.Hour}

	adminRole := &model.SystemRole{Name: "SYSTEM_ADMIN"}
	teacherRole := &model.SystemRole{Name: "TEACHER"}
	applicantRole := &model.SystemRole{Name: "TEACHER_APPLICANT"}
	studentRole := &model.SystemRole{Name: "STUDENT"}
	for _, r := range []*model.SystemRole{adminRole, teacherRole, applicantRole, studentRole} {
		r.ID = uuid.New()
	}
	srRepo := &apvSystemRoleRepo{perms: map[uuid.UUID][]string{
		adminRole.ID:     {"COURSES_APPROVE_ALL", "ROLES_MANAGE_SYSTEM", "COURSES_CREATE", "COURSES_UPDATE_OWN"},
		teacherRole.ID:   {"COURSES_CREATE", "COURSES_UPDATE_OWN", "COURSES_DELETE_OWN", "LESSONS_MANAGE"},
		applicantRole.ID: {"APPLICATION_VIEW_STATUS", "TEACHER_PROFILE_UPDATE"},
	}}

	e := &apvEnv{cfg: cfg, rdb: rdb, adminID: uuid.New(), teacherID: uuid.New(), otherTeacherID: uuid.New(),
		applicantID: uuid.New(), studentID: uuid.New(), deviceID: uuid.New(), draftCourseID: uuid.New()}
	e.usr = &apvUserSystemRoleRepo{roles: map[uuid.UUID][]*model.SystemRole{
		e.adminID: {adminRole}, e.teacherID: {teacherRole}, e.otherTeacherID: {teacherRole},
		e.applicantID: {applicantRole}, e.studentID: {studentRole},
	}}
	draft := &model.Course{InstructorID: e.teacherID, Title: "Go co ban", Status: model.CourseStatusDraft,
		Instructor: model.User{Email: "teacher1@demo.com", UserName: "teacher1"}}
	draft.ID = e.draftCourseID
	e.courses = &apvCourseReviewRepo{courses: map[uuid.UUID]*model.Course{draft.ID: draft}}
	e.teacherApp = &apvTeacherAppRepo{usr: e.usr, teacher: teacherRole, applicant: applicantRole,
		profiles: map[uuid.UUID]*model.TeacherProfile{e.applicantID: {UserID: e.applicantID, ApprovalStatus: model.TeacherApprovalPending}}}
	e.courseRepo = &apvCourseRepo{}

	pc := middleware.NewPermissionChecker(e.usr, srRepo, nil, nil)
	authSvc := service.NewAuthService(cfg, nil, nil, nil, e.usr, srRepo, nil, rdb)
	approvalH := handler.NewApprovalHandler(
		service.NewCourseReviewService(e.courses),
		service.NewTeacherApplicationService(e.teacherApp, &apvTeacherProfileRepo{app: e.teacherApp}, authSvc),
	)
	courseH := handler.NewCourseHandler(service.NewCourseService(e.courseRepo, nil, nil, nil), pc)

	ctx := context.Background()
	tok := func(id uuid.UUID, role string) (string, string) {
		if err := rdb.Set(ctx, constants.KeyUserVersion(id.String()), 1, 0).Err(); err != nil {
			t.Fatalf("seed user_version: %v", err)
		}
		access, refresh, err := utils.GenerateTokens(cfg, id, e.deviceID, role, nil, 1)
		if err != nil {
			t.Fatalf("GenerateTokens: %v", err)
		}
		if err := rdb.HSet(ctx, constants.KeyRefresh(id.String()), e.deviceID.String(), refresh).Err(); err != nil {
			t.Fatalf("seed refresh: %v", err)
		}
		return access, refresh
	}
	e.adminTok, _ = tok(e.adminID, "SYSTEM_ADMIN")
	e.teacherTok, _ = tok(e.teacherID, "TEACHER")
	e.otherTeacherTok, _ = tok(e.otherTeacherID, "TEACHER")
	e.applicantTok, e.applicantRefresh = tok(e.applicantID, "TEACHER_APPLICANT")
	e.studentTok, _ = tok(e.studentID, "STUDENT")

	app := fiber.New()
	api := app.Group("/api")
	SetupApprovalRoutes(api, cfg, approvalH, rdb, pc)
	SetupAuthRoutes(api, cfg, handler.NewAuthHandler(authSvc), nil, rdb, nil)
	SetupCourseRoutes(api, cfg, courseH, nil, nil, nil, nil, nil, nil, rdb)
	e.app = app
	return e
}
