package seeds

// Lane B seed demo — lớp học, lịch, điểm danh, livestream, bài tập, bài nộp, điểm. Chạy trên Postgres
// THẬT trong schema tạm (pgtest.IsolatedSchema), DROP khi xong. Không có Postgres: Skip ở local,
// FAIL khi CI=true.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

type demoClassCounts struct {
	classes, teacherLinks, studentLinks, schedules, sessions, attendances int64
	lives, assignments, testCases, submissions, grades                    int64
}

// expectedDemoClassCounts tính số bản ghi kỳ vọng từ spec và ngày khai giảng ĐÃ LƯU của từng lớp,
// độc lập với vòng lặp sinh buổi trong seed (duyệt lịch theo tuần thay vì theo ngày).
func expectedDemoClassCounts(t *testing.T, s *Seeder) demoClassCounts {
	t.Helper()
	var c demoClassCounts
	today := demoToday()
	for _, spec := range demoClassSpecs() {
		var class model.Class
		if err := s.db.Where("name = ?", spec.Name).First(&class).Error; err != nil {
			t.Fatalf("đọc lớp %q: %v", spec.Name, err)
		}
		c.classes++
		c.teacherLinks++
		c.studentLinks += int64(len(spec.StudentEmails))
		c.schedules += int64(len(spec.Days))

		start := dateOrToday(class.StartDate)
		last := today.AddDate(0, 0, demoSessionLookaheadDays)
		for _, day := range spec.Days {
			first := start.AddDate(0, 0, (day-int(start.Weekday())+7)%7)
			for d := first; !d.After(last); d = d.AddDate(0, 0, 7) {
				c.sessions++
				if d.Before(today) {
					c.attendances += int64(len(spec.StudentEmails))
				}
			}
		}

		c.lives += int64(len(spec.Lives))
		for _, as := range spec.Assignments {
			c.assignments++
			c.testCases += int64(len(as.TestCases))
			for _, sub := range as.Results {
				c.submissions++
				if sub.Score > 0 {
					c.grades++
				}
			}
		}
		c.grades += int64(len(spec.ExtraGrades))
	}
	return c
}

func actualDemoClassCounts(t *testing.T, s *Seeder) demoClassCounts {
	t.Helper()
	return demoClassCounts{
		classes: countRows(t, s.db, &model.Class{}), teacherLinks: countRows(t, s.db, &model.TeacherClass{}),
		studentLinks: countRows(t, s.db, &model.StudentClass{}), schedules: countRows(t, s.db, &model.ClassSchedule{}),
		sessions: countRows(t, s.db, &model.ClassSession{}), attendances: countRows(t, s.db, &model.SessionAttendance{}),
		lives: countRows(t, s.db, &model.LivestreamSession{}), assignments: countRows(t, s.db, &model.Assignment{}),
		testCases: countRows(t, s.db, &model.TestCase{}), submissions: countRows(t, s.db, &model.Submission{}),
		grades: countRows(t, s.db, &model.Grade{}),
	}
}

// TestSeedDemoClasses_Postgres_IdempotentVaHienDungNguoi:
//   - số bản ghi mọi bảng khớp spec, chạy lần 2 không tăng;
//   - dữ liệu lọt qua đúng các truy vấn mà API của trang học sinh/giáo viên/phụ huynh dùng.
func TestSeedDemoClasses_Postgres_IdempotentVaHienDungNguoi(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	ctx := context.Background()

	users := map[string]model.User{}
	for _, email := range []string{"teacher1@demo.com", "teacher2@demo.com", "student1@demo.com", "student2@demo.com"} {
		u := model.User{Email: email, PasswordHash: "x", UserName: "qa-seed-" + uuid.NewString()[:8]}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", email, err)
		}
		users[email] = u
	}
	courses := seedDemoCoursesOnce(t, s, users)

	var first demoClassCounts
	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoClasses(users, courses); err != nil {
			t.Fatalf("lần %d: SeedDemoClasses: %v", run, err)
		}
		got := actualDemoClassCounts(t, s)
		if run == 1 {
			want := expectedDemoClassCounts(t, s)
			if got != want {
				t.Fatalf("lần 1: số bản ghi %+v, muốn %+v", got, want)
			}
			first = got
			continue
		}
		if got != first {
			t.Errorf("lần 2: số bản ghi %+v khác lần 1 %+v — seed lại không được nhân bản", got, first)
		}
	}

	student1, student2 := users["student1@demo.com"], users["student2@demo.com"]
	teacher1 := users["teacher1@demo.com"]
	scheduleRepo := repository.NewScheduleRepository(db)

	// (app)/schedule + phụ huynh timetable: /me/timetable = lớp của học sinh -> lịch is_active.
	classIDs, err := scheduleRepo.GetStudentClassIDs(ctx, student1.ID)
	if err != nil || len(classIDs) != 3 {
		t.Fatalf("student1 phải học 3 lớp, có %d (err %v)", len(classIDs), err)
	}
	schedules, err := scheduleRepo.GetSchedulesByClassIDs(ctx, classIDs)
	if err != nil || len(schedules) != 6 {
		t.Errorf("thời khoá biểu student1: %d lịch, muốn 6 (err %v)", len(schedules), err)
	}

	// Phụ huynh: buổi sắp tới (status scheduled, date >= CURRENT_DATE) phải có ở MỌI lớp.
	for _, id := range classIDs {
		upcoming, err := scheduleRepo.GetUpcomingSessions(ctx, id, 5)
		if err != nil || len(upcoming) == 0 {
			t.Errorf("lớp %s không có buổi sắp tới (err %v)", id, err)
		}
	}

	// (app)/my-attendance + phụ huynh: đủ các trạng thái để thống kê có ý nghĩa.
	atts, total, err := scheduleRepo.GetStudentAttendanceHistory(ctx, student1.ID, 1, 50)
	if err != nil || total == 0 {
		t.Fatalf("student1 không có điểm danh (err %v)", err)
	}
	statuses := map[model.AttendanceStatus]bool{}
	for _, a := range atts {
		statuses[a.Status] = true
		if a.Session.Class.Name == "" {
			t.Errorf("điểm danh %s thiếu tên lớp (preload Session.Class)", a.ID)
		}
	}
	for _, want := range []model.AttendanceStatus{model.AttendancePresent, model.AttendanceLate, model.AttendanceAbsent, model.AttendanceExcused} {
		if !statuses[want] {
			t.Errorf("điểm danh student1 thiếu trạng thái %s", want)
		}
	}

	// teacher/schedule + teacher/assignments: /livestream?host_id=teacher1 (React 3 + Flutter 2).
	liveRepo := repository.NewLivestreamRepository(db)
	lives, _, err := liveRepo.GetAll(ctx, teacher1.ID, false, 1, 100, "", &teacher1.ID, nil)
	if err != nil || len(lives) != 5 {
		t.Errorf("teacher1 thấy %d phiên livestream, muốn 5 (err %v)", len(lives), err)
	}
	// (app)/my-assignments: student2 chỉ thấy phiên của lớp mình học (React 3 + Python 2).
	studentLives, _, err := liveRepo.GetAll(ctx, student2.ID, false, 1, 100, "", nil, nil)
	if err != nil || len(studentLives) != 5 {
		t.Errorf("student2 thấy %d phiên livestream, muốn 5 (err %v)", len(studentLives), err)
	}
	assignRepo := repository.NewAssignmentRepository(db)
	publishedVisible := 0
	for _, l := range studentLives {
		list, _, err := assignRepo.GetBySession(ctx, l.ID, 1, 50, true)
		if err != nil {
			t.Fatalf("liệt kê bài tập phiên %s: %v", l.ID, err)
		}
		publishedVisible += len(list)
	}
	if publishedVisible == 0 {
		t.Error("student2 không thấy bài tập đã publish nào qua các phiên livestream")
	}

	// Phụ huynh + /me/grades: điểm có tên lớp và nhận xét.
	grades, err := repository.NewGradeRepository(db).GetGradesByStudentID(ctx, student1.ID)
	if err != nil || len(grades) == 0 {
		t.Fatalf("student1 không có điểm (err %v)", err)
	}
	for _, g := range grades {
		if g.Class.Name == "" || g.Feedback == nil || *g.Feedback == "" {
			t.Errorf("điểm %q thiếu tên lớp hoặc nhận xét", g.Title)
		}
	}

	// Bài nộp student1 có cả đã chấm (accepted) lẫn chờ chấm (pending).
	subs, _, err := repository.NewSubmissionRepository(db).GetByUser(ctx, student1.ID, 1, 50)
	if err != nil {
		t.Fatalf("bài nộp student1: %v", err)
	}
	verdicts := map[model.SubmissionVerdict]bool{}
	for _, sub := range subs {
		verdicts[sub.Verdict] = true
	}
	if !verdicts[model.VerdictAccepted] || !verdicts[model.VerdictPending] {
		t.Errorf("bài nộp student1 phải có cả accepted và pending, có %v", verdicts)
	}
}
