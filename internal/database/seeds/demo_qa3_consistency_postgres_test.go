package seeds

// QA3 Q22 / G-11 — dữ liệu demo không được mâu thuẫn: gói xu trùng nhau, số học viên ghi cứng khác số ghi danh, tên cuộc
// thi theo thời điểm nói dối, nhóm 0 thành viên dù có người. Mỗi test dựng trạng thái "DB đã seed bằng bản cũ" rồi chạy
// seed hai lần trên Postgres thật (schema tạm, DROP khi xong).

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestSeedDemoCoins_Postgres_RetiresLegacyNearDuplicatePackages(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "student1@demo.com", "student2@demo.com")

	// DB dev seed bằng bản cũ: 4 gói gần trùng, trong đó "Tiết Kiệm" cũng nổi bật.
	for i, name := range legacyCoinPackageNames {
		pkg := model.CoinPackage{Name: name, CoinAmount: 100 * int64(i+1), Price: money(20000 * int64(i+1)), Currency: "VND",
			IsActive: true, IsFeatured: name == "Gói Tiết Kiệm", SortOrder: i + 1}
		if err := db.Create(&pkg).Error; err != nil {
			t.Fatalf("tạo gói cũ %s: %v", name, err)
		}
	}

	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoCoins(users); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
		var active []model.CoinPackage
		if err := db.Where("is_active = true").Find(&active).Error; err != nil {
			t.Fatal(err)
		}
		if len(active) != len(demoCoinPackages) {
			t.Errorf("lần %d: %d gói đang bán, muốn %d (chỉ các gói demo hiện tại)", run, len(active), len(demoCoinPackages))
		}
		featured := 0
		for _, p := range active {
			for _, legacy := range legacyCoinPackageNames {
				if p.Name == legacy {
					t.Errorf("lần %d: gói cũ %q vẫn đang bán", run, legacy)
				}
			}
			if p.IsFeatured {
				featured++
			}
		}
		if featured != 1 {
			t.Errorf("lần %d: %d gói nổi bật đang bán, muốn đúng 1 (một nhãn Phổ biến)", run, featured)
		}
	}
	// Gói cũ chỉ tắt, không xoá (lịch sử mua có thể trỏ tới).
	if n := countRows(t, db, &model.CoinPackage{}); n != int64(len(demoCoinPackages)+len(legacyCoinPackageNames)) {
		t.Errorf("tổng gói = %d, muốn %d (gói cũ giữ lại ở trạng thái tắt)", n, len(demoCoinPackages)+len(legacyCoinPackageNames))
	}
}

func TestSeedDemoEnrollments_Postgres_TotalStudentsMatchesRealEnrollments(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "teacher1@demo.com", "teacher2@demo.com", "student1@demo.com", "student2@demo.com")
	courses := seedDemoCoursesOnce(t, s, users)

	// DB dev seed số giả (1.250 / 2.100 học viên) từ bản cũ.
	if err := db.Model(&model.Course{}).Where("1 = 1").UpdateColumn("total_students", 2100).Error; err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoEnrollments(users, courses); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
		var all []model.Course
		if err := db.Find(&all).Error; err != nil {
			t.Fatal(err)
		}
		for _, c := range all {
			var real int64
			db.Model(&model.Enrollment{}).Where("course_id = ?", c.ID).Count(&real)
			if int64(c.TotalStudents) != real {
				t.Errorf("lần %d: khoá %s total_students=%d, số ghi danh thật=%d", run, c.Slug, c.TotalStudents, real)
			}
		}
	}
	var git model.Course
	if err := db.Where("slug = ?", "git-github-cho-nguoi-moi-bat-dau").First(&git).Error; err != nil {
		t.Fatal(err)
	}
	if git.TotalStudents != 1 {
		t.Errorf("khoá Git total_students = %d, muốn 1 (chỉ student1 ghi danh), không còn 2.100", git.TotalStudents)
	}
}

func TestSeedDemoContests_Postgres_RenamesTimeBoundLegacyNamesInPlace(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "admin@demo.com", "teacher1@demo.com", "teacher2@demo.com", "student1@demo.com", "student2@demo.com")

	// DB đã seed bằng bản cũ: slug/tiêu đề theo thời điểm, lịch đã trôi về quá khứ (đã kết thúc).
	creator := users["teacher1@demo.com"].ID
	legacy := map[string]uuid.UUID{}
	for _, c := range []struct{ slug, title string }{
		{"demo-thi-javascript-sap-dien-ra", "Thử thách JavaScript ES6+ tháng 10"},
		{"demo-thi-react-hooks-sap-dien-ra", "React Hooks Challenge"},
		{"demo-thi-git-dang-dien-ra", "Thi nhanh Git cơ bản"},
	} {
		row := model.Contest{Slug: c.slug, Title: c.title, Type: model.ContestTypeQuiz, Status: model.ContestStatusPublished,
			StartTime: daysAgo(10), EndTime: daysAgo(9), IsPublic: true, CreatedBy: creator}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("tạo cuộc thi cũ %s: %v", c.slug, err)
		}
		legacy[c.slug] = row.ID
	}

	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoContests(users); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
		var lying int64
		db.Model(&model.Contest{}).Where("slug LIKE ? OR slug LIKE ?", "%sap-dien-ra", "%dang-dien-ra").Count(&lying)
		if lying != 0 {
			t.Errorf("lần %d: còn %d cuộc thi có slug theo thời điểm", run, lying)
		}
		for oldSlug, id := range legacy {
			var n int64
			db.Model(&model.Contest{}).Where("id = ?", id).Count(&n)
			if n != 1 {
				t.Errorf("lần %d: dòng cũ %s bị mất/nhân đôi (đếm theo id = %d)", run, oldSlug, n)
			}
		}
		var js model.Contest
		if err := db.Where("slug = ?", "demo-thi-javascript-es6").First(&js).Error; err != nil {
			t.Fatalf("lần %d: không có cuộc thi demo-thi-javascript-es6: %v", run, err)
		}
		if js.ID != legacy["demo-thi-javascript-sap-dien-ra"] || js.Title != "Thử thách JavaScript ES6+" {
			t.Errorf("lần %d: JS phải là dòng cũ đổi tên (id %s, title %q)", run, js.ID, js.Title)
		}
		if !js.StartTime.After(daysAgo(0)) {
			t.Errorf("lần %d: lịch cuộc thi sắp diễn ra phải được đưa lại tương lai, start=%v", run, js.StartTime)
		}
		var total int64
		db.Model(&model.Contest{}).Count(&total)
		if want := int64(len(allDemoContestSlugs())); total != want {
			t.Errorf("lần %d: %d cuộc thi, muốn %d (không nhân bản bản cũ)", run, total, want)
		}
	}
}

func TestSeedDemoGroups_Postgres_FixesLegacyZeroMemberCount(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := socialTestUsers(t, db)
	courses := seedDemoCoursesOnce(t, s, users)

	owner := users["teacher1@demo.com"]
	legacy := model.Group{Name: "Nhóm dự án Web ForteX", Slug: "nhom-du-an-web-forte-x", Type: model.GroupTypeCustom,
		Privacy: model.GroupPrivacyPublic, MaxMembers: 100, CreatedBy: owner.ID}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"student1@demo.com", "student2@demo.com", "teacher1@demo.com"} {
		m := model.GroupMember{GroupID: legacy.ID, UserID: users[email].ID, Role: model.GroupRoleMember, Status: model.GroupMemberActive}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	banned := model.GroupMember{GroupID: legacy.ID, UserID: users["teacher2@demo.com"].ID, Role: model.GroupRoleMember, Status: model.GroupMemberBanned}
	if err := db.Create(&banned).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Group{}).Where("id = ?", legacy.ID).UpdateColumn("member_count", 0).Error; err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if _, err := s.SeedDemoGroups(users, courses); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
		var got model.Group
		if err := db.First(&got, "id = ?", legacy.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.MemberCount != 3 {
			t.Errorf("lần %d: member_count = %d, muốn 3 (chỉ thành viên ACTIVE)", run, got.MemberCount)
		}
	}
}
