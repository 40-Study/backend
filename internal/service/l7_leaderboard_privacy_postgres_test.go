package service

// Lane L7 (Postgres thật, schema tạm): `leaderboard_display` có tác dụng trên bảng xếp hạng điểm thưởng
// (GET /leaderboard, /leaderboard/me) và trên BXH cuộc thi (RankedRows -> contestLeaderboardItems), qua SQL thật
// (LEFT JOIN user_preferences ở cả đường sống lẫn đường đã chốt). Bỏ JOIN thì mọi người đều như "name" và test ĐỎ.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestL7_LeaderboardDisplay_Postgres(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	db := f.db

	mk := func(kind, full, avatar string, points int, display string) model.User {
		s := uuid.NewString()[:8]
		u := model.User{Email: "l7-lb-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "l7_" + kind + "_" + s, IsActive: true}
		if full != "" {
			u.FullName = &full
		}
		if avatar != "" {
			u.AvatarURL = &avatar
		}
		mustCreate(t, db, &u)
		grantSystemRole(t, db, u.ID, "STUDENT") // B-20: bảng xếp hạng chỉ liệt kê học viên
		mustCreate(t, db, &model.UserPoint{UserID: u.ID, TotalPoints: points})
		if display != "" {
			mustCreate(t, db, &model.UserPreference{UserID: u.ID, LeaderboardDisplay: display})
		}
		return u
	}
	anon := mk("anon", "Trần Thị Bí Mật", "https://cdn.test/secret.png", 900, LeaderboardDisplayAnonymous)
	byUsername := mk("uname", "Phạm Văn Kín", "https://cdn.test/kin.png", 800, LeaderboardDisplayUsername)
	open := mk("open", "Lê Văn Hiện", "", 700, LeaderboardDisplayName)
	noPref := mk("nopref", "Võ Thị Mặc Định", "", 600, "") // chưa có dòng cài đặt
	viewer := mk("viewer", "Người Xem", "", 10, "")

	// Kỳ tuần: thêm dòng leaderboard_entries cho cả bốn người để đường weekly cũng được kiểm.
	period := repository.CurrentPeriod("weekly")
	for i, u := range []model.User{anon, byUsername, open, noPref} {
		mustCreate(t, db, &model.LeaderboardEntry{UserID: u.ID, Period: period, PeriodType: "weekly", Points: 400 - i*10})
	}

	svc := NewLeaderboardService(repository.NewLeaderboardRepository(db))
	get := func(period string, v *LeaderboardViewer) (string, map[uuid.UUID]int) {
		resp, err := svc.GetLeaderboard(ctx, period, 100, v, nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[uuid.UUID]int{}
		for i, e := range resp.Entries {
			if e.UserID != nil {
				byID[*e.UserID] = i
			}
		}
		return string(raw), byID
	}

	for _, period := range []string{"all_time", "weekly"} {
		t.Run(period+": khách và người khác không thấy danh tính người ẩn danh", func(t *testing.T) {
			for name, v := range map[string]*LeaderboardViewer{"khách": nil, "người khác": {UserID: viewer.ID}} {
				raw, byID := get(period, v)
				for _, secret := range []string{*anon.FullName, anon.UserName, *anon.AvatarURL, anon.ID.String()} {
					if strings.Contains(raw, secret) {
						t.Errorf("%s: JSON lộ %q của người ẩn danh: %s", name, secret, raw)
					}
				}
				if !strings.Contains(raw, AnonymousLearnerLabel) {
					t.Errorf("%s: thiếu nhãn %q: %s", name, AnonymousLearnerLabel, raw)
				}
				if _, ok := byID[anon.ID]; ok {
					t.Errorf("%s: người ẩn danh vẫn có user_id trong bảng", name)
				}
				// Thứ hạng và điểm vẫn nguyên: ẩn danh chỉ che danh tính, không loại khỏi bảng.
				if !strings.Contains(raw, `"rank":1`) {
					t.Errorf("%s: mất dòng hạng 1 của người ẩn danh: %s", name, raw)
				}
				// username: giữ tên đăng nhập, bỏ họ tên.
				if strings.Contains(raw, *byUsername.FullName) || !strings.Contains(raw, byUsername.UserName) {
					t.Errorf("%s: người chọn username phải hiện tên đăng nhập và không hiện họ tên: %s", name, raw)
				}
				// name và chưa-có-cài-đặt: như cũ.
				for _, u := range []model.User{open, noPref} {
					if !strings.Contains(raw, *u.FullName) || !strings.Contains(raw, u.ID.String()) {
						t.Errorf("%s: %s phải hiện đầy đủ như trước: %s", name, u.UserName, raw)
					}
				}
			}
		})

		t.Run(period+": chính chủ và admin thấy tên thật", func(t *testing.T) {
			for name, v := range map[string]*LeaderboardViewer{
				"chính chủ": {UserID: anon.ID}, "admin": {UserID: viewer.ID, IsAdmin: true},
			} {
				raw, byID := get(period, v)
				if !strings.Contains(raw, *anon.FullName) || !strings.Contains(raw, anon.ID.String()) {
					t.Errorf("%s phải thấy tên thật của người ẩn danh: %s", name, raw)
				}
				if _, ok := byID[anon.ID]; !ok {
					t.Errorf("%s: thiếu user_id của người ẩn danh", name)
				}
			}
		})

		t.Run(period+": /me luôn thấy tên thật của chính mình", func(t *testing.T) {
			my, err := svc.GetMyRank(ctx, anon.ID, period)
			if err != nil || my.Entry == nil {
				t.Fatalf("GetMyRank: %+v err=%v", my, err)
			}
			if my.Entry.DisplayName != *anon.FullName || my.Entry.UserID == nil || *my.Entry.UserID != anon.ID || !my.Entry.IsMe {
				t.Errorf("người ẩn danh xem hạng của mình: %+v", my.Entry)
			}
		})
	}

	// BXH cuộc thi: dựng cuộc thi có 2 thí sinh (ẩn danh và công khai) rồi đọc qua RankedRows thật, cả đường sống
	// (chưa chốt) lẫn đường đã chốt (đọc contest_participants.rank).
	t.Run("cuộc thi: RankedRows mang cài đặt và BXH áp ẩn danh", func(t *testing.T) {
		creator := f.user("creator")
		q := f.standaloneQuiz(creator, false)
		now := time.Now()
		c := model.Contest{Title: "L7 contest", Slug: "l7-" + uuid.NewString()[:8], Type: model.ContestTypeQuiz, Status: "PUBLISHED",
			StartTime: now.Add(-48 * time.Hour), EndTime: now.Add(-24 * time.Hour), CreatedBy: creator, QuizID: &q.ID}
		mustCreate(t, db, &c)
		for _, u := range []model.User{anon, open} {
			attempt := f.startContestAttempt(q.ID, u.ID)
			done := now.Add(-25 * time.Hour)
			if err := db.Model(&model.QuizAttempt{}).Where("id = ?", attempt).
				Updates(map[string]any{"completed_at": done, "score": 4, "total_points": 5, "percentage": 80}).Error; err != nil {
				t.Fatal(err)
			}
			mustCreate(t, db, &model.ContestParticipant{ContestID: c.ID, UserID: u.ID, AttemptID: &attempt})
		}
		repo := repository.NewContestRepository(db)

		check := func(label string, finalized bool) {
			rows, _, err := repo.RankedRows(db, c.ID, finalized, 1, 20)
			if err != nil || len(rows) != 2 {
				t.Fatalf("%s: RankedRows: %d dòng err=%v", label, len(rows), err)
			}
			byUser := map[uuid.UUID]repository.RankedRow{}
			for _, r := range rows {
				byUser[r.UserID] = r
			}
			if got := byUser[anon.ID].LeaderboardDisplay; got != LeaderboardDisplayAnonymous {
				t.Errorf("%s: LeaderboardDisplay của người ẩn danh = %q", label, got)
			}
			if got := byUser[open.ID].LeaderboardDisplay; got != LeaderboardDisplayName {
				t.Errorf("%s: LeaderboardDisplay của người công khai = %q", label, got)
			}
			raw, _ := json.Marshal(contestLeaderboardItems(rows, &LeaderboardViewer{UserID: viewer.ID}))
			for _, secret := range []string{*anon.FullName, anon.UserName, *anon.AvatarURL} {
				if strings.Contains(string(raw), secret) {
					t.Errorf("%s: BXH cuộc thi lộ %q: %s", label, secret, raw)
				}
			}
			if !strings.Contains(string(raw), AnonymousLearnerLabel) || !strings.Contains(string(raw), *open.FullName) {
				t.Errorf("%s: BXH thiếu nhãn ẩn danh hoặc tên người công khai: %s", label, raw)
			}
		}
		check("đang xếp hạng sống", false)

		// Qua ContestService.Leaderboard (đường production): admin và chính chủ thấy tên thật, người lạ thì không.
		svc := NewContestService(repo, nil, nil, nil)
		for _, tc := range []struct {
			name     string
			actor    *ContestActor
			wantReal bool
		}{
			{"admin", &ContestActor{UserID: viewer.ID, ActiveRole: "SYSTEM_ADMIN", IsAdmin: true}, true},
			{"chính chủ", &ContestActor{UserID: anon.ID, ActiveRole: "STUDENT"}, true},
			{"người lạ", &ContestActor{UserID: viewer.ID, ActiveRole: "STUDENT"}, false},
		} {
			page, err := svc.Leaderboard(context.Background(), c.ID, tc.actor, 1, 20)
			if err != nil {
				t.Fatalf("Leaderboard(%s): %v", tc.name, err)
			}
			raw, _ := json.Marshal(page.Items)
			sawReal := strings.Contains(string(raw), *anon.FullName)
			if sawReal != tc.wantReal {
				t.Errorf("BXH cuộc thi cho %s: thấy tên thật = %v, muốn %v: %s", tc.name, sawReal, tc.wantReal, raw)
			}
			if !tc.wantReal && !strings.Contains(string(raw), AnonymousLearnerLabel) {
				t.Errorf("BXH cuộc thi cho %s: thiếu nhãn ẩn danh: %s", tc.name, raw)
			}
		}

		for rank, u := range []model.User{anon, open} {
			if err := repo.SetRankTx(db, c.ID, u.ID, rank+1); err != nil {
				t.Fatal(err)
			}
		}
		check("đã chốt", true)
	})
}
