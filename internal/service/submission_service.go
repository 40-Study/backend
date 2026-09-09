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
	"study.com/v1/internal/utils"
)

type SubmissionServiceInterface interface {
	Submit(ctx context.Context, req dto.CreateSubmissionDTO) (*model.Submission, error)
	GetByID(ctx context.Context, id, requesterID uuid.UUID) (*model.Submission, error)
	GetByAssignment(ctx context.Context, assignmentID, requesterID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.SubmissionListDTO, error)
	GetByUser(ctx context.Context, userID, requesterID uuid.UUID, page, pageSize int) (*dto.SubmissionListDTO, error)
	GetUserSubmissionsForAssignment(ctx context.Context, assignmentID, userID uuid.UUID) ([]model.Submission, error)
	RunCode(ctx context.Context, req dto.RunCodeDTO) (*dto.RunCodeResponseDTO, error)
	RunCustomCode(ctx context.Context, req dto.RunCustomCodeDTO) (*dto.RunCodeResponseDTO, error)
	ExecuteCode(ctx context.Context, req dto.ExecuteCodeDTO) (*dto.ExecuteCodeResponseDTO, error)
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

// createJudge0Request creates a Judge0 submission request
// cpuTimeLimit is per test case (not total assignment time), default 5 seconds, max 20 seconds
func createJudge0Request(sourceCode string, languageID int, stdin string, expectedOutput string, cpuTimeLimit int, memoryLimit int) dto.Judge0SubmissionDTO {
	// Default 5 seconds per test case if not specified or too high
	cpuLimit := 5.0
	if cpuTimeLimit > 0 && cpuTimeLimit <= 20 {
		cpuLimit = float64(cpuTimeLimit)
	}
	return dto.Judge0SubmissionDTO{
		SourceCode:     sourceCode,
		LanguageID:     languageID,
		Stdin:          stdin,
		ExpectedOutput: expectedOutput,
		CPUTimeLimit:   cpuLimit,
		MemoryLimit:    memoryLimit,
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

// ErrSubmissionForbidden: C-10 (audit 260909) — trước đây GetByID/GetByUser/GetMySubmissions
// không kiểm tra quyền, cho phép đọc/liệt kê bài nộp (và điểm) của bất kỳ ai.
var ErrSubmissionForbidden = errors.New("forbidden: not the owner")

type SubmissionService struct {
	repo          repository.SubmissionRepositoryInterface
	assignmentSvc AssignmentServiceInterface
	testCaseRepo  repository.TestCaseRepositoryInterface
	scheduleRepo  repository.ScheduleRepositoryInterface
	classRepo     repository.ClassRepositoryInterface
	judge0Client  Judge0Client
	redis         *redis.Client
	cfg           *config.Config
}

func NewSubmissionService(
	repo repository.SubmissionRepositoryInterface,
	assignmentSvc AssignmentServiceInterface,
	testCaseRepo repository.TestCaseRepositoryInterface,
	scheduleRepo repository.ScheduleRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
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
		scheduleRepo:  scheduleRepo,
		classRepo:     classRepo,
		judge0Client:  NewHTTPJudge0Client(judge0URL),
		redis:         redis,
		cfg:           cfg,
	}
}

// canManageAssignment (H-03, review vòng 1): kiểm tra requesterID có phải là giáo viên "sở
// hữu" assignment hay không, dùng chung cho canAccessSubmission (GetByID) VÀ GetByAssignment.
// Assignment có 2 nguồn ownership loại trừ nhau (assignment.go): SessionID (buổi live coding)
// hoặc ClassID (bài tập về nhà/homework, SessionID nil) — kiểm cả hai, không chỉ SessionID
// như canAccessSubmission cũ (đó chính là "nghịch lý" review nêu: giáo viên KHÔNG xem được
// bài tập về nhà của học sinh qua đường hợp lệ, chỉ có lỗ hổng GetByAssignment không-check-gì
// mới "chạy được"). isAdmin=true bỏ qua toàn bộ kiểm tra (SYSTEM_ADMIN kiểm duyệt).
func (s *SubmissionService) canManageAssignment(ctx context.Context, assignment *model.Assignment, requesterID uuid.UUID, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	if assignment == nil {
		return false, nil
	}
	if assignment.SessionID != nil {
		return s.scheduleRepo.TeacherCanManageSession(ctx, *assignment.SessionID, requesterID)
	}
	if assignment.ClassID != nil {
		return s.classRepo.TeacherClassExists(ctx, *assignment.ClassID, requesterID)
	}
	return false, nil
}

// canAccessSubmission cho phép: (1) chính chủ bài nộp, hoặc (2) giáo viên "sở hữu" assignment
// (xem canManageAssignment). Trước đây chỉ kiểm SessionID -> giáo viên không xem được bài nộp
// bài tập về nhà (ClassID) của chính lớp mình qua đường hợp lệ này.
func (s *SubmissionService) canAccessSubmission(ctx context.Context, sub *model.Submission, requesterID uuid.UUID) (bool, error) {
	if sub.UserID == requesterID {
		return true, nil
	}
	assignment, err := s.assignmentSvc.GetByID(ctx, sub.AssignmentID, false)
	if err != nil {
		return false, err
	}
	return s.canManageAssignment(ctx, assignment, requesterID, false)
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

	// M-05 (audit 260909): bọc bằng SafeGo — panic khi chấm bài (vd lỗi parse JSON, Judge0
	// timeout) không được recover trước đây có thể làm sập cả server.
	utils.SafeGo(func() {
		_ = s.ProcessSubmission(context.Background(), submission.ID)
	})

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

	// Handle multiple choice assignments
	if submission.Language == "json" {
		return s.processMultipleChoiceSubmission(ctx, submission, assignment)
	}

	// Handle essay assignments (no auto-grading)
	if submission.Language == "text" {
		return s.repo.UpdateVerdict(ctx, submissionID, model.VerdictAccepted, 0, 0, 0, 0)
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
		judgeReq := createJudge0Request(
			submission.Code,
			languageID,
			tc.Input,
			tc.ExpectedOutput,
			assignment.TimeLimit,
			assignment.MemoryLimit*1024,
		)

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
		totalMemory += result.Memory
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

// processMultipleChoiceSubmission handles scoring for multiple choice assignments
func (s *SubmissionService) processMultipleChoiceSubmission(ctx context.Context, submission *model.Submission, assignment *model.Assignment) error {
	// Parse assignment config to get correct answers
	var assignmentConfig struct {
		Type      string `json:"type"`
		Questions []struct {
			Question     string   `json:"question"`
			Answers      []string `json:"answers"`
			CorrectIndex int      `json:"correctIndex"`
		} `json:"questions"`
		// Old format support
		OldAnswers      []string `json:"answers"`
		OldCorrectIndex int      `json:"correctIndex"`
	}

	if err := json.Unmarshal([]byte(assignment.StarterCode), &assignmentConfig); err != nil {
		return s.repo.UpdateVerdict(ctx, submission.ID, model.VerdictRuntimeError, 0, 0, 0, 0)
	}

	// Parse student submission
	var studentSubmission struct {
		Answers []int `json:"answers"`
	}

	if err := json.Unmarshal([]byte(submission.Code), &studentSubmission); err != nil {
		return s.repo.UpdateVerdict(ctx, submission.ID, model.VerdictRuntimeError, 0, 0, 0, 0)
	}

	// Handle old format (single question)
	questions := assignmentConfig.Questions
	if len(questions) == 0 && len(assignmentConfig.OldAnswers) > 0 {
		questions = []struct {
			Question     string   `json:"question"`
			Answers      []string `json:"answers"`
			CorrectIndex int      `json:"correctIndex"`
		}{
			{
				Question:     assignment.Description,
				Answers:      assignmentConfig.OldAnswers,
				CorrectIndex: assignmentConfig.OldCorrectIndex,
			},
		}
	}

	// Score the submission
	totalQuestions := len(questions)
	correctCount := 0

	for i, q := range questions {
		if i < len(studentSubmission.Answers) && studentSubmission.Answers[i] == q.CorrectIndex {
			correctCount++
		}
	}

	// Calculate score (0-100)
	score := 0
	if totalQuestions > 0 {
		score = (correctCount * 100) / totalQuestions
	}

	// Update submission with score in Code field (JSON with results)
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"answers": studentSubmission.Answers,
		"score":   score,
		"correct": correctCount,
		"total":   totalQuestions,
	})
	submission.Code = string(resultJSON)

	// Use TestCasesPassed/TotalTestCases to store correct/total
	verdict := model.VerdictAccepted
	if correctCount < totalQuestions {
		verdict = model.VerdictWrongAnswer
	}

	return s.repo.UpdateVerdictWithCode(ctx, submission.ID, verdict, 0, 0, correctCount, totalQuestions, string(resultJSON))
}

func (s *SubmissionService) GetByID(ctx context.Context, id, requesterID uuid.UUID) (*model.Submission, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil || sub == nil {
		return sub, err
	}
	allowed, err := s.canAccessSubmission(ctx, sub, requesterID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrSubmissionForbidden
	}
	return sub, nil
}

// GetByAssignment (H-03, review vòng 1): TRƯỚC ĐÂY không nhận requesterID, không lọc gì —
// bất kỳ user đã đăng nhập nào biết assignment_id đều đọc được TOÀN BỘ bài nộp (mã nguồn,
// điểm) của mọi học sinh cho assignment đó. Giờ chỉ giáo viên "sở hữu" assignment (qua
// canManageAssignment: session hoặc class) hoặc admin mới xem được danh sách này.
func (s *SubmissionService) GetByAssignment(ctx context.Context, assignmentID, requesterID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.SubmissionListDTO, error) {
	assignment, err := s.assignmentSvc.GetByID(ctx, assignmentID, false)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}
	allowed, err := s.canManageAssignment(ctx, assignment, requesterID, isAdmin)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrSubmissionForbidden
	}

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

func (s *SubmissionService) GetByUser(ctx context.Context, userID, requesterID uuid.UUID, page, pageSize int) (*dto.SubmissionListDTO, error) {
	// C-10: chỉ chính chủ mới liệt kê được toàn bộ bài nộp của mình qua endpoint này.
	if userID != requesterID {
		return nil, ErrSubmissionForbidden
	}
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

	judgeReq := createJudge0Request(
		req.Code,
		languageID,
		tc.Input,
		tc.ExpectedOutput,
		assignment.TimeLimit,
		assignment.MemoryLimit*1024,
	)

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
	response.MemUsed = result.Memory

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
	resp := dto.SubmissionResponseDTO{
		ID:              sub.ID,
		AssignmentID:    sub.AssignmentID,
		UserID:          sub.UserID,
		Language:        sub.Language,
		Code:            sub.Code,
		Verdict:         string(sub.Verdict),
		ExecutionTime:   sub.ExecutionTime,
		MemoryUsed:      sub.MemoryUsed,
		TestCasesPassed: sub.TestCasesPassed,
		TotalTestCases:  sub.TotalTestCases,
		CreatedAt:       sub.CreatedAt.Format(time.RFC3339),
	}

	// Calculate score (percentage of passed test cases)
	if sub.TotalTestCases > 0 {
		resp.Score = (sub.TestCasesPassed * 100) / sub.TotalTestCases
	}

	// Include user info if available
	if sub.User != nil && sub.User.ID != uuid.Nil {
		resp.User = &dto.SubmissionUserDTO{
			ID:       sub.User.ID,
			Username: sub.User.UserName,
			Email:    sub.User.Email,
		}
	}

	return resp
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

	judgeReq := createJudge0Request(
		req.Code,
		languageID,
		req.CustomInput,
		"",
		assignment.TimeLimit,
		assignment.MemoryLimit*1024,
	)

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
	response.MemUsed = result.Memory

	return response, nil
}

// ExecuteCode - Free sandbox execution without assignment (default limits: 5s CPU, 128MB memory)
func (s *SubmissionService) ExecuteCode(ctx context.Context, req dto.ExecuteCodeDTO) (*dto.ExecuteCodeResponseDTO, error) {
	languageID := s.getLanguageID(req.Language)

	judgeReq := createJudge0Request(
		req.Code,
		languageID,
		req.Stdin,
		"",
		5,         // 5 seconds default
		128*1024,  // 128MB default
	)

	result, err := s.judge0Client.Submit(ctx, judgeReq)
	if err != nil {
		return &dto.ExecuteCodeResponseDTO{
			Error: err.Error(),
		}, nil
	}

	response := &dto.ExecuteCodeResponseDTO{
		Stdout: result.Stdout,
		Stderr: result.Stderr,
		Time:   result.Time,
	}

	if result.CompileOutput != "" {
		response.Error = result.CompileOutput
	}

	// Parse exit code from status
	if result.Status.ID == 3 {
		response.ExitCode = 0
	} else {
		response.ExitCode = result.Status.ID
	}

	response.Memory = result.Memory

	return response, nil
}
