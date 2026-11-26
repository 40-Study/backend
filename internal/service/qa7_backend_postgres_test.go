package service

// QA7 backend lane (Postgres THẬT, schema tạm): C14 tên vai trò hệ thống trùng/dành riêng, C15 session_start_time của
// /me/attendances, C16 GET /me/assignments (GetMine).
// Mutation đã kiểm: bỏ isUniqueViolation/OrgRoleNameReserved ở SystemRoleService (C14); bỏ gán SessionStartTime ở GetMyAttendances
// (C15); bỏ điều kiện lớp active, lọc is_published hoặc NOT EXISTS participants.is_kicked ở buildStudentVisibleAssignmentsQuery (C16).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestQA7_C14_SystemRole_TrungTenVaTenDanhRieng_Postgres(t *testing.T) {
	db := isolatedAPISchema(t)
	svc := NewSystemRoleService(repository.NewSystemRoleRepository(db), nil)
	ctx := context.Background()
	name := "QA7_ROLE_" + strings.ToUpper(uuid.NewString()[:8])

	first, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: name}); !errors.Is(err, ErrSystemRoleNameExists) {
		t.Fatalf("tạo trùng tên: err=%v, muốn ErrSystemRoleNameExists", err)
	}
	for _, reserved := range []string{"STUDENT", "teacher", " SYSTEM_ADMIN "} {
		if _, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: reserved}); !errors.Is(err, ErrSystemRoleNameExists) {
			t.Errorf("tạo tên dành riêng %q: err=%v, muốn ErrSystemRoleNameExists", reserved, err)
		}
	}

	// Vai đã xoá mềm vẫn giữ chỗ của unique index: tạo lại tên đó phải là 409, không phải lỗi DB thô.
	other := "QA7_ROLE_" + strings.ToUpper(uuid.NewString()[:8])
	gone, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: other})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSystemRole(ctx, gone.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: other}); !errors.Is(err, ErrSystemRoleNameExists) {
		t.Fatalf("tạo lại tên của vai đã xoá mềm: err=%v, muốn ErrSystemRoleNameExists", err)
	}

	// Đổi tên: sang tên đã có / dành riêng bị chặn; giữ nguyên tên của chính mình thì được.
	second, err := svc.CreateSystemRole(ctx, dto.CreateSystemRoleDTO{Name: name + "_B"})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{name, "PARENT"} {
		if _, err := svc.UpdateSystemRole(ctx, second.ID, dto.UpdateSystemRoleDTO{Name: &target}); !errors.Is(err, ErrSystemRoleNameExists) {
			t.Errorf("đổi sang %q: err=%v, muốn ErrSystemRoleNameExists", target, err)
		}
	}
	same := name + "_B"
	if _, err := svc.UpdateSystemRole(ctx, second.ID, dto.UpdateSystemRoleDTO{Name: &same}); err != nil {
		t.Errorf("giữ nguyên tên của chính mình: %v", err)
	}
	fresh := name + "_C"
	if _, err := svc.UpdateSystemRole(ctx, second.ID, dto.UpdateSystemRoleDTO{Name: &fresh}); err != nil {
		t.Errorf("đổi sang tên mới: %v", err)
	}
	_ = first
}

func TestQA7_C15_MyAttendances_SessionStartTimeLaGioBuoiHoc_Postgres(t *testing.T) {
	e := newS4ClassEnv(t)
	svc := newS5ScheduleSvc(e)
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	sess := model.ClassSession{ClassID: e.class.ID, SessionNumber: 1, Date: day, StartTime: "19:30", EndTime: "21:30", Status: model.SessionCompleted}
	if err := e.f.db.Create(&sess).Error; err != nil {
		t.Fatal(err)
	}
	// Vắng: không có check-in, created_at là lúc giáo viên ghi (khác hẳn giờ buổi học).
	att := model.SessionAttendance{SessionID: sess.ID, StudentID: e.student.ID, Status: model.AttendanceAbsent}
	if err := e.f.db.Create(&att).Error; err != nil {
		t.Fatal(err)
	}

	rows, _, err := svc.GetMyAttendances(context.Background(), e.student.ID, 1, 20)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if rows[0].SessionStartTime == nil {
		t.Fatal("thiếu session_start_time")
	}
	if got := rows[0].SessionStartTime.Format(time.RFC3339); got != "2026-11-10T19:30:00+07:00" {
		t.Fatalf("session_start_time = %s, muốn 2026-11-10T19:30:00+07:00 (giờ bắt đầu theo lịch, múi giờ Việt Nam)", got)
	}
	raw, _ := json.Marshal(rows[0])
	if !strings.Contains(string(raw), `"session_start_time":"2026-11-10T19:30:00+07:00"`) {
		t.Fatalf("JSON không mang session_start_time RFC3339: %s", raw)
	}
}

func TestQA7_C16_GetMineAssignments_HocVienThayMoiLopVaPhienDangHoc_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	me, host := f.user("c16-me"), f.user("c16-host")
	af := &assignmentsFx{t: t, db: f.db, child: me}

	active := af.class("C16 đang học", "active")
	active2 := af.class("C16 đang học 2", "active")
	dropped := af.class("C16 đã rời", "dropped")
	foreign := af.class("C16 không tham gia", "")

	session := func(class model.Class) model.LivestreamSession {
		s := model.LivestreamSession{Title: "s", HostID: host.ID, ClassID: class.ID, RoomName: "c16-" + uuid.NewString()[:8]}
		if err := f.db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	live := func(s model.LivestreamSession, title string, published bool) model.Assignment {
		a := model.Assignment{Title: title, Description: "d", Language: pq.StringArray{"python"}, SessionID: &s.ID, IsPublished: published}
		if err := f.db.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
		if !published {
			f.db.Model(&a).Update("is_published", false)
		}
		return a
	}
	sActive, sActive2, sKicked, sDropped, sForeign := session(active), session(active2), session(active), session(dropped), session(foreign)
	if err := f.db.Create(&model.Participant{SessionID: sKicked.ID, UserID: me.ID, Role: model.ParticipantRole("viewer"), IsKicked: true}).Error; err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{ // true = học viên PHẢI thấy
		"class-hw-published":       true,
		"class2-hw-published":      true,
		"session-published":        true,
		"session2-published":       true,
		"class-hw-draft":           false,
		"dropped-class-hw":         false,
		"foreign-class-hw":         false,
		"session-draft":            false,
		"kicked-session-published": false,
		"dropped-session":          false,
		"foreign-session":          false,
	}
	hw := func(c model.Class, title string, published bool) { af.assignment(c, title, nil, published) }
	hw(active, "class-hw-published", true)
	hw(active2, "class2-hw-published", true)
	hw(active, "class-hw-draft", false)
	hw(dropped, "dropped-class-hw", true)
	hw(foreign, "foreign-class-hw", true)
	live(sActive, "session-published", true)
	live(sActive2, "session2-published", true)
	live(sActive, "session-draft", false)
	live(sKicked, "kicked-session-published", true)
	live(sDropped, "dropped-session", true)
	live(sForeign, "foreign-session", true)

	mineA := af.assignment(active, "class-hw-with-sub", nil, true)
	want["class-hw-with-sub"] = true
	af.submit(me.ID, mineA, model.VerdictAccepted, 4, 4)

	svc := NewAssignmentService(repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db),
		repository.NewSubmissionRepository(f.db), repository.NewClassRepository(f.db), nil)

	all, err := svc.GetMine(ctx, me.ID, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]dto.AssignmentResponseDTO{}
	for _, a := range all.Data {
		got[a.Title] = a
	}
	for title, visible := range want {
		if _, ok := got[title]; ok != visible {
			t.Errorf("%q: thấy=%v, muốn %v", title, ok, visible)
		}
	}
	if int(all.Total) != len(all.Data) || len(all.Data) != 5 {
		t.Errorf("total=%d len=%d, muốn 5 bài hiển thị", all.Total, len(all.Data))
	}
	if sub := got["class-hw-with-sub"].MySubmission; sub == nil || sub.Status != "submitted" {
		t.Errorf("my_submission = %+v, muốn đã nộp", sub)
	}
	if got["class-hw-published"].MySubmission != nil {
		t.Error("bài chưa nộp phải có my_submission null")
	}

	// Người khác (không học lớp nào) không thấy gì.
	if none, err := svc.GetMine(ctx, host.ID, 1, 100); err != nil || len(none.Data) != 0 || none.Total != 0 {
		t.Errorf("người không học lớp nào: %+v err=%v, muốn rỗng", none, err)
	}

	// Phân trang ổn định, không trùng/không sót giữa các trang.
	seen := map[uuid.UUID]bool{}
	for page := 1; page <= 3; page++ {
		p, err := svc.GetMine(ctx, me.ID, page, 2)
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 5 || p.Page != page || p.PageSize != 2 {
			t.Fatalf("trang %d: %+v", page, p)
		}
		for _, a := range p.Data {
			if seen[a.ID] {
				t.Errorf("bài %s xuất hiện ở hai trang", a.ID)
			}
			seen[a.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("duyệt qua các trang thấy %d bài, muốn 5", len(seen))
	}
}
