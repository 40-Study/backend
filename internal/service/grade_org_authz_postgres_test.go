package service

// Lane L2 (Postgres thật, schema tạm): chủ/quản trị tổ chức được chấm điểm lớp của tổ chức, có lưu người
// chấm (graded_by + graded_by_name). Test đi theo từng vai:
//   - giảng viên được gán vào lớp, instructor chủ khoá (nhóm vốn đã hoặc nay được chấm),
//   - chủ tổ chức A (ORG_MEMBERS_MANAGE trong org role đang active) của lớp thuộc tổ chức A: được chấm,
//   - thành viên tổ chức A không có quyền quản trị, chủ tổ chức B, chủ tổ chức A đã bị gỡ role,
//     chủ tổ chức A với lớp KHÔNG thuộc tổ chức A, người lạ: không xem được lớp -> 404,
//   - học viên trong lớp: xem được nhưng không chấm -> 403,
//   - admin hệ thống: được chấm.
// Bỏ nhánh orgManagesClass trong ensureClassGrade thì "chủ tổ chức A" ĐỎ; đổi 404 thành 403 (hoặc ngược lại)
// thì các vai không-xem-được / học viên ĐỎ; bỏ Omit(clause.Associations) ở UpdateGrade thì graded_by sau
// khi chủ tổ chức sửa điểm vẫn là giảng viên cũ và test ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/data"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type gradeOrgEnv struct {
	*s6OrgEnv
	svc                                                 *GradeService
	instructor, coTeacher, student, stranger            model.User
	ownerA, memberA, ownerB, revokedOwnerA, systemAdmin model.User
	class, foreignClass                                 model.Class
}

func newGradeOrgEnv(t *testing.T) *gradeOrgEnv {
	t.Helper()
	e := &gradeOrgEnv{s6OrgEnv: newS6OrgEnv(t)}
	for name, u := range map[string]*model.User{
		"instructor": &e.instructor, "co-teacher": &e.coTeacher, "student": &e.student, "stranger": &e.stranger,
		"owner-a": &e.ownerA, "member-a": &e.memberA, "owner-b": &e.ownerB, "revoked-owner-a": &e.revokedOwnerA,
		"system-admin": &e.systemAdmin,
	} {
		*u = e.user(name)
	}

	// Lớp thuộc tổ chức A vì instructor chủ khoá là thành viên active của A (vai không mang quyền nào).
	course := e.course(e.instructor)
	e.class = model.Class{Name: "L2 grade", CourseID: &course.ID, Status: "active"}
	if err := e.db.Create(&e.class).Error; err != nil {
		t.Fatal(err)
	}
	mustCreate(t, e.db, &model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: e.class.ID, Role: "primary"})
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: e.class.ID, Status: "active"})
	e.grant(e.instructor, e.orgA, e.orgRole(e.orgA, "GIANG_VIEN"))

	// Lớp không có nhân sự nào thuộc tổ chức A.
	foreignCourse := e.course(e.stranger)
	e.foreignClass = model.Class{Name: "L2 foreign", CourseID: &foreignCourse.ID, Status: "active"}
	mustCreate(t, e.db, &e.foreignClass)
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: e.foreignClass.ID, Status: "active"})

	e.grant(e.ownerA, e.orgA, e.orgRole(e.orgA, "ORG_OWNER", "ORG_MEMBERS_MANAGE", "ORG_ROLES_MANAGE"))
	e.grant(e.memberA, e.orgA, e.orgRole(e.orgA, "THANH_VIEN"))
	e.grant(e.ownerB, e.orgB, e.orgRole(e.orgB, "ORG_OWNER", "ORG_MEMBERS_MANAGE"))
	e.grant(e.revokedOwnerA, e.orgA, e.orgRole(e.orgA, "ORG_OWNER", "ORG_MEMBERS_MANAGE"))
	if err := e.db.Model(&model.UserOrganizationRole{}).Where("user_id = ?", e.revokedOwnerA.ID).
		Update("status", model.UserOrgRoleStatusInactive).Error; err != nil {
		t.Fatal(err)
	}

	sys := model.SystemRole{Name: "L2_SYSTEM_ADMIN", Status: "active"}
	mustCreate(t, e.db, &sys)
	mustCreate(t, e.db, &model.SystemRolePermission{SystemRoleID: sys.ID, PermissionID: e.permIDs["SYSTEM_SETTINGS_MANAGE"]})
	mustCreate(t, e.db, &model.UserSystemRole{UserID: e.systemAdmin.ID, SystemRoleID: sys.ID, Status: model.UserSystemRoleStatusActive})

	e.svc = NewGradeService(repository.NewGradeRepository(e.db), repository.NewClassRepository(e.db),
		repository.NewCourseRepository(e.db), e.checker, nil)
	return e
}

func mustCreate(t *testing.T, db *gorm.DB, v any) {
	t.Helper()
	if err := db.Create(v).Error; err != nil {
		t.Fatal(err)
	}
}

func (e *gradeOrgEnv) gradeDTO() dto.CreateGradeDTO {
	return dto.CreateGradeDTO{StudentID: e.student.ID.String(), GradeType: "assignment", Title: "Bài 1", Score: 8, MaxScore: 10}
}

func TestL2_Grade_QuyenChamDiemTheoVai(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		actor   model.User
		classID uuid.UUID
		want    error // nil = được chấm
	}{
		{"giảng viên được gán vào lớp", e.coTeacher, e.class.ID, nil},
		{"instructor chủ khoá", e.instructor, e.class.ID, nil},
		{"chủ tổ chức A, lớp thuộc A", e.ownerA, e.class.ID, nil},
		{"admin hệ thống", e.systemAdmin, e.class.ID, nil},
		{"thành viên tổ chức A không có quyền quản trị", e.memberA, e.class.ID, ErrClassNotFound},
		{"chủ tổ chức B", e.ownerB, e.class.ID, ErrClassNotFound},
		{"chủ tổ chức A đã bị gỡ role", e.revokedOwnerA, e.class.ID, ErrClassNotFound},
		{"chủ tổ chức A, lớp KHÔNG thuộc A", e.ownerA, e.foreignClass.ID, ErrClassNotFound},
		{"người lạ", e.stranger, e.class.ID, ErrClassNotFound},
		{"học viên trong lớp", e.student, e.class.ID, ErrNotClassTeacher},
		{"lớp không tồn tại", e.ownerA, uuid.New(), ErrClassNotFound},
	}
	for _, c := range cases {
		t.Run("chấm điểm: "+c.name, func(t *testing.T) {
			got, err := e.svc.CreateGrade(ctx, c.classID, c.actor.ID, e.gradeDTO())
			if c.want == nil {
				if err != nil {
					t.Fatalf("bị chặn nhầm: %v", err)
				}
				if got.GradedBy != c.actor.ID {
					t.Errorf("graded_by=%v, muốn %v", got.GradedBy, c.actor.ID)
				}
				if got.GradedByName != c.actor.UserName {
					t.Errorf("graded_by_name=%q, muốn %q", got.GradedByName, c.actor.UserName)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v, muốn %v", err, c.want)
			}
		})
		t.Run("xem bảng điểm: "+c.name, func(t *testing.T) {
			_, err := e.svc.GetGradesByClass(ctx, c.classID, c.actor.ID)
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v, muốn %v", err, c.want)
			}
		})
	}

	// Chỉ các lần chấm được phép để lại dòng điểm: 4 vai được chấm, đúng 4 dòng.
	var n int64
	e.db.Model(&model.Grade{}).Where("class_id = ?", e.class.ID).Count(&n)
	if n != 4 {
		t.Errorf("có %d dòng điểm, muốn 4 (chỉ người được phép chấm)", n)
	}
}

// Chủ tổ chức sửa điểm do giảng viên chấm: graded_by đổi sang chủ tổ chức (GORM từng ghi đè lại bằng id người
// chấm cũ vì Grader đã Preload), tên trong response và ở danh sách là người chấm mới. Học viên đọc thấy cùng tên.
func TestL2_Grade_ChuToChucSuaDiem_GhiNguoiCham(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()

	g, err := e.svc.CreateGrade(ctx, e.class.ID, e.coTeacher.ID, e.gradeDTO())
	if err != nil {
		t.Fatal(err)
	}
	if g.GradedBy != e.coTeacher.ID || g.GradedByName != e.coTeacher.UserName {
		t.Fatalf("giảng viên chấm: graded_by=%v tên=%q", g.GradedBy, g.GradedByName)
	}

	score := 9.5
	up, err := e.svc.UpdateGrade(ctx, g.ID, e.ownerA.ID, dto.UpdateGradeDTO{Score: &score})
	if err != nil {
		t.Fatalf("chủ tổ chức sửa điểm: %v", err)
	}
	if up.GradedBy != e.ownerA.ID || up.GradedByName != e.ownerA.UserName {
		t.Errorf("response sau sửa: graded_by=%v tên=%q, muốn chủ tổ chức", up.GradedBy, up.GradedByName)
	}
	var stored model.Grade
	if err := e.db.First(&stored, "id = ?", g.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.GradedBy != e.ownerA.ID {
		t.Errorf("DB graded_by=%v, muốn chủ tổ chức %v", stored.GradedBy, e.ownerA.ID)
	}

	book, err := e.svc.GetGradesByClass(ctx, e.class.ID, e.ownerA.ID)
	if err != nil || len(book.Students) != 1 || len(book.Students[0].Grades) != 1 {
		t.Fatalf("bảng điểm: %+v err=%v", book, err)
	}
	if name := book.Students[0].Grades[0].GradedByName; name != e.ownerA.UserName {
		t.Errorf("bảng điểm lớp: graded_by_name=%q, muốn %q", name, e.ownerA.UserName)
	}
	mine, err := e.svc.GetMyGrades(ctx, e.student.ID)
	if err != nil || len(mine) != 1 || mine[0].GradedByName != e.ownerA.UserName {
		t.Errorf("điểm của học viên: %+v err=%v, muốn graded_by_name=%q", mine, err, e.ownerA.UserName)
	}

	// Học viên không sửa/xoá được điểm của chính mình (403), người lạ nhận 404.
	if _, err := e.svc.UpdateGrade(ctx, g.ID, e.student.ID, dto.UpdateGradeDTO{Score: &score}); !errors.Is(err, ErrNotClassTeacher) {
		t.Errorf("học viên sửa điểm: err=%v, muốn ErrNotClassTeacher", err)
	}
	if err := e.svc.DeleteGrade(ctx, g.ID, e.stranger.ID); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("người lạ xoá điểm: err=%v, muốn ErrClassNotFound", err)
	}
	if err := e.svc.DeleteGrade(ctx, g.ID, e.ownerA.ID); err != nil {
		t.Errorf("chủ tổ chức xoá điểm: %v", err)
	}
}

// Chấm hàng loạt: cả lô theo cùng luật quyền (chủ tổ chức được, người ngoài/học viên bị chặn trước khi ghi).
func TestL2_Grade_ChamHangLoat(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()
	req := dto.BulkCreateGradesDTO{Grades: []dto.CreateGradeDTO{e.gradeDTO(), e.gradeDTO()}}

	if out, err := e.svc.BulkCreateGrades(ctx, e.class.ID, e.ownerA.ID, req); err != nil || len(out) != 2 || out[0].GradedBy != e.ownerA.ID {
		t.Fatalf("chủ tổ chức chấm lô: %v err=%v", out, err)
	}
	if _, err := e.svc.BulkCreateGrades(ctx, e.class.ID, e.ownerB.ID, req); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("chủ tổ chức B chấm lô: err=%v, muốn ErrClassNotFound", err)
	}
	if _, err := e.svc.BulkCreateGrades(ctx, e.class.ID, e.student.ID, req); !errors.Is(err, ErrNotClassTeacher) {
		t.Errorf("học viên chấm lô: err=%v, muốn ErrNotClassTeacher", err)
	}
	var n int64
	e.db.Model(&model.Grade{}).Where("class_id = ?", e.class.ID).Count(&n)
	if n != 2 {
		t.Errorf("có %d dòng điểm, muốn 2 (lô bị từ chối không ghi gì)", n)
	}
}

// Không có Authorizer (nil): chỉ người quản lý lớp theo dữ liệu lớp/khoá chấm được; chủ tổ chức không tự có quyền.
func TestL2_Grade_KhongCoAuthorizer_FailClosed(t *testing.T) {
	e := newGradeOrgEnv(t)
	svc := NewGradeService(repository.NewGradeRepository(e.db), repository.NewClassRepository(e.db),
		repository.NewCourseRepository(e.db), nil, nil)
	if _, err := svc.CreateGrade(context.Background(), e.class.ID, e.ownerA.ID, e.gradeDTO()); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("chủ tổ chức khi không có authorizer: err=%v, muốn ErrClassNotFound", err)
	}
	if _, err := svc.CreateGrade(context.Background(), e.class.ID, e.coTeacher.ID, e.gradeDTO()); err != nil {
		t.Errorf("giảng viên lớp bị chặn nhầm: %v", err)
	}
}

// Quyền dùng để xác định "chủ/quản trị tổ chức" phải thuộc phạm vi tổ chức, nếu không bộ lọc S6 ở
// PermissionChecker sẽ loại nó khỏi org role và chủ tổ chức mất quyền chấm một cách im lặng.
func TestL2_OrgClassManagePermission_ThuocPhamViToChuc(t *testing.T) {
	if !data.IsOrgPermission(orgClassManagePermission) {
		t.Fatalf("%s không nằm trong data/permissions/org_owner_permissions.json", orgClassManagePermission)
	}
}
