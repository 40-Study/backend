package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type ContestServiceInterface interface {
	CreateContest(ctx context.Context, userID uuid.UUID, req dto.CreateContestRequest) (*dto.ContestResponse, error)
	UpdateContest(ctx context.Context, userID, contestID uuid.UUID, req dto.UpdateContestRequest) (*dto.ContestResponse, error)
	DeleteContest(ctx context.Context, userID, contestID uuid.UUID) error
	GetContestBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.ContestResponse, error)
	ListContests(ctx context.Context, keyword, status string, page, pageSize int) (*dto.ContestListResponse, error)
	PublishContest(ctx context.Context, userID, contestID uuid.UUID) (*dto.ContestResponse, error)
	GetMyContests(ctx context.Context, userID uuid.UUID) ([]dto.ContestResponse, error)

	CreateProblem(ctx context.Context, userID, contestID uuid.UUID, req dto.CreateProblemRequest) (*dto.ContestProblemResponse, error)
	UpdateProblem(ctx context.Context, userID, contestID, problemID uuid.UUID, req dto.UpdateProblemRequest) (*dto.ContestProblemResponse, error)
	DeleteProblem(ctx context.Context, userID, contestID, problemID uuid.UUID) error
	GetProblems(ctx context.Context, contestID uuid.UUID) ([]dto.ContestProblemResponse, error)

	JoinContest(ctx context.Context, userID, contestID uuid.UUID) (*dto.ContestParticipantResponse, error)
	GetLeaderboard(ctx context.Context, contestID uuid.UUID, page, pageSize int) (*dto.ContestLeaderboardResponse, error)
	SubmitAnswer(ctx context.Context, userID, contestID, problemID uuid.UUID, req dto.SubmitAnswerRequest) (*dto.ContestSubmissionResponse, error)
	GetMySubmissions(ctx context.Context, userID, contestID uuid.UUID) ([]dto.ContestSubmissionResponse, error)
}

type ContestService struct {
	contestRepo     *repository.ContestRepository
	problemRepo     *repository.ContestProblemRepository
	participantRepo *repository.ContestParticipantRepository
	submissionRepo  *repository.ContestSubmissionRepository
}

func NewContestService(
	contestRepo *repository.ContestRepository,
	problemRepo *repository.ContestProblemRepository,
	participantRepo *repository.ContestParticipantRepository,
	submissionRepo *repository.ContestSubmissionRepository,
) *ContestService {
	return &ContestService{
		contestRepo:     contestRepo,
		problemRepo:     problemRepo,
		participantRepo: participantRepo,
		submissionRepo:  submissionRepo,
	}
}

// ============================================================================
// CONTEST CRUD
// ============================================================================

func (s *ContestService) CreateContest(ctx context.Context, userID uuid.UUID, req dto.CreateContestRequest) (*dto.ContestResponse, error) {
	slug, err := utils.GenerateUniqueSlug(req.Title, func(slug string) (bool, error) {
		c, err := s.contestRepo.GetBySlug(ctx, slug)
		if err != nil {
			return false, err
		}
		return c != nil, nil
	})
	if err != nil {
		return nil, err
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	contest := &model.Contest{
		Title:           req.Title,
		Slug:            slug,
		Description:     req.Description,
		Type:            model.ContestType(req.Type),
		Status:          model.ContestStatusDraft,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		Duration:        req.Duration,
		MaxParticipants: req.MaxParticipants,
		IsPublic:        isPublic,
		CreatedBy:       userID,
	}

	if err := s.contestRepo.Create(ctx, contest); err != nil {
		return nil, err
	}

	return s.toContestResponse(contest, nil), nil
}

func (s *ContestService) UpdateContest(ctx context.Context, userID, contestID uuid.UUID, req dto.UpdateContestRequest) (*dto.ContestResponse, error) {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return nil, err
	}
	if contest == nil {
		return nil, errors.New("contest not found")
	}

	if contest.CreatedBy != userID {
		return nil, errors.New("only contest creator can update the contest")
	}

	if req.Title != nil {
		contest.Title = *req.Title
	}
	if req.Description != nil {
		contest.Description = req.Description
	}
	if req.Type != nil {
		contest.Type = model.ContestType(*req.Type)
	}
	if req.StartTime != nil {
		contest.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		contest.EndTime = *req.EndTime
	}
	if req.Duration != nil {
		contest.Duration = req.Duration
	}
	if req.MaxParticipants != nil {
		contest.MaxParticipants = *req.MaxParticipants
	}
	if req.IsPublic != nil {
		contest.IsPublic = *req.IsPublic
	}

	if err := s.contestRepo.Update(ctx, contest); err != nil {
		return nil, err
	}

	return s.toContestResponse(contest, &userID), nil
}

func (s *ContestService) DeleteContest(ctx context.Context, userID, contestID uuid.UUID) error {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return err
	}
	if contest == nil {
		return errors.New("contest not found")
	}

	if contest.CreatedBy != userID {
		return errors.New("only contest creator can delete the contest")
	}

	return s.contestRepo.Delete(ctx, contestID)
}

func (s *ContestService) GetContestBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.ContestResponse, error) {
	contest, err := s.contestRepo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if contest == nil {
		return nil, errors.New("contest not found")
	}

	return s.toContestResponse(contest, userID), nil
}

func (s *ContestService) ListContests(ctx context.Context, keyword, status string, page, pageSize int) (*dto.ContestListResponse, error) {
	contests, total, err := s.contestRepo.List(ctx, keyword, status, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ContestResponse, len(contests))
	for i, c := range contests {
		responses[i] = *s.toContestResponse(&c, nil)
	}

	return &dto.ContestListResponse{
		Contests:   responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *ContestService) PublishContest(ctx context.Context, userID, contestID uuid.UUID) (*dto.ContestResponse, error) {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return nil, err
	}
	if contest == nil {
		return nil, errors.New("contest not found")
	}

	if contest.CreatedBy != userID {
		return nil, errors.New("only contest creator can publish the contest")
	}

	if contest.Status != model.ContestStatusDraft {
		return nil, errors.New("only draft contests can be published")
	}

	if err := s.contestRepo.UpdateStatus(ctx, contestID, model.ContestStatusUpcoming); err != nil {
		return nil, err
	}

	contest.Status = model.ContestStatusUpcoming
	return s.toContestResponse(contest, &userID), nil
}

func (s *ContestService) GetMyContests(ctx context.Context, userID uuid.UUID) ([]dto.ContestResponse, error) {
	contests, err := s.contestRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ContestResponse, len(contests))
	for i, c := range contests {
		responses[i] = *s.toContestResponse(&c, &userID)
	}

	return responses, nil
}

// ============================================================================
// PROBLEMS
// ============================================================================

func (s *ContestService) CreateProblem(ctx context.Context, userID, contestID uuid.UUID, req dto.CreateProblemRequest) (*dto.ContestProblemResponse, error) {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return nil, err
	}
	if contest == nil {
		return nil, errors.New("contest not found")
	}

	if contest.CreatedBy != userID {
		return nil, errors.New("only contest creator can add problems")
	}

	testCasesJSON := datatypes.JSON("[]")
	if req.TestCases != nil {
		b, err := json.Marshal(req.TestCases)
		if err != nil {
			return nil, errors.New("invalid test_cases format")
		}
		testCasesJSON = datatypes.JSON(b)
	}

	optionsJSON := datatypes.JSON("[]")
	if req.Options != nil {
		b, err := json.Marshal(req.Options)
		if err != nil {
			return nil, errors.New("invalid options format")
		}
		optionsJSON = datatypes.JSON(b)
	}

	problem := &model.ContestProblem{
		ContestID:     contestID,
		Title:         req.Title,
		Description:   req.Description,
		Type:          model.ContestProblemType(req.Type),
		Difficulty:    model.ContestProblemDifficulty(req.Difficulty),
		Points:        req.Points,
		DisplayOrder:  req.DisplayOrder,
		TimeLimit:     req.TimeLimit,
		MemoryLimit:   req.MemoryLimit,
		InputFormat:   req.InputFormat,
		OutputFormat:  req.OutputFormat,
		SampleInput:   req.SampleInput,
		SampleOutput:  req.SampleOutput,
		TestCases:     testCasesJSON,
		Options:       optionsJSON,
		CorrectAnswer: req.CorrectAnswer,
	}

	if err := s.problemRepo.Create(ctx, problem); err != nil {
		return nil, err
	}

	return s.toProblemResponse(problem), nil
}

func (s *ContestService) UpdateProblem(ctx context.Context, userID, contestID, problemID uuid.UUID, req dto.UpdateProblemRequest) (*dto.ContestProblemResponse, error) {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return nil, err
	}
	if contest == nil {
		return nil, errors.New("contest not found")
	}

	if contest.CreatedBy != userID {
		return nil, errors.New("only contest creator can update problems")
	}

	problem, err := s.problemRepo.GetByID(ctx, problemID)
	if err != nil {
		return nil, err
	}
	if problem == nil {
		return nil, errors.New("problem not found")
	}
	if problem.ContestID != contestID {
		return nil, errors.New("problem does not belong to this contest")
	}

	if req.Title != nil {
		problem.Title = *req.Title
	}
	if req.Description != nil {
		problem.Description = *req.Description
	}
	if req.Type != nil {
		problem.Type = model.ContestProblemType(*req.Type)
	}
	if req.Difficulty != nil {
		problem.Difficulty = model.ContestProblemDifficulty(*req.Difficulty)
	}
	if req.Points != nil {
		problem.Points = *req.Points
	}
	if req.DisplayOrder != nil {
		problem.DisplayOrder = *req.DisplayOrder
	}
	if req.TimeLimit != nil {
		problem.TimeLimit = req.TimeLimit
	}
	if req.MemoryLimit != nil {
		problem.MemoryLimit = req.MemoryLimit
	}
	if req.InputFormat != nil {
		problem.InputFormat = req.InputFormat
	}
	if req.OutputFormat != nil {
		problem.OutputFormat = req.OutputFormat
	}
	if req.SampleInput != nil {
		problem.SampleInput = req.SampleInput
	}
	if req.SampleOutput != nil {
		problem.SampleOutput = req.SampleOutput
	}
	if req.TestCases != nil {
		b, err := json.Marshal(req.TestCases)
		if err != nil {
			return nil, errors.New("invalid test_cases format")
		}
		problem.TestCases = datatypes.JSON(b)
	}
	if req.Options != nil {
		b, err := json.Marshal(req.Options)
		if err != nil {
			return nil, errors.New("invalid options format")
		}
		problem.Options = datatypes.JSON(b)
	}
	if req.CorrectAnswer != nil {
		problem.CorrectAnswer = req.CorrectAnswer
	}

	if err := s.problemRepo.Update(ctx, problem); err != nil {
		return nil, err
	}

	return s.toProblemResponse(problem), nil
}

func (s *ContestService) DeleteProblem(ctx context.Context, userID, contestID, problemID uuid.UUID) error {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return err
	}
	if contest == nil {
		return errors.New("contest not found")
	}

	if contest.CreatedBy != userID {
		return errors.New("only contest creator can delete problems")
	}

	problem, err := s.problemRepo.GetByID(ctx, problemID)
	if err != nil {
		return err
	}
	if problem == nil {
		return errors.New("problem not found")
	}
	if problem.ContestID != contestID {
		return errors.New("problem does not belong to this contest")
	}

	return s.problemRepo.Delete(ctx, problemID)
}

func (s *ContestService) GetProblems(ctx context.Context, contestID uuid.UUID) ([]dto.ContestProblemResponse, error) {
	problems, err := s.problemRepo.ListByContestID(ctx, contestID)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ContestProblemResponse, len(problems))
	for i, p := range problems {
		responses[i] = *s.toProblemResponse(&p)
	}

	return responses, nil
}

// ============================================================================
// PARTICIPATION
// ============================================================================

func (s *ContestService) JoinContest(ctx context.Context, userID, contestID uuid.UUID) (*dto.ContestParticipantResponse, error) {
	contest, err := s.contestRepo.GetByID(ctx, contestID)
	if err != nil {
		return nil, err
	}
	if contest == nil {
		return nil, errors.New("contest not found")
	}

	if contest.Status != model.ContestStatusActive && contest.Status != model.ContestStatusUpcoming {
		return nil, errors.New("contest is not accepting participants")
	}

	if contest.MaxParticipants > 0 && contest.ParticipantCount >= contest.MaxParticipants {
		return nil, errors.New("contest is full")
	}

	existing, err := s.participantRepo.GetByContestAndUser(ctx, contestID, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("already joined this contest")
	}

	now := time.Now()
	participant := &model.ContestParticipant{
		ContestID: contestID,
		UserID:    userID,
		StartedAt: &now,
	}

	if err := s.participantRepo.Create(ctx, participant); err != nil {
		return nil, err
	}

	_ = s.contestRepo.IncrementParticipantCount(ctx, contestID)

	return &dto.ContestParticipantResponse{
		ID:         participant.ID,
		ContestID:  participant.ContestID,
		UserID:     participant.UserID,
		TotalScore: participant.TotalScore,
		Rank:       participant.Rank,
		StartedAt:  participant.StartedAt,
		FinishedAt: participant.FinishedAt,
		CreatedAt:  participant.CreatedAt,
	}, nil
}

func (s *ContestService) GetLeaderboard(ctx context.Context, contestID uuid.UUID, page, pageSize int) (*dto.ContestLeaderboardResponse, error) {
	participants, total, err := s.participantRepo.ListByContestID(ctx, contestID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ContestParticipantResponse, len(participants))
	for i, p := range participants {
		rank := i + 1 + (page-1)*pageSize
		responses[i] = dto.ContestParticipantResponse{
			ID:         p.ID,
			ContestID:  p.ContestID,
			UserID:     p.UserID,
			UserName:   p.User.UserName,
			AvatarURL:  p.User.AvatarURL,
			TotalScore: p.TotalScore,
			Rank:       &rank,
			StartedAt:  p.StartedAt,
			FinishedAt: p.FinishedAt,
			CreatedAt:  p.CreatedAt,
		}
	}

	return &dto.ContestLeaderboardResponse{
		Participants: responses,
		TotalCount:   total,
	}, nil
}

// ============================================================================
// SUBMISSIONS
// ============================================================================

func (s *ContestService) SubmitAnswer(ctx context.Context, userID, contestID, problemID uuid.UUID, req dto.SubmitAnswerRequest) (*dto.ContestSubmissionResponse, error) {
	// Validate participation
	participant, err := s.participantRepo.GetByContestAndUser(ctx, contestID, userID)
	if err != nil {
		return nil, err
	}
	if participant == nil {
		return nil, errors.New("you must join the contest before submitting")
	}

	// Validate problem
	problem, err := s.problemRepo.GetByID(ctx, problemID)
	if err != nil {
		return nil, err
	}
	if problem == nil {
		return nil, errors.New("problem not found")
	}
	if problem.ContestID != contestID {
		return nil, errors.New("problem does not belong to this contest")
	}

	// Create submission
	submission := &model.ContestSubmission{
		ContestID: contestID,
		ProblemID: problemID,
		UserID:    userID,
		Code:      req.Code,
		Language:  req.Language,
		Answer:    req.Answer,
		MaxScore:  problem.Points,
		Status:    model.ContestSubmissionStatusPending,
	}

	// For non-CODE problems, evaluate immediately
	if problem.Type == model.ContestProblemTypeMultipleChoice || problem.Type == model.ContestProblemTypeShortAnswer {
		if req.Answer != nil && problem.CorrectAnswer != nil && *req.Answer == *problem.CorrectAnswer {
			submission.Score = problem.Points
			submission.Status = model.ContestSubmissionStatusAccepted
		} else {
			submission.Score = 0
			submission.Status = model.ContestSubmissionStatusWrongAnswer
		}
	}

	if err := s.submissionRepo.Create(ctx, submission); err != nil {
		return nil, err
	}

	// Update participant total score (sum of best scores per problem)
	s.recalculateParticipantScore(ctx, participant, contestID, userID)

	return s.toSubmissionResponse(submission), nil
}

func (s *ContestService) GetMySubmissions(ctx context.Context, userID, contestID uuid.UUID) ([]dto.ContestSubmissionResponse, error) {
	submissions, err := s.submissionRepo.ListByContestAndUser(ctx, contestID, userID)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ContestSubmissionResponse, len(submissions))
	for i, sub := range submissions {
		responses[i] = *s.toSubmissionResponse(&sub)
	}

	return responses, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func (s *ContestService) recalculateParticipantScore(ctx context.Context, participant *model.ContestParticipant, contestID, userID uuid.UUID) {
	problems, err := s.problemRepo.ListByContestID(ctx, contestID)
	if err != nil {
		return
	}

	totalScore := 0
	for _, problem := range problems {
		best, err := s.submissionRepo.GetBestByContestProblemUser(ctx, contestID, problem.ID, userID)
		if err != nil || best == nil {
			continue
		}
		totalScore += best.Score
	}

	participant.TotalScore = totalScore
	_ = s.participantRepo.UpdateScore(ctx, participant)
}

func (s *ContestService) toContestResponse(contest *model.Contest, userID *uuid.UUID) *dto.ContestResponse {
	resp := &dto.ContestResponse{
		ID:               contest.ID,
		Title:            contest.Title,
		Slug:             contest.Slug,
		Description:      contest.Description,
		BannerURL:        contest.BannerURL,
		Type:             string(contest.Type),
		Status:           string(contest.Status),
		StartTime:        contest.StartTime,
		EndTime:          contest.EndTime,
		Duration:         contest.Duration,
		MaxParticipants:  contest.MaxParticipants,
		ParticipantCount: contest.ParticipantCount,
		IsPublic:         contest.IsPublic,
		CreatedBy:        contest.CreatedBy,
		OrganizationID:   contest.OrganizationID,
		CreatedAt:        contest.CreatedAt,
		UpdatedAt:        contest.UpdatedAt,
	}

	if userID != nil {
		participant, err := s.participantRepo.GetByContestAndUser(context.Background(), contest.ID, *userID)
		if err == nil && participant != nil {
			resp.MyParticipation = &dto.ContestParticipantResponse{
				ID:         participant.ID,
				ContestID:  participant.ContestID,
				UserID:     participant.UserID,
				TotalScore: participant.TotalScore,
				Rank:       participant.Rank,
				StartedAt:  participant.StartedAt,
				FinishedAt: participant.FinishedAt,
				CreatedAt:  participant.CreatedAt,
			}
		}
	}

	return resp
}

func (s *ContestService) toProblemResponse(problem *model.ContestProblem) *dto.ContestProblemResponse {
	var testCases interface{}
	if len(problem.TestCases) > 0 {
		_ = json.Unmarshal(problem.TestCases, &testCases)
	}

	var options interface{}
	if len(problem.Options) > 0 {
		_ = json.Unmarshal(problem.Options, &options)
	}

	return &dto.ContestProblemResponse{
		ID:            problem.ID,
		ContestID:     problem.ContestID,
		Title:         problem.Title,
		Description:   problem.Description,
		Type:          string(problem.Type),
		Difficulty:    string(problem.Difficulty),
		Points:        problem.Points,
		DisplayOrder:  problem.DisplayOrder,
		TimeLimit:     problem.TimeLimit,
		MemoryLimit:   problem.MemoryLimit,
		InputFormat:   problem.InputFormat,
		OutputFormat:  problem.OutputFormat,
		SampleInput:   problem.SampleInput,
		SampleOutput:  problem.SampleOutput,
		TestCases:     testCases,
		Options:       options,
		CorrectAnswer: problem.CorrectAnswer,
		CreatedAt:     problem.CreatedAt,
		UpdatedAt:     problem.UpdatedAt,
	}
}

func (s *ContestService) toSubmissionResponse(submission *model.ContestSubmission) *dto.ContestSubmissionResponse {
	return &dto.ContestSubmissionResponse{
		ID:            submission.ID,
		ContestID:     submission.ContestID,
		ProblemID:     submission.ProblemID,
		UserID:        submission.UserID,
		Code:          submission.Code,
		Language:      submission.Language,
		Answer:        submission.Answer,
		Score:         submission.Score,
		MaxScore:      submission.MaxScore,
		Status:        string(submission.Status),
		ExecutionTime: submission.ExecutionTime,
		MemoryUsed:    submission.MemoryUsed,
		Output:        submission.Output,
		CreatedAt:     submission.CreatedAt,
	}
}
