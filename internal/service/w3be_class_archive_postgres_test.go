package service

// Lane W3-BE (Postgres thật, schema tạm), quyết định 04/10: lớp đã lưu trữ (classes.status = "archived") CHỈ ĐỌC.
//   - mọi thao tác GHI (ghi danh/gỡ học viên, gán/gỡ giảng viên, lịch/buổi học, điểm danh, bài tập, chấm điểm) trả
//     ErrClassArchived (handler: 409 CLASS_ARCHIVED); thao tác ĐỌC vẫn chạy; mở lại lớp thì ghi lại được;
//   - thứ tự kiểm: quyền (404/403) TRƯỚC, lưu trữ (409) SAU, nên người ngoài không dò được lớp nào đã lưu trữ.
// Bỏ ensureClassWritable ở một service thì dòng thao tác tương ứng ĐỎ; kiểm lưu trữ trước quyền thì ca người ngoài ĐỎ;
// chặn cả UpdateClass mở lại thì ca "mở lại" ĐỎ.
//
// GetAllVisible (SQL) song song với ensureClassView/orgManagesClass (Go): test hợp đồng so hai bên theo từng vai.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type w3Op struct {
	name string
	// permissionless: thao tác không nhận người gọi (bài tập: quyền kiểm ở handler), bỏ qua vòng kiểm thứ tự quyền.
	permissionless bool
	run            func(actor model.User) error
}

func TestW3BE_LopLuuTru_ChiDoc_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	class := e.classInOrgA(t)
	grades := e.newGradeService(e.checker)
	classRepo, courseRepo := repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db)
	assignments := NewAssignmentService(repository.NewAssignmentRepository(e.db), repository.NewTestCaseRepository(e.db),
		repository.NewSubmissionRepository(e.db), nil, nil)
	assignments.SetClassGate(NewClassOrgAccess(classRepo, courseRepo, e.checker))

	// Dữ liệu có sẵn TRƯỚC khi lưu trữ: một lịch, một buổi, một điểm, một bài tập.
	sch := model.ClassSchedule{ClassID: class.ID, DayOfWeek: 3, StartTime: "08:00", EndTime: "09:00", IsActive: true, EffectiveFrom: time.Now()}
	mustCreate(t, e.db, &sch)
	ses := model.ClassSession{ClassID: class.ID, SessionNumber: 1, Date: time.Date(2031, 6, 2, 0, 0, 0, 0, time.UTC), StartTime: "08:00", EndTime: "09:00", Status: model.SessionScheduled}
	mustCreate(t, e.db, &ses)
	grade, err := grades.CreateGrade(ctx, class.ID, e.ownerA.ID, e.gradeDTO())
	if err != nil {
		t.Fatalf("tạo điểm trước khi lưu trữ: %v", err)
	}
	classID := class.ID.String()
	assignment, err := assignments.Create(ctx, e.systemAdmin.ID, true, dto.CreateAssignmentDTO{
		ClassID: classID, Title: "Bài trước lưu trữ", Description: "d", Difficulty: "easy", Language: []string{"go"}})
	if err != nil {
		t.Fatalf("tạo bài tập trước khi lưu trữ: %v", err)
	}

	name, note, topic := "Tên mới", "ghi chú", "chủ đề"
	// Thứ tự cố ý: thao tác phá dữ liệu (gỡ học viên/giảng viên, xoá điểm) đứng CUỐI để các ca trước còn dữ liệu.
	ops := []w3Op{
		{"sửa lớp", false, func(a model.User) error {
			_, err := e.classes.UpdateClass(ctx, class.ID, a.ID, false, dto.UpdateClassDTO{Name: &name})
			return err
		}},
		{"ghi danh", false, func(a model.User) error {
			_, err := e.classes.EnrollStudentToClass(ctx, class.ID, a.ID, false, dto.EnrollStudentDTO{StudentID: e.newStudent.ID})
			return err
		}},
		{"gán giảng viên", false, func(a model.User) error {
			_, err := e.classes.AssignTeacherToClass(ctx, class.ID, a.ID, false, dto.AssignTeacherDTO{TeacherID: e.newTeacher.ID})
			return err
		}},
		{"gán giảng viên (lô)", false, func(a model.User) error {
			_, err := e.classes.AssignTeachersToClass(ctx, class.ID, a.ID, false, dto.AssignTeachersDTO{Teachers: []dto.AssignTeacherDTO{{TeacherID: e.newTeacher.ID}}})
			return err
		}},
		{"tạo lịch", false, func(a model.User) error {
			_, err := e.schedule.CreateSchedule(ctx, class.ID, a.ID, false, dto.CreateClassScheduleDTO{DayOfWeek: 2, StartTime: "10:00", EndTime: "11:00", EffectiveFrom: "2031-01-01"})
			return err
		}},
		{"sửa lịch", false, func(a model.User) error {
			room := "P1"
			_, err := e.schedule.UpdateSchedule(ctx, sch.ID, a.ID, false, dto.UpdateClassScheduleDTO{Room: &room})
			return err
		}},
		{"tạo buổi học", false, func(a model.User) error {
			_, err := e.schedule.CreateSession(ctx, class.ID, a.ID, false, dto.CreateClassSessionDTO{Date: "2031-06-09", StartTime: "10:00", EndTime: "11:00"})
			return err
		}},
		{"sửa buổi học", false, func(a model.User) error {
			_, err := e.schedule.UpdateSession(ctx, ses.ID, a.ID, false, dto.UpdateClassSessionDTO{Topic: &topic})
			return err
		}},
		{"sinh buổi", false, func(a model.User) error {
			_, err := e.schedule.GenerateSessions(ctx, class.ID, a.ID, false, dto.GenerateSessionsDTO{StartDate: "2032-01-05", EndDate: "2032-01-12"})
			return err
		}},
		{"điểm danh lớp", false, func(a model.User) error {
			_, err := e.attendance.MarkAttendance(ctx, class.ID, a.ID, false, dto.BulkCreateAttendanceDTO{
				Date: "2031-06-02", Attendances: []dto.AttendanceEntryDTO{{StudentID: e.student.ID, Status: "present"}}})
			return err
		}},
		{"điểm danh buổi", false, func(a model.User) error {
			_, err := e.schedule.MarkAttendance(ctx, ses.ID, dto.MarkAttendanceDTO{StudentID: e.student.ID.String(), Status: "present"}, a.ID, false)
			return err
		}},
		{"tạo cột điểm", false, func(a model.User) error {
			_, err := grades.CreateGradeColumn(ctx, class.ID, a.ID, dto.CreateGradeColumnDTO{Name: "Cột " + uuid.NewString()[:4], GradeType: "quiz", Weight: 1})
			return err
		}},
		{"chấm điểm mới", false, func(a model.User) error {
			_, err := grades.CreateGrade(ctx, class.ID, a.ID, e.gradeDTO())
			return err
		}},
		{"sửa điểm", false, func(a model.User) error {
			_, err := grades.UpdateGrade(ctx, grade.ID, a.ID, dto.UpdateGradeDTO{Feedback: &note})
			return err
		}},
		{"tính điểm tổng kết", false, func(a model.User) error {
			_, err := grades.CalculateFinalGrades(ctx, class.ID, a.ID)
			return err
		}},
		{"tạo bài tập", true, func(a model.User) error {
			_, err := assignments.Create(ctx, e.systemAdmin.ID, true, dto.CreateAssignmentDTO{
				ClassID: classID, Title: "Bài mới", Description: "d", Difficulty: "easy", Language: []string{"go"}})
			return err
		}},
		{"sửa bài tập", true, func(a model.User) error {
			title := "Bài đã sửa"
			_, err := assignments.Update(ctx, assignment.ID, dto.UpdateAssignmentDTO{Title: &title})
			return err
		}},
		{"thêm test case", true, func(a model.User) error {
			_, err := assignments.AddTestCase(ctx, assignment.ID, dto.CreateTestCaseDTO{Input: "1", ExpectedOutput: "1"})
			return err
		}},
		{"gỡ giảng viên", false, func(a model.User) error {
			return e.classes.RemoveTeacherFromClass(ctx, class.ID, e.coTeacher.ID, a.ID, false)
		}},
		{"xoá điểm", false, func(a model.User) error {
			return grades.DeleteGrade(ctx, grade.ID, a.ID)
		}},
		{"gỡ học viên", false, func(a model.User) error {
			return e.classes.RemoveStudentFromClass(ctx, class.ID, e.student.ID, a.ID, false)
		}},
		{"xoá bài tập", true, func(a model.User) error {
			return assignments.Delete(ctx, assignment.ID)
		}},
	}
	snapshot := func() [5]int64 {
		var s [5]int64
		e.db.Model(&model.StudentClass{}).Where("class_id = ?", class.ID).Count(&s[0])
		e.db.Model(&model.TeacherClass{}).Where("class_id = ?", class.ID).Count(&s[1])
		e.db.Model(&model.ClassSession{}).Where("class_id = ?", class.ID).Count(&s[2])
		e.db.Model(&model.Grade{}).Where("class_id = ?", class.ID).Count(&s[3])
		e.db.Model(&model.Assignment{}).Where("class_id = ?", class.ID).Count(&s[4])
		return s
	}

	// Chủ tổ chức A lưu trữ lớp (thao tác này phải đi được).
	archived := "archived"
	if got, err := e.classes.UpdateClass(ctx, class.ID, e.ownerA.ID, false, dto.UpdateClassDTO{Status: &archived}); err != nil || got.Status != "archived" {
		t.Fatalf("lưu trữ lớp: %+v err=%v", got, err)
	}
	before := snapshot()

	t.Run("đang lưu trữ: mọi thao tác ghi bị ErrClassArchived và không đổi dữ liệu", func(t *testing.T) {
		for _, op := range ops {
			if err := op.run(e.ownerA); !errors.Is(err, ErrClassArchived) {
				t.Errorf("%s: err=%v, muốn ErrClassArchived", op.name, err)
			}
		}
		// Sửa thông tin khác khi vẫn lưu trữ, hoặc "lưu trữ lại", cũng bị chặn.
		if _, err := e.classes.UpdateClass(ctx, class.ID, e.ownerA.ID, false, dto.UpdateClassDTO{Status: &archived}); !errors.Is(err, ErrClassArchived) {
			t.Errorf("lưu trữ lại: err=%v, muốn ErrClassArchived", err)
		}
		if after := snapshot(); after != before {
			t.Errorf("dữ liệu đổi dù lớp lưu trữ: trước %v sau %v", before, after)
		}
	})

	t.Run("đang lưu trữ: quyền xét TRƯỚC, người ngoài 404 và học viên 403, không lộ 409", func(t *testing.T) {
		for _, op := range ops {
			if op.permissionless {
				continue
			}
			if err := op.run(e.ownerB); !errors.Is(err, ErrClassNotFound) {
				t.Errorf("%s / chủ tổ chức B: err=%v, muốn ErrClassNotFound (404)", op.name, err)
			}
			if err := op.run(e.student); err == nil || errors.Is(err, ErrClassArchived) || errors.Is(err, ErrClassNotFound) {
				t.Errorf("%s / học viên: err=%v, muốn lỗi quyền 403 (không phải 409/404)", op.name, err)
			}
		}
	})

	t.Run("đang lưu trữ: đọc vẫn được và người quản lý vẫn thấy can_manage để mở lại", func(t *testing.T) {
		got, err := e.classes.GetClassByID(ctx, class.ID, e.ownerA.ID, false)
		if err != nil || got.Status != "archived" || !got.CanManage {
			t.Fatalf("GET lớp lưu trữ: %+v err=%v, muốn status=archived và can_manage", got, err)
		}
		reads := map[string]error{}
		_, reads["học viên"] = e.classes.GetStudentsByClass(ctx, class.ID, e.ownerA.ID, false, 1, 20)
		_, reads["giảng viên"] = e.classes.GetTeachersByClass(ctx, class.ID, e.ownerA.ID, false, 1, 20)
		_, reads["buổi học"] = e.schedule.GetSessionsByClass(ctx, class.ID, e.ownerA.ID, false, 1, 20)
		_, reads["thời khoá biểu"] = e.schedule.GetClassTimetable(ctx, class.ID, e.ownerA.ID, false)
		_, reads["điểm danh"] = e.attendance.GetAllAttendances(ctx, class.ID, e.ownerA.ID, false, "", 1, 20)
		_, reads["bảng điểm"] = grades.GetGradesByClass(ctx, class.ID, e.ownerA.ID)
		_, reads["cột điểm"] = grades.GetGradeColumns(ctx, class.ID, e.ownerA.ID)
		_, reads["điểm của học viên"] = grades.GetStudentGrades(ctx, class.ID, e.student.ID, e.student.ID)
		for what, err := range reads {
			if err != nil {
				t.Errorf("đọc %s của lớp lưu trữ bị chặn: %v", what, err)
			}
		}
	})

	t.Run("mở lại lớp thì ghi lại được", func(t *testing.T) {
		active := "active"
		if got, err := e.classes.UpdateClass(ctx, class.ID, e.ownerA.ID, false, dto.UpdateClassDTO{Status: &active}); err != nil || got.Status != "active" {
			t.Fatalf("mở lại lớp bị chặn: %+v err=%v", got, err)
		}
		for _, op := range ops {
			if err := op.run(e.ownerA); errors.Is(err, ErrClassArchived) {
				t.Errorf("%s: vẫn ErrClassArchived sau khi mở lại lớp", op.name)
			}
		}
		if after := snapshot(); after == before {
			t.Error("mở lại lớp nhưng không thao tác ghi nào đổi được dữ liệu")
		}
	})
}

// Hợp đồng GetAllVisible (SQL) <-> ensureClassView/orgManagesClass (Go): với MỌI vai và MỌI lớp, "lớp có trong danh
// sách GET /classes" phải đúng bằng "GET /classes/:id không 404". Sửa một bên (vd bỏ điều kiện học viên active ở SQL,
// hoặc thêm vai xem được ở Go) mà không sửa bên kia thì test ĐỎ. Bảng `want` ghim thêm các ô cốt lõi để hai bên cùng
// đổi sai một cách không đáng có cũng ĐỎ.
func TestW3BE_GetAllVisible_KhopEnsureClassView_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()

	inA := e.classInOrgA(t) // tổ chức A, chủ khoá instructor, coTeacher được gán, student đang học
	createdBy := model.Class{Name: "W3 do newTeacher tạo " + uuid.NewString()[:6], Status: "active", CreatedBy: &e.newTeacher.ID}
	mustCreate(t, e.db, &createdBy)   // không khoá, không tổ chức: chỉ người tạo
	personal := e.classOutsideOrgA(t) // lớp cá nhân của stranger (chủ khoá), student đang học
	// Học viên ĐÃ RÚT khỏi lớp A và một học viên đã hoàn thành lớp cá nhân: không còn là thành viên.
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.newStudent.ID, ClassID: inA.ID, Status: "dropped"})
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.newStudent.ID, ClassID: personal.ID, Status: "completed"})
	classes := []model.Class{inA, createdBy, personal}

	type actor struct {
		name  string
		user  model.User
		admin bool
	}
	actors := []actor{
		{"admin hệ thống", e.systemAdmin, true},
		{"giảng viên được gán", e.coTeacher, false},
		{"người tạo lớp", e.newTeacher, false},
		{"chủ khoá", e.instructor, false},
		{"học viên đang học", e.student, false},
		{"học viên đã rút", e.newStudent, false},
		{"chủ tổ chức đúng (A)", e.ownerA, false},
		{"chủ tổ chức sai (B)", e.ownerB, false},
		{"thành viên A không quản trị", e.memberA, false},
		{"chủ A đã bị gỡ role", e.revokedOwnerA, false},
		{"chủ khoá lớp khác", e.stranger, false},
	}
	// ô cốt lõi: (vai, lớp) -> xem được
	want := map[string]map[uuid.UUID]bool{
		"admin hệ thống":              {inA.ID: true, createdBy.ID: true, personal.ID: true},
		"giảng viên được gán":         {inA.ID: true},
		"người tạo lớp":               {createdBy.ID: true},
		"chủ khoá":                    {inA.ID: true},
		"học viên đang học":           {inA.ID: true, personal.ID: true},
		"học viên đã rút":             {},
		"chủ tổ chức đúng (A)":        {inA.ID: true},
		"chủ tổ chức sai (B)":         {},
		"chủ A đã bị gỡ role":         {},
		"thành viên A không quản trị": {},
		"chủ khoá lớp khác":           {personal.ID: true},
	}

	for _, a := range actors {
		list, err := e.classes.GetAllClasses(ctx, a.user.ID, a.admin, 1, 100, "", "")
		if err != nil {
			t.Fatalf("%s: GetAllClasses: %v", a.name, err)
		}
		inList := map[uuid.UUID]bool{}
		for _, c := range list.Classes {
			inList[c.ID] = true
		}
		for _, c := range classes {
			_, err := e.classes.GetClassByID(ctx, c.ID, a.user.ID, a.admin)
			if err != nil && !errors.Is(err, ErrClassNotFound) {
				t.Fatalf("%s / %s: GetClassByID: %v", a.name, c.Name, err)
			}
			viewable := err == nil
			if inList[c.ID] != viewable {
				t.Errorf("%s / %s: trong danh sách=%v nhưng xem được=%v (SQL GetAllVisible lệch ensureClassView)", a.name, c.Name, inList[c.ID], viewable)
			}
			if w := want[a.name][c.ID]; viewable != w {
				t.Errorf("%s / %s: xem được=%v, muốn %v", a.name, c.Name, viewable, w)
			}
		}
	}
}

// Race ROLE_IN_USE đi qua đường gán THẬT (UserOrganizationRoleRepository.AssignRolesWithTx), không gọi LockRolesForShare
// trực tiếp: một transaction giữ khoá FOR UPDATE trên dòng role (như DeleteRoleIfUnused đang xoá) thì lượt gán phải CHỜ,
// rồi thấy role đã mất (ErrRoleGone) và không để lại bản ghi gán treo. Bỏ LockRolesForShare khỏi AssignRolesWithTx thì
// lượt gán không chờ, tạo bản ghi trỏ vào role đã xoá, và test ĐỎ.
func TestW3BE_AssignRolesWithTx_ChoXoaRole_Postgres(t *testing.T) {
	e := newS6OrgEnv(t)
	role := e.orgRole(e.orgA, "TRO_GIANG_W3")
	holder := e.user("holder-w3")
	repo := repository.NewUserOrganizationRoleRepository(e.db)

	tx := e.db.Begin()
	var locked model.Role
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", role.ID).First(&locked).Error; err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- repo.AssignRolesWithTx(context.Background(), nil, []model.UserOrganizationRole{{
			UserID: holder.ID, RoleID: role.ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}})
	}()

	select {
	case err := <-done:
		tx.Rollback()
		t.Fatalf("AssignRolesWithTx không chờ lượt xoá role đang giữ khoá (err=%v): còn race ROLE_IN_USE", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx.Delete(&model.Role{}, "id = ?", role.ID).Error; err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, repository.ErrRoleGone) {
			t.Fatalf("gán vào role vừa bị xoá: err=%v, muốn ErrRoleGone", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("AssignRolesWithTx treo sau khi xoá role commit")
	}
	var dangling int64
	e.db.Model(&model.UserOrganizationRole{}).Where("user_id = ? AND role_id = ?", holder.ID, role.ID).Count(&dangling)
	if dangling != 0 {
		t.Errorf("còn %d bản ghi gán treo trỏ vào role đã xoá", dangling)
	}
}

// GenerateSessions không sinh lại buổi đã HUỶ: sinh, huỷ một buổi, sinh lại (cả đúng khoảng cũ lẫn khoảng kéo dài) thì
// buổi huỷ không quay về. Bỏ nhánh wasCancelled thì buổi 10/03 được sinh mới (buổi huỷ không chiếm giờ) và test ĐỎ.
func TestW3BE_GenerateSessions_KhongSinhLaiBuoiDaHuy_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	class := e.classInOrgA(t)
	if _, err := e.schedule.CreateSchedule(ctx, class.ID, e.ownerA.ID, false, dto.CreateClassScheduleDTO{
		DayOfWeek: 1, StartTime: "19:00", EndTime: "21:00", EffectiveFrom: "2031-01-01", EffectiveUntil: "2031-12-31"}); err != nil {
		t.Fatal(err)
	}
	out, err := e.schedule.GenerateSessions(ctx, class.ID, e.ownerA.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-17"})
	if err != nil || len(out) != 3 {
		t.Fatalf("sinh lần đầu: %d buổi err=%v, muốn 3 (03, 10, 17/03)", len(out), err)
	}
	if err := e.schedule.CancelSession(ctx, out[1].ID, e.ownerA.ID, false, "nghỉ lễ"); err != nil { // thứ Hai 10/03
		t.Fatal(err)
	}

	count := func(date string) (live, cancelled int64) {
		e.db.Model(&model.ClassSession{}).Where("class_id = ? AND date = ? AND status <> ?", class.ID, date, model.SessionCancelled).Count(&live)
		e.db.Model(&model.ClassSession{}).Where("class_id = ? AND date = ? AND status = ?", class.ID, date, model.SessionCancelled).Count(&cancelled)
		return
	}

	// Sinh lại đúng khoảng cũ: không còn gì mới (buổi huỷ cũng không quay lại) -> ErrSessionOverlap, không thêm hàng.
	if _, err := e.schedule.GenerateSessions(ctx, class.ID, e.ownerA.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-17"}); !errors.Is(err, ErrSessionOverlap) {
		t.Fatalf("sinh lại khoảng đã sinh: err=%v, muốn ErrSessionOverlap", err)
	}
	if live, cancelled := count("2031-03-10"); live != 0 || cancelled != 1 {
		t.Fatalf("buổi huỷ 10/03 quay lại: live=%d cancelled=%d, muốn 0 và 1", live, cancelled)
	}

	// Kéo dài khoảng: chỉ sinh tuần mới 24/03, buổi huỷ 10/03 vẫn nằm yên.
	more, err := e.schedule.GenerateSessions(ctx, class.ID, e.ownerA.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-24"})
	if err != nil || len(more) != 1 || more[0].Date != "2031-03-24" {
		t.Fatalf("kéo dài tới 24/03: %+v err=%v, muốn đúng một buổi 2031-03-24", more, err)
	}
	if live, cancelled := count("2031-03-10"); live != 0 || cancelled != 1 {
		t.Errorf("sau khi kéo dài, buổi huỷ 10/03 bị sinh lại: live=%d cancelled=%d", live, cancelled)
	}
}
