package service

// Lane L7 (Postgres thật, schema tạm): các việc còn lại sau review L2.
//
//  1. Tổ chức bị xoá MỀM: chủ cũ không còn chấm được lớp của tổ chức, không ai tạo thêm lớp vào đó.
//     Bỏ điều kiện deleted_at ở ActiveOrgMemberExists hoặc ở orgManagesClass thì test tương ứng ĐỎ.
//  2. Role tổ chức `inactive` không cấp quyền (PermissionChecker). Bỏ bộ lọc Status ở
//     resolveOrgRolePermissions thì test ĐỎ.
//  3. Ba khoảng trống mutation: `status = active` trong ActiveOrgMemberExists, và `GradedByName` trang phụ huynh
//     (ErrNotOrgMember -> 403 nằm ở handler/l7_class_error_status_test.go).
//
// MỘT fixture (một lần Migrate đầy đủ) cho cả nhóm; mỗi ca con tự dựng tổ chức/lớp riêng.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestL7_ClassFollowups_Postgres(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()
	classRepo := repository.NewClassRepository(e.db)

	newClassSvc := func(teachers ...model.User) *ClassService {
		set := map[uuid.UUID]bool{}
		for _, u := range teachers {
			set[u.ID] = true
		}
		return NewClassService(classRepo, repository.NewCourseRepository(e.db), s4TeacherRepo{teachers: set}, nil, nil)
	}
	newOrg := func(label string) model.Organization {
		o := model.Organization{Name: "L7 " + label + " " + uuid.NewString()[:8]}
		mustCreate(t, e.db, &o)
		return o
	}
	classIn := func(org model.Organization, instructor model.User) model.Class {
		course := e.course(instructor)
		c := model.Class{Name: "L7 class " + uuid.NewString()[:6], CourseID: &course.ID, Status: "active", OrganizationID: &org.ID}
		mustCreate(t, e.db, &c)
		mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: c.ID, Status: "active"})
		return c
	}

	// 1. Tổ chức bị xoá mềm (đường mặc định của DeleteOrganization: hardDelete=false). FK SET NULL chỉ chạy khi
	// xoá CỨNG, nên classes.organization_id vẫn giữ id: quyền phải được cắt ở chỗ kiểm quyền, không nhờ FK.
	t.Run("tổ chức bị xoá mềm: chủ cũ không chấm được, không tạo thêm lớp", func(t *testing.T) {
		orgC := newOrg("org C")
		ownerC, memberC := e.user("owner-c"), e.user("member-c")
		e.grant(ownerC, orgC, e.orgRole(orgC, "ORG_OWNER", "ORG_MEMBERS_MANAGE"))
		e.grant(memberC, orgC, e.orgRole(orgC, "THANH_VIEN"))
		class := classIn(orgC, e.instructor)
		svc := newClassSvc(memberC)

		// Trước khi xoá: cả hai đường đều mở (chứng tỏ test không đỏ vì lý do khác).
		if _, err := e.svc.CreateGrade(ctx, class.ID, ownerC.ID, e.gradeDTO()); err != nil {
			t.Fatalf("trước khi xoá, chủ tổ chức bị chặn nhầm: %v", err)
		}
		if _, err := svc.CreateClass(ctx, memberC.ID, false, dto.CreateClassDTO{Name: "trước khi xoá", OrganizationID: &orgC.ID}); err != nil {
			t.Fatalf("trước khi xoá, thành viên không tạo được lớp: %v", err)
		}

		if err := repository.NewOrganizationRepository(e.db).DeleteOrganization(ctx, orgC.ID, false); err != nil {
			t.Fatalf("xoá mềm tổ chức: %v", err)
		}
		var stored model.Class
		if err := e.db.First(&stored, "id = ?", class.ID).Error; err != nil || stored.OrganizationID == nil || *stored.OrganizationID != orgC.ID {
			t.Fatalf("lớp phải giữ organization_id sau xoá mềm (FK chỉ chạy khi xoá cứng): %+v err=%v", stored.OrganizationID, err)
		}

		if _, err := e.svc.CreateGrade(ctx, class.ID, ownerC.ID, e.gradeDTO()); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ cũ chấm lớp của tổ chức đã xoá: err=%v, muốn ErrClassNotFound", err)
		}
		if _, err := e.svc.GetGradesByClass(ctx, class.ID, ownerC.ID); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ cũ xem bảng điểm: err=%v, muốn ErrClassNotFound", err)
		}
		if _, err := svc.CreateClass(ctx, memberC.ID, false, dto.CreateClassDTO{Name: "sau khi xoá", OrganizationID: &orgC.ID}); !errors.Is(err, ErrNotOrgMember) {
			t.Errorf("thành viên tạo lớp vào tổ chức đã xoá: err=%v, muốn ErrNotOrgMember", err)
		}
		if _, err := svc.CreateClass(ctx, e.systemAdmin.ID, true, dto.CreateClassDTO{Name: "admin", OrganizationID: &orgC.ID}); err == nil {
			t.Error("admin tạo lớp vào tổ chức đã xoá phải lỗi")
		}
		// Việc xoá tổ chức không tước quyền của người chủ lớp theo dữ liệu lớp/khoá: instructor vẫn chấm được.
		if _, err := e.svc.CreateGrade(ctx, class.ID, e.instructor.ID, e.gradeDTO()); err != nil {
			t.Errorf("instructor chủ khoá bị chặn nhầm sau khi tổ chức bị xoá: %v", err)
		}
	})

	// 2. Role tổ chức `inactive` không cấp quyền, kể cả cho đường chấm điểm.
	t.Run("role tổ chức inactive không cấp quyền", func(t *testing.T) {
		owner := e.user("owner-inactive-role")
		role := e.orgRole(e.orgA, "ORG_OWNER_X", "ORG_MEMBERS_MANAGE")
		e.grant(owner, e.orgA, role)
		class := e.classInOrgA(t)

		if !e.has(owner, &e.orgA, "ORG_MEMBERS_MANAGE") {
			t.Fatal("trước khi vô hiệu hoá role, chủ tổ chức phải có ORG_MEMBERS_MANAGE")
		}
		if _, err := e.svc.CreateGrade(ctx, class.ID, owner.ID, e.gradeDTO()); err != nil {
			t.Fatalf("trước khi vô hiệu hoá role, chấm điểm bị chặn nhầm: %v", err)
		}

		if err := e.db.Model(&model.Role{}).Where("id = ?", role.ID).Update("status", "inactive").Error; err != nil {
			t.Fatal(err)
		}
		if e.has(owner, &e.orgA, "ORG_MEMBERS_MANAGE") {
			t.Error("role inactive vẫn cấp ORG_MEMBERS_MANAGE qua HasPermission")
		}
		if ok, err := e.checker.HasOrgRolePermission(ctx, owner.ID, e.orgA.ID, "ORG_MEMBERS_MANAGE"); err != nil || ok {
			t.Errorf("HasOrgRolePermission với role inactive: ok=%v err=%v, muốn false", ok, err)
		}
		if _, err := e.svc.CreateGrade(ctx, class.ID, owner.ID, e.gradeDTO()); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chấm điểm bằng role inactive: err=%v, muốn ErrClassNotFound", err)
		}

		// Người giữ thêm một role KHÁC còn active vẫn có quyền: chỉ role inactive bị bỏ qua, không phải cả user.
		e.grant(owner, e.orgA, e.orgRole(e.orgA, "ORG_OWNER_Y", "ORG_MEMBERS_MANAGE"))
		if !e.has(owner, &e.orgA, "ORG_MEMBERS_MANAGE") {
			t.Error("người còn role active khác phải giữ quyền")
		}
		if _, err := e.svc.CreateGrade(ctx, class.ID, owner.ID, e.gradeDTO()); err != nil {
			t.Errorf("chấm điểm bằng role active còn lại: %v", err)
		}
	})

	// 3a. `status = active` trong ActiveOrgMemberExists: thành viên bị gỡ role không còn là thành viên.
	t.Run("thành viên bị gỡ role không tạo được lớp vào tổ chức", func(t *testing.T) {
		if ok, err := classRepo.ActiveOrgMemberExists(ctx, e.ownerA.ID, e.orgA.ID); err != nil || !ok {
			t.Errorf("chủ tổ chức A đang active: ok=%v err=%v, muốn true", ok, err)
		}
		if ok, err := classRepo.ActiveOrgMemberExists(ctx, e.revokedOwnerA.ID, e.orgA.ID); err != nil || ok {
			t.Errorf("chủ tổ chức A đã bị gỡ role: ok=%v err=%v, muốn false", ok, err)
		}
		svc := newClassSvc(e.revokedOwnerA)
		if _, err := svc.CreateClass(ctx, e.revokedOwnerA.ID, false, dto.CreateClassDTO{Name: "x", OrganizationID: &e.orgA.ID}); !errors.Is(err, ErrNotOrgMember) {
			t.Errorf("thành viên bị gỡ role tạo lớp: err=%v, muốn ErrNotOrgMember", err)
		}
	})

	// 3c. Trang phụ huynh phải trả người chấm (graded_by + graded_by_name), kể cả khi người chấm là chủ tổ chức.
	t.Run("phụ huynh thấy người chấm điểm của con", func(t *testing.T) {
		// Học viên riêng: các ca con khác đã chấm điểm cho e.student.
		parent, child := e.user("parent-l7"), e.user("child-l7")
		mustCreate(t, e.db, &model.ParentStudentRelation{
			ParentUserID: parent.ID, StudentUserID: child.ID, Status: model.ParentStudentStatusActive, CanViewGrades: true,
		})
		class := e.classInOrgA(t)
		mustCreate(t, e.db, &model.StudentClass{StudentID: child.ID, ClassID: class.ID, Status: "active"})
		in := e.gradeDTO()
		in.StudentID = child.ID.String()
		if _, err := e.svc.CreateGrade(ctx, class.ID, e.ownerA.ID, in); err != nil {
			t.Fatal(err)
		}
		dash := NewParentDashboardService(repository.NewParentStudentRepository(e.db), repository.NewUserRepository(e.db),
			repository.NewEnrollmentRepository(e.db), repository.NewGradeRepository(e.db), nil, nil, nil)
		out, err := dash.GetChildGrades(ctx, parent.ID, child.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Grades) != 1 {
			t.Fatalf("có %d dòng điểm, muốn 1", len(out.Grades))
		}
		if g := out.Grades[0]; g.GradedBy != e.ownerA.ID.String() || g.GradedByName != e.ownerA.UserName {
			t.Errorf("graded_by=%q graded_by_name=%q, muốn %q / %q", g.GradedBy, g.GradedByName, e.ownerA.ID, e.ownerA.UserName)
		}
	})
}
