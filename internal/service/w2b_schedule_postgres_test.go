package service

// W2-B (review R3 MINOR 1-4, 04/10/2026), Postgres thật, schema tạm:
//   - lịch lặp tuần (Create/UpdateSchedule) có end<=start bị ErrSessionTimeOrder;
//   - mở lại buổi đã huỷ phải kiểm trùng giờ dù không đổi ngày/giờ;
//   - buổi (hoặc lớp) đã xoá mềm không chiếm giờ;
//   - GenerateSessions bỏ qua buổi trùng giờ, chạy lại thì idempotent.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestW2B_Schedule_RejectsEndNotAfterStart(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	create := func(start, end string) (*dto.ClassScheduleResponseDTO, error) {
		return svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
			DayOfWeek: 1, StartTime: start, EndTime: end, EffectiveFrom: "2031-01-01"})
	}

	if _, err := create("21:00", "19:00"); !errors.Is(err, ErrSessionTimeOrder) {
		t.Errorf("tạo lịch end<start: err=%v, muốn ErrSessionTimeOrder", err)
	}
	if _, err := create("19:00", "19:00"); !errors.Is(err, ErrSessionTimeOrder) {
		t.Errorf("tạo lịch end=start: err=%v, muốn ErrSessionTimeOrder", err)
	}
	sch, err := create("19:00", "21:00")
	if err != nil {
		t.Fatalf("lịch hợp lệ: %v", err)
	}

	// Sửa MỘT đầu giờ làm lịch end<=start cũng bị chặn (kiểm trên giá trị sau khi áp).
	early := "18:00"
	if _, err := svc.UpdateSchedule(ctx, sch.ID, e.owner.ID, false, dto.UpdateClassScheduleDTO{EndTime: &early}); !errors.Is(err, ErrSessionTimeOrder) {
		t.Errorf("sửa end về 18:00 (start 19:00): err=%v, muốn ErrSessionTimeOrder", err)
	}
	late := "22:00"
	if _, err := svc.UpdateSchedule(ctx, sch.ID, e.owner.ID, false, dto.UpdateClassScheduleDTO{StartTime: &late}); !errors.Is(err, ErrSessionTimeOrder) {
		t.Errorf("sửa start về 22:00 (end 21:00): err=%v, muốn ErrSessionTimeOrder", err)
	}
	ok := "20:00"
	if _, err := svc.UpdateSchedule(ctx, sch.ID, e.owner.ID, false, dto.UpdateClassScheduleDTO{EndTime: &ok}); err != nil {
		t.Errorf("sửa end về 20:00 hợp lệ: %v", err)
	}
}

func TestW2B_Session_ReactivatingCancelledChecksOverlap(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	create := func(date, start, end string) (*dto.ClassSessionResponseDTO, error) {
		return svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: date, StartTime: start, EndTime: end})
	}

	first, err := create("2031-04-07", "19:00", "20:30")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelSession(ctx, first.ID, e.owner.ID, false, "nghỉ"); err != nil {
		t.Fatal(err)
	}
	// Trong lúc huỷ, một buổi khác vào đúng chỗ đó.
	if _, err := create("2031-04-07", "19:00", "20:30"); err != nil {
		t.Fatalf("buổi huỷ không chiếm giờ: %v", err)
	}

	// Mở lại buổi huỷ mà KHÔNG đổi ngày/giờ: vẫn phải bị 409.
	scheduled := "scheduled"
	if _, err := svc.UpdateSession(ctx, first.ID, e.owner.ID, false, dto.UpdateClassSessionDTO{Status: &scheduled}); !errors.Is(err, ErrSessionOverlap) {
		t.Fatalf("mở lại buổi huỷ vào giờ đã có buổi khác: err=%v, muốn ErrSessionOverlap", err)
	}
	var stored model.ClassSession
	if err := e.f.db.First(&stored, "id = ?", first.ID).Error; err != nil || stored.Status != model.SessionCancelled {
		t.Fatalf("buổi vẫn phải ở trạng thái huỷ sau khi bị từ chối (status=%q err=%v)", stored.Status, err)
	}

	// Chỗ trống thì mở lại được; đổi ghi chú trên buổi đang huỷ không bị chặn.
	free, err := create("2031-04-14", "19:00", "20:30")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelSession(ctx, free.ID, e.owner.ID, false, "nghỉ"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateSession(ctx, free.ID, e.owner.ID, false, dto.UpdateClassSessionDTO{Status: &scheduled}); err != nil {
		t.Errorf("mở lại buổi huỷ ở giờ trống: %v", err)
	}
}

func TestW2B_Overlap_IgnoresSoftDeletedSessionsAndClasses(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)

	// Buổi xoá mềm không chiếm giờ.
	first, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-05-05", StartTime: "19:00", EndTime: "20:30"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-05-05", StartTime: "19:00", EndTime: "20:30"}); !errors.Is(err, ErrSessionOverlap) {
		t.Fatalf("đối chứng: buổi còn sống phải chiếm giờ, err=%v", err)
	}
	if err := e.f.db.Delete(&model.ClassSession{}, "id = ?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-05-05", StartTime: "19:00", EndTime: "20:30"}); err != nil {
		t.Errorf("buổi đã xoá mềm vẫn chiếm giờ: %v", err)
	}

	// Buổi của lớp đã xoá mềm (cùng giảng viên) không chiếm giờ của giảng viên.
	other := model.Class{Name: "QA-w2b-other", CourseID: &e.course.ID, Status: "active", CreatedBy: &e.creator.ID}
	if err := e.f.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.f.db.Create(&model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: other.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, other.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-05-12", StartTime: "19:00", EndTime: "20:30"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-05-12", StartTime: "19:00", EndTime: "20:30"}); !errors.Is(err, ErrSessionOverlap) {
		t.Fatalf("đối chứng: buổi lớp khác của cùng giảng viên phải chiếm giờ, err=%v", err)
	}
	if err := e.f.db.Delete(&model.Class{}, "id = ?", other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-05-12", StartTime: "19:00", EndTime: "20:30"}); err != nil {
		t.Errorf("buổi của lớp đã xoá mềm vẫn chiếm giờ giảng viên: %v", err)
	}
}

// Review R3 MINOR 8: buổi sinh từ lịch lặp đã huỷ không nằm trong entries (giữ nguyên hợp đồng B-08) nhưng phải
// có trong cancelled_occurrences để giao diện không vẽ lại ngày đó theo giờ lặp gốc.
func TestW2B_Timetable_ReportsCancelledGeneratedSessions(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	sch, err := svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
		DayOfWeek: 1, StartTime: "19:00", EndTime: "21:00", EffectiveFrom: "2031-01-01", EffectiveUntil: "2031-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-17"})
	if err != nil || len(out) != 3 {
		t.Fatalf("sinh buổi: %d err=%v", len(out), err)
	}
	if err := svc.CancelSession(ctx, out[1].ID, e.owner.ID, false, "nghỉ lễ"); err != nil { // thứ Hai 10/03
		t.Fatal(err)
	}
	// Một buổi tay (không thuộc lịch lặp) bị huỷ KHÔNG được báo như ngày của lịch lặp.
	manual, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-03-12", StartTime: "19:00", EndTime: "20:00"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelSession(ctx, manual.ID, e.owner.ID, false, "x"); err != nil {
		t.Fatal(err)
	}

	from, _ := time.Parse("2006-01-02", "2031-03-01")
	to, _ := time.Parse("2006-01-02", "2031-03-31")
	got, err := svc.GetMyTimetableWithSessions(ctx, e.coTeacher.ID, "TEACHER", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CancelledOccurrences) != 1 || got.CancelledOccurrences[0].Date != "2031-03-10" || got.CancelledOccurrences[0].ScheduleID.String() != sch.ID.String() {
		t.Fatalf("cancelled_occurrences=%+v, muốn đúng một ngày 2031-03-10 của lịch %s", got.CancelledOccurrences, sch.ID)
	}
	for _, en := range got.Entries {
		if en.SessionID != nil && (en.Status == string(model.SessionCancelled)) {
			t.Errorf("buổi huỷ lọt vào entries: %+v", en)
		}
	}
}

func TestW2B_GenerateSessions_SkipsOverlapAndIsIdempotent(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	if _, err := svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
		DayOfWeek: 1, StartTime: "19:00", EndTime: "21:00", EffectiveFrom: "2031-01-01", EffectiveUntil: "2031-12-31"}); err != nil {
		t.Fatal(err)
	}
	// Một buổi tay đã chiếm thứ Hai 10/03/2031.
	manual, err := svc.CreateSession(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassSessionDTO{Date: "2031-03-10", StartTime: "19:30", EndTime: "20:30"})
	if err != nil {
		t.Fatal(err)
	}

	gen := dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-24"} // 4 thứ Hai: 3, 10, 17, 24
	out, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, gen)
	if err != nil {
		t.Fatalf("sinh buổi: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("sinh %d buổi, muốn 3 (thứ Hai 10/03 trùng buổi tay bị bỏ qua)", len(out))
	}
	for i, s := range out {
		if s.Date == "2031-03-10" {
			t.Errorf("buổi 10/03 trùng giờ vẫn được sinh: %+v", s)
		}
		if want := manual.SessionNumber + 1 + i; s.SessionNumber != want {
			t.Errorf("buổi %d có số thứ tự %d, muốn %d (không để lỗ hổng vì buổi bị bỏ qua)", i, s.SessionNumber, want)
		}
	}

	// Chạy lại đúng khoảng đó: mọi buổi đã có -> 409, không nhân đôi.
	if _, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, gen); !errors.Is(err, ErrSessionOverlap) {
		t.Fatalf("sinh lại khoảng đã sinh: err=%v, muốn ErrSessionOverlap", err)
	}
	var n int64
	if err := e.f.db.Model(&model.ClassSession{}).Where("class_id = ?", e.class.ID).Count(&n).Error; err != nil || n != 4 {
		t.Fatalf("lớp có %d buổi (err=%v), muốn đúng 4 (1 tay + 3 sinh)", n, err)
	}

	// Kéo dài khoảng ngày: chỉ sinh thêm các tuần mới.
	more, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-04-07"})
	if err != nil || len(more) != 2 {
		t.Fatalf("kéo dài tới 07/04: %d buổi err=%v, muốn 2 (31/03 và 07/04)", len(more), err)
	}
}
