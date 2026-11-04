package service

// Q3-06 (QA3): lớp mới tạo phải có chính người tạo làm giảng viên chính, nếu không lớp biến mất khỏi "Lớp học của tôi"
// (GET /classes/me liệt kê theo teacher_classes) và teacher_count = 0.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestCreateClass_CreatorBecomesPrimaryTeacherAndSeesItInMyClasses(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	cid := e.course.ID

	cases := []struct {
		name        string
		user        model.User
		isAdmin     bool
		req         dto.CreateClassDTO
		wantTeacher bool
	}{
		{"giảng viên tạo lớp cá nhân", e.newTeacher, false, dto.CreateClassDTO{Name: "lớp cá nhân"}, true},
		{"chủ khoá tạo lớp vào khoá của mình", e.owner, false, dto.CreateClassDTO{Name: "lớp theo khoá", CourseID: &cid}, true},
		{"admin không phải giảng viên tạo hộ", e.admin, true, dto.CreateClassDTO{Name: "lớp do admin"}, false},
	}
	for _, c := range cases {
		got, err := e.svc.CreateClass(ctx, c.user.ID, c.isAdmin, c.req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		wantCount := int64(0)
		if c.wantTeacher {
			wantCount = 1
		}
		if got.TeacherCount != wantCount {
			t.Errorf("%s: teacher_count = %d, muốn %d", c.name, got.TeacherCount, wantCount)
		}
		var role string
		e.f.db.Model(&model.TeacherClass{}).Select("role").Where("class_id = ? AND teacher_id = ?", got.ID, c.user.ID).Scan(&role)
		if c.wantTeacher && role != "primary" {
			t.Errorf("%s: role = %q, muốn primary", c.name, role)
		}

		mine, err := e.svc.GetMyClasses(ctx, c.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, m := range mine {
			found = found || m.ID == got.ID
		}
		if found != c.wantTeacher {
			t.Errorf("%s: lớp có trong /classes/me = %v, muốn %v", c.name, found, c.wantTeacher)
		}
	}
}

// Lớp và dòng giảng viên cùng transaction: gán giảng viên lỗi thì lớp cũng không được để lại.
func TestCreateClass_RollsBackClassWhenTeacherAssignmentFails(t *testing.T) {
	e := newS4ClassEnv(t)
	ghost := uuid.New() // không có trong users -> vi phạm FK teacher_classes.teacher_id
	class := &model.Class{Name: "lớp nên bị rollback", Status: "draft"}
	err := e.svc.classRepo.CreateWithPrimaryTeacher(context.Background(), class, &ghost)
	if err == nil {
		t.Fatal("muốn lỗi FK khi gán giảng viên không tồn tại")
	}
	var n int64
	e.f.db.Model(&model.Class{}).Where("name = ?", "lớp nên bị rollback").Count(&n)
	if n != 0 {
		t.Fatalf("lớp vẫn còn %d dòng sau khi gán giảng viên lỗi: không atomic", n)
	}
}
