package service

// Lane L7: cài đặt riêng tư `leaderboard_display` phải CÓ TÁC DỤNG trên mọi bảng xếp hạng (trước đây chỉ được lưu).
// Hàm thuần, không cần DB. Bỏ nhánh anonymous hoặc username trong presentLeaderboardUser, hoặc quên áp nó ở
// contestLeaderboardItems, thì test tương ứng ĐỎ. Phần chạy trên Postgres thật: l7_leaderboard_privacy_postgres_test.go.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/repository"
)

func TestL7_PresentLeaderboardUser_TheoNguoiXem(t *testing.T) {
	owner, other, admin := uuid.New(), uuid.New(), uuid.New()
	full, avatar := "Nguyễn Văn An", "https://cdn.test/an.png"
	viewers := map[string]*LeaderboardViewer{
		"khách":      nil,
		"người khác": {UserID: other},
		"chính chủ":  {UserID: owner},
		"admin":      {UserID: admin, IsAdmin: true},
	}

	cases := []struct {
		display, viewer string
		wantID          bool
		wantFull        bool
		wantAvatar      bool
		wantLabel       string
	}{
		{LeaderboardDisplayAnonymous, "khách", false, false, false, AnonymousLearnerLabel},
		{LeaderboardDisplayAnonymous, "người khác", false, false, false, AnonymousLearnerLabel},
		{LeaderboardDisplayAnonymous, "chính chủ", true, true, true, full},
		{LeaderboardDisplayAnonymous, "admin", true, true, true, full},
		{LeaderboardDisplayUsername, "khách", true, false, true, "an_nguyen"},
		{LeaderboardDisplayUsername, "người khác", true, false, true, "an_nguyen"},
		{LeaderboardDisplayUsername, "chính chủ", true, true, true, full},
		{LeaderboardDisplayUsername, "admin", true, true, true, full},
		{LeaderboardDisplayName, "khách", true, true, true, full},
		{"", "khách", true, true, true, full},           // chưa có dòng cài đặt
		{"giá trị lạ", "khách", true, true, true, full}, // không nhận ra: như mặc định, không ẩn danh
	}
	for _, c := range cases {
		t.Run(c.display+"/"+c.viewer, func(t *testing.T) {
			p := presentLeaderboardUser(c.display, owner, &full, "an_nguyen", &avatar, viewers[c.viewer])
			if (p.UserID != nil) != c.wantID {
				t.Errorf("UserID có=%v, muốn %v", p.UserID != nil, c.wantID)
			}
			if p.UserID != nil && *p.UserID != owner {
				t.Errorf("UserID=%v, muốn %v", *p.UserID, owner)
			}
			if (p.FullName != nil) != c.wantFull {
				t.Errorf("FullName có=%v, muốn %v", p.FullName != nil, c.wantFull)
			}
			if (p.AvatarURL != nil) != c.wantAvatar {
				t.Errorf("AvatarURL có=%v, muốn %v", p.AvatarURL != nil, c.wantAvatar)
			}
			if p.DisplayName != c.wantLabel {
				t.Errorf("DisplayName=%q, muốn %q", p.DisplayName, c.wantLabel)
			}
			if c.display == LeaderboardDisplayAnonymous && !c.wantID && p.UserName != "" {
				t.Errorf("ẩn danh mà vẫn lộ user_name %q", p.UserName)
			}
			if p.IsMe != (c.viewer == "chính chủ") {
				t.Errorf("IsMe=%v, viewer %s", p.IsMe, c.viewer)
			}
		})
	}
}

// Dòng BXH cuộc thi: người đặt ẩn danh hiện là "Học viên ẩn danh" với mọi người (kể cả khi tên thật và avatar nằm
// trong dòng DB) trừ chính họ và admin; JSON không được chứa tên thật ở bất kỳ field nào.
func TestL7_ContestLeaderboardItems_AnDanh(t *testing.T) {
	anon, shown, other, admin := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	anonFull, avatar := "Trần Thị Bí Mật", "https://cdn.test/secret.png"
	shownFull := "Lê Văn Hiện"
	rows := []repository.RankedRow{
		{UserID: anon, Rank: 1, UserName: anonFull, FullName: &anonFull, LoginName: "bi_mat_99", AvatarURL: &avatar,
			LeaderboardDisplay: LeaderboardDisplayAnonymous, Score: decimal.NewFromInt(9), CompletedAt: time.Now()},
		{UserID: shown, Rank: 2, UserName: shownFull, FullName: &shownFull, LoginName: "hien_le", LeaderboardDisplay: LeaderboardDisplayName,
			Score: decimal.NewFromInt(8), CompletedAt: time.Now()},
	}

	guest := contestLeaderboardItems(rows, &LeaderboardViewer{UserID: other})
	if len(guest) != 2 {
		t.Fatalf("có %d dòng, muốn 2", len(guest))
	}
	if guest[0].UserName != AnonymousLearnerLabel || guest[0].AvatarURL != nil || guest[0].IsMe {
		t.Errorf("dòng ẩn danh: %+v", guest[0])
	}
	if guest[1].UserName != shownFull {
		t.Errorf("dòng công khai: UserName=%q, muốn %q", guest[1].UserName, shownFull)
	}
	raw, _ := json.Marshal(guest)
	for _, secret := range []string{anonFull, "bi_mat_99", avatar, anon.String()} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("JSON bảng xếp hạng lộ %q: %s", secret, raw)
		}
	}

	if self := contestLeaderboardItems(rows, &LeaderboardViewer{UserID: anon}); self[0].UserName != anonFull || !self[0].IsMe || self[0].AvatarURL == nil {
		t.Errorf("chính chủ phải thấy tên thật của mình: %+v", self[0])
	}
	if adm := contestLeaderboardItems(rows, &LeaderboardViewer{UserID: admin, IsAdmin: true}); adm[0].UserName != anonFull || adm[0].IsMe {
		t.Errorf("admin phải thấy tên thật (và không phải 'tôi'): %+v", adm[0])
	}
	if g := contestLeaderboardItems(rows, nil); g[0].UserName != AnonymousLearnerLabel {
		t.Errorf("khách chưa đăng nhập: %+v", g[0])
	}
	// username: chỉ tên đăng nhập.
	rows[1].LeaderboardDisplay = LeaderboardDisplayUsername
	if u := contestLeaderboardItems(rows, nil); u[1].UserName != "hien_le" {
		t.Errorf("username: UserName=%q, muốn hien_le", u[1].UserName)
	}
	if len(contestLeaderboardItems(nil, nil)) != 0 || contestLeaderboardItems(nil, nil) == nil {
		t.Error("không có dòng nào phải trả slice rỗng không nil")
	}
}
