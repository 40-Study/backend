package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type QuizRepositoryInterface interface {
	// Quiz
	CreateQuiz(ctx context.Context, quiz *model.Quiz) error
	GetQuizByID(ctx context.Context, id uuid.UUID) (*model.Quiz, error)
	GetQuizWithQuestions(ctx context.Context, id uuid.UUID) (*model.Quiz, error)
	ListQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, page, pageSize int) ([]model.Quiz, int64, error)
	UpdateQuiz(ctx context.Context, quiz *model.Quiz) error
	DeleteQuiz(ctx context.Context, id uuid.UUID) error
	GetQuizzesByCreator(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Quiz, int64, error)

	// Question
	CreateQuestion(ctx context.Context, question *model.Question) error
	BulkCreateQuestions(ctx context.Context, questions []model.Question) error
	GetQuestionByID(ctx context.Context, id uuid.UUID) (*model.Question, error)
	GetQuestionsByQuizID(ctx context.Context, quizID uuid.UUID) ([]model.Question, error)
	UpdateQuestion(ctx context.Context, question *model.Question) error
	DeleteQuestion(ctx context.Context, id uuid.UUID) error
	ReorderQuestions(ctx context.Context, quizID uuid.UUID, questionIDs []uuid.UUID) error

	// QuestionAnswer
	CreateAnswers(ctx context.Context, answers []model.QuestionAnswer) error
	DeleteAnswersByQuestionID(ctx context.Context, questionID uuid.UUID) error
	GetCorrectAnswers(ctx context.Context, questionID uuid.UUID) ([]model.QuestionAnswer, error)

	// QuizAttempt
	CreateAttempt(ctx context.Context, attempt *model.QuizAttempt) error
	GetAttemptByID(ctx context.Context, id uuid.UUID) (*model.QuizAttempt, error)
	GetAttemptWithAnswers(ctx context.Context, id uuid.UUID) (*model.QuizAttempt, error)
	UpdateAttempt(ctx context.Context, attempt *model.QuizAttempt) error
	GetAttemptsByUserAndQuiz(ctx context.Context, userID, quizID uuid.UUID) ([]model.QuizAttempt, error)
	GetAttemptsByQuiz(ctx context.Context, quizID uuid.UUID) ([]model.QuizAttempt, error)
	CountAttemptsByUserAndQuiz(ctx context.Context, userID, quizID uuid.UUID) (int64, error)
	// CompleteAttemptIfPending (R8, review 260919 — nộp quiz hai lần ghi trùng answers): cập
	// nhật kết quả chấm điểm của attempt (score/total_points/percentage/is_passed/
	// time_spent_seconds/completed_at, đọc từ CÁC field tương ứng của `attempt`) VÀ chèn
	// `answers` vào CÙNG một transaction, với điều kiện `completed_at IS NULL` ngay trên câu
	// UPDATE — đây là chốt chặn race (so với đọc rồi update riêng lẻ), Postgres tự đảm bảo tính
	// nguyên tử của UPDATE ... WHERE dù có 2 request chạy đồng thời. updated=false nghĩa là 0
	// dòng bị ảnh hưởng (attempt đã completed_at != nil từ trước, do request khác nộp trước) —
	// answers KHÔNG được insert trong trường hợp này; caller phải coi đây là lỗi "đã nộp".
	CompleteAttemptIfPending(ctx context.Context, attempt *model.QuizAttempt, answers []model.QuizAttemptAnswer) (updated bool, err error)

	// QuizAttemptAnswer
	CreateAttemptAnswers(ctx context.Context, answers []model.QuizAttemptAnswer) error
	GetAttemptAnswersByAttemptID(ctx context.Context, attemptID uuid.UUID) ([]model.QuizAttemptAnswer, error)

	// Statistics
	GetQuizStatistics(ctx context.Context, quizID uuid.UUID) (totalAttempts int64, avgScore, highScore, lowScore float64, passCount int64, avgTime float64, err error)
}

type QuizRepository struct {
	db *gorm.DB
}

func NewQuizRepository(db *gorm.DB) *QuizRepository {
	return &QuizRepository{db: db}
}

// ============================================================================
// QUIZ
// ============================================================================

func (r *QuizRepository) CreateQuiz(ctx context.Context, quiz *model.Quiz) error {
	return r.db.WithContext(ctx).Create(quiz).Error
}

func (r *QuizRepository) GetQuizByID(ctx context.Context, id uuid.UUID) (*model.Quiz, error) {
	var quiz model.Quiz
	err := r.db.WithContext(ctx).First(&quiz, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &quiz, nil
}

func (r *QuizRepository) GetQuizWithQuestions(ctx context.Context, id uuid.UUID) (*model.Quiz, error) {
	var quiz model.Quiz
	err := r.db.WithContext(ctx).
		Preload("Questions", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Questions.Answers", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		First(&quiz, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &quiz, nil
}

func (r *QuizRepository) ListQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, page, pageSize int) ([]model.Quiz, int64, error) {
	var quizzes []model.Quiz
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Quiz{})
	if lessonID != nil {
		query = query.Where("lesson_id = ?", *lessonID)
	}
	if courseID != nil {
		query = query.Where("course_id = ?", *courseID)
	}
	if sessionID != nil {
		query = query.Where("session_id = ?", *sessionID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&quizzes).Error
	return quizzes, total, err
}

func (r *QuizRepository) UpdateQuiz(ctx context.Context, quiz *model.Quiz) error {
	return r.db.WithContext(ctx).Save(quiz).Error
}

func (r *QuizRepository) DeleteQuiz(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Quiz{}, "id = ?", id).Error
}

func (r *QuizRepository) GetQuizzesByCreator(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Quiz, int64, error) {
	// Quizzes don't have a direct creator field, filter by lesson/course ownership would be complex.
	// For now return all quizzes with pagination.
	var quizzes []model.Quiz
	var total int64
	query := r.db.WithContext(ctx).Model(&model.Quiz{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&quizzes).Error
	return quizzes, total, err
}

// ============================================================================
// QUESTION
// ============================================================================

func (r *QuizRepository) CreateQuestion(ctx context.Context, question *model.Question) error {
	return r.db.WithContext(ctx).Create(question).Error
}

func (r *QuizRepository) BulkCreateQuestions(ctx context.Context, questions []model.Question) error {
	return r.db.WithContext(ctx).Create(&questions).Error
}

func (r *QuizRepository) GetQuestionByID(ctx context.Context, id uuid.UUID) (*model.Question, error) {
	var q model.Question
	err := r.db.WithContext(ctx).
		Preload("Answers", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		First(&q, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &q, nil
}

func (r *QuizRepository) GetQuestionsByQuizID(ctx context.Context, quizID uuid.UUID) ([]model.Question, error) {
	var questions []model.Question
	err := r.db.WithContext(ctx).
		Preload("Answers", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Where("quiz_id = ?", quizID).
		Order("display_order ASC").
		Find(&questions).Error
	return questions, err
}

func (r *QuizRepository) UpdateQuestion(ctx context.Context, question *model.Question) error {
	return r.db.WithContext(ctx).Save(question).Error
}

func (r *QuizRepository) DeleteQuestion(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Question{}, "id = ?", id).Error
}

func (r *QuizRepository) ReorderQuestions(ctx context.Context, quizID uuid.UUID, questionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, qID := range questionIDs {
			if err := tx.Model(&model.Question{}).
				Where("id = ? AND quiz_id = ?", qID, quizID).
				Update("display_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ============================================================================
// QUESTION ANSWER
// ============================================================================

func (r *QuizRepository) CreateAnswers(ctx context.Context, answers []model.QuestionAnswer) error {
	return r.db.WithContext(ctx).Create(&answers).Error
}

func (r *QuizRepository) DeleteAnswersByQuestionID(ctx context.Context, questionID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("question_id = ?", questionID).Delete(&model.QuestionAnswer{}).Error
}

func (r *QuizRepository) GetCorrectAnswers(ctx context.Context, questionID uuid.UUID) ([]model.QuestionAnswer, error) {
	var answers []model.QuestionAnswer
	err := r.db.WithContext(ctx).
		Where("question_id = ? AND is_correct = true", questionID).
		Find(&answers).Error
	return answers, err
}

// ============================================================================
// QUIZ ATTEMPT
// ============================================================================

func (r *QuizRepository) CreateAttempt(ctx context.Context, attempt *model.QuizAttempt) error {
	return r.db.WithContext(ctx).Create(attempt).Error
}

func (r *QuizRepository) GetAttemptByID(ctx context.Context, id uuid.UUID) (*model.QuizAttempt, error) {
	var attempt model.QuizAttempt
	err := r.db.WithContext(ctx).First(&attempt, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &attempt, nil
}

func (r *QuizRepository) GetAttemptWithAnswers(ctx context.Context, id uuid.UUID) (*model.QuizAttempt, error) {
	var attempt model.QuizAttempt
	err := r.db.WithContext(ctx).
		Preload("Answers").
		Preload("Answers.Question").
		// Phase 1 §6: can Answers.Question.Answers (danh sach dap an cua CAU HOI, kem
		// IsCorrect) de tinh correct_answer_ids — khac voi Answers (dap an NGUOI DUNG da chon).
		Preload("Answers.Question.Answers").
		First(&attempt, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &attempt, nil
}

func (r *QuizRepository) UpdateAttempt(ctx context.Context, attempt *model.QuizAttempt) error {
	return r.db.WithContext(ctx).Save(attempt).Error
}

// CompleteAttemptIfPending: xem chú thích tại interface. Dùng map cho Updates (không dùng
// struct/Save) vì các field như IsPassed=false hay TimeSpentSecs=0 là giá trị hợp lệ — Updates
// bằng struct sẽ bỏ qua zero-value, giống lý do UpdateLessonProgressFields (enrollment_repository.go)
// dùng map. "time_spent_seconds" khớp tên cột thật (model.QuizAttempt.TimeSpentSecs có
// gorm:"column:time_spent_seconds"), không phải tên field Go.
func (r *QuizRepository) CompleteAttemptIfPending(ctx context.Context, attempt *model.QuizAttempt, answers []model.QuizAttemptAnswer) (updated bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.QuizAttempt{}).
			Where("id = ? AND completed_at IS NULL", attempt.ID).
			Updates(map[string]interface{}{
				"score":              attempt.Score,
				"total_points":       attempt.TotalPoints,
				"percentage":         attempt.Percentage,
				"is_passed":          attempt.IsPassed,
				"time_spent_seconds": attempt.TimeSpentSecs,
				"completed_at":       attempt.CompletedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			updated = false
			return nil
		}
		updated = true
		if len(answers) > 0 {
			if err := tx.Create(&answers).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}

func (r *QuizRepository) GetAttemptsByUserAndQuiz(ctx context.Context, userID, quizID uuid.UUID) ([]model.QuizAttempt, error) {
	var attempts []model.QuizAttempt
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND quiz_id = ?", userID, quizID).
		Order("created_at DESC").
		Find(&attempts).Error
	return attempts, err
}

// GetAttemptsByQuiz (CAO-6, review vòng 2): CHỈ lấy attempt mode="official" — day la nguon cho
// GV xem GetQuizResults (danh sach attempt cua hoc vien) va moi tinh toan "best score", KHONG
// duoc de attempt "practice" (hoc thu, khong tinh diem chinh thuc) lam nhieu thong ke cua GV.
func (r *QuizRepository) GetAttemptsByQuiz(ctx context.Context, quizID uuid.UUID) ([]model.QuizAttempt, error) {
	var attempts []model.QuizAttempt
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("quiz_id = ? AND mode = ?", quizID, "official").
		Order("created_at DESC").
		Find(&attempts).Error
	return attempts, err
}

// CountAttemptsByUserAndQuiz (Phase 1 §6): CHỈ đếm attempt mode="official" — dùng riêng cho
// gate quiz_max_attempts, và contract ghi rõ practice "không đếm vào quiz_max_attempts".
func (r *QuizRepository) CountAttemptsByUserAndQuiz(ctx context.Context, userID, quizID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.QuizAttempt{}).
		Where("user_id = ? AND quiz_id = ? AND mode = ?", userID, quizID, "official").
		Count(&count).Error
	return count, err
}

// ============================================================================
// QUIZ ATTEMPT ANSWER
// ============================================================================

func (r *QuizRepository) CreateAttemptAnswers(ctx context.Context, answers []model.QuizAttemptAnswer) error {
	return r.db.WithContext(ctx).Create(&answers).Error
}

func (r *QuizRepository) GetAttemptAnswersByAttemptID(ctx context.Context, attemptID uuid.UUID) ([]model.QuizAttemptAnswer, error) {
	var answers []model.QuizAttemptAnswer
	err := r.db.WithContext(ctx).
		Preload("Question").
		Where("attempt_id = ?", attemptID).
		Find(&answers).Error
	return answers, err
}

// ============================================================================
// STATISTICS
// ============================================================================

// GetQuizStatistics (CAO-6, review vòng 2): chỉ tính trên attempt mode="official" — cùng lý do
// với GetAttemptsByQuiz phía trên, tránh attempt "practice" kéo lệch điểm trung bình/cao/thấp
// mà GV nhìn thấy.
func (r *QuizRepository) GetQuizStatistics(ctx context.Context, quizID uuid.UUID) (totalAttempts int64, avgScore, highScore, lowScore float64, passCount int64, avgTime float64, err error) {
	row := r.db.WithContext(ctx).
		Model(&model.QuizAttempt{}).
		Where("quiz_id = ? AND mode = ? AND completed_at IS NOT NULL", quizID, "official").
		Select(`
			COUNT(*) as total_attempts,
			COALESCE(AVG(score), 0) as avg_score,
			COALESCE(MAX(score), 0) as high_score,
			COALESCE(MIN(score), 0) as low_score,
			COUNT(CASE WHEN is_passed = true THEN 1 END) as pass_count,
			COALESCE(AVG(time_spent_seconds), 0) as avg_time
		`).Row()

	err = row.Scan(&totalAttempts, &avgScore, &highScore, &lowScore, &passCount, &avgTime)
	return
}
