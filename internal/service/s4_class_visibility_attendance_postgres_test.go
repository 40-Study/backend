package service

// Lane S4 (vòng 2, Postgres thật), theo từng vai: giảng viên lạ, học viên trong lớp, giảng viên lớp,
// chủ khoá, người tạo lớp, admin.
//
//  1. Điểm danh của lớp (/classes/:classId/attendances): ghi (tạo, sửa, xoá) chỉ người quản lý lớp, người
//     khác 403; đọc chỉ người quản lý lớp, người khác 404 (học viên xem điểm danh của mình qua endpoint
//     riêng /me/attendances trên bảng session_attendances, không qua route quản lý này). Quyền xét trên lớp
//     trong URL và bản ghi phải thuộc đúng lớp đó (không sửa/xoá điểm danh lớp khác bằng id của nó).
//  2. Chi tiết lớp và danh sách học viên của lớp: chỉ thành viên lớp, người quản lý lớp và admin; người
//     khác 404.
//  3. Analytics của assignment không tồn tại: 404 (ErrAssignmentNotFound), không phải data null.
//
// Bỏ kiểm quyền ở bất kỳ hàm nào dưới đây thì test ĐỎ.

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

func TestS4_Attendance_QuyenGhiDocVaThuocDungLop(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := NewAttendanceService(repository.NewAttendanceRepository(e.f.db), repository.NewClassRepository(e.f.db), repository.NewCourseRepository(e.f.db))

	// Một lớp KHÁC (khoá của stranger) có sẵn một bản ghi điểm danh: dùng để thử đặt id của lớp khác vào URL của lớp mình.
	otherClass := model.Class{Name: "QA-s4-other", CourseID: &e.foreignCourse.ID, Status: "active"}
	if err := e.f.db.Create(&otherClass).Error; err != nil {
		t.Fatal(err)
	}
	otherRow := model.Attendance{ClassID: otherClass.ID, StudentID: e.student.ID, Date: time.Now(), Status: "present"}
	if err := e.f.db.Create(&otherRow).Error; err != nil {
		t.Fatal(err)
	}
	ownRow := model.Attendance{ClassID: e.class.ID, StudentID: e.student.ID, Date: time.Now().AddDate(0, 0, -30), Status: "present"}
	if err := e.f.db.Create(&ownRow).Error; err != nil {
		t.Fatal(err)
	}
	count := func() int64 {
		var n int64
		e.f.db.Model(&model.Attendance{}).Count(&n)
		return n
	}

	type actor struct {
		name    string
		user    model.User
		isAdmin bool
		manager bool
	}
	actors := []actor{
		{"giảng viên lạ", e.stranger, false, false},
		{"học viên trong lớp", e.student, false, false},
		{"giảng viên lớp", e.coTeacher, false, true},
		{"chủ khoá", e.owner, false, true},
		{"người tạo lớp", e.creator, false, true},
		{"admin", e.admin, true, true},
	}

	day := 0
	for _, a := range actors {
		day++
		// W2-A: giang vien la khong xem duoc lop -> ErrClassNotFound (404); hoc vien xem duoc -> ErrNotClassTeacher (403).
		denied := ErrNotClassTeacher
		if a.user.ID == e.stranger.ID {
			denied = ErrClassNotFound
		}
		date := time.Now().AddDate(0, 0, day).Format("2006-01-02")

		// ghi: tạo
		before := count()
		_, err := svc.MarkAttendance(ctx, e.class.ID, a.user.ID, a.isAdmin, dto.BulkCreateAttendanceDTO{Date: date, Attendances: []dto.AttendanceEntryDTO{{StudentID: e.student.ID, Status: "present"}}})
		if a.manager && err != nil {
			t.Errorf("Mark / %s: bị chặn nhầm (%v)", a.name, err)
		}
		if !a.manager {
			if !errors.Is(err, denied) {
				t.Errorf("Mark / %s: err=%v, muon loi tu choi (denied)", a.name, err)
			}
			if count() != before {
				t.Errorf("Mark / %s: bản ghi vẫn được tạo dù không có quyền", a.name)
			}
		}

		// ghi: sửa, xoá bản ghi của lớp mình
		row := model.Attendance{ClassID: e.class.ID, StudentID: e.student.ID, Date: time.Now().AddDate(0, 0, -100-day), Status: "present"}
		if err := e.f.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		absent := "absent"
		_, err = svc.UpdateAttendance(ctx, e.class.ID, row.ID, a.user.ID, a.isAdmin, dto.UpdateAttendanceDTO{Status: &absent})
		var got model.Attendance
		e.f.db.First(&got, "id = ?", row.ID)
		switch {
		case a.manager && (err != nil || got.Status != "absent"):
			t.Errorf("Update / %s: err=%v status=%s, muốn sửa được", a.name, err, got.Status)
		case !a.manager && (!errors.Is(err, denied) || got.Status != "present"):
			t.Errorf("Update / %s: err=%v status=%s, muon loi tu choi (denied) và không đổi", a.name, err, got.Status)
		}
		err = svc.DeleteAttendance(ctx, e.class.ID, row.ID, a.user.ID, a.isAdmin)
		var n int64
		e.f.db.Model(&model.Attendance{}).Where("id = ?", row.ID).Count(&n)
		switch {
		case a.manager && (err != nil || n != 0):
			t.Errorf("Delete / %s: err=%v còn=%d, muốn xoá được", a.name, err, n)
		case !a.manager && (!errors.Is(err, denied) || n != 1):
			t.Errorf("Delete / %s: err=%v còn=%d, muon loi tu choi (denied) và không xoá", a.name, err, n)
		}

		// đọc: danh sách, một bản ghi
		list, err := svc.GetAllAttendances(ctx, e.class.ID, a.user.ID, a.isAdmin, "", 1, 50)
		one, err2 := svc.GetAttendanceByID(ctx, e.class.ID, ownRow.ID, a.user.ID, a.isAdmin)
		if a.manager {
			if err != nil || list == nil || len(list.Attendances) == 0 {
				t.Errorf("List / %s: err=%v, muốn đọc được", a.name, err)
			}
			if err2 != nil || one == nil {
				t.Errorf("Get / %s: err=%v, muốn đọc được", a.name, err2)
			}
		} else {
			if !errors.Is(err, ErrClassNotFound) || list != nil {
				t.Errorf("List / %s: err=%v, muốn ErrClassNotFound và không có dữ liệu", a.name, err)
			}
			if !errors.Is(err2, ErrClassNotFound) || one != nil {
				t.Errorf("Get / %s: err=%v, muốn ErrClassNotFound và không có dữ liệu", a.name, err2)
			}
		}
	}

	// Đặt id của bản ghi thuộc lớp KHÁC vào URL của lớp mình: người quản lý lớp mình vẫn nhận ErrAttendanceNotFound.
	if _, err := svc.GetAttendanceByID(ctx, e.class.ID, otherRow.ID, e.owner.ID, false); !errors.Is(err, ErrAttendanceNotFound) {
		t.Errorf("Get chéo lớp: err=%v, muốn ErrAttendanceNotFound", err)
	}
	absent := "absent"
	if _, err := svc.UpdateAttendance(ctx, e.class.ID, otherRow.ID, e.owner.ID, false, dto.UpdateAttendanceDTO{Status: &absent}); !errors.Is(err, ErrAttendanceNotFound) {
		t.Errorf("Update chéo lớp: err=%v, muốn ErrAttendanceNotFound", err)
	}
	if err := svc.DeleteAttendance(ctx, e.class.ID, otherRow.ID, e.owner.ID, false); !errors.Is(err, ErrAttendanceNotFound) {
		t.Errorf("Delete chéo lớp: err=%v, muốn ErrAttendanceNotFound", err)
	}
	var still model.Attendance
	if err := e.f.db.First(&still, "id = ?", otherRow.ID).Error; err != nil || still.Status != "present" {
		t.Errorf("bản ghi của lớp khác bị đụng tới: %v status=%s", err, still.Status)
	}
	if _, err := svc.GetAttendanceByID(ctx, e.class.ID, uuid.New(), e.owner.ID, false); !errors.Is(err, ErrAttendanceNotFound) {
		t.Errorf("Get id không tồn tại: err=%v, muốn ErrAttendanceNotFound", err)
	}
}

func TestS4_Class_ChiTietVaDanhSachHocVienChiThanhVienQuanLyVaAdmin(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	actors := []struct {
		name    string
		user    model.User
		isAdmin bool
		visible bool
	}{
		{"giảng viên lạ", e.stranger, false, false},
		{"học viên trong lớp", e.student, false, true},
		{"giảng viên lớp", e.coTeacher, false, true},
		{"chủ khoá", e.owner, false, true},
		{"người tạo lớp", e.creator, false, true},
		{"admin", e.admin, true, true},
	}
	for _, a := range actors {
		detail, err := e.svc.GetClassByID(ctx, e.class.ID, a.user.ID, a.isAdmin)
		roster, err2 := e.svc.GetStudentsByClass(ctx, e.class.ID, a.user.ID, a.isAdmin, 1, 20)
		if a.visible {
			if err != nil || detail == nil {
				t.Errorf("GetClassByID / %s: bị chặn nhầm (%v)", a.name, err)
			}
			if err2 != nil || roster == nil || roster.Total != 1 {
				t.Errorf("GetStudentsByClass / %s: err=%v roster=%+v, muốn thấy 1 học viên", a.name, err2, roster)
			}
			continue
		}
		if !errors.Is(err, ErrClassNotFound) || detail != nil {
			t.Errorf("GetClassByID / %s: err=%v, muốn ErrClassNotFound", a.name, err)
		}
		if !errors.Is(err2, ErrClassNotFound) || roster != nil {
			t.Errorf("GetStudentsByClass / %s: err=%v, muốn ErrClassNotFound", a.name, err2)
		}
	}
	// Học viên đã rời lớp (status dropped) không còn thấy lớp.
	dropped := e.f.user("dropped")
	if err := e.f.db.Create(&model.StudentClass{StudentID: dropped.ID, ClassID: e.class.ID, Status: "dropped"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetClassByID(ctx, e.class.ID, dropped.ID, false); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("học viên đã rời lớp: err=%v, muốn ErrClassNotFound", err)
	}
	// Lớp không tồn tại: người thường nhận cùng lỗi với lớp không xem được.
	if _, err := e.svc.GetClassByID(ctx, uuid.New(), e.stranger.ID, false); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("lớp không tồn tại: err=%v, muốn ErrClassNotFound", err)
	}
}

func TestS4_Analytics_AssignmentKhongTonTai404(t *testing.T) {
	e := newS4SubmissionEnv(t)
	f := e.f
	svc := NewAnalyticsService(repository.NewAnalyticsRepository(f.db), repository.NewParticipantRepository(f.db), repository.NewSubmissionRepository(f.db),
		repository.NewAssignmentRepository(f.db), repository.NewLivestreamRepository(f.db), repository.NewClassRepository(f.db), repository.NewCourseRepository(f.db))
	for _, tc := range []struct {
		name    string
		user    model.User
		isAdmin bool
	}{{"giảng viên", e.host, false}, {"admin", e.admin, true}, {"học viên", e.student, false}} {
		got, err := svc.GetAssignmentAnalytics(context.Background(), uuid.New(), tc.user.ID, tc.isAdmin)
		if !errors.Is(err, ErrAssignmentNotFound) || got != nil {
			t.Errorf("%s: got=%v err=%v, muốn ErrAssignmentNotFound (404), không phải data null", tc.name, got, err)
		}
	}
}
