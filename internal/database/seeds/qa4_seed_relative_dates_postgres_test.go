package seeds

// QA4 N-13 / S20 / A17: ngày của dữ liệu demo tương đối với LÚC SEED, kể cả trên DB đã seed từ trước. Mỗi test dựng trạng
// thái "seed từ 2 tháng trước" (lùi mọi mốc 60 ngày) rồi seed lại và kiểm các dòng "sắp tới" lại ở tương lai. Postgres thật,
// schema tạm. Đổi Assign về Attrs (chỉ đặt lúc tạo) ở seed thì các test này ĐỎ.

import (
	"testing"
	"time"

	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

const qa4AgeDays = 60

// ageDemoSchedule lùi mọi mốc lịch đã seed qa4AgeDays ngày, như DB seed từ 2 tháng trước.
func ageDemoSchedule(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, q := range []string{
		"UPDATE assignments SET end_time = end_time - interval '60 days', published_at = published_at - interval '60 days', start_time = start_time - interval '60 days'",
		"UPDATE livestream_sessions SET scheduled_at = scheduled_at - interval '60 days'",
		"UPDATE classes SET start_date = start_date - 60, end_date = end_date - 60",
		"UPDATE class_schedules SET effective_from = effective_from - 60, effective_until = effective_until - 60",
		"UPDATE class_sessions SET date = date - 60",
		"UPDATE submissions SET created_at = created_at - interval '60 days'",
		"UPDATE lesson_progress SET completed_at = completed_at - interval '60 days'",
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatalf("lùi mốc: %v (%s)", err, q)
		}
	}
}

func TestQA4_SeedDemoClasses_UpcomingRowsStayUpcomingAfterReseed(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "teacher1@demo.com", "teacher2@demo.com", "student1@demo.com", "student2@demo.com")
	courses := seedDemoCoursesOnce(t, s, users)
	if err := s.SeedDemoClasses(users, courses); err != nil {
		t.Fatal(err)
	}
	ageDemoSchedule(t, db)
	if err := s.SeedDemoClasses(users, courses); err != nil {
		t.Fatalf("seed lại: %v", err)
	}
	now := time.Now()
	s1, t1 := users["student1@demo.com"].ID, users["teacher1@demo.com"].ID

	// (1) Bài tập sắp hạn của student1: ít nhất 2, có một bài trong 48 giờ tới.
	var upcoming []model.Assignment
	if err := db.Where("is_published AND end_time > ? AND class_id IN (SELECT class_id FROM student_classes WHERE student_id = ?)", now, s1).
		Find(&upcoming).Error; err != nil {
		t.Fatal(err)
	}
	within48h := 0
	for _, a := range upcoming {
		if a.EndTime.Before(now.Add(48 * time.Hour)) {
			within48h++
		}
	}
	if len(upcoming) < 2 || within48h < 1 {
		t.Errorf("student1 có %d bài sắp hạn (%d trong 48h), muốn >= 2 và >= 1", len(upcoming), within48h)
	}

	// (4) teacher1 có buổi live tuần sau.
	var nextWeek int64
	db.Model(&model.LivestreamSession{}).Where("host_id = ? AND status = ? AND scheduled_at BETWEEN ? AND ?",
		t1, model.LivestreamStatusScheduled, now.AddDate(0, 0, 7), now.AddDate(0, 0, 14)).Count(&nextWeek)
	if nextWeek == 0 {
		t.Error("teacher1 không có buổi livestream nào tuần sau")
	}

	// (6) Mọi lớp demo có buổi học tuần này và tuần sau.
	today := demoToday()
	for _, spec := range demoClassSpecs() {
		var class model.Class
		if err := db.Where("name = ?", spec.Name).First(&class).Error; err != nil {
			t.Fatal(err)
		}
		for _, w := range []struct {
			name     string
			from, to time.Time
		}{{"7 ngày tới", today, today.AddDate(0, 0, 6)}, {"tuần sau", today.AddDate(0, 0, 7), today.AddDate(0, 0, 13)}} {
			var n int64
			db.Model(&model.ClassSession{}).Where("class_id = ? AND date BETWEEN ? AND ?", class.ID,
				w.from.Format("2006-01-02"), w.to.Format("2006-01-02")).Count(&n)
			if n == 0 {
				t.Errorf("lớp %q không có buổi học %s", spec.Name, w.name)
			}
		}
	}

	// Không có sự kiện "đã xảy ra" ở tương lai (bài nộp, điểm).
	var future int64
	db.Model(&model.Submission{}).Where("created_at > ?", now).Count(&future)
	if future != 0 {
		t.Errorf("%d bài nộp có giờ nộp ở tương lai", future)
	}
	db.Model(&model.Grade{}).Where("graded_at > ?", now).Count(&future)
	if future != 0 {
		t.Errorf("%d điểm có giờ chấm ở tương lai", future)
	}
}

func TestQA4_SeedDemoOrgMembers_EveryDemoOrgHasOwnerAndMembers(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	laneADemoUsers(t, db, "teacher1@demo.com", "teacher2@demo.com", "student1@demo.com", "student2@demo.com", "student3@demo.com")
	if err := s.SeedDemoOrganizations(); err != nil {
		t.Fatal(err)
	}
	// Tổ chức fortex.local không có trong schema test: phải bỏ qua, không lỗi.
	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoOrgMembers(); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
	}
	for _, m := range demoOrgMemberships {
		if !m.Required {
			continue
		}
		var counts []struct {
			Name string
			N    int
		}
		if err := db.Raw(`SELECT r.name, count(*) AS n FROM user_organization_roles u
			JOIN roles r ON r.id = u.role_id JOIN organizations o ON o.id = u.organization_id
			WHERE o.name = ? AND u.status = 'active' AND u.deleted_at IS NULL GROUP BY r.name`, m.Org).Scan(&counts).Error; err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, c := range counts {
			got[c.Name] = c.N
		}
		want := map[string]int{repository.OrgOwnerRoleName: 1, repository.OrgTeacherRoleName: len(m.Teachers), repository.OrgStudentRoleName: len(m.Students)}
		for role, n := range want {
			if got[role] != n {
				t.Errorf("%q: %d %s (chạy seed 2 lần), muốn %d", m.Org, got[role], role, n)
			}
		}
	}
}

func TestQA4_SeedDemoGamification_WeeklyPointsNeedRecentActivity(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "teacher1@demo.com", "teacher2@demo.com", "student1@demo.com", "student2@demo.com",
		"student3@demo.com", "student4@demo.com", "student5@demo.com")
	courses := seedDemoCoursesOnce(t, s, users)
	if err := s.SeedDemoEnrollments(users, courses); err != nil {
		t.Fatal(err)
	}
	ageDemoSchedule(t, db) // hoạt động học cũ 60 ngày
	if err := s.SeedDemoEnrollments(users, courses); err != nil {
		t.Fatalf("seed lại ghi danh: %v", err)
	}
	if err := s.SeedDemoGamification(users); err != nil {
		t.Fatal(err)
	}
	weekly := repository.CurrentPeriod("weekly")
	for _, st := range demoLearnerStats {
		u := users[st.Email]
		var recent int64
		db.Model(&model.LessonProgress{}).Where("user_id = ? AND status = 'completed' AND completed_at >= ?", u.ID, time.Now().AddDate(0, 0, -7)).Count(&recent)
		var entry model.LeaderboardEntry
		if err := db.Where("user_id = ? AND period = ? AND period_type = 'weekly'", u.ID, weekly).First(&entry).Error; err != nil {
			t.Fatalf("%s: %v", st.Email, err)
		}
		if recent == 0 && entry.Points != 0 {
			t.Errorf("%s: %d điểm tuần mà 0 bài hoàn thành 7 ngày qua", st.Email, entry.Points)
		}
		if entry.LessonsCompleted != int(recent) {
			t.Errorf("%s: lessons_completed=%d, muốn %d (bài hoàn thành 7 ngày qua)", st.Email, entry.LessonsCompleted, recent)
		}
	}
	// student1 có bài đã hoàn thành: sau seed lại, hoạt động gần nhất là hôm nay/hôm qua, không phải 60 ngày trước.
	var last time.Time
	db.Raw("SELECT max(completed_at) FROM lesson_progress WHERE user_id = ? AND status = 'completed'", users["student1@demo.com"].ID).Scan(&last)
	if last.Before(time.Now().AddDate(0, 0, -2)) {
		t.Errorf("hoạt động gần nhất của student1 là %v, muốn trong 2 ngày qua", last)
	}
}

func TestQA4_RepairDemoTimestamps_DerivesFromCreatedAt(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "student1@demo.com", "real@example.com")
	created := time.Now().AddDate(0, 0, -10)
	for _, u := range users {
		if err := db.Exec("UPDATE users SET created_at = ?, last_login_at = ? WHERE id = ?", created, created.AddDate(0, 0, -3), u.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	fixed, _, err := s.repairDemoTimestamps()
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 1 {
		t.Errorf("sửa %d tài khoản, muốn đúng 1 (chỉ tài khoản demo)", fixed)
	}
	var demo, real model.User
	db.First(&demo, "id = ?", users["student1@demo.com"].ID)
	db.First(&real, "id = ?", users["real@example.com"].ID)
	if demo.LastLoginAt == nil || demo.LastLoginAt.Before(demo.CreatedAt) {
		t.Errorf("demo: last_login_at=%v vẫn trước created_at=%v", demo.LastLoginAt, demo.CreatedAt)
	}
	if real.LastLoginAt == nil || !real.LastLoginAt.Before(real.CreatedAt) {
		t.Errorf("tài khoản không phải demo bị sửa: %v", real.LastLoginAt)
	}
	if again, _, _ := s.repairDemoTimestamps(); again != 0 {
		t.Errorf("chạy lại sửa thêm %d dòng, muốn 0", again)
	}
}
