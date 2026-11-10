package router

// QA-C-01 (261110): GET /api/organizations/:organization_id/user-lookup?email= đi bằng route THẬT (AuthMiddleware ->
// RequireOrgPermission ORG_MEMBERS_MANAGE -> handler -> service -> Postgres schema tạm). Bốn nhánh của contract:
// 200 (email khác hoa thường + khoảng trắng), 404 (không có / bị khoá / đã xoá mềm / chỉ khớp một phần), 400 (thiếu
// hoặc sai định dạng), 403 (không có quyền, hoặc chủ tổ chức A tra trong tổ chức B). Gỡ route thì mọi ca ĐỎ.

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func TestQAC01_OrgUserLookup_Route(t *testing.T) {
	e := newS6OrgRouteEnv(t)
	suffix := uuid.NewString()[:6]
	mk := func(name string, active bool) model.User {
		full := "Nguyen " + name
		u := model.User{Email: "lookup-" + name + "-" + suffix + "@40study.test", PasswordHash: "x", UserName: "lookup" + name + suffix, FullName: &full, IsActive: true}
		if err := e.db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		if !active { // default:true của GORM nuốt false lúc Create
			e.db.Model(&u).Update("is_active", false)
		}
		return u
	}
	target, locked, gone := mk("target", true), mk("locked", false), mk("gone", true)
	e.db.Delete(&gone)

	path := func(org uuid.UUID, email string) string {
		return "/api/organizations/" + org.String() + "/user-lookup?email=" + url.QueryEscape(email)
	}
	code := func(raw string) string {
		var b struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal([]byte(raw), &b)
		return b.Code
	}

	// 200: email viết hoa + khoảng trắng hai đầu vẫn khớp.
	status, raw := e.do(t, e.ownerTok, "GET", path(e.orgA.ID, "  LOOKUP-TARGET-"+suffix+"@40Study.TEST "), "")
	if status != 200 {
		t.Fatalf("200: got %d %s", status, raw)
	}
	var ok struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &ok); err != nil {
		t.Fatal(err)
	}
	if ok.Data["id"] != target.ID.String() || ok.Data["email"] != target.Email || ok.Data["user_name"] != target.UserName || ok.Data["full_name"] != *target.FullName {
		t.Errorf("data sai: %v", ok.Data)
	}
	if v, has := ok.Data["avatar_url"]; !has || v != nil {
		t.Errorf("avatar_url phải có mặt và null: %v", ok.Data)
	}
	if len(ok.Data) != 5 {
		t.Errorf("data chỉ được có 5 trường, có %v", ok.Data)
	}
	// Admin hệ thống cũng tra được (quyền qua system role).
	if status, raw := e.do(t, e.adminTok, "GET", path(e.orgB.ID, target.Email), ""); status != 200 {
		t.Errorf("admin: got %d %s", status, raw)
	}

	// 404: không có, bị khoá, đã xoá mềm, chỉ khớp một phần.
	for name, email := range map[string]string{
		"không có":     "nobody-" + suffix + "@40study.test",
		"bị khoá":      locked.Email,
		"đã xoá mềm":   gone.Email,
		"một phần":     "lookup-target-" + suffix + "@40study",
		"tiền tố rỗng": "%@40study.test",
	} {
		if status, raw := e.do(t, e.ownerTok, "GET", path(e.orgA.ID, email), ""); status != 404 && !(name == "một phần" && status == 400) {
			t.Errorf("404 %s: got %d %s", name, status, raw)
		} else if status == 404 && code(raw) != "USER_NOT_FOUND" {
			t.Errorf("404 %s: code=%q", name, code(raw))
		}
	}

	// 400: thiếu / sai định dạng.
	for _, email := range []string{"", "   ", "khong-phai-email", "Ten <" + target.Email + ">", "a b@40study.test"} {
		if status, raw := e.do(t, e.ownerTok, "GET", path(e.orgA.ID, email), ""); status != 400 || code(raw) != "INVALID_EMAIL" {
			t.Errorf("400 %q: got %d %s", email, status, raw)
		}
	}

	// 403: không có quyền; chủ tổ chức A tra trong tổ chức B (active_org lệch).
	if status, raw := e.do(t, e.studentTok, "GET", path(e.orgA.ID, target.Email), ""); status != 403 {
		t.Errorf("403 học viên: got %d %s", status, raw)
	}
	if status, raw := e.do(t, e.ownerTok, "GET", path(e.orgB.ID, target.Email), ""); status != 403 {
		t.Errorf("403 chủ A tra ở B: got %d %s", status, raw)
	}
}
