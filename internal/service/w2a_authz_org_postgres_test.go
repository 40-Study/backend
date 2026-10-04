package service

// Lane W2-A (Postgres thật, schema tạm), theo quyết định của chủ dự án 04/10 và luật "không xem được thì 404,
// xem được mà không có quyền thì 403":
//   - lớp của tổ chức chỉ nhận giảng viên đã là thành viên active của tổ chức (admin hệ thống không bị giới hạn);
//   - xoá vĩnh viễn lớp chỉ admin hệ thống, chủ/quản trị tổ chức chỉ lưu trữ (status=archived);
//   - route GHI vào lớp (sửa, ghi danh, gỡ học viên, gán/gỡ giảng viên, điểm danh) trả 404 cho người không xem được lớp.
// Bỏ requireOrgMemberTeacher thì ca "gán người ngoài" ĐỎ; bỏ nhánh kiểm xoá ở DeleteClass thì ca chủ tổ chức/hard
// delete ĐỎ; đổi requireClassTeacherOrAdmin về ensureClassManage thuần thì bảng ghi 404 ĐỎ.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestW2A_GanGiangVien_ChiThanhVienToChuc_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	class := e.classInOrgA(t)
	assign := func(actor model.User, isAdmin bool, teacher model.User) error {
		_, err := e.classes.AssignTeacherToClass(ctx, class.ID, actor.ID, isAdmin, dto.AssignTeacherDTO{TeacherID: teacher.ID})
		return err
	}
	rows := func(teacher model.User) int64 {
		var n int64
		e.db.Model(&model.TeacherClass{}).Where("class_id = ? AND teacher_id = ?", class.ID, teacher.ID).Count(&n)
		return n
	}

	if err := assign(e.ownerA, false, e.newTeacher); !errors.Is(err, ErrTeacherNotOrgMember) {
		t.Fatalf("chủ A gán giảng viên ngoài tổ chức: err=%v, muốn ErrTeacherNotOrgMember", err)
	}
	if rows(e.newTeacher) != 0 {
		t.Error("giảng viên ngoài tổ chức vẫn bị ghi vào lớp")
	}
	// Gán hàng loạt: một người ngoài làm hỏng cả lô, không ghi dở.
	batch := dto.AssignTeachersDTO{Teachers: []dto.AssignTeacherDTO{{TeacherID: e.instructor.ID}, {TeacherID: e.newTeacher.ID}}}
	if _, err := e.classes.AssignTeachersToClass(ctx, class.ID, e.ownerA.ID, false, batch); !errors.Is(err, ErrTeacherNotOrgMember) {
		t.Fatalf("gán lô có người ngoài: err=%v, muốn ErrTeacherNotOrgMember", err)
	}
	if rows(e.instructor) != 0 {
		t.Error("lô bị từ chối nhưng vẫn ghi một phần")
	}

	// Thành viên đã bị gỡ (inactive) cũng không được gán; active thì được.
	role := e.orgRole(e.orgA, "GIANG_VIEN")
	e.grant(e.newTeacher, e.orgA, role)
	e.db.Model(&model.UserOrganizationRole{}).Where("user_id = ?", e.newTeacher.ID).Update("status", model.UserOrgRoleStatusInactive)
	if err := assign(e.ownerA, false, e.newTeacher); !errors.Is(err, ErrTeacherNotOrgMember) {
		t.Errorf("thành viên inactive: err=%v, muốn ErrTeacherNotOrgMember", err)
	}
	e.db.Model(&model.UserOrganizationRole{}).Where("user_id = ?", e.newTeacher.ID).Update("status", model.UserOrgRoleStatusActive)
	if err := assign(e.ownerA, false, e.newTeacher); err != nil {
		t.Errorf("thành viên active bị chặn nhầm: %v", err)
	}

	// Admin hệ thống không bị giới hạn; lớp cá nhân (không tổ chức) không bị giới hạn.
	e.db.Model(&model.UserOrganizationRole{}).Where("user_id = ?", e.newTeacher.ID).Update("status", model.UserOrgRoleStatusInactive)
	other := e.classInOrgA(t)
	if _, err := e.classes.AssignTeacherToClass(ctx, other.ID, e.systemAdmin.ID, true, dto.AssignTeacherDTO{TeacherID: e.newTeacher.ID}); err != nil {
		t.Errorf("admin hệ thống gán người ngoài tổ chức: %v", err)
	}
	personal := e.personalClassOfOrgMember(t)
	if _, err := e.classes.AssignTeacherToClass(ctx, personal.ID, e.instructor.ID, false, dto.AssignTeacherDTO{TeacherID: e.coTeacher.ID}); err != nil {
		t.Errorf("lớp cá nhân bị giới hạn nhầm: %v", err)
	}
	// Ghi danh học viên vẫn chọn bất kỳ học viên (B2C), kể cả người không thuộc tổ chức.
	if _, err := e.classes.EnrollStudentToClass(ctx, class.ID, e.ownerA.ID, false, dto.EnrollStudentDTO{StudentID: e.newStudent.ID}); err != nil {
		t.Errorf("ghi danh học viên ngoài tổ chức bị chặn: %v", err)
	}
}

func TestW2A_XoaLop_HardDeleteChiAdmin_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	exists := func(c model.Class) bool {
		var n int64
		e.db.Model(&model.Class{}).Where("id = ?", c.ID).Count(&n)
		return n > 0
	}

	inA := e.classInOrgA(t)
	for _, hard := range []bool{false, true} {
		if err := e.classes.DeleteClass(ctx, inA.ID, e.ownerA.ID, false, hard); !errors.Is(err, ErrClassDeleteAdminOnly) {
			t.Errorf("chủ tổ chức xoá (hard=%v): err=%v, muốn ErrClassDeleteAdminOnly", hard, err)
		}
	}
	if !exists(inA) {
		t.Fatal("chủ tổ chức xoá được lớp")
	}
	archived := "archived"
	if got, err := e.classes.UpdateClass(ctx, inA.ID, e.ownerA.ID, false, dto.UpdateClassDTO{Status: &archived}); err != nil || got.Status != "archived" {
		t.Errorf("chủ tổ chức lưu trữ lớp: %+v err=%v", got, err)
	}

	// Người xoá vĩnh viễn không phải admin (giảng viên chủ khoá): từ chối; xoá mềm vẫn được như cũ.
	if err := e.classes.DeleteClass(ctx, inA.ID, e.instructor.ID, false, true); !errors.Is(err, ErrClassDeleteAdminOnly) {
		t.Errorf("giảng viên hard delete: err=%v, muốn ErrClassDeleteAdminOnly", err)
	}
	// Người không xem được lớp vẫn nhận 404 dù xin hard delete (không dò được lớp); học viên nhận 403.
	if err := e.classes.DeleteClass(ctx, inA.ID, e.ownerB.ID, false, true); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("chủ B hard delete lớp của A: err=%v, muốn ErrClassNotFound", err)
	}
	if err := e.classes.DeleteClass(ctx, inA.ID, e.student.ID, false, false); !errors.Is(err, ErrNotClassTeacher) {
		t.Errorf("học viên xoá lớp: err=%v, muốn ErrNotClassTeacher", err)
	}
	if err := e.classes.DeleteClass(ctx, inA.ID, e.instructor.ID, false, false); err != nil {
		t.Errorf("giảng viên chủ khoá xoá mềm bị chặn nhầm: %v", err)
	}
	if exists(inA) {
		t.Error("xoá mềm không ẩn lớp")
	}

	hardClass := e.classInOrgA(t)
	if err := e.classes.DeleteClass(ctx, hardClass.ID, e.systemAdmin.ID, true, true); err != nil {
		t.Fatalf("admin hệ thống hard delete: %v", err)
	}
	var n int64
	e.db.Unscoped().Model(&model.Class{}).Where("id = ?", hardClass.ID).Count(&n)
	if n != 0 {
		t.Error("hard delete của admin không xoá hàng khỏi DB")
	}
}

// Bảng ghi: mọi thao tác GHI vào lớp, mọi vai. Người không xem được lớp -> ErrClassNotFound (404) và KHÔNG đổi dữ liệu;
// học viên trong lớp (xem được) -> 403; chủ tổ chức A và giảng viên chủ khoá -> làm được.
func TestW2A_GhiVaoLop_KhongXemDuoc404_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	class := e.classInOrgA(t)
	active := "active"
	ops := map[string]func(actor model.User) error{
		"sửa lớp": func(a model.User) error {
			_, err := e.classes.UpdateClass(ctx, class.ID, a.ID, false, dto.UpdateClassDTO{Status: &active})
			return err
		},
		"ghi danh": func(a model.User) error {
			_, err := e.classes.EnrollStudentToClass(ctx, class.ID, a.ID, false, dto.EnrollStudentDTO{StudentID: e.newStudent.ID})
			return err
		},
		"gỡ học viên": func(a model.User) error {
			return e.classes.RemoveStudentFromClass(ctx, class.ID, e.student.ID, a.ID, false)
		},
		"tìm ứng viên": func(a model.User) error {
			_, err := e.classes.SearchEnrollableStudents(ctx, class.ID, a.ID, false, "")
			return err
		},
		"điểm danh": func(a model.User) error {
			req := dto.BulkCreateAttendanceDTO{Date: "2026-10-06", Attendances: []dto.AttendanceEntryDTO{{StudentID: e.student.ID, Status: "present"}}}
			_, err := e.attendance.MarkAttendance(ctx, class.ID, a.ID, false, req)
			return err
		},
	}
	outsiders := map[string]model.User{"chủ tổ chức B": e.ownerB, "thành viên A không quản trị": e.memberA, "chủ A đã bị gỡ role": e.revokedOwnerA}
	for op, run := range ops {
		for name, u := range outsiders {
			if err := run(u); !errors.Is(err, ErrClassNotFound) {
				t.Errorf("%s / %s: err=%v, muốn ErrClassNotFound (404)", op, name, err)
			}
		}
		if err := run(e.student); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("%s / học viên trong lớp: err=%v, muốn ErrNotClassTeacher (403)", op, err)
		}
	}
	// Không thao tác nào của người ngoài để lại dấu vết.
	var stored model.Class
	e.db.First(&stored, "id = ?", class.ID)
	var enrolled int64
	e.db.Model(&model.StudentClass{}).Where("class_id = ? AND student_id = ?", class.ID, e.newStudent.ID).Count(&enrolled)
	var attendances int64
	e.db.Model(&model.Attendance{}).Where("class_id = ?", class.ID).Count(&attendances)
	if stored.Status != "active" || enrolled != 0 || attendances != 0 {
		t.Errorf("thao tác bị từ chối vẫn đổi dữ liệu: status=%q enrolled=%d attendances=%d", stored.Status, enrolled, attendances)
	}
	// Lớp không tồn tại: cùng 404.
	if _, err := e.classes.EnrollStudentToClass(ctx, uuid.New(), e.ownerA.ID, false, dto.EnrollStudentDTO{StudentID: e.newStudent.ID}); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("ghi danh vào lớp không tồn tại: err=%v, muốn ErrClassNotFound", err)
	}
	// Chiều dương: người có quyền vẫn làm được.
	for _, ok := range []model.User{e.ownerA, e.instructor} {
		if err := ops["ghi danh"](ok); err != nil && err.Error() != "student is already enrolled in this class" {
			t.Errorf("ghi danh bởi người có quyền: %v", err)
		}
	}
}

// Ô chọn giảng viên cùng luật với việc gán: lớp của tổ chức chỉ liệt kê thành viên active của tổ chức, admin hệ thống
// thấy mọi giảng viên, người không phải chủ lớp không dùng được (404 nếu không xem được, 403 nếu xem được).
func TestW2A_OChonGiangVien_ChiThanhVienToChuc_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	teacherRole := model.SystemRole{Name: "TEACHER", Status: "active"}
	mustCreate(t, e.db, &teacherRole)
	for _, u := range []model.User{e.instructor, e.coTeacher, e.newTeacher} {
		mustCreate(t, e.db, &model.UserSystemRole{UserID: u.ID, SystemRoleID: teacherRole.ID, Status: model.UserSystemRoleStatusActive})
	}
	e.grant(e.newTeacher, e.orgA, e.orgRole(e.orgA, "GIANG_VIEN"))
	svc := NewClassService(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db),
		repository.NewTeacherRepository(e.db), repository.NewStudentRepository(e.db), nil).WithAuthorizer(e.checker)
	class := e.classInOrgA(t)

	ids := func(actor model.User, isAdmin bool, keyword string) map[uuid.UUID]bool {
		got, err := svc.SearchAssignableTeachers(ctx, class.ID, actor.ID, isAdmin, keyword)
		if err != nil {
			t.Fatalf("SearchAssignableTeachers: %v", err)
		}
		out := map[uuid.UUID]bool{}
		for _, g := range got {
			out[g.ID] = true
		}
		return out
	}
	byOwner := ids(e.ownerA, false, "")
	if !byOwner[e.instructor.ID] || !byOwner[e.newTeacher.ID] || byOwner[e.coTeacher.ID] {
		t.Errorf("chủ tổ chức thấy %v, muốn instructor + newTeacher (thành viên) và KHÔNG có coTeacher (ngoài tổ chức)", byOwner)
	}
	if byName := ids(e.ownerA, false, e.newTeacher.UserName[:12]); !byName[e.newTeacher.ID] || len(byName) != 1 {
		t.Errorf("lọc theo tên: %v, muốn đúng newTeacher", byName)
	}
	if all := ids(e.systemAdmin, true, ""); !all[e.coTeacher.ID] {
		t.Errorf("admin hệ thống phải thấy cả giảng viên ngoài tổ chức: %v", all)
	}
	// Không phải chủ lớp: học viên trong lớp xem được lớp nên 403; người ngoài 404.
	if _, err := svc.SearchAssignableTeachers(ctx, class.ID, e.student.ID, false, ""); !errors.Is(err, ErrNotClassOwner) {
		t.Errorf("học viên: err=%v, muốn ErrNotClassOwner", err)
	}
	if _, err := svc.SearchAssignableTeachers(ctx, class.ID, e.ownerB.ID, false, ""); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("chủ tổ chức B: err=%v, muốn ErrClassNotFound", err)
	}
}

// ROLE_IN_USE không còn race: gán role (giữ FOR SHARE trên dòng role) và xoá role (FOR UPDATE) loại trừ lẫn nhau, nên
// không thể có bản ghi gán "active" trỏ vào role đã xoá. Mô phỏng một lượt gán đang dở: transaction giữ khoá, xoá phải
// CHỜ, và khi gán commit xong thì xoá thấy người giữ và không xoá gì.
func TestW2A_XoaRoleDangGan_KhongRace_Postgres(t *testing.T) {
	e := newS6OrgEnv(t)
	roleRepo := repository.NewRoleRepository(e.db)
	role := e.orgRole(e.orgA, "TRO_GIANG")
	holder := e.user("holder")

	tx := e.db.Begin()
	if err := repository.LockRolesForShare(tx, []uuid.UUID{role.ID}); err != nil {
		t.Fatal(err)
	}
	var finished atomic.Bool
	type result struct {
		assigned int64
		err      error
	}
	done := make(chan result, 1)
	go func() {
		n, err := roleRepo.DeleteRoleIfUnused(context.Background(), role.ID, false)
		finished.Store(true)
		done <- result{n, err}
	}()

	time.Sleep(500 * time.Millisecond)
	if finished.Load() {
		tx.Rollback()
		t.Fatal("xoá role không chờ lượt gán đang dở: còn race giữa đếm và xoá")
	}
	if err := tx.Create(&model.UserOrganizationRole{UserID: holder.ID, RoleID: role.ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}).Error; err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-done:
		if r.err != nil || r.assigned != 1 {
			t.Fatalf("xoá sau khi gán: assigned=%d err=%v, muốn assigned=1 và không lỗi", r.assigned, r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("xoá role treo")
	}
	var alive int64
	e.db.Model(&model.Role{}).Where("id = ?", role.ID).Count(&alive)
	if alive != 1 {
		t.Error("role còn người giữ nhưng đã bị xoá")
	}

	// Chiều ngược: role đã xoá thì gán vào đó bị từ chối, không tạo bản ghi treo.
	free := e.orgRole(e.orgA, "RONG")
	if _, err := roleRepo.DeleteRoleIfUnused(context.Background(), free.ID, false); err != nil {
		t.Fatal(err)
	}
	tx2 := e.db.Begin()
	defer tx2.Rollback()
	if err := repository.LockRolesForShare(tx2, []uuid.UUID{free.ID}); !errors.Is(err, repository.ErrRoleGone) {
		t.Errorf("gán vào role đã xoá: err=%v, muốn ErrRoleGone", err)
	}
}
