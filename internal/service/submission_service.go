package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type SubmissionServiceInterface interface {
	Submit(ctx context.Context, req dto.CreateSubmissionDTO) (*model.Submission, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Submission, error)
	GetByAssignment(ctx context.Context, assignmentID uuid.UUID, page, pageSize int) (*dto.SubmissionListDTO, error)
	GetByUser(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.SubmissionListDTO, error)
	GetUserSubmissionsForAssignment(ctx context.Context, assignmentID, userID uuid.UUID) ([]model.Submission, error)
	RunCode(ctx context.Context, req dto.RunCodeDTO) (*dto.RunCodeResponseDTO, error)
	RunCustomCode(ctx context.Context, req dto.RunCustomCodeDTO) (*dto.RunCodeResponseDTO, error)
	ProcessSubmission(ctx context.Context, submissionID uuid.UUID) error
}

type Judge0Client interface {
	Submit(ctx context.Context, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error)
	GetResult(ctx context.Context, token string) (*dto.Judge0ResponseDTO, error)
}

type HTTPJudge0Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPJudge0Client(baseURL string) *HTTPJudge0Client {
	return &HTTPJudge0Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *HTTPJudge0Client) Submit(ctx context.Context, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/submissions?wait=true", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("judge0 error: %s", string(respBody))
	}

	var result dto.Judge0ResponseDTO
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *HTTPJudge0Client) GetResult(ctx context.Context, token string) (*dto.Judge0ResponseDTO, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/submissions/"+token, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result dto.Judge0ResponseDTO
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

type SubmissionService struct {
	repo          repository.SubmissionRepositoryInterface
	assignmentSvc AssignmentServiceInterface
	testCaseRepo  repository.TestCaseRepositoryInterface
	judge0Client  Judge0Client
	redis         *redis.Client
	cfg           *config.Config
}

func NewSubmissionService(
	repo repository.SubmissionRepositoryInterface,
	assignmentSvc AssignmentServiceInterface,
	testCaseRepo repository.TestCaseRepositoryInterface,
	redis *redis.Client,
	cfg *config.Config,
) *SubmissionService {
	judge0URL := "http://judge0:2358"
	if cfg.Environment == "dev" {
		judge0URL = "http://localhost:2358"
	}

	return &SubmissionService{
		repo:          repo,
		assignmentSvc: assignmentSvc,
		testCaseRepo:  testCaseRepo,
		judge0Client:  NewHTTPJudge0Client(judge0URL),
		redis:         redis,
		cfg:           cfg,
	}
}

func (s *SubmissionService) Submit(ctx context.Context, req dto.CreateSubmissionDTO) (*model.Submission, error) {
	assignmentID, err := uuid.Parse(req.AssignmentID)
	if err != nil {
		return nil, errors.New("invalid assignment_id")
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, errors.New("invalid user_id")
	}

	assignment, err := s.assignmentSvc.GetByID(ctx, assignmentID, false)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	if !assignment.IsPublished {
		return nil, errors.New("assignment is not published")
	}

	submission := &model.Submission{
		AssignmentID: assignmentID,
		UserID:       userID,
		Language:     req.Language,
		Code:         req.Code,
		Verdict:      model.VerdictPending,
	}

	if err := s.repo.Create(ctx, submission); err != nil {
		return nil, err
	}

	go func() {
		_ = s.ProcessSubmission(context.Background(), submission.ID)
	}()

	return submission, nil
}

func (s *SubmissionService) ProcessSubmission(ctx context.Context, submissionID uuid.UUID) error {
	submission, err := s.repo.GetByID(ctx, submissionID)
	if err != nil {
		return err
	}
	if submission == nil {
		return errors.New("submission not found")
	}

	assignment, err := s.assignmentSvc.GetByID(ctx, submission.AssignmentID, true)
	if err != nil {
		return err
	}
	if assignment == nil {
		return errors.New("assignment not found")
	}

	testCases, err := s.testCaseRepo.GetByAssignment(ctx, assignment.ID)
	if err != nil {
		return err
	}

	if len(testCases) == 0 {
		return s.repo.UpdateVerdict(ctx, submissionID, model.VerdictAccepted, 0, 0, 0, 0)
	}

	languageID := s.getLanguageID(submission.Language)
	totalPassed := 0
	totalTime := 0
	totalMemory := 0
	finalVerdict := model.VerdictAccepted

	for _, tc := range testCases {
		judgeReq := dto.Judge0SubmissionDTO{
			SourceCode:     submission.Code,
			LanguageID:     languageID,
			Stdin:          tc.Input,
			ExpectedOutput: tc.ExpectedOutput,
			CPUTimeLimit:   assignment.TimeLimit,
			MemoryLimit:    assignment.MemoryLimit * 1024,
		}

		result, err := s.judge0Client.Submit(ctx, judgeReq)
		if err != nil {
			finalVerdict = model.VerdictRuntimeError
			continue
		}

		verdict := s.mapJudge0Status(result.Status.ID)
		if verdict != model.VerdictAccepted {
			if finalVerdict == model.VerdictAccepted || finalVerdict == model.VerdictWrongAnswer {
				finalVerdict = verdict
			}
		} else {
			totalPassed++
		}

		if result.Time != "" {
			var execTime float64
			fmt.Sscanf(result.Time, "%f", &execTime)
			totalTime += int(execTime * 1000)
		}
		if result.Memory != "" {
			var memUsed int
			fmt.Sscanf(result.Memory, "%d", &memUsed)
			totalMemory += memUsed
		}
	}

	if totalPassed == len(testCases) {
		finalVerdict = model.VerdictAccepted
	} else if finalVerdict == model.VerdictAccepted && totalPassed < len(testCases) {
		finalVerdict = model.VerdictWrongAnswer
	}

	avgTime := 0
	avgMemory := 0
	if len(testCases) > 0 {
		avgTime = totalTime / len(testCases)
		avgMemory = totalMemory / len(testCases)
	}

	return s.repo.UpdateVerdict(ctx, submissionID, finalVerdict, avgTime, avgMemory, totalPassed, len(testCases))
}

func (s *SubmissionService) GetByID(ctx context.Context, id uuid.UUID) (*model.Submission, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *SubmissionService) GetByAssignment(ctx context.Context, assignmentID uuid.UUID, page, pageSize int) (*dto.SubmissionListDTO, error) {
	submissions, total, err := s.repo.GetByAssignment(ctx, assignmentID, page, pageSize)
	if err != nil {
		return nil, err
	}

	var data []dto.SubmissionResponseDTO
	for _, sub := range submissions {
		data = append(data, s.toResponseDTO(sub))
	}

	return &dto.SubmissionListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *SubmissionService) GetByUser(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.SubmissionListDTO, error) {
	submissions, total, err := s.repo.GetByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	var data []dto.SubmissionResponseDTO
	for _, sub := range submissions {
		data = append(data, s.toResponseDTO(sub))
	}

	return &dto.SubmissionListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *SubmissionService) GetUserSubmissionsForAssignment(ctx context.Context, assignmentID, userID uuid.UUID) ([]model.Submission, error) {
	return s.repo.GetByAssignmentAndUser(ctx, assignmentID, userID)
}

func (s *SubmissionService) RunCode(ctx context.Context, req dto.RunCodeDTO) (*dto.RunCodeResponseDTO, error) {
	assignmentID, err := uuid.Parse(req.AssignmentID)
	if err != nil {
		return nil, errors.New("invalid assignment_id")
	}

	assignment, err := s.assignmentSvc.GetByID(ctx, assignmentID, false)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	testCases, err := s.testCaseRepo.GetNonHiddenByAssignment(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	if len(testCases) == 0 {
		return &dto.RunCodeResponseDTO{
			Output:  "",
			Verdict: "no_test_cases",
		}, nil
	}

	tc := testCases[0]
	languageID := s.getLanguageID(req.Language)

	judgeReq := dto.Judge0SubmissionDTO{
		SourceCode:     req.Code,
		LanguageID:     languageID,
		Stdin:          tc.Input,
		ExpectedOutput: tc.ExpectedOutput,
		CPUTimeLimit:   assignment.TimeLimit,
		MemoryLimit:    assignment.MemoryLimit * 1024,
	}

	result, err := s.judge0Client.Submit(ctx, judgeReq)
	if err != nil {
		return &dto.RunCodeResponseDTO{
			Error:   err.Error(),
			Verdict: "error",
		}, nil
	}

	response := &dto.RunCodeResponseDTO{
		Output:  result.Stdout,
		Verdict: s.mapJudge0Status(result.Status.ID).String(),
	}

	if result.CompileOutput != "" {
		response.Error = result.CompileOutput
	}
	if result.Stderr != "" {
		response.Error = result.Stderr
	}

	if result.Time != "" {
		var execTime float64
		fmt.Sscanf(result.Time, "%f", &execTime)
		response.ExecTime = int(execTime * 1000)
	}
	if result.Memory != "" {
		fmt.Sscanf(result.Memory, "%d", &response.MemUsed)
	}

	return response, nil
}

func (s *SubmissionService) getLanguageID(language string) int {
	languageMap := map[string]int{
		"c":          50,
		"c++":        54,
		"cpp":        54,
		"java":       62,
		"python":     71,
		"python3":    71,
		"javascript": 63,
		"js":         63,
		"typescript": 74,
		"ts":         74,
		"go":         60,
		"rust":       73,
		"ruby":       72,
		"php":        68,
		"c#":         51,
		"cs":         51,
		"swift":      83,
		"kotlin":     78,
	}

	if id, ok := languageMap[language]; ok {
		return id
	}
	return 71
}

func (s *SubmissionService) mapJudge0Status(statusID int) model.SubmissionVerdict {
	switch statusID {
	case 1, 2:
		return model.VerdictPending
	case 3:
		return model.VerdictAccepted
	case 4:
		return model.VerdictWrongAnswer
	case 5:
		return model.VerdictTimeLimitExceed
	case 6:
		return model.VerdictCompilationError
	case 7, 8, 9, 10, 11, 12:
		return model.VerdictRuntimeError
	case 13:
		return model.VerdictMemoryLimitExceed
	default:
		return model.VerdictRuntimeError
	}
}

func (s *SubmissionService) toResponseDTO(sub model.Submission) dto.SubmissionResponseDTO {
	return dto.SubmissionResponseDTO{
		ID:              sub.ID,
		AssignmentID:    sub.AssignmentID,
		UserID:          sub.UserID,
		Code:            sub.Code,
		Verdict:         string(sub.Verdict),
		ExecutionTime:   sub.ExecutionTime,
		MemoryUsed:      sub.MemoryUsed,
		TestCasesPassed: sub.TestCasesPassed,
		TotalTestCases:  sub.TotalTestCases,
		CreatedAt:       sub.CreatedAt.Format(time.RFC3339),
	}
}

func (s *SubmissionService) RunCustomCode(ctx context.Context, req dto.RunCustomCodeDTO) (*dto.RunCodeResponseDTO, error) {
	assignmentID, err := uuid.Parse(req.AssignmentID)
	if err != nil {
		return nil, errors.New("invalid assignment_id")
	}

	assignment, err := s.assignmentSvc.GetByID(ctx, assignmentID, false)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	languageID := s.getLanguageID(req.Language)

	judgeReq := dto.Judge0SubmissionDTO{
		SourceCode:   req.Code,
		LanguageID:   languageID,
		Stdin:        req.CustomInput,
		CPUTimeLimit: assignment.TimeLimit,
		MemoryLimit:  assignment.MemoryLimit * 1024,
	}

	result, err := s.judge0Client.Submit(ctx, judgeReq)
	if err != nil {
		return &dto.RunCodeResponseDTO{
			Error:   err.Error(),
			Verdict: "error",
		}, nil
	}

	response := &dto.RunCodeResponseDTO{
		Output:  result.Stdout,
		Verdict: s.mapJudge0Status(result.Status.ID).String(),
	}

	if result.CompileOutput != "" {
		response.Error = result.CompileOutput
	}
	if result.Stderr != "" {
		response.Error = result.Stderr
	}

	if result.Time != "" {
		var execTime float64
		fmt.Sscanf(result.Time, "%f", &execTime)
		response.ExecTime = int(execTime * 1000)
	}
	if result.Memory != "" {
		fmt.Sscanf(result.Memory, "%d", &response.MemUsed)
	}

	return response, nil
}
