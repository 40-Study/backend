package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
)

// Contract §6/§9 (l): sau khi B1 merge, QuizService PHẢI có gate — nếu wireContest quên
// SetContestGate, quiz gắn cuộc thi sẽ lộ nguyên đề + đáp án qua /api/quizzes/:id cho học viên.
// Kiểm bằng HÀNH VI thật (gate là field private của QuizService): người ngoài đọc quiz đang gắn
// cuộc thi phải nhận ErrQuizLockedByContest; người tạo cuộc thi thì không.
func TestWireContest_QuizGateLocksContestQuiz(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	repos := InitRepositories(db)
	s := &Services{Quiz: service.NewQuizService(repos.Quiz, nil, repos.Course, repos.Section, repos.Lesson, repos.Livestream, repos.Enrollment)}
	wireContest(s, repos)
	if s.Contest == nil {
		t.Fatal("wireContest khong tao ContestService")
	}

	owner := model.User{Email: "qa-contest-owner@wire.test", UserName: "owner", PasswordHash: "x"}
	stranger := model.User{Email: "qa-contest-student@wire.test", UserName: "student", PasswordHash: "x"}
	for _, u := range []*model.User{&owner, &stranger} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	quiz := model.Quiz{Title: "QA-contest wire quiz", TriggerType: "manual"}
	if err := db.Create(&quiz).Error; err != nil {
		t.Fatal(err)
	}
	d := 30
	c := model.Contest{Title: "QA-contest wire", Slug: "qa-contest-wire-" + uuid.NewString()[:6], Type: model.ContestTypeQuiz,
		Status: model.ContestStatusPublished, StartTime: time.Now().Add(time.Hour), EndTime: time.Now().Add(3 * time.Hour),
		DurationMinutes: &d, CreatedBy: owner.ID, QuizID: &quiz.ID}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err := s.Quiz.GetQuizByID(ctx, quiz.ID, stranger.ID, false); !errors.Is(err, service.ErrQuizLockedByContest) {
		t.Fatalf("hoc vien doc quiz cua cuoc thi: err=%v, muon ErrQuizLockedByContest (gate chua duoc noi?)", err)
	}
	if _, err := s.Quiz.GetQuizByID(ctx, quiz.ID, owner.ID, false); errors.Is(err, service.ErrQuizLockedByContest) {
		t.Fatal("nguoi tao cuoc thi bi khoa nham")
	}
}
