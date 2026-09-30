package seeds

// Lane D seed demo — chạy SeedDemoSocialAndPayouts HAI lần trên Postgres THẬT (schema tạm
// pgtest.IsolatedSchema, DROP khi xong; không có Postgres: Skip ở local, FAIL khi CI=true).
// Kiểm số bản ghi đúng kỳ vọng, không tăng ở lần 2, và dữ liệu hiện ra qua CHÍNH repository mà
// các trang web dùng (danh sách nhóm, hội thoại, ví giảng viên, lịch cá nhân).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

func socialTestUsers(t *testing.T, db *gorm.DB) map[string]model.User {
	t.Helper()
	users := map[string]model.User{}
	for _, email := range append([]string{"parent1@demo.com"}, socialDemoEmails...) {
		u := model.User{Email: email, PasswordHash: "x", UserName: "qa-social-" + uuid.NewString()[:8]}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", email, err)
		}
		users[email] = u
	}
	return users
}

func TestSeedDemoSocialAndPayouts_Postgres_IdempotentVaHienTrenTrang(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := socialTestUsers(t, db)
	courses := seedDemoCoursesOnce(t, s, users)

	want := []struct {
		model interface{}
		n     int64
	}{
		{&model.Group{}, 3}, {&model.GroupMember{}, 7}, {&model.GroupJoinRequest{}, 1},
		{&model.Conversation{}, 6}, {&model.ConversationParticipant{}, 13}, {&model.Message{}, 22},
		{&model.Order{}, 3}, {&model.OrderItem{}, 3}, {&model.InstructorPayout{}, 5}, {&model.TeacherProfile{}, 2},
		{&model.CourseExercise{}, 1}, {&model.ExerciseTestCase{}, 4},
		{&model.PersonalEvent{}, 5}, {&model.Report{}, 4},
	}
	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoSocialAndPayouts(users, courses); err != nil {
			t.Fatalf("lần %d: seed: %v", run, err)
		}
		for _, w := range want {
			if n := countRows(t, db, w.model); n != w.n {
				t.Errorf("lần %d: %T có %d bản ghi, muốn %d", run, w.model, n, w.n)
			}
		}
	}

	ctx := context.Background()
	id := func(email string) uuid.UUID { return users[email].ID }

	// (app)/groups: "Khám phá" không bao giờ lộ nhóm SECRET; "Nhóm của tôi" của student2 có cả 3 nhóm.
	groupRepo := repository.NewGroupRepository(db)
	if _, total, err := groupRepo.List(ctx, "", "", 1, 50); err != nil || total != 2 {
		t.Errorf("List nhóm công khai = %d (err %v), muốn 2 (PUBLIC+PRIVATE, không SECRET)", total, err)
	}
	if _, total, err := groupRepo.GetByUserID(ctx, id("student2@demo.com"), 1, 50); err != nil || total != 3 {
		t.Errorf("Nhóm của student2 = %d (err %v), muốn 3", total, err)
	}

	// (app)/messages: student1 thấy 4 hội thoại, 3 tin chưa đọc; message_count khớp số tin thật.
	if _, total, err := repository.NewConversationRepository(db).ListByUserID(ctx, id("student1@demo.com"), 1, 50); err != nil || total != 4 {
		t.Errorf("Hội thoại của student1 = %d (err %v), muốn 4", total, err)
	}
	if unread, err := repository.NewConversationParticipantRepository(db).GetTotalUnread(ctx, id("student1@demo.com")); err != nil || unread != 3 {
		t.Errorf("Tin chưa đọc của student1 = %d (err %v), muốn 3", unread, err)
	}
	var drift int64
	db.Raw(`SELECT COUNT(*) FROM conversations c WHERE c.message_count <>
		(SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id) OR c.last_message_id IS NULL`).Scan(&drift)
	if drift != 0 {
		t.Errorf("%d hội thoại có message_count/last_message_id lệch bảng messages", drift)
	}

	// (teacher)/teacher/wallet: số dư = phần GV của đơn completed − payout pending/approved/completed.
	wallet := repository.NewWalletRepository(db)
	for email, wantAvail := range map[string]int64{"teacher1@demo.com": 438200, "teacher2@demo.com": 429100} {
		earn, err := wallet.GetTeacherEarnings(id(email))
		if err != nil {
			t.Fatalf("earnings %s: %v", email, err)
		}
		sums, err := wallet.GetTeacherPayoutSums(id(email))
		if err != nil {
			t.Fatalf("payout sums %s: %v", email, err)
		}
		if avail := earn.TotalEarnings.Sub(sums.Reserved()); !avail.Equal(decimal.NewFromInt(wantAvail)) {
			t.Errorf("%s số dư khả dụng = %s, muốn %d", email, avail, wantAvail)
		}
		var open int64
		db.Model(&model.InstructorPayout{}).Where("instructor_id = ? AND status IN ?", id(email), model.PayoutOpenStatuses).Count(&open)
		if open != 1 {
			t.Errorf("%s có %d yêu cầu rút đang mở, muốn đúng 1", email, open)
		}
	}

	// Bài tập code đã gắn vào content "exercise" của bài thực hành Todo App.
	var unlinked int64
	db.Model(&model.LessonContent{}).Where("type = ? AND exercise_id IS NULL", "exercise").Count(&unlinked)
	if unlinked != 0 {
		t.Errorf("còn %d content exercise chưa gắn bài tập", unlinked)
	}

	// Lịch cá nhân trong 2 tuần tới (lớp + livestream do SeedDemoClasses đảm nhận).
	events, err := repository.NewPersonalEventRepository(db).ListByUserAndDateRange(ctx, id("student1@demo.com"), time.Now(), time.Now().AddDate(0, 0, 14))
	if err != nil || len(events) != 3 {
		t.Errorf("Lịch cá nhân student1 = %d (err %v), muốn 3", len(events), err)
	}

	var statuses int64
	db.Model(&model.Report{}).Distinct("status").Count(&statuses)
	if statuses != 4 {
		t.Errorf("reports có %d trạng thái khác nhau, muốn 4 (pending/reviewing/resolved/dismissed)", statuses)
	}
}
