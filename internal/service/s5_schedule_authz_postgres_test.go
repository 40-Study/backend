package service

// Lane S5 (Postgres thật), theo từng vai: giảng viên lạ, học viên trong lớp, giảng viên lớp, chủ khoá, người
// tạo lớp, admin. Lịch học và buổi học của lớp trước đây KHÔNG kiểm quyền: học viên bất kỳ tạo, sửa, xoá,
// huỷ buổi và đọc lịch của lớp bất kỳ. Bỏ requireClassWrite/requireClassRead ở một hàm nào dưới đây thì test ĐỎ.
//
//  - ghi (lịch, buổi, sinh buổi, điểm danh buổi): người quản lý lớp và admin; thành viên lớp 403; người ngoài 404.
//  - đọc lịch, buổi, thời khoá biểu: thành viên lớp, người quản lý lớp, admin; người ngoài 404.
//  - danh sách điểm danh buổi: chỉ người quản lý lớp và admin; người khác (kể cả học viên) 404.
//  - quyền xét trên lớp THẬT của bản ghi: đặt id lịch/buổi của lớp khác vào là 404.

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

type s5Actor struct {
	name    string
	user    model.User
	isAdmin bool
	manager bool // quản lý lớp (ghi được)
	member  bool // đọc được lịch (thành viên hoặc quản lý)
}

func s5Actors(e *s4ClassEnv) []s5Actor {
	return []s5Actor{
		{"giảng viên lạ", e.stranger, false, false, false},
		{"học viên trong lớp", e.student, false, false, true},
		{"giảng viên lớp", e.coTeacher, false, true, true},
		{"chủ khoá", e.owner, false, true, true},
		{"người tạo lớp", e.creator, false, true, true},
		{"admin", e.admin, true, true, true},
	}
}

func newS5ScheduleSvc(e *s4ClassEnv) *ScheduleService {
	return NewScheduleService(repository.NewScheduleRepository(e.f.db), repository.NewClassRepository(e.f.db), repository.NewCourseRepository(e.f.db), nil, nil)
}

func s5MakeSchedule(t *testing.T, e *s4ClassEnv, classID uuid.UUID) model.ClassSchedule {
	t.Helper()
	sch := model.ClassSchedule{ClassID: classID, DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", IsActive: true, EffectiveFrom: time.Now()}
	if err := e.f.db.Create(&sch).Error; err != nil {
		t.Fatal(err)
	}
	return sch
}

func s5MakeSession(t *testing.T, e *s4ClassEnv, classID uuid.UUID, n int) model.ClassSession {
	t.Helper()
	s := model.ClassSession{ClassID: classID, SessionNumber: n, Date: time.Now().AddDate(0, 0, n), StartTime: "08:00", EndTime: "09:00", Status: model.SessionScheduled}
	if err := e.f.db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	return s
}

// s5WantWrite: manager thì phải qua (err == nil), thành viên lớp 403, người ngoài 404.
func s5WantWrite(t *testing.T, what string, a s5Actor, err error) {
	t.Helper()
	switch {
	case a.manager && err != nil:
		t.Errorf("%s / %s: bị chặn nhầm (%v)", what, a.name, err)
	case !a.manager && a.member && !errors.Is(err, ErrNotClassTeacher):
		t.Errorf("%s / %s: err=%v, muốn ErrNotClassTeacher (403)", what, a.name, err)
	case !a.manager && !a.member && !errors.Is(err, ErrClassNotFound):
		t.Errorf("%s / %s: err=%v, muốn ErrClassNotFound (404)", what, a.name, err)
	}
}

func TestS5_Schedule_GhiLichVaBuoiChiNguoiQuanLyLop(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	var schedules, sessions int64
	count := func() { e.f.db.Model(&model.ClassSchedule{}).Where("class_id = ?", e.class.ID).Count(&schedules); e.f.db.Model(&model.ClassSession{}).Where("class_id = ?", e.class.ID).Count(&sessions) }

	for i, a := range s5Actors(e) {
		count()
		beforeSch, beforeSes := schedules, sessions

		_, err := svc.CreateSchedule(ctx, e.class.ID, a.user.ID, a.isAdmin, dto.CreateClassScheduleDTO{DayOfWeek: 2, StartTime: "10:00", EndTime: "11:00", EffectiveFrom: "2026-01-01"})
		s5WantWrite(t, "CreateSchedule", a, err)
		_, err2 := svc.CreateSession(ctx, e.class.ID, a.user.ID, a.isAdmin, dto.CreateClassSessionDTO{Date: time.Date(2026, 10, 10+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), StartTime: "10:00", EndTime: "11:00"}) // mỗi vai một ngày riêng: buổi trùng giờ giờ bị 409 (B-10)
		s5WantWrite(t, "CreateSession", a, err2)
		count()
		if !a.manager && (schedules != beforeSch || sessions != beforeSes) {
			t.Errorf("%s: dữ liệu vẫn được tạo dù không có quyền", a.name)
		}

		sch := s5MakeSchedule(t, e, e.class.ID)
		room := "hack"
		_, err = svc.UpdateSchedule(ctx, sch.ID, a.user.ID, a.isAdmin, dto.UpdateClassScheduleDTO{Room: &room})
		s5WantWrite(t, "UpdateSchedule", a, err)
		var gotSch model.ClassSchedule
		e.f.db.First(&gotSch, "id = ?", sch.ID)
		if !a.manager && gotSch.Room != nil {
			t.Errorf("UpdateSchedule / %s: lịch bị sửa dù không có quyền", a.name)
		}
		err = svc.DeleteSchedule(ctx, sch.ID, a.user.ID, a.isAdmin)
		s5WantWrite(t, "DeleteSchedule", a, err)
		var left int64
		e.f.db.Model(&model.ClassSchedule{}).Where("id = ?", sch.ID).Count(&left)
		if !a.manager && left != 1 {
			t.Errorf("DeleteSchedule / %s: lịch bị xoá dù không có quyền", a.name)
		}

		ses := s5MakeSession(t, e, e.class.ID, 100+i)
		topic := "hack"
		_, err = svc.UpdateSession(ctx, ses.ID, a.user.ID, a.isAdmin, dto.UpdateClassSessionDTO{Topic: &topic})
		s5WantWrite(t, "UpdateSession", a, err)
		err = svc.CancelSession(ctx, ses.ID, a.user.ID, a.isAdmin, "x")
		s5WantWrite(t, "CancelSession", a, err)
		var gotSes model.ClassSession
		e.f.db.First(&gotSes, "id = ?", ses.ID)
		if !a.manager && (gotSes.Status != model.SessionScheduled || gotSes.Topic != nil) {
			t.Errorf("Update/CancelSession / %s: buổi bị đổi dù không có quyền (status=%s)", a.name, gotSes.Status)
		}

		gen := s5MakeSchedule(t, e, e.class.ID) // để GenerateSessions có lịch
		_ = gen
		_, err = svc.GenerateSessions(ctx, e.class.ID, a.user.ID, a.isAdmin, dto.GenerateSessionsDTO{StartDate: "2030-01-01", EndDate: "2030-01-07"})
		s5WantWrite(t, "GenerateSessions", a, err)
	}
}

func TestS5_Schedule_DocLichBuoiVaThoiKhoaBieu(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	sch := s5MakeSchedule(t, e, e.class.ID)
	ses := s5MakeSession(t, e, e.class.ID, 1)

	for _, a := range s5Actors(e) {
		list, err := svc.GetSchedulesByClass(ctx, e.class.ID, a.user.ID, a.isAdmin)
		one, err2 := svc.GetScheduleByID(ctx, sch.ID, a.user.ID, a.isAdmin)
		sl, err3 := svc.GetSessionsByClass(ctx, e.class.ID, a.user.ID, a.isAdmin, 1, 20)
		so, err4 := svc.GetSessionByID(ctx, ses.ID, a.user.ID, a.isAdmin)
		tt, err5 := svc.GetClassTimetable(ctx, e.class.ID, a.user.ID, a.isAdmin)
		if a.member {
			if err != nil || len(list) != 1 || err2 != nil || one == nil || err3 != nil || sl == nil || sl.Total != 1 || err4 != nil || so == nil || err5 != nil || tt == nil || len(tt.Entries) != 1 {
				t.Errorf("%s: bị chặn nhầm hoặc thiếu dữ liệu (%v %v %v %v %v)", a.name, err, err2, err3, err4, err5)
			}
			continue
		}
		for name, e := range map[string]error{"GetSchedulesByClass": err, "GetScheduleByID": err2, "GetSessionsByClass": err3, "GetSessionByID": err4, "GetClassTimetable": err5} {
			if !errors.Is(e, ErrClassNotFound) {
				t.Errorf("%s / %s: err=%v, muốn ErrClassNotFound (404)", name, a.name, e)
			}
		}
		if list != nil || one != nil || sl != nil || so != nil || tt != nil {
			t.Errorf("%s: vẫn nhận dữ liệu dù không xem được lớp", a.name)
		}
	}
	// id không tồn tại: 404 cho cả người có quyền
	if _, err := svc.GetScheduleByID(ctx, uuid.New(), e.owner.ID, false); !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("lịch không tồn tại: err=%v, muốn ErrScheduleNotFound", err)
	}
	if _, err := svc.GetSessionByID(ctx, uuid.New(), e.owner.ID, false); !errors.Is(err, ErrClassSessionNotFound) {
		t.Errorf("buổi không tồn tại: err=%v, muốn ErrClassSessionNotFound", err)
	}
}

// Quyền xét trên lớp THẬT của bản ghi: giảng viên lớp A đặt id lịch/buổi của lớp B (lớp ngoài tầm) vào là 404 và
// bản ghi của B không đổi.
func TestS5_Schedule_IdCuaLopKhacKhongDiQuaDuoc(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	other := model.Class{Name: "QA-s5-other", CourseID: &e.foreignCourse.ID, Status: "active"}
	if err := e.f.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	sch := s5MakeSchedule(t, e, other.ID)
	ses := s5MakeSession(t, e, other.ID, 1)
	room, topic := "hack", "hack"

	if _, err := svc.GetScheduleByID(ctx, sch.ID, e.coTeacher.ID, false); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("Get lịch lớp khác: err=%v, muốn ErrClassNotFound", err)
	}
	if _, err := svc.UpdateSchedule(ctx, sch.ID, e.coTeacher.ID, false, dto.UpdateClassScheduleDTO{Room: &room}); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("Update lịch lớp khác: err=%v, muốn ErrClassNotFound", err)
	}
	if err := svc.DeleteSchedule(ctx, sch.ID, e.coTeacher.ID, false); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("Delete lịch lớp khác: err=%v, muốn ErrClassNotFound", err)
	}
	if _, err := svc.UpdateSession(ctx, ses.ID, e.coTeacher.ID, false, dto.UpdateClassSessionDTO{Topic: &topic}); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("Update buổi lớp khác: err=%v, muốn ErrClassNotFound", err)
	}
	if err := svc.CancelSession(ctx, ses.ID, e.coTeacher.ID, false, "x"); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("Cancel buổi lớp khác: err=%v, muốn ErrClassNotFound", err)
	}
	var gotSch model.ClassSchedule
	var gotSes model.ClassSession
	if err := e.f.db.First(&gotSch, "id = ?", sch.ID).Error; err != nil || gotSch.Room != nil {
		t.Errorf("lịch lớp khác bị đụng tới: %v", err)
	}
	if err := e.f.db.First(&gotSes, "id = ?", ses.ID).Error; err != nil || gotSes.Status != model.SessionScheduled || gotSes.Topic != nil {
		t.Errorf("buổi lớp khác bị đụng tới: %v status=%s", err, gotSes.Status)
	}
}

func TestS5_SessionAttendance_QuyenGhiDocVaHocVienTrongLop(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	outsider := e.f.user("outsider")
	own := model.SessionAttendance{}
	ses := s5MakeSession(t, e, e.class.ID, 1)
	own = model.SessionAttendance{SessionID: ses.ID, StudentID: e.student.ID, Status: model.AttendancePresent}
	if err := e.f.db.Create(&own).Error; err != nil {
		t.Fatal(err)
	}

	for i, a := range s5Actors(e) {
		list, err := svc.GetSessionAttendances(ctx, ses.ID, a.user.ID, a.isAdmin)
		if a.manager {
			if err != nil || len(list) == 0 {
				t.Errorf("GetSessionAttendances / %s: err=%v, muốn đọc được", a.name, err)
			}
		} else if !errors.Is(err, ErrClassNotFound) || list != nil {
			t.Errorf("GetSessionAttendances / %s: err=%v, muốn ErrClassNotFound (kể cả học viên trong lớp)", a.name, err)
		}

		// Mark: học viên hợp lệ mỗi lần là một người mới trong lớp để khỏi trùng bản ghi.
		fresh := e.f.user("fresh")
		if err := e.f.db.Create(&model.StudentClass{StudentID: fresh.ID, ClassID: e.class.ID, Status: "active"}).Error; err != nil {
			t.Fatal(err)
		}
		_, err = svc.MarkAttendance(ctx, ses.ID, dto.MarkAttendanceDTO{StudentID: fresh.ID.String(), Status: "present"}, a.user.ID, a.isAdmin)
		s5WantWrite(t, "MarkAttendance", a, err)
		fresh2 := e.f.user("fresh2")
		if err := e.f.db.Create(&model.StudentClass{StudentID: fresh2.ID, ClassID: e.class.ID, Status: "active"}).Error; err != nil {
			t.Fatal(err)
		}
		_, err = svc.BulkMarkAttendance(ctx, ses.ID, dto.BulkMarkAttendanceDTO{Attendances: []dto.MarkAttendanceDTO{{StudentID: fresh2.ID.String(), Status: "late"}}}, a.user.ID, a.isAdmin)
		s5WantWrite(t, "BulkMarkAttendance", a, err)

		absent := "absent"
		_, err = svc.UpdateAttendance(ctx, ses.ID, own.ID, dto.UpdateSessionAttendanceDTO{Status: &absent}, a.user.ID, a.isAdmin)
		s5WantWrite(t, "UpdateAttendance", a, err)
		var got model.SessionAttendance
		e.f.db.First(&got, "id = ?", own.ID)
		if !a.manager && got.Status != model.AttendancePresent {
			t.Errorf("UpdateAttendance / %s: điểm danh bị sửa dù không có quyền (i=%d)", a.name, i)
		}
		e.f.db.Model(&model.SessionAttendance{}).Where("id = ?", own.ID).Update("status", model.AttendancePresent)
	}

	// Học viên không thuộc lớp của buổi: bị từ chối cả Mark lẫn Bulk, không ghi dòng nào.
	var before, after int64
	e.f.db.Model(&model.SessionAttendance{}).Count(&before)
	if _, err := svc.MarkAttendance(ctx, ses.ID, dto.MarkAttendanceDTO{StudentID: outsider.ID.String(), Status: "present"}, e.owner.ID, false); !errors.Is(err, ErrStudentNotInSession) {
		t.Errorf("Mark học viên ngoài lớp: err=%v, muốn ErrStudentNotInSession", err)
	}
	if _, err := svc.BulkMarkAttendance(ctx, ses.ID, dto.BulkMarkAttendanceDTO{Attendances: []dto.MarkAttendanceDTO{{StudentID: outsider.ID.String(), Status: "present"}}}, e.owner.ID, false); !errors.Is(err, ErrStudentNotInSession) {
		t.Errorf("Bulk học viên ngoài lớp: err=%v, muốn ErrStudentNotInSession", err)
	}
	e.f.db.Model(&model.SessionAttendance{}).Count(&after)
	if after != before {
		t.Errorf("đã ghi %d dòng cho học viên ngoài lớp", after-before)
	}

	// Điểm danh của buổi khác không sửa được bằng id đặt vào buổi mình.
	ses2 := s5MakeSession(t, e, e.class.ID, 2)
	absent := "absent"
	if _, err := svc.UpdateAttendance(ctx, ses2.ID, own.ID, dto.UpdateSessionAttendanceDTO{Status: &absent}, e.owner.ID, false); err == nil {
		t.Errorf("UpdateAttendance với id thuộc buổi khác phải bị từ chối")
	}

	// Học viên tự check-in: trong lớp qua, người ngoài lớp 404. Check-in chỉ mở đúng ngày của buổi
	// (lane P), nên đưa buổi này về hôm nay; cửa sổ ngày có test riêng ở schedule_checkin_window_postgres_test.go.
	if err := e.f.db.Exec("UPDATE class_sessions SET date = ? WHERE id = ?", time.Now().In(sessionDayZone).Format("2006-01-02"), ses2.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StudentCheckIn(ctx, ses2.ID, e.student.ID); err != nil {
		t.Errorf("check-in học viên trong lớp bị chặn nhầm: %v", err)
	}
	if _, err := svc.StudentCheckIn(ctx, ses2.ID, outsider.ID); !errors.Is(err, ErrClassSessionNotFound) {
		t.Errorf("check-in người ngoài lớp: err=%v, muốn ErrClassSessionNotFound", err)
	}
}