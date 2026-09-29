package service

// Lane S3, lỗi 1 (tầng service, Postgres thật): ai được TẠO assignment vào một phiên/lớp, ai XEM
// được một assignment, ai liệt kê được assignment của một phiên. Bốn vai: học viên trong lớp, giảng
// viên khác (không dính gì), chủ khoá, admin. Nới điều kiện ở AssignmentRepository.CanManageTarget,
// AssignmentService.CanView hoặc Create thì test này ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// s3SessionGate giả EnsureSessionMember: chỉ những id trong members là thành viên phiên. Phần
// "thành viên phiên là gì" đã có test riêng của LivestreamService; ở đây chỉ cần biết AssignmentService
// hỏi đúng cổng đó và tôn trọng câu trả lời.
type s3SessionGate map[uuid.UUID]bool

func (g s3SessionGate) EnsureSessionMember(_ context.Context, _, userID uuid.UUID) error {
	if g[userID] {
		return nil
	}
	return ErrNotSessionMember
}

func TestS3_Assignment_TaoXemLietKe(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	host, classTeacher, courseOwner := f.user("host"), f.user("class-teacher"), f.user("course-owner")
	student, outsider, otherTeacher, admin := f.user("student"), f.user("outsider"), f.user("other-teacher"), f.user("admin")

	course := f.course(courseOwner)
	class := model.Class{Name: "QA-s3", CourseID: &course.ID, Status: "active"}
	if err := f.db.Create(&class).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.TeacherClass{TeacherID: classTeacher.ID, ClassID: class.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	// Lớp và phiên của một khoá KHÁC, để thử "trộn lớp của mình với phiên của người khác".
	foreignCourse := f.course(otherTeacher)
	foreignClass := model.Class{Name: "QA-s3-foreign", CourseID: &foreignCourse.ID, Status: "active"}
	if err := f.db.Create(&foreignClass).Error; err != nil {
		t.Fatal(err)
	}
	session := model.LivestreamSession{Title: "s", HostID: host.ID, ClassID: class.ID, RoomName: "room-" + uuid.NewString()[:8]}
	if err := f.db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}

	newAssignment := func(a model.Assignment) model.Assignment {
		t.Helper()
		a.Title, a.Description, a.Language = "t", "d", []string{"python"}
		if err := f.db.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
		return a
	}
	liveDraft := newAssignment(model.Assignment{SessionID: &session.ID})
	livePublished := newAssignment(model.Assignment{SessionID: &session.ID, IsPublished: true})
	hwDraft := newAssignment(model.Assignment{ClassID: &class.ID, Type: "homework"})
	hwPublished := newAssignment(model.Assignment{ClassID: &class.ID, Type: "homework", IsPublished: true})

	svc := NewAssignmentService(
		repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db), nil,
		repository.NewClassRepository(f.db),
		s3SessionGate{student.ID: true}, // học viên là thành viên phiên; outsider thì không
	)

	t.Run("CanView", func(t *testing.T) {
		check := func(name string, a model.Assignment, u model.User, isAdmin, want bool) {
			t.Helper()
			got, err := svc.CanView(ctx, a.ID, u.ID, isAdmin)
			if err != nil || got != want {
				t.Errorf("%s: CanView=%v (err=%v), muốn %v", name, got, err, want)
			}
		}
		check("học viên trong lớp, bài tập về nhà đã publish", hwPublished, student, false, true)
		check("học viên trong lớp, bài tập về nhà còn nháp", hwDraft, student, false, false)
		check("học viên thành viên phiên, live đã publish", livePublished, student, false, true)
		check("học viên thành viên phiên, live còn nháp", liveDraft, student, false, false)
		check("người ngoài, bài tập về nhà đã publish", hwPublished, outsider, false, false)
		check("người ngoài, live đã publish", livePublished, outsider, false, false)
		check("giảng viên khác, bài tập về nhà đã publish", hwPublished, otherTeacher, false, false)
		check("giảng viên lớp, bản nháp", hwDraft, classTeacher, false, true)
		check("chủ khoá, bản nháp live", liveDraft, courseOwner, false, true)
		check("host phiên, bản nháp live", liveDraft, host, false, true)
		check("admin, bản nháp", hwDraft, admin, true, true)
		if got, err := svc.CanView(ctx, uuid.New(), admin.ID, true); err != nil || got {
			t.Errorf("id không tồn tại: CanView=%v (err=%v), muốn false kể cả admin", got, err)
		}
	})

	t.Run("Create", func(t *testing.T) {
		req := func(sessionID, classID string) dto.CreateAssignmentDTO {
			return dto.CreateAssignmentDTO{SessionID: sessionID, ClassID: classID, Title: "new", Description: "d", Language: []string{"python"}}
		}
		check := func(name string, u model.User, isAdmin bool, r dto.CreateAssignmentDTO, want error) {
			t.Helper()
			a, err := svc.Create(ctx, u.ID, isAdmin, r)
			if !errors.Is(err, want) {
				t.Errorf("%s: err=%v, muốn %v", name, err, want)
			}
			if want == nil && a == nil {
				t.Errorf("%s: không trả assignment", name)
			}
		}
		sid, cid, fcid := session.ID.String(), class.ID.String(), foreignClass.ID.String()
		check("chủ khoá tạo vào phiên", courseOwner, false, req(sid, ""), nil)
		check("host phiên tạo vào phiên", host, false, req(sid, ""), nil)
		check("giảng viên lớp tạo bài tập về nhà", classTeacher, false, req("", cid), nil)
		check("admin tạo vào phiên", admin, true, req(sid, ""), nil)
		check("admin tạo không gắn đâu", admin, true, req("", ""), nil)
		check("học viên tạo vào phiên", student, false, req(sid, ""), ErrAssignmentForbidden)
		check("học viên tạo bài tập về nhà", student, false, req("", cid), ErrAssignmentForbidden)
		check("giảng viên khác tạo vào phiên", otherTeacher, false, req(sid, ""), ErrAssignmentForbidden)
		check("giảng viên khác tạo bài tập về nhà cho lớp này", otherTeacher, false, req("", cid), ErrAssignmentForbidden)
		check("người thường không gắn vào đâu", courseOwner, false, req("", ""), ErrAssignmentTargetRequired)
		// Trộn: lớp của chính otherTeacher (khoá của họ) + phiên của người khác. Mỗi nơi phải qua kiểm riêng.
		check("giảng viên khác trộn lớp của mình với phiên của người khác", otherTeacher, false, req(sid, fcid), ErrAssignmentForbidden)
	})

	t.Run("GetBySession", func(t *testing.T) {
		count := func(u model.User, isAdmin bool) (int, error) {
			res, err := svc.GetBySession(ctx, u.ID, isAdmin, session.ID, 1, 50)
			if err != nil {
				return 0, err
			}
			return len(res.Data), nil
		}
		// Chủ thấy cả nháp lẫn đã publish (liveDraft + livePublished + 2 bản Create ở trên ghi vào phiên).
		if n, err := count(courseOwner, false); err != nil || n < 2 {
			t.Errorf("chủ khoá: n=%d err=%v, muốn thấy ít nhất cả nháp lẫn đã publish", n, err)
		}
		if n, err := count(admin, true); err != nil || n < 2 {
			t.Errorf("admin: n=%d err=%v", n, err)
		}
		// Thành viên phiên chỉ thấy đã publish: đúng 1 (livePublished).
		if n, err := count(student, false); err != nil || n != 1 {
			t.Errorf("học viên thành viên phiên: n=%d err=%v, muốn đúng 1 bản đã publish", n, err)
		}
		if _, err := count(outsider, false); !errors.Is(err, ErrAssignmentNotFound) {
			t.Errorf("người ngoài phiên: err=%v, muốn ErrAssignmentNotFound", err)
		}
		if _, err := count(otherTeacher, false); !errors.Is(err, ErrAssignmentNotFound) {
			t.Errorf("giảng viên khác: err=%v, muốn ErrAssignmentNotFound", err)
		}
	})
}
