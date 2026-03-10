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
	GetTestCases(ctx context.Context, assignmentID uuid.UUID, includeHidden bool) ([]model.TestCase, error)
}

type AssignmentService struct {
	repo         repository.AssignmentRepositoryInterface
	testCaseRepo repository.TestCaseRepositoryInterface
}

func NewAssignmentService(
	repo repository.AssignmentRepositoryInterface,
	testCaseRepo repository.TestCaseRepositoryInterface,
) *AssignmentService {
	return &AssignmentService{
		repo:         repo,
		testCaseRepo: testCaseRepo,
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

	var data []dto.AssignmentResponseDTO
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

	if err := s.repo.Update(ctx, assignment); err != nil {
		return nil, err
	}

	return assignment, nil
}

func (s *AssignmentService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *AssignmentService) Publish(ctx context.Context, id uuid.UUID, livekitSvc LivekitServiceInterface) (*model.Assignment, error) {
	assignment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, errors.New("assignment not found")
	}

	if err := s.repo.Publish(ctx, id); err != nil {
		return nil, err
	}

	event := map[string]interface{}{
		"type":          "assignment_published",
		"assignment_id": id.String(),
		"title":         assignment.Title,
		"language":      assignment.Language,
		"difficulty":    string(assignment.Difficulty),
		"timestamp":     time.Now().Format(time.RFC3339),
	}
	eventJSON, _ := json.Marshal(event)

	sendReq := dto.SendDataDTO{
		Data:  string(eventJSON),
		Topic: "collaboration",
	}
	_ = livekitSvc.SendData(context.Background(), assignment.SessionID.String(), sendReq)

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

	return dto.AssignmentResponseDTO{
		ID:          a.ID,
		SessionID:   a.SessionID,
		Title:       a.Title,
		Description: a.Description,
		Difficulty:  string(a.Difficulty),
		Language:    a.Language,
		StarterCode: a.StarterCode,
		TimeLimit:   a.TimeLimit,
		MemoryLimit: a.MemoryLimit,
		IsPublished: a.IsPublished,
		PublishedAt: publishedAt,
		CreatedAt:   a.CreatedAt.Format(time.RFC3339),
	}
}
