package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type QuizServiceInterface interface {
	CreateQuiz(ctx context.Context, req dto.CreateQuizDTO) (*dto.QuizResponseDTO, error)
	GetAllQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, page, pageSize int) (*dto.QuizListDTO, error)
	GetQuizByID(ctx context.Context, id uuid.UUID) (*dto.QuizDetailDTO, error)
	UpdateQuiz(ctx context.Context, id uuid.UUID, req dto.UpdateQuizDTO) (*dto.QuizResponseDTO, error)
	DeleteQuiz(ctx context.Context, id uuid.UUID) error
	DuplicateQuiz(ctx context.Context, id uuid.UUID) (*dto.QuizResponseDTO, error)

	// Questions
	CreateQuestion(ctx context.Context, quizID uuid.UUID, req dto.CreateQuestionDTO) (*dto.QuestionResponseDTO, error)
	GetQuestionsByQuiz(ctx context.Context, quizID uuid.UUID) ([]dto.QuestionResponseDTO, error)
	UpdateQuestion(ctx context.Context, quizID, questionID uuid.UUID, req dto.UpdateQuestionDTO) (*dto.QuestionResponseDTO, error)
	DeleteQuestion(ctx context.Context, quizID, questionID uuid.UUID) error
	ReorderQuestions(ctx context.Context, quizID uuid.UUID, req dto.ReorderQuestionsDTO) error
	BulkCreateQuestions(ctx context.Context, quizID uuid.UUID, req dto.BulkCreateQuestionsDTO) ([]dto.QuestionResponseDTO, error)

	// Attempts
	StartQuiz(ctx context.Context, quizID, userID uuid.UUID) (*dto.StartQuizResponseDTO, error)
	SubmitQuiz(ctx context.Context, quizID, userID uuid.UUID, req dto.SubmitQuizDTO) (*dto.QuizAttemptResponseDTO, error)
	GetMyAttempts(ctx context.Context, quizID, userID uuid.UUID) ([]dto.QuizAttemptResponseDTO, error)
	GetAttemptByID(ctx context.Context, attemptID, userID uuid.UUID) (*dto.QuizAttemptDetailDTO, error)
	GetQuizResults(ctx context.Context, quizID uuid.UUID) (*dto.QuizResultsDTO, error)
	GetQuizStatistics(ctx context.Context, quizID uuid.UUID) (*dto.QuizStatisticsDTO, error)
	SaveAnswer(ctx context.Context, attemptID, userID uuid.UUID, req dto.SaveAnswerDTO) error
	GetAttemptProgress(ctx context.Context, attemptID, userID uuid.UUID) (*dto.QuizAttemptDetailDTO, error)
	GetMyCreatedQuizzes(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.QuizListDTO, error)
	GetMyQuizHistory(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]dto.QuizAttemptResponseDTO, error)
}

type QuizService struct {
	repo  repository.QuizRepositoryInterface
	redis *redis.Client
}

func NewQuizService(repo repository.QuizRepositoryInterface, redis *redis.Client) *QuizService {
	return &QuizService{repo: repo, redis: redis}
}

const (
	quizCachePrefix     = "quiz:"
	quizCacheTTL        = 10 * time.Minute
	attemptCachePrefix  = "quiz_attempt:"
	attemptCacheTTL     = 30 * time.Minute
)

func (s *QuizService) invalidateQuizCache(ctx context.Context, quizID uuid.UUID) {
	if s.redis != nil {
		s.redis.Del(ctx, quizCachePrefix+quizID.String())
	}
}

// ============================================================================
// QUIZ CRUD
// ============================================================================

func (s *QuizService) CreateQuiz(ctx context.Context, req dto.CreateQuizDTO) (*dto.QuizResponseDTO, error) {
	quiz := &model.Quiz{
		Title:       req.Title,
		TriggerType: "manual",
	}

	if req.Description != "" {
		quiz.Description = &req.Description
	}
	if req.LessonID != "" {
		id, _ := uuid.Parse(req.LessonID)
		quiz.LessonID = &id
	}
	if req.CourseID != "" {
		id, _ := uuid.Parse(req.CourseID)
		quiz.CourseID = &id
	}
	if req.SessionID != "" {
		id, _ := uuid.Parse(req.SessionID)
		quiz.SessionID = &id
	}
	if req.TimeLimitMins != nil {
		quiz.TimeLimitMins = req.TimeLimitMins
	}
	if req.PassPercentage != nil {
		quiz.PassPercentage = decimal.NewFromFloat(*req.PassPercentage)
	}
	if req.MaxAttempts != nil {
		quiz.MaxAttempts = req.MaxAttempts
	}
	if req.TriggerType != "" {
		quiz.TriggerType = req.TriggerType
	}
	if req.ScheduledAt != "" {
		t, _ := time.Parse(time.RFC3339, req.ScheduledAt)
		quiz.ScheduledAt = &t
	}
	if req.VideoTimestamp != nil {
		quiz.VideoTimestamp = req.VideoTimestamp
	}
	if req.ShuffleQuestions != nil {
		quiz.ShuffleQuestions = *req.ShuffleQuestions
	}
	if req.ShuffleAnswers != nil {
		quiz.ShuffleAnswers = *req.ShuffleAnswers
	}
	if req.ShowCorrectAnswers != nil {
		quiz.ShowCorrectAnswers = *req.ShowCorrectAnswers
	}

	if err := s.repo.CreateQuiz(ctx, quiz); err != nil {
		return nil, err
	}

	return s.mapQuizToDTO(quiz, 0), nil
}

func (s *QuizService) GetAllQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, page, pageSize int) (*dto.QuizListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	quizzes, total, err := s.repo.ListQuizzes(ctx, lessonID, courseID, sessionID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.QuizResponseDTO, len(quizzes))
	for i, q := range quizzes {
		data[i] = *s.mapQuizToDTO(&q, 0)
	}

	return &dto.QuizListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *QuizService) GetQuizByID(ctx context.Context, id uuid.UUID) (*dto.QuizDetailDTO, error) {
	// Check cache
	if s.redis != nil {
		cacheKey := quizCachePrefix + id.String()
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result dto.QuizDetailDTO
			if json.Unmarshal([]byte(cached), &result) == nil {
				return &result, nil
			}
		}
	}

	quiz, err := s.repo.GetQuizWithQuestions(ctx, id)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}

	questions := make([]dto.QuestionResponseDTO, len(quiz.Questions))
	for i, q := range quiz.Questions {
		questions[i] = *s.mapQuestionToDTO(&q)
	}

	result := &dto.QuizDetailDTO{
		QuizResponseDTO: *s.mapQuizToDTO(quiz, len(quiz.Questions)),
		Questions:       questions,
	}

	// Cache
	if s.redis != nil {
		if data, err := json.Marshal(result); err == nil {
			s.redis.Set(ctx, quizCachePrefix+id.String(), data, quizCacheTTL)
		}
	}

	return result, nil
}

func (s *QuizService) UpdateQuiz(ctx context.Context, id uuid.UUID, req dto.UpdateQuizDTO) (*dto.QuizResponseDTO, error) {
	quiz, err := s.repo.GetQuizByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}

	if req.Title != nil {
		quiz.Title = *req.Title
	}
	if req.Description != nil {
		quiz.Description = req.Description
	}
	if req.TimeLimitMins != nil {
		quiz.TimeLimitMins = req.TimeLimitMins
	}
	if req.PassPercentage != nil {
		quiz.PassPercentage = decimal.NewFromFloat(*req.PassPercentage)
	}
	if req.MaxAttempts != nil {
		quiz.MaxAttempts = req.MaxAttempts
	}
	if req.TriggerType != nil {
		quiz.TriggerType = *req.TriggerType
	}
	if req.ShuffleQuestions != nil {
		quiz.ShuffleQuestions = *req.ShuffleQuestions
	}
	if req.ShuffleAnswers != nil {
		quiz.ShuffleAnswers = *req.ShuffleAnswers
	}
	if req.ShowCorrectAnswers != nil {
		quiz.ShowCorrectAnswers = *req.ShowCorrectAnswers
	}

	if err := s.repo.UpdateQuiz(ctx, quiz); err != nil {
		return nil, err
	}

	s.invalidateQuizCache(ctx, id)
	return s.mapQuizToDTO(quiz, 0), nil
}

func (s *QuizService) DeleteQuiz(ctx context.Context, id uuid.UUID) error {
	quiz, err := s.repo.GetQuizByID(ctx, id)
	if err != nil {
		return err
	}
	if quiz == nil {
		return errors.New("quiz not found")
	}

	if err := s.repo.DeleteQuiz(ctx, id); err != nil {
		return err
	}

	s.invalidateQuizCache(ctx, id)
	return nil
}

func (s *QuizService) DuplicateQuiz(ctx context.Context, id uuid.UUID) (*dto.QuizResponseDTO, error) {
	original, err := s.repo.GetQuizWithQuestions(ctx, id)
	if err != nil {
		return nil, err
	}
	if original == nil {
		return nil, errors.New("quiz not found")
	}

	newQuiz := &model.Quiz{
		LessonID:           original.LessonID,
		CourseID:           original.CourseID,
		SessionID:          original.SessionID,
		Title:              original.Title + " (Copy)",
		Description:        original.Description,
		TimeLimitMins:      original.TimeLimitMins,
		PassPercentage:     original.PassPercentage,
		MaxAttempts:        original.MaxAttempts,
		TriggerType:        original.TriggerType,
		ShuffleQuestions:   original.ShuffleQuestions,
		ShuffleAnswers:     original.ShuffleAnswers,
		ShowCorrectAnswers: original.ShowCorrectAnswers,
	}

	if err := s.repo.CreateQuiz(ctx, newQuiz); err != nil {
		return nil, err
	}

	for _, q := range original.Questions {
		newQ := model.Question{
			QuizID:       newQuiz.ID,
			QuestionText: q.QuestionText,
			QuestionType: q.QuestionType,
			Explanation:  q.Explanation,
			Points:       q.Points,
			DisplayOrder: q.DisplayOrder,
			ImageURL:     q.ImageURL,
		}
		if err := s.repo.CreateQuestion(ctx, &newQ); err != nil {
			continue
		}
		var answers []model.QuestionAnswer
		for _, a := range q.Answers {
			answers = append(answers, model.QuestionAnswer{
				QuestionID:   newQ.ID,
				AnswerText:   a.AnswerText,
				IsCorrect:    a.IsCorrect,
				DisplayOrder: a.DisplayOrder,
			})
		}
		if len(answers) > 0 {
			s.repo.CreateAnswers(ctx, answers)
		}
	}

	return s.mapQuizToDTO(newQuiz, len(original.Questions)), nil
}

// ============================================================================
// QUESTION
// ============================================================================

func (s *QuizService) CreateQuestion(ctx context.Context, quizID uuid.UUID, req dto.CreateQuestionDTO) (*dto.QuestionResponseDTO, error) {
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	points := decimal.NewFromFloat(1.0)
	if req.Points != nil {
		points = decimal.NewFromFloat(*req.Points)
	}

	question := &model.Question{
		QuizID:       quizID,
		QuestionText: req.QuestionText,
		QuestionType: req.QuestionType,
		Points:       points,
		DisplayOrder: req.DisplayOrder,
	}
	if req.Explanation != "" {
		question.Explanation = &req.Explanation
	}
	if req.ImageURL != "" {
		question.ImageURL = &req.ImageURL
	}

	if err := s.repo.CreateQuestion(ctx, question); err != nil {
		return nil, err
	}

	if len(req.Answers) > 0 {
		var answers []model.QuestionAnswer
		for _, a := range req.Answers {
			answers = append(answers, model.QuestionAnswer{
				QuestionID:   question.ID,
				AnswerText:   a.AnswerText,
				IsCorrect:    a.IsCorrect,
				DisplayOrder: a.DisplayOrder,
			})
		}
		if err := s.repo.CreateAnswers(ctx, answers); err != nil {
			return nil, err
		}
		question.Answers = answers
	}

	s.invalidateQuizCache(ctx, quizID)
	return s.mapQuestionToDTO(question), nil
}

func (s *QuizService) GetQuestionsByQuiz(ctx context.Context, quizID uuid.UUID) ([]dto.QuestionResponseDTO, error) {
	questions, err := s.repo.GetQuestionsByQuizID(ctx, quizID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.QuestionResponseDTO, len(questions))
	for i, q := range questions {
		result[i] = *s.mapQuestionToDTO(&q)
	}
	return result, nil
}

func (s *QuizService) UpdateQuestion(ctx context.Context, quizID, questionID uuid.UUID, req dto.UpdateQuestionDTO) (*dto.QuestionResponseDTO, error) {
	question, err := s.repo.GetQuestionByID(ctx, questionID)
	if err != nil || question == nil {
		return nil, errors.New("question not found")
	}
	if question.QuizID != quizID {
		return nil, errors.New("question does not belong to this quiz")
	}

	if req.QuestionText != nil {
		question.QuestionText = *req.QuestionText
	}
	if req.QuestionType != nil {
		question.QuestionType = *req.QuestionType
	}
	if req.Explanation != nil {
		question.Explanation = req.Explanation
	}
	if req.Points != nil {
		question.Points = decimal.NewFromFloat(*req.Points)
	}
	if req.DisplayOrder != nil {
		question.DisplayOrder = *req.DisplayOrder
	}
	if req.ImageURL != nil {
		question.ImageURL = req.ImageURL
	}

	if err := s.repo.UpdateQuestion(ctx, question); err != nil {
		return nil, err
	}

	// Replace answers if provided
	if req.Answers != nil {
		s.repo.DeleteAnswersByQuestionID(ctx, questionID)
		var answers []model.QuestionAnswer
		for _, a := range req.Answers {
			answers = append(answers, model.QuestionAnswer{
				QuestionID:   questionID,
				AnswerText:   a.AnswerText,
				IsCorrect:    a.IsCorrect,
				DisplayOrder: a.DisplayOrder,
			})
		}
		if len(answers) > 0 {
			s.repo.CreateAnswers(ctx, answers)
		}
		question.Answers = answers
	}

	s.invalidateQuizCache(ctx, quizID)
	return s.mapQuestionToDTO(question), nil
}

func (s *QuizService) DeleteQuestion(ctx context.Context, quizID, questionID uuid.UUID) error {
	question, err := s.repo.GetQuestionByID(ctx, questionID)
	if err != nil || question == nil {
		return errors.New("question not found")
	}
	if question.QuizID != quizID {
		return errors.New("question does not belong to this quiz")
	}

	if err := s.repo.DeleteQuestion(ctx, questionID); err != nil {
		return err
	}

	s.invalidateQuizCache(ctx, quizID)
	return nil
}

func (s *QuizService) ReorderQuestions(ctx context.Context, quizID uuid.UUID, req dto.ReorderQuestionsDTO) error {
	ids := make([]uuid.UUID, len(req.QuestionIDs))
	for i, idStr := range req.QuestionIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return errors.New("invalid question_id: " + idStr)
		}
		ids[i] = id
	}

	if err := s.repo.ReorderQuestions(ctx, quizID, ids); err != nil {
		return err
	}

	s.invalidateQuizCache(ctx, quizID)
	return nil
}

func (s *QuizService) BulkCreateQuestions(ctx context.Context, quizID uuid.UUID, req dto.BulkCreateQuestionsDTO) ([]dto.QuestionResponseDTO, error) {
	var results []dto.QuestionResponseDTO
	for _, qReq := range req.Questions {
		result, err := s.CreateQuestion(ctx, quizID, qReq)
		if err != nil {
			return nil, err
		}
		results = append(results, *result)
	}
	return results, nil
}

// ============================================================================
// QUIZ ATTEMPTS
// ============================================================================

func (s *QuizService) StartQuiz(ctx context.Context, quizID, userID uuid.UUID) (*dto.StartQuizResponseDTO, error) {
	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	// Check max attempts
	if quiz.MaxAttempts != nil {
		count, _ := s.repo.CountAttemptsByUserAndQuiz(ctx, userID, quizID)
		if count >= int64(*quiz.MaxAttempts) {
			return nil, errors.New("max attempts reached")
		}
	}

	attempt := &model.QuizAttempt{
		UserID:    userID,
		QuizID:    quizID,
		StartedAt: time.Now(),
	}

	if err := s.repo.CreateAttempt(ctx, attempt); err != nil {
		return nil, err
	}

	// Build questions for attempt (without correct answers)
	questions := make([]dto.AttemptQuestionDTO, len(quiz.Questions))
	for i, q := range quiz.Questions {
		answers := make([]dto.AttemptAnswerDTO, len(q.Answers))
		for j, a := range q.Answers {
			answers[j] = dto.AttemptAnswerDTO{
				ID:           a.ID,
				AnswerText:   a.AnswerText,
				DisplayOrder: a.DisplayOrder,
			}
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

	return &dto.StartQuizResponseDTO{
		AttemptID:     attempt.ID,
		QuizID:        quizID,
		Title:         quiz.Title,
		TimeLimitMins: quiz.TimeLimitMins,
		Questions:     questions,
		StartedAt:     attempt.StartedAt,
	}, nil
}

func (s *QuizService) SubmitQuiz(ctx context.Context, quizID, userID uuid.UUID, req dto.SubmitQuizDTO) (*dto.QuizAttemptResponseDTO, error) {
	// Find the latest in-progress attempt
	attempts, err := s.repo.GetAttemptsByUserAndQuiz(ctx, userID, quizID)
	if err != nil {
		return nil, err
	}

	var attempt *model.QuizAttempt
	for i := range attempts {
		if attempts[i].CompletedAt == nil {
			attempt = &attempts[i]
			break
		}
	}
	if attempt == nil {
		return nil, errors.New("no active attempt found")
	}

	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	// Grade answers
	totalPoints := decimal.Zero
	earnedPoints := decimal.Zero

	var attemptAnswers []model.QuizAttemptAnswer
	for _, ans := range req.Answers {
		questionID, _ := uuid.Parse(ans.QuestionID)

		// Find the question
		var question *model.Question
		for i := range quiz.Questions {
			if quiz.Questions[i].ID == questionID {
				question = &quiz.Questions[i]
				break
			}
		}
		if question == nil {
			continue
		}

		totalPoints = totalPoints.Add(question.Points)

		// Check correctness
		correct := s.checkAnswer(question, ans)
		earned := decimal.Zero
		if correct {
			earned = question.Points
		}
		earnedPoints = earnedPoints.Add(earned)

		selectedIDs := pq.StringArray(ans.SelectedAnswerIDs)
		aa := model.QuizAttemptAnswer{
			AttemptID:         attempt.ID,
			QuestionID:        questionID,
			SelectedAnswerIDs: selectedIDs,
			IsCorrect:         &correct,
			PointsEarned:      earned,
		}
		if ans.TextAnswer != "" {
			aa.TextAnswer = &ans.TextAnswer
		}
		attemptAnswers = append(attemptAnswers, aa)
	}

	if err := s.repo.CreateAttemptAnswers(ctx, attemptAnswers); err != nil {
		return nil, err
	}

	// Calculate result
	now := time.Now()
	timeSpent := int(now.Sub(attempt.StartedAt).Seconds())
	percentage := decimal.Zero
	if !totalPoints.IsZero() {
		percentage = earnedPoints.Div(totalPoints).Mul(decimal.NewFromInt(100))
	}
	isPassed := percentage.GreaterThanOrEqual(quiz.PassPercentage)

	attempt.Score = &earnedPoints
	attempt.TotalPoints = &totalPoints
	attempt.Percentage = &percentage
	attempt.IsPassed = &isPassed
	attempt.TimeSpentSecs = &timeSpent
	attempt.CompletedAt = &now

	if err := s.repo.UpdateAttempt(ctx, attempt); err != nil {
		return nil, err
	}

	return s.mapAttemptToDTO(attempt), nil
}

func (s *QuizService) checkAnswer(question *model.Question, answer dto.SubmitAnswerDTO) bool {
	switch question.QuestionType {
	case "single_choice", "true_false":
		if len(answer.SelectedAnswerIDs) != 1 {
			return false
		}
		for _, a := range question.Answers {
			if a.ID.String() == answer.SelectedAnswerIDs[0] && a.IsCorrect {
				return true
			}
		}
		return false

	case "multiple_choice":
		correctSet := make(map[string]bool)
		for _, a := range question.Answers {
			if a.IsCorrect {
				correctSet[a.ID.String()] = true
			}
		}
		if len(answer.SelectedAnswerIDs) != len(correctSet) {
			return false
		}
		for _, id := range answer.SelectedAnswerIDs {
			if !correctSet[id] {
				return false
			}
		}
		return true

	case "fill_blank":
		// Text-based matching
		for _, a := range question.Answers {
			if a.IsCorrect && a.AnswerText == answer.TextAnswer {
				return true
			}
		}
		return false

	case "essay":
		// Essays need manual grading
		return false
	}
	return false
}

func (s *QuizService) GetMyAttempts(ctx context.Context, quizID, userID uuid.UUID) ([]dto.QuizAttemptResponseDTO, error) {
	attempts, err := s.repo.GetAttemptsByUserAndQuiz(ctx, userID, quizID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.QuizAttemptResponseDTO, len(attempts))
	for i, a := range attempts {
		result[i] = *s.mapAttemptToDTO(&a)
	}
	return result, nil
}

func (s *QuizService) GetAttemptByID(ctx context.Context, attemptID, userID uuid.UUID) (*dto.QuizAttemptDetailDTO, error) {
	attempt, err := s.repo.GetAttemptWithAnswers(ctx, attemptID)
	if err != nil || attempt == nil {
		return nil, errors.New("attempt not found")
	}
	if attempt.UserID != userID {
		return nil, errors.New("forbidden")
	}

	answers := make([]dto.QuizAttemptAnswerDTO, len(attempt.Answers))
	for i, aa := range attempt.Answers {
		answers[i] = dto.QuizAttemptAnswerDTO{
			ID:                aa.ID,
			QuestionID:        aa.QuestionID,
			QuestionText:      aa.Question.QuestionText,
			SelectedAnswerIDs: aa.SelectedAnswerIDs,
			TextAnswer:        aa.TextAnswer,
			IsCorrect:         aa.IsCorrect,
			PointsEarned:      aa.PointsEarned,
			Explanation:       aa.Question.Explanation,
		}
	}

	return &dto.QuizAttemptDetailDTO{
		QuizAttemptResponseDTO: *s.mapAttemptToDTO(attempt),
		Answers:                answers,
	}, nil
}

func (s *QuizService) GetQuizResults(ctx context.Context, quizID uuid.UUID) (*dto.QuizResultsDTO, error) {
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	attempts, err := s.repo.GetAttemptsByQuiz(ctx, quizID)
	if err != nil {
		return nil, err
	}

	attemptDTOs := make([]dto.QuizAttemptResponseDTO, len(attempts))
	for i, a := range attempts {
		attemptDTOs[i] = *s.mapAttemptToDTO(&a)
	}

	return &dto.QuizResultsDTO{
		QuizID:        quizID,
		Title:         quiz.Title,
		TotalStudents: len(attempts),
		Attempts:      attemptDTOs,
	}, nil
}

func (s *QuizService) GetQuizStatistics(ctx context.Context, quizID uuid.UUID) (*dto.QuizStatisticsDTO, error) {
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	totalAttempts, avgScore, highScore, lowScore, passCount, avgTime, err := s.repo.GetQuizStatistics(ctx, quizID)
	if err != nil {
		return nil, err
	}

	passRate := decimal.Zero
	if totalAttempts > 0 {
		passRate = decimal.NewFromInt(passCount).Div(decimal.NewFromInt(totalAttempts)).Mul(decimal.NewFromInt(100))
	}

	return &dto.QuizStatisticsDTO{
		QuizID:          quizID,
		Title:           quiz.Title,
		TotalAttempts:   int(totalAttempts),
		AverageScore:    decimal.NewFromFloat(avgScore),
		HighestScore:    decimal.NewFromFloat(highScore),
		LowestScore:     decimal.NewFromFloat(lowScore),
		PassRate:        passRate,
		AverageTimeSecs: int(avgTime),
	}, nil
}

func (s *QuizService) SaveAnswer(ctx context.Context, attemptID, userID uuid.UUID, req dto.SaveAnswerDTO) error {
	// Save in-progress answer to Redis for auto-save
	if s.redis == nil {
		return nil
	}

	key := attemptCachePrefix + attemptID.String()
	data, _ := json.Marshal(req)
	return s.redis.HSet(ctx, key, req.QuestionID, data).Err()
}

func (s *QuizService) GetAttemptProgress(ctx context.Context, attemptID, userID uuid.UUID) (*dto.QuizAttemptDetailDTO, error) {
	return s.GetAttemptByID(ctx, attemptID, userID)
}

func (s *QuizService) GetMyCreatedQuizzes(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.QuizListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	quizzes, total, err := s.repo.GetQuizzesByCreator(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.QuizResponseDTO, len(quizzes))
	for i, q := range quizzes {
		data[i] = *s.mapQuizToDTO(&q, 0)
	}

	return &dto.QuizListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *QuizService) GetMyQuizHistory(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]dto.QuizAttemptResponseDTO, error) {
	// This would need a separate repo method, for now return empty
	return []dto.QuizAttemptResponseDTO{}, nil
}

// ============================================================================
// MAPPERS
// ============================================================================

func (s *QuizService) mapQuizToDTO(quiz *model.Quiz, questionCount int) *dto.QuizResponseDTO {
	return &dto.QuizResponseDTO{
		ID:                 quiz.ID,
		LessonID:           quiz.LessonID,
		CourseID:           quiz.CourseID,
		SessionID:          quiz.SessionID,
		Title:              quiz.Title,
		Description:        quiz.Description,
		TimeLimitMins:      quiz.TimeLimitMins,
		PassPercentage:     quiz.PassPercentage,
		MaxAttempts:        quiz.MaxAttempts,
		TriggerType:        quiz.TriggerType,
		ScheduledAt:        quiz.ScheduledAt,
		VideoTimestamp:     quiz.VideoTimestamp,
		ShuffleQuestions:   quiz.ShuffleQuestions,
		ShuffleAnswers:     quiz.ShuffleAnswers,
		ShowCorrectAnswers: quiz.ShowCorrectAnswers,
		QuestionCount:      questionCount,
		CreatedAt:          quiz.CreatedAt,
		UpdatedAt:          quiz.UpdatedAt,
	}
}

func (s *QuizService) mapQuestionToDTO(q *model.Question) *dto.QuestionResponseDTO {
	answers := make([]dto.AnswerResponseDTO, len(q.Answers))
	for i, a := range q.Answers {
		answers[i] = dto.AnswerResponseDTO{
			ID:           a.ID,
			AnswerText:   a.AnswerText,
			IsCorrect:    a.IsCorrect,
			DisplayOrder: a.DisplayOrder,
		}
	}

	return &dto.QuestionResponseDTO{
		ID:           q.ID,
		QuizID:       q.QuizID,
		QuestionText: q.QuestionText,
		QuestionType: q.QuestionType,
		Explanation:  q.Explanation,
		Points:       q.Points,
		DisplayOrder: q.DisplayOrder,
		ImageURL:     q.ImageURL,
		Answers:      answers,
	}
}

func (s *QuizService) mapAttemptToDTO(a *model.QuizAttempt) *dto.QuizAttemptResponseDTO {
	return &dto.QuizAttemptResponseDTO{
		ID:            a.ID,
		UserID:        a.UserID,
		QuizID:        a.QuizID,
		Score:         a.Score,
		TotalPoints:   a.TotalPoints,
		Percentage:    a.Percentage,
		IsPassed:      a.IsPassed,
		TimeSpentSecs: a.TimeSpentSecs,
		StartedAt:     a.StartedAt,
		CompletedAt:   a.CompletedAt,
	}
}
