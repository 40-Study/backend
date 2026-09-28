package service

// quiz_contest_engine.go — QuizService hiện thực ContestQuizEngine (contract "Cuộc thi" §4, §6).
// ContestService (lane B1) điều phối luật cuộc thi (đã join chưa, phase, hạn nộp); file này chỉ lo
// phần "làm bài": tạo attempt, phát đề không kèm đáp án, chấm, và trả bài chữa.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

var _ ContestQuizEngine = (*QuizService)(nil)

// errContestAttemptInvalid: attempt không tồn tại, không phải attempt "contest", hoặc không khớp
// quiz/người dùng. ContestService đã đối chiếu attempt_id với contest_participants trước khi gọi
// engine (CONTEST_ATTEMPT_MISMATCH), nên tới được đây là lỗi lập trình — không cần sentinel riêng.
var errContestAttemptInvalid = errors.New("contest attempt not found for this quiz/user")

// CreateContestAttemptTx tạo (hoặc trả lại) attempt "contest" của userID cho quizID, TRÊN tx của
// caller để việc gắn attempt vào contest_participants nằm chung transaction.
//
// Idempotent và an toàn khi chạy song song NGAY CẢ khi không có row lock của caller: mỗi cặp
// (quiz, user) chỉ có một attempt "contest" (quiz gắn đúng một cuộc thi, unique idx_contests_quiz_id).
// pg_advisory_xact_lock tuần tự hoá hai lần start đồng thời; lần sau đọc thấy attempt lần trước đã
// commit và trả lại nó. Không thể dùng SELECT ... FOR UPDATE ở đây vì chưa có dòng nào để khoá.
// Khoá tự nhả khi tx commit/rollback.
func (s *QuizService) CreateContestAttemptTx(ctx context.Context, tx *gorm.DB, quizID, userID uuid.UUID, startedAt time.Time) (uuid.UUID, error) {
	if tx == nil {
		return uuid.Nil, errors.New("CreateContestAttemptTx requires a transaction")
	}
	db := tx.WithContext(ctx)
	lockKey := fmt.Sprintf("contest_attempt:%s:%s", quizID, userID)
	if err := db.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey).Error; err != nil {
		return uuid.Nil, err
	}

	var existing model.QuizAttempt
	err := db.Where("quiz_id = ? AND user_id = ? AND mode = ?", quizID, userID, QuizAttemptModeContest).
		Order("created_at ASC").Take(&existing).Error
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, err
	}

	attempt := &model.QuizAttempt{
		UserID:    userID,
		QuizID:    quizID,
		Mode:      QuizAttemptModeContest,
		StartedAt: startedAt,
	}
	if err := db.Create(attempt).Error; err != nil {
		return uuid.Nil, err
	}
	return attempt.ID, nil
}

// GetContestAttemptQuestions trả đề của attemptID KHÔNG kèm is_correct/explanation (AttemptQuestionDTO
// không có các field đó). Quiz bật shuffle_questions/shuffle_answers thì xáo TẤT ĐỊNH với seed lấy từ
// attempt_id: reload trang ra đúng thứ tự cũ, còn hai thí sinh khác nhau nhận thứ tự khác nhau.
func (s *QuizService) GetContestAttemptQuestions(ctx context.Context, quizID, attemptID uuid.UUID) ([]dto.AttemptQuestionDTO, error) {
	attempt, err := s.repo.GetAttemptByID(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt == nil || attempt.QuizID != quizID || attempt.Mode != QuizAttemptModeContest {
		return nil, errContestAttemptInvalid
	}
	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}

	questions := make([]dto.AttemptQuestionDTO, len(quiz.Questions))
	for i, q := range quiz.Questions {
		answers := make([]dto.AttemptAnswerDTO, len(q.Answers))
		for j, a := range q.Answers {
			answers[j] = dto.AttemptAnswerDTO{ID: a.ID, AnswerText: a.AnswerText, DisplayOrder: a.DisplayOrder}
		}
		questions[i] = dto.AttemptQuestionDTO{
			ID:           q.ID,
			QuestionText: q.QuestionText,
			QuestionType: q.QuestionType,
			Points:       q.Points,
			DisplayOrder: q.DisplayOrder,
			ImageURL:     q.ImageURL,
			Answers:      answers,
		}
	}
	shuffleContestQuestions(questions, attemptID, quiz.ShuffleQuestions, quiz.ShuffleAnswers)
	return questions, nil
}

// shuffleContestQuestions xáo tại chỗ. Đầu vào luôn theo display_order (repo đã ORDER BY), nên cùng
// seed cho cùng kết quả. Sau khi xáo, display_order được đánh lại 1..n theo thứ tự mới để web có
// sort theo display_order cũng không vô tình "gỡ" việc xáo.
func shuffleContestQuestions(questions []dto.AttemptQuestionDTO, attemptID uuid.UUID, byQuestion, byAnswer bool) {
	rng := rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(attemptID[:8]))))
	if byQuestion {
		rng.Shuffle(len(questions), func(i, j int) { questions[i], questions[j] = questions[j], questions[i] })
		for i := range questions {
			questions[i].DisplayOrder = i + 1
		}
	}
	if byAnswer {
		for i := range questions {
			answers := questions[i].Answers
			rng.Shuffle(len(answers), func(a, b int) { answers[a], answers[b] = answers[b], answers[a] })
			for j := range answers {
				answers[j].DisplayOrder = j + 1
			}
		}
	}
}

// SubmitContestAttempt chấm bài thi bằng đúng gradeSubmission của quiz thường (câu bỏ trống = 0,
// mẫu số = tổng điểm mọi câu). Hai lần nộp đồng thời: CompleteAttemptIfPending chỉ cho một UPDATE
// "completed_at IS NULL" thắng, lần thua nhận ErrQuizAttemptAlreadySubmitted và quiz_attempt_answers
// không bị ghi trùng. Hạn nộp (deadline + 30s) do ContestService kiểm TRƯỚC khi gọi; ở đây chỉ chặn
// time_spent_seconds ≤ maxTimeSpentSeconds để thời gian vượt thời lượng không lọt vào xếp hạng.
// pass_percentage bị bỏ qua trong cuộc thi (§3.3) nên is_passed để NULL.
func (s *QuizService) SubmitContestAttempt(ctx context.Context, quizID, userID, attemptID uuid.UUID, answers []dto.SubmitAnswerDTO, maxTimeSpentSeconds int) (*dto.QuizAttemptResponseDTO, error) {
	attempt, err := s.repo.GetAttemptByID(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt == nil || attempt.QuizID != quizID || attempt.UserID != userID || attempt.Mode != QuizAttemptModeContest {
		return nil, errContestAttemptInvalid
	}
	if attempt.CompletedAt != nil {
		return nil, ErrQuizAttemptAlreadySubmitted
	}
	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}

	earned, total, rows := s.gradeSubmission(quiz, attempt.ID, answers)
	now := time.Now()
	timeSpent := int(now.Sub(attempt.StartedAt).Seconds())
	if timeSpent < 0 {
		timeSpent = 0
	}
	if maxTimeSpentSeconds > 0 && timeSpent > maxTimeSpentSeconds {
		timeSpent = maxTimeSpentSeconds
	}
	percentage := gradePercentage(earned, total)

	attempt.Score = &earned
	attempt.TotalPoints = &total
	attempt.Percentage = &percentage
	attempt.IsPassed = nil
	attempt.TimeSpentSecs = &timeSpent
	attempt.CompletedAt = &now

	updated, err := s.repo.CompleteAttemptIfPending(ctx, attempt, rows)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrQuizAttemptAlreadySubmitted
	}
	return s.mapAttemptToDTO(attempt), nil
}

// GetContestAttemptReview trả bài chữa (is_correct, correct_answer_ids, explanation) cho MỌI câu của
// quiz theo display_order, kể cả câu bỏ trống (id = uuid.Nil, is_correct=false, 0 điểm) để thí sinh
// thấy đủ đề. Chỉ trả cho attempt đã nộp: ContestService quyết định KHI NÀO được xem (phase ENDED/
// FINALIZED, §4.3), còn engine từ chối attempt dang dở để một lời gọi nhầm không lộ đáp án giữa giờ.
func (s *QuizService) GetContestAttemptReview(ctx context.Context, attemptID uuid.UUID) ([]dto.QuizAttemptAnswerDTO, error) {
	attempt, err := s.repo.GetAttemptWithAnswers(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt == nil || attempt.Mode != QuizAttemptModeContest {
		return nil, errContestAttemptInvalid
	}
	if attempt.CompletedAt == nil {
		return nil, errors.New("contest attempt is not submitted")
	}
	quiz, err := s.repo.GetQuizWithQuestions(ctx, attempt.QuizID)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}

	answered := make(map[uuid.UUID]*model.QuizAttemptAnswer, len(attempt.Answers))
	for i := range attempt.Answers {
		answered[attempt.Answers[i].QuestionID] = &attempt.Answers[i]
	}
	review := make([]dto.QuizAttemptAnswerDTO, len(quiz.Questions))
	for i := range quiz.Questions {
		q := &quiz.Questions[i]
		correctIDs := []string{}
		for _, a := range q.Answers {
			if a.IsCorrect {
				correctIDs = append(correctIDs, a.ID.String())
			}
		}
		item := dto.QuizAttemptAnswerDTO{
			QuestionID:        q.ID,
			QuestionText:      q.QuestionText,
			SelectedAnswerIDs: []string{},
			CorrectAnswerIDs:  correctIDs,
			Explanation:       q.Explanation,
		}
		if aa, ok := answered[q.ID]; ok {
			item.ID = aa.ID
			item.SelectedAnswerIDs = aa.SelectedAnswerIDs
			item.TextAnswer = aa.TextAnswer
			item.IsCorrect = aa.IsCorrect
			item.PointsEarned = aa.PointsEarned
		} else {
			notCorrect := false
			item.IsCorrect = &notCorrect
		}
		review[i] = item
	}
	return review, nil
}
