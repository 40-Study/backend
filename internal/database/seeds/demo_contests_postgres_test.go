package seeds

// Seed demo "Cuộc thi" trên Postgres THẬT trong schema tạm (pgtest.IsolatedSchema, DROP khi xong).
// Không có Postgres: Skip ở local, FAIL khi CI=true.

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

// laneADemoUsers tạo tài khoản demo tối thiểu (không cần role) cho test seed lane A.
func laneADemoUsers(t *testing.T, db *gorm.DB, emails ...string) map[string]model.User {
	t.Helper()
	users := map[string]model.User{}
	for _, email := range emails {
		u := model.User{Email: email, PasswordHash: "x", UserName: "qa-seed-" + uuid.NewString()[:8]}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", email, err)
		}
		users[email] = u
	}
	return users
}

func listContests(t *testing.T, repo *repository.ContestRepository, f repository.ContestListFilter) []model.Contest {
	t.Helper()
	f.Now, f.Page, f.Limit = time.Now(), 1, 50
	out, _, err := repo.List(context.Background(), f)
	if err != nil {
		t.Fatalf("list contests: %v", err)
	}
	return out
}

// TestSeedDemoContests_Postgres_DuTabVaIdempotent:
//   - Danh sách công khai có đủ phase UPCOMING(2)/ACTIVE(1)/ENDED(1)/FINALIZED(1) qua đúng
//     ContestRepository.List mà API dùng; có 1 PENDING_REVIEW và 1 DRAFT của teacher2.
//   - student1 có mặt ở 4 cuộc thi với 4 phase khác nhau (/contests/me).
//   - participant_count khớp contest_participants; bài làm ENDED nằm trong cửa sổ thi, điểm khớp đáp án.
//   - Chạy lại không nhân bản; UPCOMING bị "trôi" về quá khứ được đưa lại tương lai.
func TestSeedDemoContests_Postgres_DuTabVaIdempotent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	repo := repository.NewContestRepository(db)
	users := laneADemoUsers(t, db, "admin@demo.com", "teacher1@demo.com", "teacher2@demo.com",
		"student1@demo.com", "student2@demo.com")

	type counts struct{ contests, quizzes, questions, answers, prizes, participants, attempts, attemptAnswers int64 }
	snapshot := func() counts {
		return counts{countRows(t, db, &model.Contest{}), countRows(t, db, &model.Quiz{}), countRows(t, db, &model.Question{}),
			countRows(t, db, &model.QuestionAnswer{}), countRows(t, db, &model.ContestPrize{}),
			countRows(t, db, &model.ContestParticipant{}), countRows(t, db, &model.QuizAttempt{}),
			countRows(t, db, &model.QuizAttemptAnswer{})}
	}

	var first counts
	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoContests(users); err != nil {
			t.Fatalf("lần %d: seed contests: %v", run, err)
		}
		got := snapshot()
		if run == 1 {
			first = got
			if got.contests != 7 || got.participants != 7 || got.attempts != 4 || got.attemptAnswers != 8 {
				t.Fatalf("lần 1: %+v, muốn 7 contest / 7 participant / 4 attempt / 8 attempt answer", got)
			}
		} else if got != first {
			t.Errorf("lần 2: %+v, muốn giữ nguyên %+v — seed lại không được nhân bản", got, first)
		}

		for phase, want := range map[string]int{model.ContestPhaseUpcoming: 2, model.ContestPhaseActive: 1,
			model.ContestPhaseEnded: 1, model.ContestPhaseFinalized: 1} {
			if n := len(listContests(t, repo, repository.ContestListFilter{PublicOnly: true, Phase: phase})); n != want {
				t.Errorf("lần %d: danh sách công khai phase %s có %d, muốn %d", run, phase, n, want)
			}
		}
		for status, want := range map[string]int{model.ContestStatusPendingReview: 1, model.ContestStatusDraft: 1} {
			for _, c := range listContests(t, repo, repository.ContestListFilter{Status: status}) {
				want--
				if !c.StartTime.After(time.Now()) {
					t.Errorf("lần %d: %s %s có start_time quá khứ — admin/giáo viên không thao tác được", run, status, c.Slug)
				}
			}
			if want != 0 {
				t.Errorf("lần %d: số cuộc thi %s lệch %d so với kỳ vọng 1", run, status, -want)
			}
		}
		t2 := users["teacher2@demo.com"].ID
		if n := len(listContests(t, repo, repository.ContestListFilter{CreatedBy: &t2, Status: model.ContestStatusDraft})); n != 1 {
			t.Errorf("lần %d: teacher2 có %d bản nháp, muốn 1", run, n)
		}

		s1 := users["student1@demo.com"].ID
		phases := map[string]bool{}
		for _, c := range listContests(t, repo, repository.ContestListFilter{JoinedBy: &s1}) {
			phases[c.Phase(time.Now())] = true
		}
		if len(phases) != 4 {
			t.Errorf("lần %d: student1 tham gia các phase %v, muốn đủ UPCOMING/ACTIVE/ENDED/FINALIZED", run, phases)
		}

		var drift int64
		db.Raw(`SELECT COUNT(*) FROM contests c WHERE participant_count <>
			(SELECT COUNT(*) FROM contest_participants p WHERE p.contest_id = c.id)`).Scan(&drift)
		if drift != 0 {
			t.Errorf("lần %d: %d cuộc thi có participant_count lệch contest_participants", run, drift)
		}

		// Mô phỏng nhiều ngày trôi qua: cuộc thi UPCOMING đã về quá khứ, lần seed sau phải đưa lại tương lai.
		if run == 1 {
			if err := db.Model(&model.Contest{}).Where("slug LIKE ?", "%sap-dien-ra").
				Updates(map[string]interface{}{"start_time": time.Now().AddDate(0, 0, -5), "end_time": time.Now().AddDate(0, 0, -4)}).Error; err != nil {
				t.Fatalf("làm trôi lịch: %v", err)
			}
		}
	}

	var ended model.Contest
	if err := db.Where("slug = ?", "demo-thi-sql-da-ket-thuc").First(&ended).Error; err != nil {
		t.Fatalf("đọc cuộc thi ENDED: %v", err)
	}
	var attempts []model.QuizAttempt
	db.Where("quiz_id = ?", *ended.QuizID).Find(&attempts)
	for _, a := range attempts {
		if a.CompletedAt == nil || a.StartedAt.Before(ended.StartTime) || a.CompletedAt.After(ended.EndTime) {
			t.Errorf("attempt %s nằm ngoài cửa sổ thi %v-%v", a.ID, ended.StartTime, ended.EndTime)
		}
		var earned decimal.Decimal
		db.Raw("SELECT COALESCE(SUM(points_earned),0) FROM quiz_attempt_answers WHERE attempt_id = ?", a.ID).Scan(&earned)
		if a.Score == nil || !a.Score.Equal(earned) {
			t.Errorf("attempt %s score=%v, tổng điểm đáp án=%s", a.ID, a.Score, earned)
		}
	}
}
