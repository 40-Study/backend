package service

// QA vòng 2 (G1 — N10/N-12): các mapper admin từng format bằng layout "2006-01-02T15:04:05Z",
// chữ Z là literal nên giờ local đọc từ DB (+07:00) bị gắn nhãn UTC -> client hiển thị lệch +7h.
// Test đưa mốc đã biết (10:00 giờ VN) qua TỪNG mapper đã sửa và đòi chuỗi trả về parse ra đúng
// instant đó. Bản cũ trả "…T10:00:00Z" (= 17:00 giờ VN) nên mọi case đều ĐỎ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func adminTestKnownVNTime(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		loc = time.FixedZone("ICT", 7*60*60)
	}
	return time.Date(2026, 9, 28, 10, 0, 0, 0, loc)
}

func assertSameInstant(t *testing.T, name, got string, want time.Time) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("%s: %q không phải RFC3339: %v", name, got, err)
	}
	if !parsed.Equal(want) {
		t.Fatalf("%s: %q = %s UTC, muốn %s UTC (lệch %v) — giờ local bị gắn nhầm chữ Z",
			name, got, parsed.UTC().Format(time.RFC3339), want.UTC().Format(time.RFC3339), parsed.Sub(want))
	}
}

func TestAdminMappers_TraDungMocGio(t *testing.T) {
	known := adminTestKnownVNTime(t)
	base := model.BaseModel{ID: uuid.New(), CreatedAt: known, UpdatedAt: known}

	org := toOrganizationResponseDTO(&model.Organization{BaseModel: base, Name: "Org"})
	assertSameInstant(t, "organization.created_at", org.CreatedAt, known)
	assertSameInstant(t, "organization.updated_at", org.UpdatedAt, known)

	orgDetail := toOrganizationDetailResponseDTO(&model.Organization{BaseModel: base, Name: "Org"})
	assertSameInstant(t, "organization_detail.created_at", orgDetail.CreatedAt, known)

	sr := toSystemRoleResponseDTO(&model.SystemRole{BaseModel: base, Name: "STUDENT"})
	assertSameInstant(t, "system_role.created_at", sr.CreatedAt, known)

	perm := toPermissionResponseDTO(&model.Permission{BaseModel: base, Name: "X"})
	assertSameInstant(t, "permission.updated_at", perm.UpdatedAt, known)

	role := toRoleResponseDTO(&model.Role{BaseModel: base, Name: "ORG_OWNER"})
	assertSameInstant(t, "role.created_at", role.CreatedAt, known)

	reviewed := known
	tp := toTeacherProfileResponseDTO(&model.TeacherProfile{BaseModel: base, ReviewedAt: &reviewed})
	assertSameInstant(t, "teacher_profile.created_at", tp.CreatedAt, known)
	if tp.ReviewedAt == nil {
		t.Fatal("teacher_profile.reviewed_at = nil, muốn có giá trị")
	}
	assertSameInstant(t, "teacher_profile.reviewed_at", *tp.ReviewedAt, known)

	pub := toPublicTeacherProfileDTO(&model.TeacherProfile{BaseModel: base})
	assertSameInstant(t, "public_teacher_profile.updated_at", pub.UpdatedAt, known)

	att := toAttendanceResponseDTO(&model.Attendance{CreatedAt: known, Date: known})
	assertSameInstant(t, "attendance.created_at", att.CreatedAt, known)

	revoked := known
	uor := toUserOrgRoleResponseDTO(&model.UserOrganizationRole{GrantedAt: known, CreatedAt: known, UpdatedAt: known, RevokedAt: &revoked})
	assertSameInstant(t, "user_org_role.granted_at", uor.GrantedAt, known)
	if uor.RevokedAt == nil {
		t.Fatal("user_org_role.revoked_at = nil, muốn có giá trị")
	}
	assertSameInstant(t, "user_org_role.revoked_at", *uor.RevokedAt, known)
}
