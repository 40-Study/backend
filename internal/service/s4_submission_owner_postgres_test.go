package service

// Lane S4, lỗi O-2 và M-1 của review S3 (Postgres thật).
//
// O-2: quyền ĐỌC bài nộp của một assignment phải là đúng CanManage của assignment (host phiên, giảng
// viên lớp, giảng viên chủ khoá, admin), không giữ một định nghĩa "chủ" riêng. Trước đây đọc bài nộp
// tra class_sessions (assignment live lại trỏ livestream_sessions) nên host, giảng viên lớp và chủ khoá
// đều bị 403 dù sửa được chính assignment đó. Đổi canManageAssignment về định nghĩa riêng thì test ĐỎ.
//
// M-1: AssignmentService.isSessionMember không được so chuỗi thông điệp; test dùng LivestreamService
// THẬT cho các nhánh ErrParticipantKicked, ErrNotSessionMember và ErrSessionNotFound.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type s4SubmissionEnv struct {
	f                                                              *s2Fixture
	host, classTeacher, courseOwner, student, otherStudent, stranger, admin model.User
	class                                                          model.Class
	session                                                        model.LivestreamSession
	live, hw                                                       model.Assignment
	liveSub, hwSub                                                 model.Submission
	lsSvc                                                          *LivestreamService
	assignmentSvc                                                  *AssignmentService
}

func newS4SubmissionEnv(t *testing.T) *s4SubmissionEnv {
	f := newS2Fixture(t)
	e := &s4SubmissionEnv{f: f}
	e.host, e.classTeacher, e.courseOwner = f.user("host"), f.user("class-teacher"), f.user("course-owner")
	e.student, e.otherStudent, e.stranger, e.admin = f.user("student"), f.user("other-student"), f.user("stranger"), f.user("admin")
	course := f.course(e.courseOwner)
	e.class = model.Class{Name: "QA-s4-sub", CourseID: &course.ID, Status: "active"}
	must := func(v any) {
		t.Helper()
		if err := f.db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	must(&e.class)
	must(&model.TeacherClass{TeacherID: e.classTeacher.ID, ClassID: e.class.ID, Role: "primary"})
	for _, s := range []model.User{e.student, e.otherStudent} {
		must(&model.StudentClass{StudentID: s.ID, ClassID: e.class.ID, Status: "active"})
	}
	e.session = model.LivestreamSession{Title: "s", HostID: e.host.ID, ClassID: e.class.ID, CourseID: &course.ID, RoomName: "room-" + uuid.NewString()[:8]}
	must(&e.session)
	e.live = model.Assignment{SessionID: &e.session.ID, Title: "live", Description: "d", Language: []string{"python"}, IsPublished: true}
	e.hw = model.Assignment{ClassID: &e.class.ID, Type: "homework", Title: "hw", Description: "d", Language: []string{"python"}, IsPublished: true}
	must(&e.live)
	must(&e.hw)
	e.liveSub = model.Submission{AssignmentID: e.live.ID, UserID: e.student.ID, Language: "python", Code: "print(1)"}
	e.hwSub = model.Submission{AssignmentID: e.hw.ID, UserID: e.student.ID, Language: "python", Code: "print(1)"}
	must(&e.liveSub)
	must(&e.hwSub)

	classRepo, courseRepo := repository.NewClassRepository(f.db), repository.NewCourseRepository(f.db)
	e.lsSvc = NewLivestreamService(repository.NewLivestreamRepository(f.db), repository.NewParticipantRepository(f.db),
		repository.NewAnalyticsRepository(f.db), classRepo, courseRepo, repository.NewEnrollmentRepository(f.db), nil, nil, nil, nil)
	e.assignmentSvc = NewAssignmentService(repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db), nil, classRepo, e.lsSvc)
	return e
}

// W2-A: hoc vien trong lop xem duoc bai tap nen bi tu choi bang 403 (ErrSubmissionForbidden); nguoi khong xem duoc bai
// tap nhan 404 (ErrAssignmentNotFound), khong do duoc id.
func (e *s4SubmissionEnv) deniedErr(u model.User) error {
	if u.ID == e.student.ID || u.ID == e.otherStudent.ID {
		return ErrSubmissionForbidden
	}
	return ErrAssignmentNotFound
}

func TestS4_Submission_QuyenDocBaiNopLaCanManageCuaAssignment(t *testing.T) {
	e := newS4SubmissionEnv(t)
	ctx := context.Background()
	svc := &SubmissionService{repo: repository.NewSubmissionRepository(e.f.db), assignmentSvc: e.assignmentSvc}

	type actor struct {
		name    string
		user    model.User
		isAdmin bool
		live    bool // được đọc bài nộp của assignment live
		hw      bool // được đọc bài nộp của bài tập về nhà
	}
	actors := []actor{
		{"host phiên", e.host, false, true, false},
		{"giảng viên lớp", e.classTeacher, false, true, true},
		{"giảng viên chủ khoá (không thuộc teacher_classes)", e.courseOwner, false, true, true},
		{"admin", e.admin, true, true, true},
		{"học viên nộp bài (không phải chủ assignment)", e.student, false, false, false},
		{"học viên khác trong lớp", e.otherStudent, false, false, false},
		{"người lạ", e.stranger, false, false, false},
	}
	for _, a := range actors {
		for _, tc := range []struct {
			what string
			as   model.Assignment
			want bool
		}{{"assignment live", e.live, a.live}, {"bài tập về nhà", e.hw, a.hw}} {
			_, err := svc.GetByAssignment(ctx, tc.as.ID, a.user.ID, a.isAdmin, 1, 20)
			switch {
			case tc.want && err != nil:
				t.Errorf("GetByAssignment / %s / %s: bị chặn nhầm (%v)", a.name, tc.what, err)
			case !tc.want && !errors.Is(err, e.deniedErr(a.user)):
				t.Errorf("GetByAssignment / %s / %s: err=%v, muon %v", a.name, tc.what, err, e.deniedErr(a.user))
			}
		}
	}

	// Đọc MỘT bài nộp của người khác: chính chủ luôn được; còn lại theo đúng CanManage.
	for _, a := range actors {
		for _, tc := range []struct {
			what string
			sub  model.Submission
			want bool
		}{{"bài nộp assignment live", e.liveSub, a.live}, {"bài nộp bài tập về nhà", e.hwSub, a.hw}} {
			want := tc.want || a.user.ID == e.student.ID // học viên nộp bài đọc lại bài của chính mình
			got, err := svc.GetByID(ctx, tc.sub.ID, a.user.ID, a.isAdmin)
			switch {
			case want && (err != nil || got == nil):
				t.Errorf("GetByID / %s / %s: bị chặn nhầm (%v)", a.name, tc.what, err)
			case !want && !errors.Is(err, e.deniedErr(a.user)):
				t.Errorf("GetByID / %s / %s: err=%v, muon %v", a.name, tc.what, err, e.deniedErr(a.user))
			}
		}
	}
}

func TestS4_Analytics_AssignmentDungCanManage(t *testing.T) {
	e := newS4SubmissionEnv(t)
	ctx := context.Background()
	f := e.f
	svc := NewAnalyticsService(repository.NewAnalyticsRepository(f.db), repository.NewParticipantRepository(f.db), repository.NewSubmissionRepository(f.db),
		repository.NewAssignmentRepository(f.db), repository.NewLivestreamRepository(f.db), repository.NewClassRepository(f.db), repository.NewCourseRepository(f.db))
	for _, tc := range []struct {
		name    string
		user    model.User
		isAdmin bool
		want    bool
	}{
		{"host phiên", e.host, false, true},
		{"giảng viên lớp", e.classTeacher, false, true},
		{"chủ khoá", e.courseOwner, false, true},
		{"admin", e.admin, true, true},
		{"học viên", e.student, false, false},
		{"người lạ", e.stranger, false, false},
	} {
		_, err := svc.GetAssignmentAnalytics(ctx, e.live.ID, tc.user.ID, tc.isAdmin)
		switch {
		case tc.want && err != nil:
			t.Errorf("%s: bị chặn nhầm (%v)", tc.name, err)
		case !tc.want && !errors.Is(err, ErrNotAnalyticsOwner):
			t.Errorf("%s: err=%v, muốn ErrNotAnalyticsOwner", tc.name, err)
		}
	}
}

// M-1: cổng thành viên phiên THẬT. Người bị kick, người ngoài và phiên không còn phải cho ra
// "không xem được" (false, không lỗi), không phải 500; và sentinel phải phân biệt được bằng errors.Is.
func TestS4_SessionGateThat_KickNguoiNgoaiVaPhienBiXoa(t *testing.T) {
	e := newS4SubmissionEnv(t)
	ctx := context.Background()

	if err := e.lsSvc.EnsureSessionMember(ctx, e.session.ID, e.student.ID); err != nil {
		t.Fatalf("học viên trong lớp phải là thành viên phiên, nhận %v", err)
	}
	if err := e.lsSvc.EnsureSessionMember(ctx, e.session.ID, e.stranger.ID); !errors.Is(err, ErrNotSessionMember) {
		t.Fatalf("người ngoài: err=%v, muốn ErrNotSessionMember", err)
	}
	if err := e.lsSvc.EnsureSessionMember(ctx, uuid.New(), e.student.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("phiên không tồn tại: err=%v, muốn ErrSessionNotFound", err)
	}

	view := func(name string, u model.User, want bool) {
		t.Helper()
		got, err := e.assignmentSvc.CanView(ctx, e.live.ID, u.ID, false)
		if err != nil || got != want {
			t.Errorf("%s: CanView=%v (err=%v), muốn %v và không lỗi", name, got, err, want)
		}
	}
	view("học viên trong lớp", e.student, true)
	view("người ngoài", e.stranger, false)

	// Bị kick khỏi phiên: mất quyền xem đề live (ErrParticipantKicked -> false, không lỗi).
	if err := e.f.db.Create(&model.Participant{SessionID: e.session.ID, UserID: e.otherStudent.ID, IsKicked: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.lsSvc.EnsureSessionMember(ctx, e.session.ID, e.otherStudent.ID); !errors.Is(err, ErrParticipantKicked) {
		t.Fatalf("học viên bị kick: err=%v, muốn ErrParticipantKicked", err)
	}
	view("học viên bị kick", e.otherStudent, false)

	// Phiên bị xoá mềm: assignment live không còn nguồn thành viên -> không xem được, không phải 500.
	if err := e.f.db.Delete(&e.session).Error; err != nil {
		t.Fatal(err)
	}
	view("học viên, phiên đã xoá", e.student, false)
}
