package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type AssignmentServiceInterface interface {
	Create(ctx context.Context, req dto.CreateAssignmentDTO) (*model.Assignment, error)
	GetByID(ctx context.Context, id uuid.UUID, includeHidden bool) (*model.Assignment, error)
	GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) (*dto.AssignmentListDTO, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateAssignmentDTO) (*model.Assignment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Publish(ctx context.Context, id uuid.UUID, livekitSvc LivekitServiceInterface) (*model.Assignment, error)
	Unpublish(ctx context.Context, id uuid.UUID) (*model.Assignment, error)
	AddTestCase(ctx context.Context, assignmentID uuid.UUID, req dto.CreateTestCaseDTO) (*model.TestCase, error)
	DeleteTestCase(ctx context.Context, testCaseID uuid.UUID) error
	ImportTestCases(ctx context.Context, assignmentID uuid.UUID, req dto.ImportTestCasesDTO) ([]model.TestCase, error)
	GetTestCases(ctx context.Context, assignmentID uuid.UUID, includeHidden bool) ([]model.TestCase, error)
	GetSandbox(ctx context.Context, assignmentID uuid.UUID, userID uuid.UUID) (*dto.SandboxResponseDTO, error)
}

type AssignmentService struct {
	repo           repository.AssignmentRepositoryInterface
	testCaseRepo   repository.TestCaseRepositoryInterface
	submissionRepo repository.SubmissionRepositoryInterface
}

func NewAssignmentService(
	repo repository.AssignmentRepositoryInterface,
	testCaseRepo repository.TestCaseRepositoryInterface,
	submissionRepo repository.SubmissionRepositoryInterface,
) *AssignmentService {
	return &AssignmentService{
		repo:           repo,
		testCaseRepo:   testCaseRepo,
		submissionRepo: submissionRepo,
	}
}

func (s *AssignmentService) Create(ctx context.Context, req dto.CreateAssignmentDTO) (*model.Assignment, error) {
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return nil, errors.New("invalid session_id")
	}

	difficulty := model.DifficultyMedium
	switch req.Difficulty {
	case "easy":
		difficulty = model.DifficultyEasy
	case "hard":
		difficulty = model.DifficultyHard
	}

	timeLimit := req.TimeLimit
	if timeLimit <= 0 {
		timeLimit = 2
	}

	memoryLimit := req.MemoryLimit
	if memoryLimit <= 0 {
		memoryLimit = 256
	}

	assignment := &model.Assignment{
		SessionID:   sessionID,
		Title:       req.Title,
		Description: req.Description,
		Difficulty:  difficulty,
		Language:    req.Language,
		StarterCode: req.StarterCode,
		TimeLimit:   timeLimit,
		MemoryLimit: memoryLimit,
	}

	if req.StartTime != nil {
		if t, err := time.Parse(time.RFC3339, *req.StartTime); err == nil {
			assignment.StartTime = &t
		}
	}
	if req.EndTime != nil {
		if t, err := time.Parse(time.RFC3339, *req.EndTime); err == nil {
			assignment.EndTime = &t
		}
	}

	if err := s.repo.Create(ctx, assignment); err != nil {
		return nil, err
	}

	return assignment, nil
}

func (s *AssignmentService) GetByID(ctx context.Context, id uuid.UUID, includeHidden bool) (*model.Assignment, error) {
	if includeHidden {
		return s.repo.GetByIDWithTestCases(ctx, id)
	}

	assignment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, nil
	}

	testCases, err := s.testCaseRepo.GetNonHiddenByAssignment(ctx, id)
	if err != nil {
		return nil, err
	}
	assignment.TestCases = testCases

	return assignment, nil
}

func (s *AssignmentService) GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) (*dto.AssignmentListDTO, error) {
	assignments, total, err := s.repo.GetBySession(ctx, sessionID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.AssignmentResponseDTO, 0, len(assignments))
	for _, a := range assignments {
		data = append(data, s.toResponseDTO(a))
	}

	return &dto.AssignmentListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *AssignmentService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateAssignmentDTO) (*model.Assignment, error) {
	assignment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	if req.Title != nil {
		assignment.Title = *req.Title
	}
	if req.Description != nil {
		assignment.Description = *req.Description
	}
	if req.Difficulty != nil {
		switch *req.Difficulty {
		case "easy":
			assignment.Difficulty = model.DifficultyEasy
		case "medium":
			assignment.Difficulty = model.DifficultyMedium
		case "hard":
			assignment.Difficulty = model.DifficultyHard
		}
	}
	if req.Language != nil {
		assignment.Language = *req.Language
	}
	if req.StarterCode != nil {
		assignment.StarterCode = *req.StarterCode
	}
	if req.TimeLimit != nil {
		assignment.TimeLimit = *req.TimeLimit
	}
	if req.MemoryLimit != nil {
		assignment.MemoryLimit = *req.MemoryLimit
	}
	if req.StartTime != nil {
		if t, err := time.Parse(time.RFC3339, *req.StartTime); err == nil {
			assignment.StartTime = &t
		}
	}
	if req.EndTime != nil {
		if t, err := time.Parse(time.RFC3339, *req.EndTime); err == nil {
			assignment.EndTime = &t
		}
	}

	if err := s.repo.Update(ctx, assignment); err != nil {
		return nil, err
	}

	return assignment, nil
}

func (s *AssignmentService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *AssignmentService) Publish(ctx context.Context, id uuid.UUID, livekitSvc LivekitServiceInterface) (*model.Assignment, error) {
	assignment, err := s.repo.GetByIDWithSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	if err := s.repo.Publish(ctx, id); err != nil {
		return nil, err
	}

	// Broadcast to livestream room if session exists
	// BUT skip broadcasting if start_time is in the future (scheduled assignment)
	// Frontend will handle broadcasting when the schedule time arrives
	shouldBroadcast := assignment.Session != nil
	if assignment.StartTime != nil && assignment.StartTime.After(time.Now()) {
		shouldBroadcast = false // Don't broadcast for scheduled assignments
	}

	if shouldBroadcast {
		event := map[string]interface{}{
			"type":          "assignment_published",
			"assignment_id": id.String(),
			"title":         assignment.Title,
			"language":      assignment.Language,
			"difficulty":    string(assignment.Difficulty),
			"timestamp":     time.Now().Format(time.RFC3339),
		}
		if assignment.StartTime != nil {
			event["start_time"] = assignment.StartTime.Format(time.RFC3339)
		}
		if assignment.EndTime != nil {
			event["end_time"] = assignment.EndTime.Format(time.RFC3339)
		}
		eventJSON, _ := json.Marshal(event)

		sendReq := dto.SendDataDTO{
			Data:  string(eventJSON),
			Topic: "collaboration",
		}
		// Dùng SessionID (chính là room name trong LiveKit)
		_ = livekitSvc.SendData(context.Background(), assignment.SessionID.String(), sendReq)
	}

	return s.repo.GetByID(ctx, id)
}

func (s *AssignmentService) Unpublish(ctx context.Context, id uuid.UUID) (*model.Assignment, error) {
	if err := s.repo.Unpublish(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

func (s *AssignmentService) AddTestCase(ctx context.Context, assignmentID uuid.UUID, req dto.CreateTestCaseDTO) (*model.TestCase, error) {
	testCase := &model.TestCase{
		AssignmentID:   assignmentID,
		Input:          req.Input,
		ExpectedOutput: req.ExpectedOutput,
		IsHidden:       req.IsHidden,
		DisplayOrder:   req.DisplayOrder,
	}

	if err := s.testCaseRepo.Create(ctx, testCase); err != nil {
		return nil, err
	}

	return testCase, nil
}

func (s *AssignmentService) DeleteTestCase(ctx context.Context, testCaseID uuid.UUID) error {
	return s.testCaseRepo.Delete(ctx, testCaseID)
}

func (s *AssignmentService) ImportTestCases(ctx context.Context, assignmentID uuid.UUID, req dto.ImportTestCasesDTO) ([]model.TestCase, error) {
	testCases := make([]model.TestCase, 0, len(req.TestCases))
	for i, tc := range req.TestCases {
		order := tc.DisplayOrder
		if order == 0 {
			order = i + 1
		}
		testCases = append(testCases, model.TestCase{
			AssignmentID:   assignmentID,
			Input:          tc.Input,
			ExpectedOutput: tc.ExpectedOutput,
			IsHidden:       tc.IsHidden,
			DisplayOrder:   order,
		})
	}
	if err := s.testCaseRepo.CreateBatch(ctx, testCases); err != nil {
		return nil, err
	}
	return testCases, nil
}

func (s *AssignmentService) GetTestCases(ctx context.Context, assignmentID uuid.UUID, includeHidden bool) ([]model.TestCase, error) {
	if includeHidden {
		return s.testCaseRepo.GetByAssignment(ctx, assignmentID)
	}
	return s.testCaseRepo.GetNonHiddenByAssignment(ctx, assignmentID)
}

func (s *AssignmentService) toResponseDTO(a model.Assignment) dto.AssignmentResponseDTO {
	var publishedAt *string
	if a.PublishedAt != nil {
		t := a.PublishedAt.Format(time.RFC3339)
		publishedAt = &t
	}
	var startTime *string
	if a.StartTime != nil {
		t := a.StartTime.Format(time.RFC3339)
		startTime = &t
	}
	var endTime *string
	if a.EndTime != nil {
		t := a.EndTime.Format(time.RFC3339)
		endTime = &t
	}

	return dto.AssignmentResponseDTO{
		ID:          a.ID,
		SessionID:   a.SessionID,
		Title:       a.Title,
		Description: a.Description,
		Difficulty:  string(a.Difficulty),
		Language:    []string(a.Language),
		StarterCode: a.StarterCode,
		TimeLimit:   a.TimeLimit,
		MemoryLimit: a.MemoryLimit,
		IsPublished: a.IsPublished,
		PublishedAt: publishedAt,
		StartTime:   startTime,
		EndTime:     endTime,
		CreatedAt:   a.CreatedAt.Format(time.RFC3339),
	}
}

func (s *AssignmentService) GetSandbox(ctx context.Context, assignmentID uuid.UUID, userID uuid.UUID) (*dto.SandboxResponseDTO, error) {
	assignment, err := s.repo.GetByID(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	sampleTests, err := s.testCaseRepo.GetNonHiddenByAssignment(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	var sampleTestDTOs []dto.TestCaseResponseDTO
	for _, tc := range sampleTests {
		sampleTestDTOs = append(sampleTestDTOs, dto.TestCaseResponseDTO{
			ID:             tc.ID,
			AssignmentID:   tc.AssignmentID,
			Input:          tc.Input,
			ExpectedOutput: tc.ExpectedOutput,
			IsHidden:       tc.IsHidden,
			DisplayOrder:   tc.DisplayOrder,
		})
	}

	result := &dto.SandboxResponseDTO{
		Assignment:  s.toResponseDTO(*assignment),
		SampleTests: sampleTestDTOs,
	}

	lastSub, err := s.submissionRepo.GetLatestByAssignmentAndUser(ctx, assignmentID, userID)
	if err != nil {
		return nil, err
	}
	if lastSub != nil {
		result.LastSubmission = &dto.SubmissionSnapshotDTO{
			ID:              lastSub.ID,
			Language:        lastSub.Language,
			Code:            lastSub.Code,
			Verdict:         string(lastSub.Verdict),
			TestCasesPassed: lastSub.TestCasesPassed,
			TotalTestCases:  lastSub.TotalTestCases,
			SubmittedAt:     lastSub.CreatedAt.Format(time.RFC3339),
		}
	}

	return result, nil
}
