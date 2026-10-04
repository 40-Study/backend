package service

// Lane S4, lỗi O-1 (Postgres thật): gán/gỡ giảng viên của lớp chỉ dành cho CHỦ lớp (người tạo lớp
// hoặc giảng viên chủ khoá) và admin. Trước đây bất kỳ giảng viên nào tự gán mình vào lớp của người
// khác rồi có CanManage trên assignment của lớp đó. Test đi theo vai: giảng viên lạ, giảng viên đã
// được gán (không phải chủ), học viên trong lớp, chủ khoá, người tạo lớp, admin. Bỏ ensureClassOwner
// ở một trong ba hàm gán/gỡ, hoặc nới classOwner, thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// s4TeacherRepo giả TeacherRepository.Exists (thật đòi role TEACHER đã seed): chỉ những id trong tập là giảng viên.
type s4TeacherRepo struct {
	repository.TeacherRepositoryInterface
	teachers map[uuid.UUID]bool
}

func (r s4TeacherRepo) Exists(_ context.Context, id uuid.UUID) (bool, error) { return r.teachers[id], nil }

type s4ClassEnv struct {
	f                                                                     *s2Fixture
	svc                                                                   *ClassService
	owner, creator, coTeacher, stranger, student, newTeacher, admin       model.User
	course, foreignCourse                                                 model.Course
	class                                                                 model.Class
}

func newS4ClassEnv(t *testing.T) *s4ClassEnv {
	f := newS2Fixture(t)
	e := &s4ClassEnv{f: f}
	e.owner, e.creator, e.coTeacher, e.stranger = f.user("owner"), f.user("creator"), f.user("co-teacher"), f.user("stranger")
	e.student, e.newTeacher, e.admin = f.user("student"), f.user("new-teacher"), f.user("admin")
	e.course = f.course(e.owner)
	e.foreignCourse = f.course(e.stranger)
	e.class = model.Class{Name: "QA-s4", CourseID: &e.course.ID, Status: "active", CreatedBy: &e.creator.ID}
	if err := f.db.Create(&e.class).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: e.class.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudentClass{StudentID: e.student.ID, ClassID: e.class.ID, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	e.svc = NewClassService(repository.NewClassRepository(f.db), repository.NewCourseRepository(f.db),
		s4TeacherRepo{teachers: map[uuid.UUID]bool{e.owner.ID: true, e.creator.ID: true, e.coTeacher.ID: true, e.stranger.ID: true, e.newTeacher.ID: true}},
		nil, nil)
	return e
}

func (e *s4ClassEnv) isTeacherOf(t *testing.T, u model.User) bool {
	t.Helper()
	ok, err := repository.NewClassRepository(e.f.db).TeacherClassExists(context.Background(), e.class.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestS4_Class_GanGoGiangVienChiChuLop(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()

	type actor struct {
		name    string
		user    model.User
		isAdmin bool
		want    error
	}
	actors := []actor{
		{"giảng viên lạ (không dính gì tới lớp)", e.stranger, false, ErrClassNotFound},
		{"giảng viên đã được gán vào lớp nhưng không phải chủ", e.coTeacher, false, ErrNotClassOwner},
		{"học viên trong lớp", e.student, false, ErrNotClassOwner},
		{"chủ khoá", e.owner, false, nil},
		{"người tạo lớp", e.creator, false, nil},
		{"admin", e.admin, true, nil},
	}

	t.Run("gán một giảng viên", func(t *testing.T) {
		for _, a := range actors {
			// Mỗi vai gán một giảng viên MỚI khác nhau để "đã được gán" không che lỗi quyền.
			target := e.f.user("target")
			e.svc.teacherRepo = s4TeacherRepo{teachers: map[uuid.UUID]bool{target.ID: true, e.stranger.ID: true}}
			_, err := e.svc.AssignTeacherToClass(ctx, e.class.ID, a.user.ID, a.isAdmin, dto.AssignTeacherDTO{TeacherID: target.ID})
			if !errors.Is(err, a.want) {
				t.Errorf("%s: err=%v, muốn %v", a.name, err, a.want)
			}
			if got, want := e.isTeacherOf(t, target), a.want == nil; got != want {
				t.Errorf("%s: giảng viên được gán=%v, muốn %v", a.name, got, want)
			}
		}
	})

	t.Run("giảng viên lạ tự gán mình không thành CanManage trên assignment của lớp", func(t *testing.T) {
		e.svc.teacherRepo = s4TeacherRepo{teachers: map[uuid.UUID]bool{e.stranger.ID: true}}
		hw := model.Assignment{ClassID: &e.class.ID, Type: "homework", Title: "hw", Description: "d", Language: []string{"python"}}
		if err := e.f.db.Create(&hw).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.AssignTeacherToClass(ctx, e.class.ID, e.stranger.ID, false, dto.AssignTeacherDTO{TeacherID: e.stranger.ID}); err == nil {
			t.Fatal("giảng viên lạ tự gán mình phải bị chặn")
		}
		manage, err := repository.NewAssignmentRepository(e.f.db).CanManage(ctx, hw.ID, e.stranger.ID)
		if err != nil || manage {
			t.Fatalf("CanManage=%v err=%v, muốn false", manage, err)
		}
	})

	t.Run("gán nhiều giảng viên", func(t *testing.T) {
		for _, a := range actors {
			target := e.f.user("bulk")
			e.svc.teacherRepo = s4TeacherRepo{teachers: map[uuid.UUID]bool{target.ID: true}}
			_, err := e.svc.AssignTeachersToClass(ctx, e.class.ID, a.user.ID, a.isAdmin, dto.AssignTeachersDTO{Teachers: []dto.AssignTeacherDTO{{TeacherID: target.ID}}})
			if !errors.Is(err, a.want) {
				t.Errorf("%s: err=%v, muốn %v", a.name, err, a.want)
			}
		}
	})

	t.Run("gỡ giảng viên", func(t *testing.T) {
		for _, a := range actors {
			victim := e.f.user("victim")
			if err := e.f.db.Create(&model.TeacherClass{TeacherID: victim.ID, ClassID: e.class.ID, Role: "assistant"}).Error; err != nil {
				t.Fatal(err)
			}
			err := e.svc.RemoveTeacherFromClass(ctx, e.class.ID, victim.ID, a.user.ID, a.isAdmin)
			if !errors.Is(err, a.want) {
				t.Errorf("%s: err=%v, muốn %v", a.name, err, a.want)
			}
			if got, want := e.isTeacherOf(t, victim), a.want != nil; got != want {
				t.Errorf("%s: giảng viên bị gỡ khỏi lớp: còn=%v, muốn còn=%v", a.name, got, want)
			}
		}
	})

	t.Run("lớp không tồn tại", func(t *testing.T) {
		err := e.svc.RemoveTeacherFromClass(ctx, uuid.New(), e.coTeacher.ID, e.admin.ID, true)
		if !errors.Is(err, ErrClassNotFound) {
			t.Errorf("admin gỡ ở lớp không tồn tại: err=%v, muốn ErrClassNotFound", err)
		}
		err = e.svc.RemoveTeacherFromClass(ctx, uuid.New(), e.coTeacher.ID, e.stranger.ID, false)
		if !errors.Is(err, ErrClassNotFound) {
			t.Errorf("giảng viên lạ gỡ ở lớp không tồn tại: err=%v, muốn ErrClassNotFound (cùng 404 với lớp không xem được)", err)
		}
	})
}

// Rà các route ghi khác của lớp: tạo lớp và đổi khoá cũng phải gắn với chủ khoá, và chủ khoá/người
// tạo lớp (chưa được gán giảng viên) sửa được lớp của mình.
func TestS4_Class_TaoLopDoiKhoaVaQuanLy(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()

	t.Run("tạo lớp", func(t *testing.T) {
		mk := func(course *uuid.UUID) dto.CreateClassDTO { return dto.CreateClassDTO{Name: "new-class", CourseID: course} }
		cid, fid := e.course.ID, e.foreignCourse.ID
		cases := []struct {
			name    string
			user    model.User
			isAdmin bool
			req     dto.CreateClassDTO
			want    error
		}{
			{"chủ khoá tạo vào khoá của mình", e.owner, false, mk(&cid), nil},
			{"giảng viên khác tạo vào khoá của người khác", e.coTeacher, false, mk(&cid), ErrNotCourseInstructor},
			{"học viên tạo vào khoá", e.student, false, mk(&cid), ErrNotCourseInstructor},
			{"admin tạo vào khoá bất kỳ", e.admin, true, mk(&fid), nil},
			{"giảng viên tạo lớp không gắn khoá", e.newTeacher, false, mk(nil), nil},
			{"học viên tạo lớp không gắn khoá", e.student, false, mk(nil), ErrNotTeacher},
		}
		for _, c := range cases {
			got, err := e.svc.CreateClass(ctx, c.user.ID, c.isAdmin, c.req)
			if !errors.Is(err, c.want) {
				t.Errorf("%s: err=%v, muốn %v", c.name, err, c.want)
				continue
			}
			if c.want == nil {
				var row model.Class
				if err := e.f.db.First(&row, "id = ?", got.ID).Error; err != nil {
					t.Fatal(err)
				}
				if row.CreatedBy == nil || *row.CreatedBy != c.user.ID {
					t.Errorf("%s: created_by=%v, muốn %v", c.name, row.CreatedBy, c.user.ID)
				}
			}
		}
	})

	t.Run("đổi khoá", func(t *testing.T) {
		fid, cid := e.foreignCourse.ID, e.course.ID
		// Khoá thứ hai của chính chủ khoá cũ (owner), để thử "đổi khoá trong phạm vi của mình".
		ownCourse2 := e.f.course(e.owner)
		courseOf := func() uuid.UUID {
			var row model.Class
			if err := e.f.db.First(&row, "id = ?", e.class.ID).Error; err != nil || row.CourseID == nil {
				t.Fatalf("đọc lớp: %v", err)
			}
			return *row.CourseID
		}
		// giảng viên được gán vào lớp (không phải chủ) đẩy lớp sang khoá của CHÍNH MÌNH: bị chặn vì không phải chủ lớp
		coTeacherCourse := e.f.course(e.coTeacher)
		if _, err := e.svc.UpdateClass(ctx, e.class.ID, e.coTeacher.ID, false, dto.UpdateClassDTO{CourseID: &coTeacherCourse.ID}); !errors.Is(err, ErrNotClassOwner) {
			t.Errorf("giảng viên lớp kéo lớp sang khoá của mình: err=%v, muốn ErrNotClassOwner", err)
		}
		// giảng viên lớp gửi lại đúng khoá hiện tại (form sửa lớp luôn gửi course_id): không bị chặn
		if _, err := e.svc.UpdateClass(ctx, e.class.ID, e.coTeacher.ID, false, dto.UpdateClassDTO{CourseID: &cid}); err != nil {
			t.Errorf("giảng viên lớp gửi lại khoá hiện tại: %v", err)
		}
		// chủ khoá cũ đẩy lớp sang khoá của người khác: bị chặn vì không phải chủ khoá đích
		if _, err := e.svc.UpdateClass(ctx, e.class.ID, e.owner.ID, false, dto.UpdateClassDTO{CourseID: &fid}); !errors.Is(err, ErrNotCourseInstructor) {
			t.Errorf("chủ khoá đẩy lớp sang khoá của người khác: err=%v, muốn ErrNotCourseInstructor", err)
		}
		if got := courseOf(); got != cid {
			t.Fatalf("lớp bị đổi khoá dù bị chặn: %v", got)
		}
		// chủ khoá đổi trong phạm vi khoá của mình: được
		if _, err := e.svc.UpdateClass(ctx, e.class.ID, e.owner.ID, false, dto.UpdateClassDTO{CourseID: &ownCourse2.ID}); err != nil {
			t.Errorf("chủ khoá đổi sang khoá khác của mình: %v", err)
		}
		if got := courseOf(); got != ownCourse2.ID {
			t.Fatalf("khoá sau khi chủ đổi = %v, muốn %v", got, ownCourse2.ID)
		}
		// admin đổi về khoá ban đầu
		if _, err := e.svc.UpdateClass(ctx, e.class.ID, e.admin.ID, true, dto.UpdateClassDTO{CourseID: &cid}); err != nil {
			t.Errorf("admin đổi khoá: %v", err)
		}
	})

	t.Run("sửa, xoá, thêm/gỡ học viên: giảng viên lạ và học viên bị chặn, chủ khoá và người tạo qua", func(t *testing.T) {
		name := "renamed"
		for _, u := range []model.User{e.stranger, e.student} {
			// W2-A: nguoi khong xem duoc lop (giang vien la) nhan 404; hoc vien trong lop xem duoc nen nhan 403.
			wantDenied := ErrNotClassTeacher
			if u.ID == e.stranger.ID {
				wantDenied = ErrClassNotFound
			}
			if _, err := e.svc.UpdateClass(ctx, e.class.ID, u.ID, false, dto.UpdateClassDTO{Name: &name}); !errors.Is(err, wantDenied) {
				t.Errorf("UpdateClass bởi %s: err=%v, muon %v", u.UserName, err, wantDenied)
			}
			if err := e.svc.DeleteClass(ctx, e.class.ID, u.ID, false, false); !errors.Is(err, wantDenied) {
				t.Errorf("DeleteClass bởi %s: err=%v, muon %v", u.UserName, err, wantDenied)
			}
			if _, err := e.svc.EnrollStudentToClass(ctx, e.class.ID, u.ID, false, dto.EnrollStudentDTO{StudentID: e.student.ID}); !errors.Is(err, wantDenied) {
				t.Errorf("EnrollStudentToClass bởi %s: err=%v, muon %v", u.UserName, err, wantDenied)
			}
			if err := e.svc.RemoveStudentFromClass(ctx, e.class.ID, e.student.ID, u.ID, false); !errors.Is(err, wantDenied) {
				t.Errorf("RemoveStudentFromClass bởi %s: err=%v, muon %v", u.UserName, err, wantDenied)
			}
		}
		// Chủ khoá và người tạo không cần là giảng viên được gán vẫn quản lý được lớp của mình.
		for _, u := range []model.User{e.owner, e.creator} {
			if _, err := e.svc.UpdateClass(ctx, e.class.ID, u.ID, false, dto.UpdateClassDTO{Name: &name}); err != nil {
				t.Errorf("UpdateClass bởi %s: %v", u.UserName, err)
			}
		}
	})
}
