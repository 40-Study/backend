package database

// Lane R2 (QA hồi quy 03/10/2026), Postgres thật, schema tạm tối giản: hai bước sửa MỘT LẦN dữ liệu cũ.
//
//   - TestR2Cleanup_DongHoiThoaiNhomMoCoi (A-03): hội thoại GROUP của nhóm đã xoá mềm / không còn bị đóng
//     (participant có left_at, hội thoại xoá mềm); hội thoại của nhóm còn sống và DM không bị đụng.
//   - TestR2Cleanup_DoiUserNameChuaAt (B-03): user_name có '@' (kể cả tài khoản xoá mềm) đổi sang tên không còn
//     '@', hợp lệ với utils.IsValidUserName, KHÔNG suy ra từ email, duy nhất; tên hợp lệ giữ nguyên.
//   - TestR2Cleanup_ChayDungMotLan: bản ghi đánh dấu chặn lần chạy sau, kể cả khi sau đó có dữ liệu xấu mới.
//
// Bỏ UPDATE ở một bước, hoặc bỏ điều kiện/bản ghi đánh dấu, thì test tương ứng ĐỎ.

import (
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

const r2Schema = `
	CREATE TABLE users (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text, user_name text, full_name text,
		deleted_at timestamptz, updated_at timestamptz DEFAULT now());
	CREATE TABLE groups (id uuid PRIMARY KEY, deleted_at timestamptz);
	CREATE TABLE conversations (id uuid PRIMARY KEY, type text, group_id uuid, deleted_at timestamptz);
	CREATE TABLE conversation_participants (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), conversation_id uuid,
		left_at timestamptz, unread_count int DEFAULT 0);
`

const (
	r2GroupAlive   = "00000000-0000-0000-0000-0000000000a1"
	r2GroupDeleted = "00000000-0000-0000-0000-0000000000a2"
	r2GroupMissing = "00000000-0000-0000-0000-0000000000a3"
	r2ConvAlive    = "00000000-0000-0000-0000-0000000000c1"
	r2ConvDeleted  = "00000000-0000-0000-0000-0000000000c2"
	r2ConvMissing  = "00000000-0000-0000-0000-0000000000c3"
	r2ConvDirect   = "00000000-0000-0000-0000-0000000000c4"
)

func r2Setup(t *testing.T) *gorm.DB {
	t.Helper()
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(r2Schema).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func r2Count(t *testing.T, db *gorm.DB, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(sql, args...).Scan(&n).Error; err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func TestR2Cleanup_DongHoiThoaiNhomMoCoi(t *testing.T) {
	db := r2Setup(t)
	stmts := []string{
		`INSERT INTO groups VALUES ('` + r2GroupAlive + `', NULL), ('` + r2GroupDeleted + `', now())`,
		`INSERT INTO conversations VALUES
			('` + r2ConvAlive + `', 'GROUP', '` + r2GroupAlive + `', NULL),
			('` + r2ConvDeleted + `', 'GROUP', '` + r2GroupDeleted + `', NULL),
			('` + r2ConvMissing + `', 'GROUP', '` + r2GroupMissing + `', NULL),
			('` + r2ConvDirect + `', 'DIRECT', NULL, NULL)`,
	}
	for _, c := range []string{r2ConvAlive, r2ConvDeleted, r2ConvMissing, r2ConvDirect} {
		stmts = append(stmts, `INSERT INTO conversation_participants (conversation_id, unread_count) VALUES ('`+c+`', 3), ('`+c+`', 0)`)
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatal(err)
		}
	}

	for run := 1; run <= 2; run++ {
		if err := runR2SocialPrivacyCleanup(db); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
	}

	for name, c := range map[string]string{"nhóm đã xoá mềm": r2ConvDeleted, "nhóm không còn": r2ConvMissing} {
		if n := r2Count(t, db, `SELECT count(*) FROM conversations WHERE id = ? AND deleted_at IS NOT NULL`, c); n != 1 {
			t.Errorf("hội thoại của %s chưa bị đóng", name)
		}
		if n := r2Count(t, db, `SELECT count(*) FROM conversation_participants WHERE conversation_id = ? AND left_at IS NULL`, c); n != 0 {
			t.Errorf("hội thoại của %s còn %d participant chưa rời", name, n)
		}
		if n := r2Count(t, db, `SELECT count(*) FROM conversation_participants WHERE conversation_id = ? AND unread_count <> 0`, c); n != 0 {
			t.Errorf("hội thoại của %s còn chưa đọc", name)
		}
	}
	for name, c := range map[string]string{"nhóm còn sống": r2ConvAlive, "DM": r2ConvDirect} {
		if n := r2Count(t, db, `SELECT count(*) FROM conversations WHERE id = ? AND deleted_at IS NULL`, c); n != 1 {
			t.Errorf("hội thoại %s bị đóng nhầm", name)
		}
		if n := r2Count(t, db, `SELECT count(*) FROM conversation_participants WHERE conversation_id = ? AND left_at IS NULL`, c); n != 2 {
			t.Errorf("participant của hội thoại %s bị gỡ nhầm", name)
		}
	}
}

func TestR2Cleanup_DoiUserNameChuaAt(t *testing.T) {
	db := r2Setup(t)
	if err := db.Exec(`INSERT INTO users (email, user_name, full_name, deleted_at) VALUES
		('tvanle.dev@gmail.com', 'tvanle.dev@gmail.com', 'Lê Trọng', NULL),
		('x@y.vn', 'x@y.vn', NULL, now()),
		('ok@y.vn', 'student1_qa', 'Học Viên', NULL),
		('t@y.vn', 'letrong', 'Lê Trọng', NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := runR2SocialPrivacyCleanup(db); err != nil {
		t.Fatal(err)
	}

	if n := r2Count(t, db, `SELECT count(*) FROM users WHERE position('@' in user_name) > 0`); n != 0 {
		t.Fatalf("còn %d user_name chứa '@'", n)
	}
	var names []struct{ Email, UserName string }
	if err := db.Raw(`SELECT email, user_name FROM users`).Scan(&names).Error; err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	valid := regexp.MustCompile(`^[a-z0-9]+$`)
	for _, u := range names {
		if seen[strings.ToLower(u.UserName)] {
			t.Errorf("user_name trùng: %q", u.UserName)
		}
		seen[strings.ToLower(u.UserName)] = true
		if !utils.IsValidUserName(u.UserName) {
			t.Errorf("user_name %q của %s không hợp lệ theo utils.IsValidUserName", u.UserName, u.Email)
		}
		if strings.Contains(u.Email, "@") && strings.HasPrefix(u.UserName, strings.Split(u.Email, "@")[0]) && u.Email != "ok@y.vn" && u.Email != "t@y.vn" {
			t.Errorf("user_name %q suy ra từ email %s", u.UserName, u.Email)
		}
	}
	// Tên hợp lệ giữ nguyên; tên sinh mới chỉ gồm chữ thường và số.
	if n := r2Count(t, db, `SELECT count(*) FROM users WHERE user_name IN ('student1_qa', 'letrong')`); n != 2 {
		t.Errorf("tên hợp lệ bị đổi nhầm (còn %d/2 dòng giữ nguyên)", n)
	}
	for _, email := range []string{"tvanle.dev@gmail.com", "x@y.vn"} {
		var got string
		if err := db.Raw(`SELECT user_name FROM users WHERE email = ?`, email).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		if !valid.MatchString(got) {
			t.Errorf("tên mới của %s = %q, muốn chỉ gồm chữ thường và số", email, got)
		}
	}
	// Tài khoản có họ tên sinh từ họ tên không dấu ("letrong" đã bị 'letrong' chiếm nên có hậu tố ngẫu nhiên).
	var withName string
	if err := db.Raw(`SELECT user_name FROM users WHERE email = 'tvanle.dev@gmail.com'`).Scan(&withName).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(withName, "letrong") {
		t.Errorf("tên mới %q muốn bắt đầu bằng họ tên không dấu 'letrong'", withName)
	}
}

func TestR2Cleanup_ChayDungMotLan(t *testing.T) {
	db := r2Setup(t)
	if err := runR2SocialPrivacyCleanup(db); err != nil {
		t.Fatal(err)
	}
	// Sau lần chạy đầu mới xuất hiện dữ liệu xấu: bản ghi đánh dấu chặn, không quét lại.
	if err := db.Exec(`INSERT INTO users (email, user_name) VALUES ('late@y.vn', 'late@y.vn')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := runR2SocialPrivacyCleanup(db); err != nil {
		t.Fatal(err)
	}
	if n := r2Count(t, db, `SELECT count(*) FROM users WHERE user_name = 'late@y.vn'`); n != 1 {
		t.Errorf("bước sửa chạy lại dù đã có bản ghi đánh dấu (còn %d dòng 'late@y.vn')", n)
	}
	if n := r2Count(t, db, `SELECT count(*) FROM data_migrations WHERE name IN (?, ?)`,
		r2OrphanGroupConversationsName, r2UserNameWithAtRenameName); n != 2 {
		t.Errorf("data_migrations có %d bản ghi đánh dấu của R2, muốn đúng 2", n)
	}
}
