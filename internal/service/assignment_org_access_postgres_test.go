package service

// R4: chủ/quản trị tổ chức chấm được điểm lớp của tổ chức (L2) nhưng màn chấm còn cần đọc bài tập và liệt kê bài
// nộp, hai việc vốn chỉ dành cho host/giảng viên lớp/chủ khoá/admin. Test này khoá:
//   - chủ tổ chức A của lớp thuộc A: xem được bài tập (kể cả bản nháp) và bài nộp, nhưng CanManage vẫn false
//     (không được sửa/publish/xem test ẩn);
//   - thành viên A không có quyền quản trị, chủ tổ chức B, chủ A đã bị gỡ role, chủ A với lớp CÁ NHÂN, học viên,
//     người lạ: không xem được.
// Bỏ nhánh orgManagesAssignmentClass thì ca "chủ tổ chức A" ĐỎ; nới CanManage thay vì CanView thì ca CanManage ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestAssignmentOrgAccess_OwnerReadsButCannotManage(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()

	assignmentSvc := NewAssignmentService(repository.NewAssignmentRepository(e.db), repository.NewTestCaseRepository(e.db), nil,
		repository.NewClassRepository(e.db), nil)
	assignmentSvc.SetOrgAccess(NewClassOrgAccess(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db), e.checker))
	submissionSvc := NewSubmissionService(repository.NewSubmissionRepository(e.db), assignmentSvc, repository.NewTestCaseRepository(e.db), nil, &config.Config{})
	submissionSvc.SetOrgAccess(NewClassOrgAccess(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db), e.checker))

	mk := func(class model.Class, published bool) model.Assignment {
		a := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "QA-R4 " + uuid.NewString()[:6], Description: "d", Language: []string{"python"}, IsPublished: published}
		mustCreate(t, e.db, &a)
		mustCreate(t, e.db, &model.Submission{AssignmentID: a.ID, UserID: e.student.ID, Language: "python", Code: "print(1)"})
		return a
	}
	inA := mk(e.classInOrgA(t), false) // bản nháp: chỉ người quản lý/tổ chức mới thấy
	personal := mk(e.personalClassOfOrgMember(t), true)

	cases := []struct {
		name       string
		actor      model.User
		assignment model.Assignment
		wantRead   bool
	}{
		{"chủ tổ chức A, lớp thuộc A", e.ownerA, inA, true},
		{"giảng viên được gán vào lớp (đã có từ trước)", e.coTeacher, inA, true},
		{"thành viên A không quản trị", e.memberA, inA, false},
		{"chủ tổ chức B", e.ownerB, inA, false},
		{"chủ A đã bị gỡ role", e.revokedOwnerA, inA, false},
		{"người lạ", e.stranger, inA, false},
		{"chủ tổ chức A nhưng lớp CÁ NHÂN", e.ownerA, personal, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			canView, err := assignmentSvc.CanView(ctx, c.assignment.ID, c.actor.ID, false)
			if err != nil || canView != c.wantRead {
				t.Errorf("CanView=%v err=%v, muốn %v", canView, err, c.wantRead)
			}
			list, err := submissionSvc.GetByAssignment(ctx, c.assignment.ID, c.actor.ID, false, 1, 20)
			if c.wantRead {
				if err != nil || list == nil || len(list.Data) != 1 {
					t.Errorf("GetByAssignment: %+v err=%v, muốn đúng 1 bài nộp", list, err)
				}
			} else if !errors.Is(err, ErrSubmissionForbidden) {
				t.Errorf("GetByAssignment err=%v, muốn ErrSubmissionForbidden", err)
			}
		})
	}

	// Quyền đọc không kéo theo quyền quản lý: chủ tổ chức không được sửa/publish/xem test ẩn.
	if manage, err := assignmentSvc.CanManage(ctx, inA.ID, e.ownerA.ID, false); err != nil || manage {
		t.Errorf("CanManage của chủ tổ chức=%v err=%v, muốn false", manage, err)
	}
}

// GET /classes/:id/assignments (R4): quản lý lớp (kể cả chủ/quản trị tổ chức của lớp) thấy cả bản nháp, học viên đang
// học chỉ thấy bản đã công bố, người ngoài và lớp không tồn tại đều là ErrClassNotFound (404, không lộ lớp tồn tại).
func TestAssignmentOrgAccess_ListByClass(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()
	classRepo := repository.NewClassRepository(e.db)
	svc := NewAssignmentService(repository.NewAssignmentRepository(e.db), repository.NewTestCaseRepository(e.db), nil, classRepo, nil)
	svc.SetOrgAccess(NewClassOrgAccess(classRepo, repository.NewCourseRepository(e.db), e.checker))

	inA, personal := e.classInOrgA(t), e.personalClassOfOrgMember(t)
	for _, c := range []struct {
		class     model.Class
		published bool
	}{{inA, true}, {inA, false}, {personal, true}} {
		mustCreate(t, e.db, &model.Assignment{ClassID: &c.class.ID, Type: "homework", Title: "QA-R4 " + uuid.NewString()[:6], Description: "d", Language: []string{"python"}, IsPublished: c.published})
	}

	cases := []struct {
		name  string
		actor model.User
		class model.Class
		want  int // số bài thấy được; -1 = ErrClassNotFound
	}{
		{"chủ tổ chức A, lớp thuộc A: cả bản nháp", e.ownerA, inA, 2},
		{"giảng viên được gán vào lớp: cả bản nháp", e.coTeacher, inA, 2},
		{"chủ khoá của lớp: cả bản nháp", e.instructor, inA, 2},
		{"học viên trong lớp: chỉ bản đã công bố", e.student, inA, 1},
		{"thành viên A không quản trị", e.memberA, inA, -1},
		{"chủ tổ chức B: lớp của tổ chức khác", e.ownerB, inA, -1},
		{"chủ A đã bị gỡ role", e.revokedOwnerA, inA, -1},
		{"người lạ", e.stranger, inA, -1},
		{"chủ tổ chức A nhưng lớp CÁ NHÂN", e.ownerA, personal, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := svc.GetByClass(ctx, c.actor.ID, false, c.class.ID, 1, 20)
			if c.want < 0 {
				if !errors.Is(err, ErrClassNotFound) {
					t.Fatalf("err=%v, muốn ErrClassNotFound", err)
				}
				return
			}
			if err != nil || res == nil || len(res.Data) != c.want || res.Total != int64(c.want) {
				t.Fatalf("res=%+v err=%v, muốn %d bài", res, err, c.want)
			}
		})
	}

	if _, err := svc.GetByClass(ctx, e.stranger.ID, false, uuid.New(), 1, 20); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("lớp không tồn tại: err=%v, muốn ErrClassNotFound", err)
	}
	if res, err := svc.GetByClass(ctx, e.stranger.ID, true, inA.ID, 1, 20); err != nil || len(res.Data) != 2 {
		t.Errorf("admin hệ thống: %+v err=%v, muốn thấy 2 bài", res, err)
	}
}
