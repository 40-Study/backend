package service

// Lane B2 "Cuộc thi" — ContestQuizEngine trên Postgres thật: start idempotent + race, không lộ đáp án,
// xáo tất định, nộp đôi (contract §4, §9 B2 (c), (d)).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
)

// Attempt tạo trong transaction bị rollback không được để lại dòng nào — engine phải ghi TRÊN tx của
// caller, không trên kết nối gốc.
func TestCreateContestAttemptTx_Rollback_KhongDeLaiAttempt(t *testing.T) {
	f := newContestFixture(t)
	owner, student := f.user("owner"), f.user("student")
	cq := f.standaloneQuiz(owner, false)

	rollback := errors.New("rollback có chủ đích")
	err := f.db.Transaction(func(tx *gorm.DB) error {
		if _, err := f.quiz.CreateContestAttemptTx(context.Background(), tx, cq.ID, student, time.Now()); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("muốn lỗi rollback có chủ đích, nhận %v", err)
	}
	if n := f.count("quiz_attempts", "quiz_id = ?", cq.ID); n != 0 {
		t.Errorf("tx đã rollback mà còn %d attempt", n)
	}
}

// Start lần 2 (reload trang) trả lại ĐÚNG attempt cũ, mode "contest".
func TestCreateContestAttemptTx_StartLai_TraDungAttemptCu(t *testing.T) {
	f := newContestFixture(t)
	owner, student := f.user("owner"), f.user("student")
	cq := f.standaloneQuiz(owner, false)

	first := f.startContestAttempt(cq.ID, student)
	second := f.startContestAttempt(cq.ID, student)
	if first != second {
		t.Errorf("start lại phải trả attempt cũ %s, nhận %s", first, second)
	}
	if n := f.count("quiz_attempts", "quiz_id = ? AND user_id = ? AND mode = ?", cq.ID, student, QuizAttemptModeContest); n != 1 {
		t.Errorf("muốn đúng 1 attempt contest, có %d", n)
	}
}

// Hai request start đồng thời: A tạo attempt nhưng CHƯA commit, B start giữa chừng. B phải đợi A rồi
// nhận đúng attempt của A — không được tạo attempt thứ hai. Kịch bản dựng tất định (A giữ tx mở tới
// khi B đã gọi), không phụ thuộc may rủi của goroutine.
func TestCreateContestAttemptTx_HaiLanStartDongThoi_MotAttempt(t *testing.T) {
	f := newContestFixture(t)
	owner, student := f.user("owner"), f.user("student")
	cq := f.standaloneQuiz(owner, false)
	ctx := context.Background()

	aCreated := make(chan struct{})
	bCalling := make(chan struct{})
	var idA, idB uuid.UUID
	var errA, errB error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errA = f.db.Transaction(func(tx *gorm.DB) error {
			var err error
			if idA, err = f.quiz.CreateContestAttemptTx(ctx, tx, cq.ID, student, time.Now()); err != nil {
				close(aCreated)
				return err
			}
			close(aCreated)
			<-bCalling
			time.Sleep(300 * time.Millisecond) // để B chắc chắn đã chạm tới khoá trước khi A commit
			return nil
		})
	}()
	go func() {
		defer wg.Done()
		<-aCreated
		errB = f.db.Transaction(func(tx *gorm.DB) error {
			close(bCalling)
			var err error
			idB, err = f.quiz.CreateContestAttemptTx(ctx, tx, cq.ID, student, time.Now())
			return err
		})
	}()
	wg.Wait()
	if errA != nil || errB != nil {
		t.Fatalf("start lỗi: A=%v B=%v", errA, errB)
	}
	if idA != idB {
		t.Errorf("hai lần start đồng thời ra 2 attempt khác nhau: %s và %s", idA, idB)
	}
	if n := f.count("quiz_attempts", "quiz_id = ? AND user_id = ?", cq.ID, student); n != 1 {
		t.Errorf("muốn đúng 1 attempt, có %d", n)
	}
}

// (d) Đề thi không mang bất kỳ field đáp án nào; reload ra cùng thứ tự.
func TestGetContestAttemptQuestions_KhongLoDapAn_OnDinhKhiReload(t *testing.T) {
	f := newContestFixture(t)
	owner, student := f.user("owner"), f.user("student")
	cq := f.standaloneQuiz(owner, true)
	attemptID := f.startContestAttempt(cq.ID, student)
	ctx := context.Background()

	first, err := f.quiz.GetContestAttemptQuestions(ctx, cq.ID, attemptID)
	if err != nil {
		t.Fatalf("GetContestAttemptQuestions: %v", err)
	}
	if len(first) != 4 {
		t.Fatalf("muốn 4 câu, nhận %d", len(first))
	}
	raw, _ := json.Marshal(first)
	// Quét cả TÊN field lẫn GIÁ TRỊ đáp án thật: "Hà Nội" là đáp án câu fill_blank (review PR #80,
	// F1 — bản trước chỉ quét tên field nên xanh dù đề đang lộ đáp án).
	for _, leak := range []string{"is_correct", "correct_answer_ids", "explanation", "Giải thích bí mật", "answer_key", "Hà Nội"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("đề thi lộ %q: %s", leak, raw)
		}
	}
	second, err := f.quiz.GetContestAttemptQuestions(ctx, cq.ID, attemptID)
	if err != nil {
		t.Fatalf("lần 2: %v", err)
	}
	raw2, _ := json.Marshal(second)
	if string(raw) != string(raw2) {
		t.Errorf("reload ra thứ tự khác:\n%s\n%s", raw, raw2)
	}
	// Attempt của người khác / quiz khác không đọc được đề qua engine.
	if _, err := f.quiz.GetContestAttemptQuestions(ctx, uuid.New(), attemptID); err == nil {
		t.Errorf("attempt không khớp quiz phải bị từ chối")
	}
}

// (d) Xáo tất định: cùng attempt_id -> cùng thứ tự; khác attempt_id -> khác thứ tự; tắt xáo giữ nguyên.
func TestShuffleContestQuestions_TatDinhTheoAttempt(t *testing.T) {
	build := func() []dto.AttemptQuestionDTO {
		qs := make([]dto.AttemptQuestionDTO, 8)
		for i := range qs {
			qs[i] = dto.AttemptQuestionDTO{ID: uuid.MustParse("00000000-0000-0000-0000-00000000000" + string(rune('1'+i))), DisplayOrder: i + 1}
			for j := 0; j < 4; j++ {
				qs[i].Answers = append(qs[i].Answers, dto.AttemptAnswerDTO{ID: uuid.New(), DisplayOrder: j + 1})
			}
		}
		return qs
	}
	order := func(qs []dto.AttemptQuestionDTO) string {
		var b strings.Builder
		for _, q := range qs {
			b.WriteString(q.ID.String()[35:])
		}
		return b.String()
	}
	seedA := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	seedB := uuid.MustParse("99999999-8888-7777-6666-555555555555")

	a1, a2, b1, plain := build(), build(), build(), build()
	shuffleContestQuestions(a1, seedA, true, true)
	shuffleContestQuestions(a2, seedA, true, true)
	shuffleContestQuestions(b1, seedB, true, true)
	shuffleContestQuestions(plain, seedA, false, false)

	if order(a1) != order(a2) {
		t.Errorf("cùng attempt_id phải ra cùng thứ tự: %s vs %s", order(a1), order(a2))
	}
	if order(a1) == order(b1) {
		t.Errorf("attempt_id khác nhau nhưng thứ tự giống hệt: %s", order(a1))
	}
	if order(plain) != "12345678" {
		t.Errorf("tắt xáo phải giữ thứ tự gốc, nhận %s", order(plain))
	}
	for i, q := range a1 {
		if q.DisplayOrder != i+1 {
			t.Errorf("sau khi xáo display_order phải đánh lại 1..n, câu %d có %d", i, q.DisplayOrder)
		}
	}
}

func fullMarksAnswers(cq contestQuiz) []dto.SubmitAnswerDTO {
	return []dto.SubmitAnswerDTO{
		{QuestionID: cq.SingleQ.String(), SelectedAnswerIDs: []string{cq.SingleCorrect.String()}},
		{QuestionID: cq.MultiQ.String(), SelectedAnswerIDs: []string{cq.MultiCorrect[0].String(), cq.MultiCorrect[1].String()}},
		{QuestionID: cq.FillQ.String(), TextAnswer: "Hà Nội"},
	}
}

// (c) Hai lần nộp đồng thời cùng attempt: đúng 1 thành công, 1 ErrQuizAttemptAlreadySubmitted, và
// quiz_attempt_answers không bị ghi trùng. Lặp vài vòng để hai goroutine thật sự chồng lên nhau.
func TestSubmitContestAttempt_HaiLanNopDongThoi_MotThanhCong(t *testing.T) {
	f := newContestFixture(t)
	owner := f.user("owner")
	cq := f.standaloneQuiz(owner, false)
	ctx := context.Background()
	answers := fullMarksAnswers(cq)

	for round := 0; round < 5; round++ {
		student := f.user("student")
		attemptID := f.startContestAttempt(cq.ID, student)
		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = f.quiz.SubmitContestAttempt(ctx, cq.ID, student, attemptID, answers, 600)
			}(i)
		}
		close(start)
		wg.Wait()

		ok, dup := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrQuizAttemptAlreadySubmitted):
				dup++
			default:
				t.Fatalf("vòng %d: lỗi không mong đợi %v", round, err)
			}
		}
		if ok != 1 || dup != 1 {
			t.Errorf("vòng %d: muốn 1 thành công + 1 đã nộp, nhận %d + %d", round, ok, dup)
		}
		if n := f.count("quiz_attempt_answers", "attempt_id = ?", attemptID); n != int64(len(answers)) {
			t.Errorf("vòng %d: quiz_attempt_answers = %d, muốn %d (không trùng)", round, n, len(answers))
		}
	}
}

// Chấm điểm đúng luật quiz thường (câu bỏ trống = 0, mẫu số = tổng mọi câu), time_spent bị chặn ở
// thời lượng, is_passed bỏ trống, nộp lại tuần tự bị từ chối, và attempt người khác bị từ chối.
func TestSubmitContestAttempt_ChamDiem_GioiHanThoiGian_ChongNopLai(t *testing.T) {
	f := newContestFixture(t)
	owner, student, other := f.user("owner"), f.user("student"), f.user("other")
	cq := f.standaloneQuiz(owner, false)
	ctx := context.Background()
	attemptID := f.startContestAttempt(cq.ID, student)
	// Giả lập đã bắt đầu 1 giờ trước: thời gian thật vượt xa thời lượng 60 giây.
	if err := f.db.Exec("UPDATE quiz_attempts SET started_at = ? WHERE id = ?", time.Now().Add(-time.Hour), attemptID).Error; err != nil {
		t.Fatalf("lùi started_at: %v", err)
	}

	if _, err := f.quiz.SubmitContestAttempt(ctx, cq.ID, other, attemptID, nil, 60); err == nil {
		t.Errorf("người khác nộp hộ attempt phải bị từ chối")
	}

	res, err := f.quiz.SubmitContestAttempt(ctx, cq.ID, student, attemptID, fullMarksAnswers(cq), 60)
	if err != nil {
		t.Fatalf("SubmitContestAttempt: %v", err)
	}
	// 4 điểm (1 + 2 + 1) trên tổng 5 vì bỏ trống câu true_false.
	if res.Score == nil || res.Score.String() != "4" || res.TotalPoints == nil || res.TotalPoints.String() != "5" {
		t.Errorf("điểm = %v/%v, muốn 4/5", res.Score, res.TotalPoints)
	}
	if res.Percentage == nil || res.Percentage.String() != "80" {
		t.Errorf("phần trăm = %v, muốn 80", res.Percentage)
	}
	if res.TimeSpentSecs == nil || *res.TimeSpentSecs != 60 {
		t.Errorf("time_spent_seconds = %v, muốn bị chặn ở 60", res.TimeSpentSecs)
	}
	if res.IsPassed != nil {
		t.Errorf("is_passed phải NULL trong cuộc thi, nhận %v", *res.IsPassed)
	}

	if _, err := f.quiz.SubmitContestAttempt(ctx, cq.ID, student, attemptID, fullMarksAnswers(cq), 60); !errors.Is(err, ErrQuizAttemptAlreadySubmitted) {
		t.Errorf("nộp lần 2: muốn ErrQuizAttemptAlreadySubmitted, nhận %v", err)
	}
	if n := f.count("quiz_attempt_answers", "attempt_id = ?", attemptID); n != 3 {
		t.Errorf("quiz_attempt_answers = %d, muốn 3", n)
	}
}

// Bài chữa chỉ có sau khi nộp, gồm MỌI câu (kể cả câu bỏ trống) với đáp án đúng + giải thích.
func TestGetContestAttemptReview_ChiSauKhiNop_DuMoiCau(t *testing.T) {
	f := newContestFixture(t)
	owner, student := f.user("owner"), f.user("student")
	cq := f.standaloneQuiz(owner, false)
	ctx := context.Background()
	attemptID := f.startContestAttempt(cq.ID, student)

	if _, err := f.quiz.GetContestAttemptReview(ctx, attemptID); err == nil {
		t.Errorf("attempt chưa nộp không được trả bài chữa")
	}
	if _, err := f.quiz.SubmitContestAttempt(ctx, cq.ID, student, attemptID, fullMarksAnswers(cq), 600); err != nil {
		t.Fatalf("nộp: %v", err)
	}
	review, err := f.quiz.GetContestAttemptReview(ctx, attemptID)
	if err != nil {
		t.Fatalf("GetContestAttemptReview: %v", err)
	}
	if len(review) != 4 {
		t.Fatalf("muốn đủ 4 câu, nhận %d", len(review))
	}
	for _, r := range review {
		if len(r.CorrectAnswerIDs) == 0 || r.Explanation == nil || r.IsCorrect == nil {
			t.Errorf("câu %s thiếu đáp án/giải thích/is_correct: %+v", r.QuestionID, r)
		}
	}
	tf := review[3]
	if tf.QuestionID != cq.TrueFalseQ || *tf.IsCorrect || tf.ID != uuid.Nil {
		t.Errorf("câu bỏ trống phải có is_correct=false, id rỗng: %+v", tf)
	}
}
