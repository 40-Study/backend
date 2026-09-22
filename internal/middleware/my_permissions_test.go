package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Fake toi thieu: nhung interface de chi cai dung cac method ma resolvePermissions goi.
type fakeUserSystemRoleRepoMP struct {
	repository.UserSystemRoleRepositoryInterface
	roles []model.UserSystemRole
}

func (f *fakeUserSystemRoleRepoMP) FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return f.roles, nil
}

type fakeSystemRoleRepoMP struct {
	repository.SystemRoleRepositoryInterface
	perms map[uuid.UUID][]string
	err   error
}

func (f *fakeSystemRoleRepoMP) GetPermissionsBySystemRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []model.Permission
	for _, n := range f.perms[roleID] {
		out = append(out, model.Permission{Name: n})
	}
	return out, nil
}

func newMyPermsApp(pc *PermissionChecker, userID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Get("/me/permissions", func(c *fiber.Ctx) error {
		if userID != uuid.Nil {
			c.Locals("user_id", userID)
		}
		return c.Next()
	}, pc.MyPermissions)
	return app
}

func callMyPerms(t *testing.T, app *fiber.App) (int, []string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/me/permissions", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Data struct {
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &parsed)
	return resp.StatusCode, parsed.Data.Permissions
}

// Nguoi dung thuong (khong co ROLES_MANAGE_SYSTEM) phai doc duoc quyen cua CHINH minh — day la
// dieu GET /system-roles/:id/permissions tu choi (403) va lam web xoa phien sau khi dang nhap.
func TestMyPermissions_ReturnsCallersOwnPermissionsDedupedSorted(t *testing.T) {
	student, teacher := uuid.New(), uuid.New()
	pc := NewPermissionChecker(
		&fakeUserSystemRoleRepoMP{roles: []model.UserSystemRole{{SystemRoleID: student}, {SystemRoleID: teacher}}},
		&fakeSystemRoleRepoMP{perms: map[uuid.UUID][]string{
			student: {"COURSE_VIEW", "QUIZ_TAKE"},
			teacher: {"COURSE_CREATE", "COURSE_VIEW"},
		}},
		nil, nil,
	)
	status, perms := callMyPerms(t, newMyPermsApp(pc, uuid.New()))
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	want := []string{"COURSE_CREATE", "COURSE_VIEW", "QUIZ_TAKE"}
	if !reflect.DeepEqual(perms, want) {
		t.Fatalf("permissions = %v, want %v", perms, want)
	}
}

func TestMyPermissions_NoRolesReturnsEmptyListNotNull(t *testing.T) {
	pc := NewPermissionChecker(&fakeUserSystemRoleRepoMP{}, &fakeSystemRoleRepoMP{}, nil, nil)
	status, perms := callMyPerms(t, newMyPermsApp(pc, uuid.New()))
	if status != 200 || perms == nil || len(perms) != 0 {
		t.Fatalf("status=%d perms=%v, want 200 va []", status, perms)
	}
}

func TestMyPermissions_Unauthenticated401(t *testing.T) {
	pc := NewPermissionChecker(&fakeUserSystemRoleRepoMP{}, &fakeSystemRoleRepoMP{}, nil, nil)
	if status, _ := callMyPerms(t, newMyPermsApp(pc, uuid.Nil)); status != 401 {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestMyPermissions_RepoError500(t *testing.T) {
	pc := NewPermissionChecker(
		&fakeUserSystemRoleRepoMP{roles: []model.UserSystemRole{{SystemRoleID: uuid.New()}}},
		&fakeSystemRoleRepoMP{err: errors.New("db down")},
		nil, nil,
	)
	if status, _ := callMyPerms(t, newMyPermsApp(pc, uuid.New())); status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
}
