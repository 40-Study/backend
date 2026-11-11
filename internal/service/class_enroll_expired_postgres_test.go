package service

// Review #144, phát hiện 3 (chủ dự án chốt 11/11/2026): ghi danh khoá đã HẾT HẠN (expires_at đã qua) không đủ điều kiện
// để được thêm vào lớp gắn khoá đó. expires_at NULL hoặc còn hạn thì được. Học viên đã ở trong lớp từ trước không bị tự
// gỡ khi ghi danh hết hạn. Postgres thật vì luật nằm ở SQL (enrollmentNotExpiredCondition).
// Bỏ điều kiện expires_at ở StudentRepository thì ca "hết hạn" (cả ghi danh lẫn ô gợi ý) ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestClassEnroll_GhiDanhKhoaHetHanBiChan_Postgres(t *testing.T) {
	e := newR5ClassEnv(t)
	ctx := context.Background()
	class := e.classInOrgA(t) // e.student đã ở trong lớp

	active, expired, unlimited := e.user("enroll-active"), e.user("enroll-expired"), e.user("enroll-unlimited")
	sys := model.SystemRole{Name: "STUDENT", Status: "active"}
	mustCreate(t, e.db, &sys)
	for _, u := range []model.User{active, expired, unlimited} {
		mustCreate(t, e.db, &model.UserSystemRole{UserID: u.ID, SystemRoleID: sys.ID, Status: model.UserSystemRoleStatusActive})
	}
	future, past := time.Now().Add(24*time.Hour), time.Now().Add(-time.Hour)
	mustCreate(t, e.db, &model.Enrollment{UserID: active.ID, CourseID: *class.CourseID, ExpiresAt: &future})
	mustCreate(t, e.db, &model.Enrollment{UserID: expired.ID, CourseID: *class.CourseID, ExpiresAt: &past})
	mustCreate(t, e.db, &model.Enrollment{UserID: unlimited.ID, CourseID: *class.CourseID})
	// Người đã ở trong lớp có ghi danh vừa hết hạn: không bị gỡ.
	mustCreate(t, e.db, &model.Enrollment{UserID: e.student.ID, CourseID: *class.CourseID, ExpiresAt: &past})

	svc := NewClassService(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db),
		s4TeacherRepo{teachers: map[uuid.UUID]bool{e.instructor.ID: true}},
		r5StudentRepo{StudentRepositoryInterface: repository.NewStudentRepository(e.db),
			students: map[uuid.UUID]bool{active.ID: true, expired.ID: true, unlimited.ID: true}}, nil).WithAuthorizer(e.checker)

	// Ô gợi ý: chỉ người còn hạn hoặc không giới hạn.
	got, err := svc.SearchEnrollableStudents(ctx, class.ID, e.ownerA.ID, false, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[uuid.UUID]bool{}
	for _, g := range got {
		ids[g.ID] = true
	}
	if !ids[active.ID] || !ids[unlimited.ID] || ids[expired.ID] || len(got) != 2 {
		t.Errorf("ô gợi ý = %v, muốn đúng người còn hạn + không giới hạn, không có người hết hạn", ids)
	}

	enroll := func(u model.User) error {
		_, err := svc.EnrollStudentToClass(ctx, class.ID, e.ownerA.ID, false, dto.EnrollStudentDTO{StudentID: u.ID})
		return err
	}
	if err := enroll(expired); !errors.Is(err, ErrStudentNotEnrolledInCourse) {
		t.Errorf("ghi danh hết hạn: err=%v, muốn ErrStudentNotEnrolledInCourse", err)
	}
	if err := enroll(active); err != nil {
		t.Errorf("ghi danh còn hạn bị chặn: %v", err)
	}
	if err := enroll(unlimited); err != nil {
		t.Errorf("ghi danh không giới hạn (expires_at NULL) bị chặn: %v", err)
	}

	students, err := svc.GetStudentsByClass(ctx, class.ID, e.ownerA.ID, false, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	inClass := map[uuid.UUID]bool{}
	for _, s := range students.Students {
		inClass[s.StudentID] = true
	}
	if students.Total != 3 || !inClass[e.student.ID] || inClass[expired.ID] {
		t.Errorf("lớp có %d học viên %v, muốn 3: người cũ (ghi danh hết hạn) vẫn còn, người hết hạn không được thêm", students.Total, inClass)
	}
}
