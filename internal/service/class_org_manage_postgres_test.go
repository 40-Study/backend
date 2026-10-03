package service

// Lane R5 (Postgres thật, schema tạm), theo từng vai: chủ/quản trị tổ chức quản lý lớp thuộc tổ chức.
//
// B-05: trước đây chủ tổ chức CHẤM được điểm lớp của tổ chức nhưng GET lớp, danh sách học viên, buổi học và điểm
// danh đều 404 vì ensureClassView/ensureClassManage không biết nhánh orgManagesClass.
// B-12: kích hoạt lớp (draft -> active), gán/gỡ giảng viên, ghi danh/gỡ học viên cũng phải theo cùng luật.
//
// Vai: chủ tổ chức A, thành viên A không có quyền quản trị, chủ tổ chức B, chủ A đã bị gỡ role, người lạ, học viên
// trong lớp, admin hệ thống. Lớp: thuộc A, cá nhân, thuộc B, không tổ chức. Bỏ classAccessAsAdmin ở ClassService,
// ScheduleService hoặc AttendanceService thì ca "chủ tổ chức A" tương ứng ĐỎ; nâng quyền cho cả lớp ngoài tổ chức
// thì các ca 404 ĐỎ; không gắn authorizer thì mọi ca chủ tổ chức quay về 404 (fail-closed).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// r5StudentRepo giả StudentRepository.Exists (thật đòi role STUDENT đã seed); SearchEnrollable dùng repo thật.
type r5StudentRepo struct {
	repository.StudentRepositoryInterface
	students map[uuid.UUID]bool
}

func (r r5StudentRepo) Exists(_ context.Context, id uuid.UUID) (bool, error) {
	return r.students[id], nil
}

type r5ClassEnv struct {
	*gradeOrgEnv
	newTeacher, newStudent model.User
	classes                *ClassService
	schedule               *ScheduleService
	attendance             *AttendanceService
}

func newR5ClassEnv(t *testing.T) *r5ClassEnv {
	t.Helper()
	e := &r5ClassEnv{gradeOrgEnv: newGradeOrgEnv(t)}
	e.newTeacher, e.newStudent = e.user("new-teacher"), e.user("new-student")
	e.classes = e.buildClassService(e.checker)
	e.schedule = NewScheduleService(repository.NewScheduleRepository(e.db), repository.NewClassRepository(e.db),
		repository.NewCourseRepository(e.db), nil, nil).WithAuthorizer(e.checker)
	e.attendance = NewAttendanceService(repository.NewAttendanceRepository(e.db), repository.NewClassRepository(e.db),
		repository.NewCourseRepository(e.db)).WithAuthorizer(e.checker)
	return e
}

func (e *r5ClassEnv) buildClassService(authz ClassAuthorizer) *ClassService {
	svc := NewClassService(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db),
		s4TeacherRepo{teachers: map[uuid.UUID]bool{e.instructor.ID: true, e.coTeacher.ID: true, e.newTeacher.ID: true}},
		r5StudentRepo{StudentRepositoryInterface: repository.NewStudentRepository(e.db), students: map[uuid.UUID]bool{e.student.ID: true, e.newStudent.ID: true}}, nil)
	if authz != nil {
		svc.WithAuthorizer(authz)
	}
	return svc
}

func TestR5_OrgOwner_QuanLyLopCuaToChuc_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()

	inA, personal, inB, foreign := e.classInOrgA(t), e.personalClassOfOrgMember(t), e.classInOrgB(t), e.classOutsideOrgA(t)

	type actor struct {
		name   string
		user   model.User
		isSys  bool
		viewOK map[uuid.UUID]bool // lớp nào xem/quản lý được; còn lại 404
	}
	actors := []actor{
		{"chủ tổ chức A", e.ownerA, false, map[uuid.UUID]bool{inA.ID: true}},
		{"chủ tổ chức B", e.ownerB, false, map[uuid.UUID]bool{inB.ID: true}},
		{"thành viên A không có quyền quản trị", e.memberA, false, map[uuid.UUID]bool{}},
		{"chủ A đã bị gỡ role", e.revokedOwnerA, false, map[uuid.UUID]bool{}},
		// stranger là instructor chủ khoá của lớp thuộc B và lớp ngoài tổ chức (fixture), nên quản lý hai lớp đó theo luật cũ.
		{"giảng viên chủ khoá của lớp B và lớp ngoài tổ chức", e.stranger, false, map[uuid.UUID]bool{inB.ID: true, foreign.ID: true}},
	}
	all := []model.Class{inA, personal, inB, foreign}

	for _, a := range actors {
		for _, c := range all {
			want := a.viewOK[c.ID]
			t.Run(a.name+" / "+c.Name, func(t *testing.T) {
				check := func(op string, err error) {
					t.Helper()
					if want && err != nil {
						t.Errorf("%s: bị chặn nhầm: %v", op, err)
					}
					if !want && !errors.Is(err, ErrClassNotFound) {
						t.Errorf("%s: err=%v, muốn ErrClassNotFound (404, không dò được lớp)", op, err)
					}
				}
				_, err := e.classes.GetClassByID(ctx, c.ID, a.user.ID, false)
				check("GET lớp", err)
				_, err = e.classes.GetStudentsByClass(ctx, c.ID, a.user.ID, false, 1, 20)
				check("GET học viên", err)
				_, err = e.classes.GetTeachersByClass(ctx, c.ID, a.user.ID, false, 1, 20)
				check("GET giảng viên", err)
				_, err = e.schedule.GetSessionsByClass(ctx, c.ID, a.user.ID, false, 1, 20)
				check("GET buổi học", err)
				_, err = e.schedule.GetClassTimetable(ctx, c.ID, a.user.ID, false)
				check("GET thời khoá biểu", err)
				_, err = e.attendance.GetAllAttendances(ctx, c.ID, a.user.ID, false, "", 1, 20)
				check("GET điểm danh", err)
			})
		}
	}

	// Học viên trong lớp vẫn xem được lớp, nhưng không quản lý được (403, không phải 404).
	t.Run("học viên: xem được, không quản lý", func(t *testing.T) {
		if _, err := e.classes.GetClassByID(ctx, inA.ID, e.student.ID, false); err != nil {
			t.Errorf("học viên xem lớp mình: %v", err)
		}
		active := "active"
		if _, err := e.classes.UpdateClass(ctx, inA.ID, e.student.ID, false, dto.UpdateClassDTO{Status: &active}); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("học viên kích hoạt lớp: err=%v, muốn ErrNotClassTeacher", err)
		}
	})

	t.Run("chủ tổ chức A: kích hoạt lớp draft", func(t *testing.T) {
		draft := model.Class{Name: "R5 draft " + uuid.NewString()[:6], Status: "draft", OrganizationID: &e.orgA.ID}
		mustCreate(t, e.db, &draft)
		active := "active"
		got, err := e.classes.UpdateClass(ctx, draft.ID, e.ownerA.ID, false, dto.UpdateClassDTO{Status: &active})
		if err != nil || got.Status != "active" {
			t.Fatalf("chủ tổ chức kích hoạt lớp: %+v err=%v", got, err)
		}
		var stored model.Class
		if err := e.db.First(&stored, "id = ?", draft.ID).Error; err != nil || stored.Status != "active" {
			t.Errorf("DB status=%q err=%v, muốn active", stored.Status, err)
		}
		// Chủ tổ chức B, và chủ A với lớp cá nhân: không kích hoạt được. Đường GHI giữ nguyên ErrNotClassTeacher của
		// requireClassTeacherOrAdmin (403), khác đường ĐỌC (404).
		archived := "archived"
		if _, err := e.classes.UpdateClass(ctx, personal.ID, e.ownerA.ID, false, dto.UpdateClassDTO{Status: &archived}); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("chủ A sửa lớp cá nhân: err=%v, muốn ErrNotClassTeacher", err)
		}
		if _, err := e.classes.UpdateClass(ctx, draft.ID, e.ownerB.ID, false, dto.UpdateClassDTO{Status: &archived}); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("chủ B sửa lớp của A: err=%v, muốn ErrNotClassTeacher", err)
		}
		var after model.Class
		if err := e.db.First(&after, "id = ?", personal.ID).Error; err != nil || after.Status != "active" {
			t.Errorf("lớp cá nhân bị đổi: status=%q err=%v", after.Status, err)
		}
	})

	t.Run("chủ tổ chức A: không chuyển lớp sang khoá của người khác", func(t *testing.T) {
		// "isAdmin" nâng quyền chỉ cho kiểm truy cập lớp; đổi khoá vẫn đòi chủ lớp thật, nếu không chủ tổ chức
		// kéo được lớp sang khoá bất kỳ.
		other := e.course(e.stranger)
		if _, err := e.classes.UpdateClass(ctx, inA.ID, e.ownerA.ID, false, dto.UpdateClassDTO{CourseID: &other.ID}); err == nil {
			t.Fatal("chủ tổ chức đổi khoá của lớp sang khoá người khác phải bị chặn")
		}
		var stored model.Class
		if err := e.db.First(&stored, "id = ?", inA.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.CourseID == nil || *stored.CourseID == other.ID {
			t.Errorf("course_id của lớp đã bị đổi sang %v", stored.CourseID)
		}
	})

	t.Run("chủ tổ chức A: gán/gỡ giảng viên, ghi danh/gỡ học viên", func(t *testing.T) {
		class := e.classInOrgA(t)
		if _, err := e.classes.AssignTeacherToClass(ctx, class.ID, e.ownerA.ID, false, dto.AssignTeacherDTO{TeacherID: e.newTeacher.ID}); err != nil {
			t.Fatalf("gán giảng viên: %v", err)
		}
		if _, err := e.classes.EnrollStudentToClass(ctx, class.ID, e.ownerA.ID, false, dto.EnrollStudentDTO{StudentID: e.newStudent.ID}); err != nil {
			t.Fatalf("ghi danh học viên: %v", err)
		}
		students, err := e.classes.GetStudentsByClass(ctx, class.ID, e.ownerA.ID, false, 1, 20)
		if err != nil || students.Total != 2 {
			t.Fatalf("danh sách học viên sau ghi danh: %+v err=%v, muốn 2", students, err)
		}
		if err := e.classes.RemoveStudentFromClass(ctx, class.ID, e.newStudent.ID, e.ownerA.ID, false); err != nil {
			t.Errorf("gỡ học viên: %v", err)
		}
		if err := e.classes.RemoveTeacherFromClass(ctx, class.ID, e.newTeacher.ID, e.ownerA.ID, false); err != nil {
			t.Errorf("gỡ giảng viên: %v", err)
		}

		// Người ngoài: 404 và không để lại dòng nào.
		if _, err := e.classes.AssignTeacherToClass(ctx, class.ID, e.ownerB.ID, false, dto.AssignTeacherDTO{TeacherID: e.newTeacher.ID}); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ B gán giảng viên vào lớp của A: err=%v, muốn ErrClassNotFound", err)
		}
		if _, err := e.classes.EnrollStudentToClass(ctx, personal.ID, e.ownerA.ID, false, dto.EnrollStudentDTO{StudentID: e.newStudent.ID}); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("chủ A ghi danh vào lớp cá nhân: err=%v, muốn ErrNotClassTeacher", err)
		}
	})

	t.Run("chủ tổ chức A: điểm danh lớp của tổ chức", func(t *testing.T) {
		class := e.classInOrgA(t)
		req := dto.BulkCreateAttendanceDTO{Date: "2026-10-05", Attendances: []dto.AttendanceEntryDTO{{StudentID: e.student.ID, Status: "present"}}}
		if out, err := e.attendance.MarkAttendance(ctx, class.ID, e.ownerA.ID, false, req); err != nil || len(out) != 1 {
			t.Fatalf("điểm danh: %v err=%v", out, err)
		}
		if _, err := e.attendance.MarkAttendance(ctx, class.ID, e.ownerB.ID, false, req); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("chủ B điểm danh lớp của A: err=%v, muốn ErrNotClassTeacher", err)
		}
	})

	t.Run("danh sách lớp của tổ chức chỉ có lớp của tổ chức đó", func(t *testing.T) {
		got, err := e.classes.GetOrganizationClasses(ctx, e.orgA.ID, 1, 100, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if got.Total == 0 {
			t.Fatal("tổ chức A có lớp nhưng danh sách rỗng")
		}
		for _, c := range got.Classes {
			if c.OrganizationID == nil || *c.OrganizationID != e.orgA.ID {
				t.Errorf("lớp %s không thuộc tổ chức A: %v", c.Name, c.OrganizationID)
			}
			if c.ID == inB.ID || c.ID == personal.ID || c.ID == foreign.ID {
				t.Errorf("danh sách tổ chức A lộ lớp ngoài tổ chức: %s", c.Name)
			}
		}
		active, err := e.classes.GetOrganizationClasses(ctx, e.orgA.ID, 1, 100, "", "draft")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range active.Classes {
			if c.Status != "draft" {
				t.Errorf("lọc status=draft nhưng có lớp %q", c.Status)
			}
		}
	})

	// can_manage / can_assign_teachers dùng đúng hàm kiểm của thao tác ghi: nút ẩn/hiện không lệch với API.
	t.Run("GET lớp: cờ can_manage và can_assign_teachers theo vai", func(t *testing.T) {
		for _, c := range []struct {
			name           string
			user           model.User
			manage, assign bool
		}{
			{"chủ tổ chức A", e.ownerA, true, true},
			{"giảng viên được gán (không phải chủ lớp)", e.coTeacher, true, false},
			{"instructor chủ khoá", e.instructor, true, true},
			{"học viên trong lớp", e.student, false, false},
		} {
			got, err := e.classes.GetClassByID(ctx, inA.ID, c.user.ID, false)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got.CanManage != c.manage || got.CanAssignTeachers != c.assign {
				t.Errorf("%s: can_manage=%v can_assign_teachers=%v, muốn %v/%v", c.name, got.CanManage, got.CanAssignTeachers, c.manage, c.assign)
			}
		}
	})

	t.Run("ô chọn học viên để ghi danh", func(t *testing.T) {
		sys := model.SystemRole{Name: "STUDENT", Status: "active"}
		mustCreate(t, e.db, &sys)
		for _, u := range []model.User{e.student, e.newStudent} {
			mustCreate(t, e.db, &model.UserSystemRole{UserID: u.ID, SystemRoleID: sys.ID, Status: model.UserSystemRoleStatusActive})
		}
		class := e.classInOrgA(t) // e.student đang học lớp này
		got, err := e.classes.SearchEnrollableStudents(ctx, class.ID, e.ownerA.ID, false, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != e.newStudent.ID {
			t.Fatalf("ứng viên ghi danh=%+v, muốn đúng 1 người chưa học lớp (new-student)", got)
		}
		if byName, err := e.classes.SearchEnrollableStudents(ctx, class.ID, e.ownerA.ID, false, "khong-ton-tai"); err != nil || len(byName) != 0 {
			t.Errorf("từ khoá không khớp: %+v err=%v, muốn rỗng", byName, err)
		}
		if byName, err := e.classes.SearchEnrollableStudents(ctx, class.ID, e.ownerA.ID, false, e.newStudent.UserName[:12]); err != nil || len(byName) != 1 {
			t.Errorf("từ khoá khớp tên: %+v err=%v, muốn 1", byName, err)
		}
		// Học viên trong lớp: xem được lớp nhưng không quản lý -> 403; người ngoài (chủ tổ chức B) -> 404.
		if _, err := e.classes.SearchEnrollableStudents(ctx, class.ID, e.student.ID, false, ""); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("học viên tìm ứng viên: err=%v, muốn ErrNotClassTeacher", err)
		}
		if _, err := e.classes.SearchEnrollableStudents(ctx, class.ID, e.ownerB.ID, false, ""); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ tổ chức B tìm ứng viên lớp của A: err=%v, muốn ErrClassNotFound", err)
		}
	})

	t.Run("không gắn authorizer thì chủ tổ chức quay về 404 (fail-closed)", func(t *testing.T) {
		svc := e.buildClassService(nil)
		if _, err := svc.GetClassByID(ctx, inA.ID, e.ownerA.ID, false); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ tổ chức khi không có authorizer: err=%v, muốn ErrClassNotFound", err)
		}
		if _, err := svc.GetClassByID(ctx, inA.ID, e.coTeacher.ID, false); err != nil {
			t.Errorf("giảng viên lớp bị chặn nhầm: %v", err)
		}
	})
}
