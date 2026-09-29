package service

// Lane S5 (Postgres thật), theo từng vai. Bốn route trước đây chỉ cần đăng nhập:
//  1. GET /courses/:id/enrollments: danh sách học viên ghi danh (id, tên, tiến độ) chỉ chủ khoá và admin;
//     khoá đã xuất bản mà người gọi không sở hữu 403, khoá nháp 404.
//  2. GET /classes/:id/teachers (review S4, M-7): thành viên lớp, người quản lý lớp, admin; người ngoài 404.
//  3. GET /groups/:id/members: nhóm SECRET chỉ thành viên xem được; người ngoài 404.
//  4. GET /reports/:id: chỉ người tạo báo cáo hoặc người kiểm duyệt; người khác 404.
// Bỏ kiểm quyền ở hàm tương ứng thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestS5_Enrollment_DanhSachGhiDanhChiChuKhoaVaAdmin(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	owner, teacher, student, admin := f.user("owner"), f.user("teacher"), f.user("student"), f.user("admin")
	published := f.course(owner)
	draft := f.course(owner)
	if err := f.db.Model(&model.Course{}).Where("id = ?", draft.ID).Update("status", model.CourseStatusDraft).Error; err != nil {
		t.Fatal(err)
	}
	for _, c := range []model.Course{published, draft} {
		if err := f.db.Create(&model.Enrollment{UserID: student.ID, CourseID: c.ID, EnrolledAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewEnrollmentService(repository.NewEnrollmentRepository(f.db), repository.NewCourseRepository(f.db), nil, nil)

	cases := []struct {
		name    string
		user    model.User
		isAdmin bool
		course  model.Course
		want    error // nil = được xem
	}{
		{"chủ khoá / khoá đã xuất bản", owner, false, published, nil},
		{"chủ khoá / khoá nháp", owner, false, draft, nil},
		{"admin / khoá nháp", admin, true, draft, nil},
		{"giảng viên khác / khoá đã xuất bản", teacher, false, published, ErrNotCourseOwner},
		{"học viên ghi danh / khoá đã xuất bản", student, false, published, ErrNotCourseOwner},
		{"giảng viên khác / khoá nháp", teacher, false, draft, ErrCourseHidden},
		{"học viên ghi danh / khoá nháp", student, false, draft, ErrCourseHidden},
	}
	for _, c := range cases {
		got, err := svc.GetCourseEnrollments(ctx, c.course.ID, c.user.ID, c.isAdmin, 1, 20)
		if c.want == nil {
			if err != nil || got == nil || got.Total != 1 {
				t.Errorf("%s: err=%v got=%+v, muốn thấy 1 ghi danh", c.name, err, got)
			}
			continue
		}
		if !errors.Is(err, c.want) || got != nil {
			t.Errorf("%s: err=%v, muốn %v và không có dữ liệu", c.name, err, c.want)
		}
	}
	if _, err := svc.GetCourseEnrollments(ctx, uuid.New(), owner.ID, false, 1, 20); !errors.Is(err, ErrCourseHidden) {
		t.Errorf("khoá không tồn tại: err=%v, muốn ErrCourseHidden", err)
	}
}

func TestS5_Class_DanhSachGiangVienChiThanhVienQuanLyVaAdmin(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	for _, a := range s5Actors(e) {
		got, err := e.svc.GetTeachersByClass(ctx, e.class.ID, a.user.ID, a.isAdmin, 1, 20)
		if a.member {
			if err != nil || got == nil {
				t.Errorf("%s: bị chặn nhầm (%v)", a.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrClassNotFound) || got != nil {
			t.Errorf("%s: err=%v, muốn ErrClassNotFound và không có dữ liệu", a.name, err)
		}
	}
	if _, err := e.svc.GetTeachersByClass(ctx, uuid.New(), e.stranger.ID, false, 1, 20); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("lớp không tồn tại: err=%v, muốn ErrClassNotFound", err)
	}
}

func TestS5_Group_DanhSachThanhVienNhomSecretChiThanhVien(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	owner, member, stranger := f.user("gowner"), f.user("gmember"), f.user("gstranger")
	svc := NewGroupService(repository.NewGroupRepository(f.db), repository.NewGroupMemberRepository(f.db), repository.NewGroupJoinRequestRepository(f.db),
		repository.NewConversationRepository(f.db), repository.NewConversationParticipantRepository(f.db))

	mk := func(privacy model.GroupPrivacy) uuid.UUID {
		g := model.Group{Name: "QA-s5", Slug: "qa-s5-" + uuid.NewString(), CreatedBy: owner.ID, MaxMembers: 100, Privacy: privacy}
		if err := f.db.Create(&g).Error; err != nil {
			t.Fatal(err)
		}
		for _, m := range []model.GroupMember{
			{GroupID: g.ID, UserID: owner.ID, Role: model.GroupRoleOwner, Status: model.GroupMemberActive},
			{GroupID: g.ID, UserID: member.ID, Role: model.GroupRoleMember, Status: model.GroupMemberActive},
		} {
			if err := f.db.Create(&m).Error; err != nil {
				t.Fatal(err)
			}
		}
		return g.ID
	}
	secret, public := mk(model.GroupPrivacySecret), mk(model.GroupPrivacyPublic)

	for _, u := range []model.User{owner, member} {
		if got, err := svc.ListMembers(ctx, u.ID, secret, 1, 20); err != nil || got == nil || got.TotalCount != 2 {
			t.Errorf("thành viên nhóm SECRET bị chặn nhầm: err=%v got=%+v", err, got)
		}
	}
	if got, err := svc.ListMembers(ctx, stranger.ID, secret, 1, 20); !errors.Is(err, ErrGroupNotFound) || got != nil {
		t.Errorf("người ngoài nhóm SECRET: err=%v, muốn ErrGroupNotFound và không có dữ liệu", err)
	}
	if got, err := svc.ListMembers(ctx, stranger.ID, public, 1, 20); err != nil || got == nil || got.TotalCount != 2 {
		t.Errorf("nhóm PUBLIC phải giữ nguyên hành vi cũ: err=%v got=%+v", err, got)
	}
	if _, err := svc.ListMembers(ctx, owner.ID, uuid.New(), 1, 20); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("nhóm không tồn tại: err=%v, muốn ErrGroupNotFound", err)
	}
}

func TestS5_Report_ChiNguoiTaoHoacNguoiKiemDuyetDoc(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	reporter, other, moderator := f.user("reporter"), f.user("other"), f.user("mod")
	report := model.Report{ReporterID: reporter.ID, ReportedType: "user", ReportedID: other.ID, Reason: "spam"}
	if err := f.db.Create(&report).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewReportService(repository.NewReportRepository(f.db))

	if got, err := svc.GetReportByID(ctx, report.ID, reporter.ID, false); err != nil || got == nil {
		t.Errorf("người tạo báo cáo bị chặn nhầm: %v", err)
	}
	if got, err := svc.GetReportByID(ctx, report.ID, moderator.ID, true); err != nil || got == nil {
		t.Errorf("người kiểm duyệt bị chặn nhầm: %v", err)
	}
	if got, err := svc.GetReportByID(ctx, report.ID, other.ID, false); !errors.Is(err, ErrReportNotFound) || got != nil {
		t.Errorf("người không liên quan: err=%v, muốn ErrReportNotFound và không có dữ liệu", err)
	}
	if _, err := svc.GetReportByID(ctx, uuid.New(), moderator.ID, true); !errors.Is(err, ErrReportNotFound) {
		t.Errorf("báo cáo không tồn tại: err=%v, muốn ErrReportNotFound", err)
	}
}