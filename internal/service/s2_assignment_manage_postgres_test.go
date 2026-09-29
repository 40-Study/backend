package service

// Lane S2, lỗi 1 (phía assignment livestream/bài tập về nhà): AssignmentService.CanManage quyết
// định ai được xem test case ẩn và sửa test case. Chủ = host phiên livestream, giảng viên được gán
// vào lớp, hoặc giảng viên chủ khoá của lớp/phiên. Bỏ hoặc nới điều kiện ở AssignmentRepository.
// CanManage thì test này ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestS2_Assignment_CanManage(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	host, classTeacher, courseOwner, stranger, admin := f.user("host"), f.user("class-teacher"), f.user("course-owner"), f.user("stranger"), f.user("admin")

	course := f.course(courseOwner)
	class := model.Class{Name: "QA-s2", CourseID: &course.ID, Status: "active"}
	if err := f.db.Create(&class).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.TeacherClass{TeacherID: classTeacher.ID, ClassID: class.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	session := model.LivestreamSession{Title: "s", HostID: host.ID, ClassID: class.ID, RoomName: "room-" + uuid.NewString()[:8]}
	if err := f.db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	live := model.Assignment{SessionID: &session.ID, Title: "live", Description: "d", Language: []string{"python"}}
	homework := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "hw", Description: "d", Language: []string{"python"}}
	orphan := model.Assignment{Title: "orphan", Description: "d", Language: []string{"python"}}
	for _, a := range []*model.Assignment{&live, &homework, &orphan} {
		if err := f.db.Create(a).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewAssignmentService(repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db), nil, nil, nil)
	check := func(name string, a model.Assignment, u model.User, isAdmin, want bool) {
		t.Helper()
		got, err := svc.CanManage(ctx, a.ID, u.ID, isAdmin)
		if err != nil || got != want {
			t.Errorf("%s: CanManage=%v (err=%v), muốn %v", name, got, err, want)
		}
	}
	check("host phiên, assignment live", live, host, false, true)
	check("giảng viên lớp, assignment live", live, classTeacher, false, true)
	check("giảng viên chủ khoá, assignment live", live, courseOwner, false, true)
	check("giảng viên lớp, bài tập về nhà", homework, classTeacher, false, true)
	check("giảng viên chủ khoá, bài tập về nhà", homework, courseOwner, false, true)
	check("người lạ, assignment live", live, stranger, false, false)
	check("người lạ, bài tập về nhà", homework, stranger, false, false)
	check("host phiên, bài tập về nhà của lớp (host không phải giảng viên lớp)", homework, host, false, false)
	check("admin", live, admin, true, true)
	check("assignment không gắn phiên/lớp, người thường", orphan, courseOwner, false, false)
	check("assignment không gắn phiên/lớp, admin", orphan, admin, true, true)
}
