package database

import (
	"strings"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// Bạn bè/nhóm (plan 260930): schema + post-migration cho friendships, user_blocks và CHECK loại thông báo.

func mustExec(t *testing.T, exec func(string, ...any) error, sql string, args ...any) {
	t.Helper()
	if err := exec(sql, args...); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func TestFriendsSchema_RangBuocTaiTangDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	exec := func(sql string, args ...any) error { return db.Exec(sql, args...).Error }

	mustExec(t, exec, `INSERT INTO users (id, email, password_hash, user_name) VALUES
		('00000000-0000-0000-0000-00000000000a', 'a@t.test', 'x', 'a'),
		('00000000-0000-0000-0000-00000000000b', 'b@t.test', 'x', 'b')`)
	const a, b = "00000000-0000-0000-0000-00000000000a", "00000000-0000-0000-0000-00000000000b"

	mustExec(t, exec, `INSERT INTO friendships (requester_id, addressee_id, status, requested_at) VALUES (?, ?, 'PENDING', now())`, a, b)

	t.Run("chống trùng HAI CHIỀU: B->A khi đã có A->B bị chặn", func(t *testing.T) {
		if err := exec(`INSERT INTO friendships (requester_id, addressee_id, status, requested_at) VALUES (?, ?, 'PENDING', now())`, b, a); err == nil {
			t.Fatal("dòng thứ hai của cùng một cặp (chiều ngược) được chấp nhận: thiếu uq_friendships_pair")
		}
	})
	t.Run("tự kết bạn với mình bị chặn", func(t *testing.T) {
		if err := exec(`INSERT INTO friendships (requester_id, addressee_id, status, requested_at) VALUES (?, ?, 'PENDING', now())`, a, a); err == nil {
			t.Fatal("requester = addressee được chấp nhận")
		}
	})
	t.Run("trạng thái ngoài SSOT bị chặn, mọi trạng thái SSOT hợp lệ", func(t *testing.T) {
		if err := exec(`UPDATE friendships SET status = 'BLOCKED'`); err == nil {
			t.Fatal("status ngoài model.FriendshipStatuses được chấp nhận")
		}
		for _, s := range model.FriendshipStatuses {
			mustExec(t, exec, `UPDATE friendships SET status = ?`, s)
		}
	})
	t.Run("user_blocks: trùng cặp và tự chặn bị chặn", func(t *testing.T) {
		mustExec(t, exec, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)`, a, b)
		if err := exec(`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)`, a, b); err == nil {
			t.Fatal("chặn trùng cặp được chấp nhận")
		}
		if err := exec(`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)`, a, a); err == nil {
			t.Fatal("tự chặn mình được chấp nhận")
		}
		// Hai chiều chặn là hai dòng hợp lệ khác nhau.
		mustExec(t, exec, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)`, b, a)
	})
}

func TestFriendsSchema_CheckLoaiThongBaoChapNhanBaLoaiMoi(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	exec := func(sql string, args ...any) error { return db.Exec(sql, args...).Error }
	mustExec(t, exec, `INSERT INTO users (id, email, password_hash, user_name) VALUES
		('00000000-0000-0000-0000-00000000000a', 'a@t.test', 'x', 'a')`)
	for _, typ := range model.NotificationTypes {
		mustExec(t, exec, `INSERT INTO notifications (user_id, title, content, notification_type) VALUES ('00000000-0000-0000-0000-00000000000a', 't', 'c', ?)`, typ)
	}
	if err := exec(`INSERT INTO notifications (user_id, title, content, notification_type) VALUES ('00000000-0000-0000-0000-00000000000a', 't', 'c', 'khong_ton_tai')`); err == nil {
		t.Fatal("loại thông báo ngoài SSOT được chấp nhận")
	}
}

// DB cũ (constraint có 11 giá trị) phải được NÂNG lên đủ 14 giá trị bởi RunPostMigrations; sau đó chạy lại
// một bản backend cũ (rollback) không được làm khởi động lỗi dù đã có dòng mang loại mới.
func TestFriendsSchema_NangCapVaRollbackCheckLoaiThongBao(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	exec := func(sql string, args ...any) error { return db.Exec(sql, args...).Error }
	mustExec(t, exec, `INSERT INTO users (id, email, password_hash, user_name) VALUES
		('00000000-0000-0000-0000-00000000000a', 'a@t.test', 'x', 'a')`)

	oldTypes := model.NotificationTypes[:11]
	mustExec(t, exec, `ALTER TABLE notifications DROP CONSTRAINT chk_notifications_notification_type`)
	mustExec(t, exec, buildCheckConstraintSQL("notifications", "chk_notifications_notification_type", "notification_type", oldTypes))
	if err := exec(`INSERT INTO notifications (user_id, title, content, notification_type) VALUES ('00000000-0000-0000-0000-00000000000a', 't', 'c', 'group_added')`); err == nil {
		t.Fatal("DB giả lập bản cũ lại nhận group_added: bài kiểm nâng cấp vô nghĩa")
	}

	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations trên DB cũ: %v", err)
	}
	mustExec(t, exec, `INSERT INTO notifications (user_id, title, content, notification_type) VALUES ('00000000-0000-0000-0000-00000000000a', 't', 'c', 'group_added')`)
	var def string
	db.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'chk_notifications_notification_type' AND conrelid = 'notifications'::regclass`).Scan(&def)
	for _, typ := range []string{"friend_request", "friend_accepted", "group_added"} {
		if !strings.Contains(def, "'"+typ+"'") {
			t.Fatalf("constraint sau nâng cấp thiếu %s: %s", typ, def)
		}
	}

	// pg_constraint là catalog TOÀN CỤC: không giới hạn conrelid thì dòng của schema test song song khác (cùng tên constraint,
	// đã validated) có thể bị đọc nhầm, làm test flaky. 'notifications'::regclass phân giải theo search_path của kết nối.
	// Rollback: bản backend cũ boot lại với danh sách 11 giá trị trong khi DB đã có dòng group_added.
	mustExec(t, exec, buildCheckConstraintSQL("notifications", "chk_notifications_notification_type", "notification_type", oldTypes))
	var validated bool
	db.Raw(`SELECT convalidated FROM pg_constraint WHERE conname = 'chk_notifications_notification_type' AND conrelid = 'notifications'::regclass`).Scan(&validated)
	if validated {
		t.Fatal("còn dòng group_added mà constraint 11 giá trị lại báo validated")
	}
}
