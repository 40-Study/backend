package service

// R3 (QA hồi quy 03/10/2026, Postgres thật, schema tạm):
//   - B-10: buổi học lớp có giờ kết thúc không sau giờ bắt đầu bị 400; trùng giờ với buổi khác của cùng lớp
//     hoặc của một giảng viên của lớp (kể cả ở lớp khác) bị 409; buổi liền kề, buổi đã huỷ không tính trùng.
//   - B-20: bảng xếp hạng chỉ liệt kê học viên (không GV/admin/phụ huynh) và class_id lọc đúng, chỉ thành
//     viên lớp hoặc admin được xem bảng theo lớp.
// Bỏ HasOverlappingSession/ensureSessionSlotFree hoặc eligibleStudentSQL/leaderboardClassFilter thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// grantSystemRole gán vai trò hệ thống (active) cho user; tạo vai trò nếu schema dùng chung chưa có.
func grantSystemRole(t *testing.T, db *gorm.DB, userID uuid.UUID, role string) {
	t.Helper()
	r := model.SystemRole{Name: role, Status: "active"}
	if err := db.Where("name = ?", role).FirstOrCreate(&r).Error; err != nil {
		t.Fatalf("tạo role %s: %v", role, err)
	}
	usr := model.UserSystemRole{UserID: userID, SystemRoleID: r.ID, Status: model.UserSystemRoleStatusActive}
	if err := db.Create(&usr).Error; err != nil {
		t.Fatalf("gán role %s: %v", role, err)
	}
}

func TestR3_Session_GioKetThucVaTrungGio(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	create := func(classID uuid.UUID, date, start, end string) (*dto.ClassSessionResponseDTO, error) {
		return svc.CreateSession(ctx, classID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: date, StartTime: start, EndTime: end})
	}

	if _, err := create(e.class.ID, "2031-03-03", "21:00", "20:00"); !errors.Is(err, ErrSessionTimeOrder) {
		t.Errorf("kết thúc trước bắt đầu: err=%v, muốn ErrSessionTimeOrder", err)
	}
	if _, err := create(e.class.ID, "2031-03-03", "19:00", "19:00"); !errors.Is(err, ErrSessionTimeOrder) {
		t.Errorf("kết thúc bằng bắt đầu: err=%v, muốn ErrSessionTimeOrder", err)
	}

	first, err := create(e.class.ID, "2031-03-03", "19:00", "20:30")
	if err != nil {
		t.Fatalf("buổi hợp lệ: %v", err)
	}
	if _, err := create(e.class.ID, "2031-03-03", "19:30", "20:00"); !errors.Is(err, ErrSessionOverlap) {
		t.Errorf("chen vào giữa buổi cùng lớp: err=%v, muốn ErrSessionOverlap", err)
	}
	if _, err := create(e.class.ID, "2031-03-03", "18:00", "19:01"); !errors.Is(err, ErrSessionOverlap) {
		t.Errorf("gối đầu 1 phút: err=%v, muốn ErrSessionOverlap", err)
	}
	if _, err := create(e.class.ID, "2031-03-03", "20:30", "21:30"); err != nil {
		t.Errorf("buổi liền kề không phải trùng giờ: %v", err)
	}
	if _, err := create(e.class.ID, "2031-03-04", "19:00", "20:30"); err != nil {
		t.Errorf("cùng giờ khác ngày không phải trùng giờ: %v", err)
	}

	// Giảng viên của lớp này dạy một lớp khác: buổi của lớp khác chiếm giờ của họ.
	otherClass := model.Class{Name: "QA-r3-other", CourseID: &e.course.ID, Status: "active", CreatedBy: &e.creator.ID}
	if err := e.f.db.Create(&otherClass).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.f.db.Create(&model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: otherClass.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := create(otherClass.ID, "2031-03-03", "20:00", "21:00"); !errors.Is(err, ErrSessionOverlap) {
		t.Errorf("trùng giờ với buổi lớp khác của cùng giảng viên: err=%v, muốn ErrSessionOverlap", err)
	}
	if _, err := create(otherClass.ID, "2031-03-05", "20:00", "21:00"); err != nil {
		t.Errorf("khác ngày thì được: %v", err)
	}

	// Sửa: tự không trùng với chính mình; dời vào giờ của buổi khác thì 409; huỷ buổi giải phóng giờ.
	same := "19:15"
	if _, err := svc.UpdateSession(ctx, first.ID, e.owner.ID, false, dto.UpdateClassSessionDTO{StartTime: &same}); err != nil {
		t.Errorf("dời giờ trong chính khung của mình: %v", err)
	}
	clash := "20:00"
	later := "2031-03-03"
	var second model.ClassSession
	if err := e.f.db.Where("class_id = ? AND start_time = ?", e.class.ID, "20:30").First(&second).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateSession(ctx, second.ID, e.owner.ID, false, dto.UpdateClassSessionDTO{Date: &later, StartTime: &clash}); !errors.Is(err, ErrSessionOverlap) {
		t.Errorf("sửa vào giờ buổi khác: err=%v, muốn ErrSessionOverlap", err)
	}
	if err := svc.CancelSession(ctx, first.ID, e.owner.ID, false, "nghỉ"); err != nil {
		t.Fatal(err)
	}
	if _, err := create(e.class.ID, "2031-03-03", "19:00", "20:00"); err != nil {
		t.Errorf("buổi đã huỷ không được chiếm giờ: %v", err)
	}
}

// B-08: lịch giảng viên cần thấy cả lịch lặp tuần lẫn buổi học cụ thể của lớp mình dạy (trước đây chỉ có livestream).
func TestR3_MyTimetable_KemBuoiHocCuThe(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	if _, err := svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
		DayOfWeek: 1, StartTime: "19:00", EndTime: "21:00", EffectiveFrom: "2031-01-01", EffectiveUntil: "2031-12-31"}); err != nil {
		t.Fatal(err)
	}
	inRange, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-03-03", StartTime: "19:00", EndTime: "20:30", Topic: "Buoi 1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-09-01", StartTime: "19:00", EndTime: "20:30"}); err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-03-04", StartTime: "19:00", EndTime: "20:30"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelSession(ctx, cancelled.ID, e.owner.ID, false, "nghỉ"); err != nil {
		t.Fatal(err)
	}

	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	got, err := svc.GetMyTimetableWithSessions(ctx, e.coTeacher.ID, "TEACHER", day("2031-03-01"), day("2031-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	var recurring, sessions int
	for _, en := range got.Entries {
		switch {
		case en.SessionID != nil && *en.SessionID == inRange.ID:
			sessions++
			if en.Date == nil || *en.Date != "2031-03-03" || en.DayOfWeek != 1 || en.StartTime != "19:00" || en.EndTime != "20:30" {
				t.Errorf("buổi cụ thể sai: %+v", en)
			}
		case en.SessionID != nil:
			t.Errorf("lọt buổi ngoài khoảng hoặc đã huỷ: %+v", en)
		case en.ScheduleID != nil:
			recurring++
			if en.EffectiveFrom == nil || *en.EffectiveFrom != "2031-01-01" || en.EffectiveUntil == nil || *en.EffectiveUntil != "2031-12-31" {
				t.Errorf("thiếu ngày hiệu lực của lịch lặp: %+v", en)
			}
		}
	}
	if recurring != 1 || sessions != 1 {
		t.Errorf("lịch lặp=%d buổi cụ thể=%d, muốn 1 và 1", recurring, sessions)
	}

	// Không xin khoảng ngày thì như cũ: chỉ lịch lặp, không có buổi cụ thể.
	plain, err := svc.GetMyTimetable(ctx, e.coTeacher.ID, "TEACHER")
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range plain.Entries {
		if en.SessionID != nil {
			t.Errorf("GetMyTimetable không kèm khoảng ngày mà lại trả buổi cụ thể: %+v", en)
		}
	}
	if _, err := svc.GetMyTimetableWithSessions(ctx, e.coTeacher.ID, "TEACHER", day("2031-03-31"), day("2031-03-01")); !errors.Is(err, ErrTimetableRangeInvalid) {
		t.Errorf("khoảng ngược: err=%v, muốn ErrTimetableRangeInvalid", err)
	}
	if _, err := svc.GetMyTimetableWithSessions(ctx, e.coTeacher.ID, "TEACHER", day("2031-01-01"), day("2032-06-01")); !errors.Is(err, ErrTimetableRangeInvalid) {
		t.Errorf("khoảng quá dài: err=%v, muốn ErrTimetableRangeInvalid", err)
	}
	// Học viên của lớp xem cùng khoảng ngày thấy cùng buổi.
	stu, err := svc.GetMyTimetableWithSessions(ctx, e.student.ID, "STUDENT", day("2031-03-01"), day("2031-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, en := range stu.Entries {
		found = found || (en.SessionID != nil && *en.SessionID == inRange.ID)
	}
	if !found {
		t.Error("học viên trong lớp không thấy buổi học cụ thể của lớp")
	}
}

func TestR3_Leaderboard_ChiHocVienVaLocTheoLop(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	db := e.f.db

	stuA, stuB := e.f.user("lb-a"), e.f.user("lb-b")
	teacher, admin, parent := e.f.user("lb-teacher"), e.f.user("lb-admin"), e.f.user("lb-parent")
	for _, u := range []model.User{stuA, stuB, teacher, admin, parent} {
		db.Model(&model.User{}).Where("id = ?", u.ID).Update("is_active", true)
	}
	for _, u := range []model.User{e.student, stuA, stuB} {
		grantSystemRole(t, db, u.ID, "STUDENT")
	}
	grantSystemRole(t, db, teacher.ID, "TEACHER")
	grantSystemRole(t, db, admin.ID, "SYSTEM_ADMIN")
	grantSystemRole(t, db, parent.ID, "PARENT")
	// Giảng viên/admin/phụ huynh có điểm cao hơn mọi học viên: nếu lọt vào bảng sẽ đứng đầu.
	for i, u := range []model.User{teacher, admin, parent, stuA, stuB, e.student} {
		if err := db.Create(&model.UserPoint{UserID: u.ID, TotalPoints: 900 - i*10}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// stuA ghi danh lớp e.class cùng e.student; stuB không ở lớp.
	if err := db.Create(&model.StudentClass{StudentID: stuA.ID, ClassID: e.class.ID, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewLeaderboardService(repository.NewLeaderboardRepository(db))
	ranked := func(classID *uuid.UUID, viewer *LeaderboardViewer) (map[uuid.UUID]bool, error) {
		resp, err := svc.GetLeaderboard(ctx, "all_time", 100, viewer, classID)
		if err != nil {
			return nil, err
		}
		got := map[uuid.UUID]bool{}
		for _, en := range resp.Entries {
			if en.UserID != nil {
				got[*en.UserID] = true
			}
		}
		return got, nil
	}

	all, err := ranked(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]model.User{"giảng viên": teacher, "admin": admin, "phụ huynh": parent} {
		if all[u.ID] {
			t.Errorf("%s lọt vào bảng xếp hạng", name)
		}
	}
	for name, u := range map[string]model.User{"học viên A": stuA, "học viên B": stuB, "học viên trong lớp": e.student} {
		if !all[u.ID] {
			t.Errorf("%s bị mất khỏi bảng xếp hạng", name)
		}
	}

	classID := e.class.ID
	inClass, err := ranked(&classID, &LeaderboardViewer{UserID: e.student.ID})
	if err != nil {
		t.Fatalf("thành viên xem bảng theo lớp: %v", err)
	}
	if !inClass[stuA.ID] || !inClass[e.student.ID] || inClass[stuB.ID] {
		t.Errorf("bảng theo lớp sai: A=%v học viên=%v B(ngoài lớp)=%v", inClass[stuA.ID], inClass[e.student.ID], inClass[stuB.ID])
	}
	if _, err := ranked(&classID, &LeaderboardViewer{UserID: e.coTeacher.ID}); err != nil {
		t.Errorf("giảng viên của lớp xem bảng theo lớp: %v", err)
	}
	if _, err := ranked(&classID, &LeaderboardViewer{UserID: admin.ID, IsAdmin: true}); err != nil {
		t.Errorf("admin xem bảng theo lớp: %v", err)
	}
	// Khách và học viên ngoài lớp không được liệt kê thành viên lớp qua class_id.
	if _, err := ranked(&classID, nil); !errors.Is(err, ErrLeaderboardClassNotFound) {
		t.Errorf("khách xem bảng theo lớp: err=%v, muốn ErrLeaderboardClassNotFound", err)
	}
	if _, err := ranked(&classID, &LeaderboardViewer{UserID: stuB.ID}); !errors.Is(err, ErrLeaderboardClassNotFound) {
		t.Errorf("học viên ngoài lớp: err=%v, muốn ErrLeaderboardClassNotFound", err)
	}
}
