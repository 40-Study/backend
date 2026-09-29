package service

// Lane S4 (review M-4, Postgres thật): MarkAttendance chỉ nhận học viên đang học lớp. Học viên ngoài
// lớp, đã rời lớp (dropped) hoặc không tồn tại bị từ chối và KHÔNG dòng nào của lô được ghi (kiểm cả
// danh sách trước khi ghi). Bỏ kiểm StudentClassExists, hoặc kiểm giữa vòng ghi, thì test ĐỎ.

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

func TestS4_Attendance_MarkChiNhanHocVienTrongLop(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := NewAttendanceService(repository.NewAttendanceRepository(e.f.db), repository.NewClassRepository(e.f.db), repository.NewCourseRepository(e.f.db))

	outsider, dropped := e.f.user("outsider"), e.f.user("dropped")
	if err := e.f.db.Create(&model.StudentClass{StudentID: dropped.ID, ClassID: e.class.ID, Status: "dropped"}).Error; err != nil {
		t.Fatal(err)
	}
	count := func() int64 {
		var n int64
		e.f.db.Model(&model.Attendance{}).Where("class_id = ?", e.class.ID).Count(&n)
		return n
	}
	mark := func(date string, ids ...uuid.UUID) error {
		entries := make([]dto.AttendanceEntryDTO, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, dto.AttendanceEntryDTO{StudentID: id, Status: "present"})
		}
		_, err := svc.MarkAttendance(ctx, e.class.ID, e.owner.ID, false, dto.BulkCreateAttendanceDTO{Date: date, Attendances: entries})
		return err
	}
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }

	for name, bad := range map[string]uuid.UUID{
		"học viên ngoài lớp":       outsider.ID,
		"học viên đã rời lớp":      dropped.ID,
		"student_id không tồn tại": uuid.New(),
	} {
		before := count()
		// Học viên hợp lệ đứng TRƯỚC học viên lạ: lô phải bị từ chối nguyên vẹn, không ghi dở dang.
		if err := mark(day(1), e.student.ID, bad); !errors.Is(err, ErrStudentNotInClass) {
			t.Errorf("%s: err=%v, muốn ErrStudentNotInClass", name, err)
		}
		if count() != before {
			t.Errorf("%s: đã ghi %d dòng dù lô bị từ chối", name, count()-before)
		}
	}
	if err := mark(day(2), e.student.ID); err != nil {
		t.Fatalf("học viên trong lớp bị chặn nhầm: %v", err)
	}
	if count() != 1 {
		t.Errorf("muốn đúng 1 dòng điểm danh, có %d", count())
	}
}