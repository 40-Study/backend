package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/repository"
)

type ExerciseServiceInterface interface {
	CreateExercise(ctx context.Context, req dto.CreateExerciseDTO) (*dto.ExerciseResponseDTO, error)
	GetExerciseByID(ctx context.Context, id uuid.UUID) (*dto.ExerciseDetailDTO, error)
	ListExercises(ctx context.Context, page, pageSize int) (*dto.ExerciseListDTO, error)
	UpdateExercise(ctx context.Context, id uuid.UUID, req dto.UpdateExerciseDTO) (*dto.ExerciseResponseDTO, error)
	DeleteExercise(ctx context.Context, id uuid.UUID) error

	// TestCase
	CreateTestCase(ctx context.Context, exerciseID uuid.UUID, req dto.CreateExerciseTestCaseDTO) (*dto.ExerciseTestCaseResponseDTO, error)
	GetTestCases(ctx context.Context, exerciseID uuid.UUID) ([]dto.ExerciseTestCaseResponseDTO, error)
	ImportTestCases(ctx context.Context, exerciseID uuid.UUID, req dto.ImportExerciseTestCasesDTO) ([]dto.ExerciseTestCaseResponseDTO, error)
	DeleteTestCase(ctx context.Context, exerciseID uuid.UUID, id uuid.UUID) error

	// Submission (via RabbitMQ for code execution)
	SubmitExercise(ctx context.Context, exerciseID, userID uuid.UUID, req dto.SubmitExerciseDTO) (*dto.ExerciseSubmissionResponseDTO, error)
	GetSubmissions(ctx context.Context, exerciseID uuid.UUID, page, pageSize int) (*dto.ExerciseSubmissionListDTO, error)
	GetMySubmissions(ctx context.Context, exerciseID, userID uuid.UUID, page, pageSize int) (*dto.ExerciseSubmissionListDTO, error)
	GetSubmissionByID(ctx context.Context, id uuid.UUID) (*dto.ExerciseSubmissionResponseDTO, error)

	// Progress
	UpdateContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID, req dto.UpdateContentProgressDTO) (*dto.ContentProgressResponseDTO, error)
	GetContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID) (*dto.ContentProgressResponseDTO, error)
}

type ExerciseService struct {
	repo     repository.ExerciseRepositoryInterface
	redis    *redis.Client
	rabbitMQ *rabbitmq_queue.RabbitMQService
}

func NewExerciseService(
	repo repository.ExerciseRepositoryInterface,
	redis *redis.Client,
	rabbitMQ *rabbitmq_queue.RabbitMQService,
) *ExerciseService {
	return &ExerciseService{repo: repo, redis: redis, rabbitMQ: rabbitMQ}
}

const (
	exerciseCachePrefix = "exercise:"
	exerciseCacheTTL    = 10 * time.Minute
)

func (s *ExerciseService) invalidateExerciseCache(ctx context.Context, exerciseID uuid.UUID) {
	if s.redis != nil {
		s.redis.Del(ctx, exerciseCachePrefix+exerciseID.String())
	}
}

// ============================================================================
// EXERCISE CRUD
// ============================================================================

func (s *ExerciseService) CreateExercise(ctx context.Context, req dto.CreateExerciseDTO) (*dto.ExerciseResponseDTO, error) {
	exercise := &model.CourseExercise{
		Title:          req.Title,
		Description:    req.Description,
		Difficulty:     "medium",
		Language:       req.Language,
		StarterCode:    req.StarterCode,
		TimeLimit:      2,
		MemoryLimit:    256,
		PassPercentage: 100,
	}

	if req.Difficulty != "" {
		exercise.Difficulty = req.Difficulty
	}
	if req.TimeLimit > 0 {
		exercise.TimeLimit = req.TimeLimit
	}
	if req.MemoryLimit > 0 {
		exercise.MemoryLimit = req.MemoryLimit
	}
	if req.PassPercentage > 0 {
		exercise.PassPercentage = req.PassPercentage
	}

	if err := s.repo.CreateExercise(ctx, exercise); err != nil {
		return nil, err
	}

	return s.mapExerciseToDTO(exercise, 0), nil
}

func (s *ExerciseService) GetExerciseByID(ctx context.Context, id uuid.UUID) (*dto.ExerciseDetailDTO, error) {
	// Check cache
	if s.redis != nil {
		cacheKey := exerciseCachePrefix + id.String()
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result dto.ExerciseDetailDTO
			if json.Unmarshal([]byte(cached), &result) == nil {
				return &result, nil
			}
		}
	}

	exercise, err := s.repo.GetExerciseWithTestCases(ctx, id)
	if err != nil {
		return nil, err
	}
	if exercise == nil {
		return nil, errors.New("exercise not found")
	}

	testCases := make([]dto.ExerciseTestCaseResponseDTO, len(exercise.TestCases))
	for i, tc := range exercise.TestCases {
		testCases[i] = *s.mapTestCaseToDTO(&tc)
	}

	result := &dto.ExerciseDetailDTO{
		ExerciseResponseDTO: *s.mapExerciseToDTO(exercise, len(exercise.TestCases)),
		TestCases:           testCases,
	}

	// Cache
	if s.redis != nil {
		if data, err := json.Marshal(result); err == nil {
			s.redis.Set(ctx, exerciseCachePrefix+id.String(), data, exerciseCacheTTL)
		}
	}

	return result, nil
}

func (s *ExerciseService) ListExercises(ctx context.Context, page, pageSize int) (*dto.ExerciseListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	exercises, total, err := s.repo.ListExercises(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.ExerciseResponseDTO, len(exercises))
	for i, ex := range exercises {
		data[i] = *s.mapExerciseToDTO(&ex, len(ex.TestCases))
	}

	return &dto.ExerciseListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ExerciseService) UpdateExercise(ctx context.Context, id uuid.UUID, req dto.UpdateExerciseDTO) (*dto.ExerciseResponseDTO, error) {
	exercise, err := s.repo.GetExerciseByID(ctx, id)
	if err != nil || exercise == nil {
		return nil, errors.New("exercise not found")
	}

	if req.Title != nil {
		exercise.Title = *req.Title
	}
	if req.Description != nil {
		exercise.Description = *req.Description
	}
	if req.Difficulty != nil {
		exercise.Difficulty = *req.Difficulty
	}
	if req.Language != nil {
		exercise.Language = *req.Language
	}
	if req.StarterCode != nil {
		exercise.StarterCode = *req.StarterCode
	}
	if req.TimeLimit != nil {
		exercise.TimeLimit = *req.TimeLimit
	}
	if req.MemoryLimit != nil {
		exercise.MemoryLimit = *req.MemoryLimit
	}
	if req.PassPercentage != nil {
		exercise.PassPercentage = *req.PassPercentage
	}

	if err := s.repo.UpdateExercise(ctx, exercise); err != nil {
		return nil, err
	}

	s.invalidateExerciseCache(ctx, id)
	return s.mapExerciseToDTO(exercise, 0), nil
}

func (s *ExerciseService) DeleteExercise(ctx context.Context, id uuid.UUID) error {
	exercise, err := s.repo.GetExerciseByID(ctx, id)
	if err != nil || exercise == nil {
		return errors.New("exercise not found")
	}

	if err := s.repo.DeleteExercise(ctx, id); err != nil {
		return err
	}

	s.invalidateExerciseCache(ctx, id)
	return nil
}

// ============================================================================
// TEST CASE
// ============================================================================

func (s *ExerciseService) CreateTestCase(ctx context.Context, exerciseID uuid.UUID, req dto.CreateExerciseTestCaseDTO) (*dto.ExerciseTestCaseResponseDTO, error) {
	tc := &model.ExerciseTestCase{
		ExerciseID:     exerciseID,
		Input:          req.Input,
		ExpectedOutput: req.ExpectedOutput,
		IsHidden:       req.IsHidden,
		DisplayOrder:   req.DisplayOrder,
	}

	if err := s.repo.CreateTestCase(ctx, tc); err != nil {
		return nil, err
	}

	s.invalidateExerciseCache(ctx, exerciseID)
	return s.mapTestCaseToDTO(tc), nil
}

func (s *ExerciseService) GetTestCases(ctx context.Context, exerciseID uuid.UUID) ([]dto.ExerciseTestCaseResponseDTO, error) {
	tcs, err := s.repo.GetTestCasesByExerciseID(ctx, exerciseID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.ExerciseTestCaseResponseDTO, len(tcs))
	for i, tc := range tcs {
		result[i] = *s.mapTestCaseToDTO(&tc)
	}
	return result, nil
}

func (s *ExerciseService) ImportTestCases(ctx context.Context, exerciseID uuid.UUID, req dto.ImportExerciseTestCasesDTO) ([]dto.ExerciseTestCaseResponseDTO, error) {
	var tcs []model.ExerciseTestCase
	for _, tcReq := range req.TestCases {
		tcs = append(tcs, model.ExerciseTestCase{
			ExerciseID:     exerciseID,
			Input:          tcReq.Input,
			ExpectedOutput: tcReq.ExpectedOutput,
			IsHidden:       tcReq.IsHidden,
			DisplayOrder:   tcReq.DisplayOrder,
		})
	}

	if err := s.repo.BulkCreateTestCases(ctx, tcs); err != nil {
		return nil, err
	}

	s.invalidateExerciseCache(ctx, exerciseID)

	result := make([]dto.ExerciseTestCaseResponseDTO, len(tcs))
	for i, tc := range tcs {
		result[i] = *s.mapTestCaseToDTO(&tc)
	}
	return result, nil
}

func (s *ExerciseService) DeleteTestCase(ctx context.Context, exerciseID uuid.UUID, id uuid.UUID) error {
	if err := s.repo.DeleteTestCase(ctx, id); err != nil {
		return err
	}
	s.invalidateExerciseCache(ctx, exerciseID)
	return nil
}

// ============================================================================
// SUBMISSION (via RabbitMQ)
// ============================================================================

// ExerciseSubmissionMessage is the RabbitMQ message for exercise code execution
type ExerciseSubmissionMessage struct {
	SubmissionID uuid.UUID `json:"submission_id"`
	ExerciseID   uuid.UUID `json:"exercise_id"`
	UserID       uuid.UUID `json:"user_id"`
	Language     string    `json:"language"`
	Code         string    `json:"code"`
	TimeLimit    int       `json:"time_limit"`
	MemoryLimit  int       `json:"memory_limit"`
	TestCases    []struct {
		Input          string `json:"input"`
		ExpectedOutput string `json:"expected_output"`
	} `json:"test_cases"`
	PassPercentage int `json:"pass_percentage"`
}

func (s *ExerciseService) SubmitExercise(ctx context.Context, exerciseID, userID uuid.UUID, req dto.SubmitExerciseDTO) (*dto.ExerciseSubmissionResponseDTO, error) {
	exercise, err := s.repo.GetExerciseWithTestCases(ctx, exerciseID)
	if err != nil || exercise == nil {
		return nil, errors.New("exercise not found")
	}

	// Create submission in pending state
	sub := &model.ExerciseSubmission{
		ExerciseID:     exerciseID,
		UserID:         userID,
		Language:       req.Language,
		Code:           req.Code,
		Verdict:        "pending",
		TotalTestCases: len(exercise.TestCases),
	}

	if err := s.repo.CreateSubmission(ctx, sub); err != nil {
		return nil, err
	}

	// Push to RabbitMQ for async execution
	if s.rabbitMQ != nil {
		msg := ExerciseSubmissionMessage{
			SubmissionID:   sub.ID,
			ExerciseID:     exerciseID,
			UserID:         userID,
			Language:       req.Language,
			Code:           req.Code,
			TimeLimit:      exercise.TimeLimit,
			MemoryLimit:    exercise.MemoryLimit,
			PassPercentage: exercise.PassPercentage,
		}
		for _, tc := range exercise.TestCases {
			msg.TestCases = append(msg.TestCases, struct {
				Input          string `json:"input"`
				ExpectedOutput string `json:"expected_output"`
			}{
				Input:          tc.Input,
				ExpectedOutput: tc.ExpectedOutput,
			})
		}

		data, _ := json.Marshal(msg)
		if err := s.rabbitMQ.PublishMessage(ctx, "exercise", "exercise.submit", data); err != nil {
			log.Printf("[exercise] Failed to publish to RabbitMQ, falling back to sync: %v", err)
			// Fallback: mark as error if queue unavailable
			sub.Verdict = "system_error"
			errMsg := "code execution queue unavailable"
			sub.ErrorMessage = &errMsg
			s.repo.UpdateSubmission(ctx, sub)
		}
	} else {
		// No RabbitMQ available — mark as error
		sub.Verdict = "system_error"
		errMsg := "code execution service not available"
		sub.ErrorMessage = &errMsg
		s.repo.UpdateSubmission(ctx, sub)
	}

	return s.mapSubmissionToDTO(sub), nil
}

func (s *ExerciseService) GetSubmissions(ctx context.Context, exerciseID uuid.UUID, page, pageSize int) (*dto.ExerciseSubmissionListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	subs, total, err := s.repo.GetSubmissionsByExercise(ctx, exerciseID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.ExerciseSubmissionResponseDTO, len(subs))
	for i, sub := range subs {
		data[i] = *s.mapSubmissionToDTO(&sub)
	}

	return &dto.ExerciseSubmissionListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ExerciseService) GetMySubmissions(ctx context.Context, exerciseID, userID uuid.UUID, page, pageSize int) (*dto.ExerciseSubmissionListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	subs, total, err := s.repo.GetSubmissionsByExerciseAndUser(ctx, exerciseID, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.ExerciseSubmissionResponseDTO, len(subs))
	for i, sub := range subs {
		data[i] = *s.mapSubmissionToDTO(&sub)
	}

	return &dto.ExerciseSubmissionListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ExerciseService) GetSubmissionByID(ctx context.Context, id uuid.UUID) (*dto.ExerciseSubmissionResponseDTO, error) {
	sub, err := s.repo.GetSubmissionByID(ctx, id)
	if err != nil || sub == nil {
		return nil, errors.New("submission not found")
	}
	return s.mapSubmissionToDTO(sub), nil
}

// ============================================================================
// CONTENT PROGRESS
// ============================================================================

func (s *ExerciseService) UpdateContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID, req dto.UpdateContentProgressDTO) (*dto.ContentProgressResponseDTO, error) {
	progress, _ := s.repo.GetContentProgress(ctx, userID, lessonContentID)
	if progress == nil {
		progress = &model.ContentProgress{
			UserID:          userID,
			LessonContentID: lessonContentID,
			Status:          "not_started",
		}
	}

	if req.Status != nil {
		progress.Status = *req.Status
		if *req.Status == "completed" {
			now := time.Now()
			progress.CompletedAt = &now
		}
	}
	if req.VideoWatchedSecs != nil {
		progress.VideoWatchedSecs = *req.VideoWatchedSecs
	}
	if req.ExercisePassed != nil {
		progress.ExercisePassed = *req.ExercisePassed
	}
	progress.LastAccessedAt = time.Now()

	if err := s.repo.UpsertContentProgress(ctx, progress); err != nil {
		return nil, err
	}

	return s.mapProgressToDTO(progress), nil
}

func (s *ExerciseService) GetContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID) (*dto.ContentProgressResponseDTO, error) {
	progress, err := s.repo.GetContentProgress(ctx, userID, lessonContentID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		return nil, nil
	}
	return s.mapProgressToDTO(progress), nil
}

// ============================================================================
// RabbitMQ Queue Setup for Exercise Submissions
// ============================================================================

func (s *ExerciseService) SetupExerciseQueues() error {
	if s.rabbitMQ == nil {
		return fmt.Errorf("RabbitMQ not available")
	}

	if err := s.rabbitMQ.SetupQueue("exercise", "exercise.submit", "exercise.submit"); err != nil {
		return fmt.Errorf("setup exercise queue: %w", err)
	}
	if err := s.rabbitMQ.SetupQueue("exercise", "exercise.result", "exercise.result"); err != nil {
		return fmt.Errorf("setup exercise result queue: %w", err)
	}

	return nil
}

// ============================================================================
// MAPPERS
// ============================================================================

func (s *ExerciseService) mapExerciseToDTO(ex *model.CourseExercise, testCaseCount int) *dto.ExerciseResponseDTO {
	return &dto.ExerciseResponseDTO{
		ID:             ex.ID,
		Title:          ex.Title,
		Description:    ex.Description,
		Difficulty:     ex.Difficulty,
		Language:       ex.Language,
		StarterCode:    ex.StarterCode,
		TimeLimit:      ex.TimeLimit,
		MemoryLimit:    ex.MemoryLimit,
		PassPercentage: ex.PassPercentage,
		TestCaseCount:  testCaseCount,
		CreatedAt:      ex.CreatedAt,
		UpdatedAt:      ex.UpdatedAt,
	}
}

func (s *ExerciseService) mapTestCaseToDTO(tc *model.ExerciseTestCase) *dto.ExerciseTestCaseResponseDTO {
	return &dto.ExerciseTestCaseResponseDTO{
		ID:             tc.ID,
		ExerciseID:     tc.ExerciseID,
		Input:          tc.Input,
		ExpectedOutput: tc.ExpectedOutput,
		IsHidden:       tc.IsHidden,
		DisplayOrder:   tc.DisplayOrder,
	}
}

func (s *ExerciseService) mapSubmissionToDTO(sub *model.ExerciseSubmission) *dto.ExerciseSubmissionResponseDTO {
	return &dto.ExerciseSubmissionResponseDTO{
		ID:              sub.ID,
		ExerciseID:      sub.ExerciseID,
		UserID:          sub.UserID,
		Language:        sub.Language,
		Code:            sub.Code,
		Verdict:         sub.Verdict,
		TestCasesPassed: sub.TestCasesPassed,
		TotalTestCases:  sub.TotalTestCases,
		ExecutionTimeMs: sub.ExecutionTimeMs,
		MemoryUsedKB:    sub.MemoryUsedKB,
		ErrorMessage:    sub.ErrorMessage,
		CreatedAt:       sub.CreatedAt,
	}
}

func (s *ExerciseService) mapProgressToDTO(p *model.ContentProgress) *dto.ContentProgressResponseDTO {
	return &dto.ContentProgressResponseDTO{
		ID:               p.ID,
		UserID:           p.UserID,
		LessonContentID:  p.LessonContentID,
		Status:           p.Status,
		VideoWatchedSecs: p.VideoWatchedSecs,
		ExercisePassed:   p.ExercisePassed,
		CompletedAt:      p.CompletedAt,
		LastAccessedAt:   p.LastAccessedAt,
	}
}
