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

// Lỗi uỷ quyền của assignment (S3). Handler ánh xạ: ErrAssignmentNotFound -> 404 (người gọi không
// có quyền xem thì không được biết assignment có tồn tại), ErrAssignmentForbidden -> 403 (người
// gọi thấy được assignment nhưng không phải chủ), ErrAssignmentTargetRequired -> 400.
var (
	ErrAssignmentNotFound       = errors.New("assignment not found")
	ErrAssignmentForbidden      = errors.New("forbidden: only the owning teacher or an admin can modify this assignment")
	ErrAssignmentTargetRequired = errors.New("session_id or class_id is required")
)

// assignmentClassAccess / assignmentSessionGate: hai phép kiểm "thành viên" có sẵn ở nơi khác
// (ClassRepository, LivestreamService) — tái dùng đúng định nghĩa thành viên của chat/bảng trắng
// thay vì tự viết lại, để một học viên bị kick hay đã rời lớp mất quyền xem đề cùng lúc mất quyền chat.
type assignmentClassAccess interface {
	IsUserRelatedToClass(ctx context.Context, classID, userID uuid.UUID) (bool, error)
}
type assignmentSessionGate interface {
	EnsureSessionMember(ctx context.Context, sessionID, userID uuid.UUID) error
}

type AssignmentServiceInterface interface {
	// Create (S3): chỉ chủ của phiên/lớp mà assignment gắn vào (hoặc admin) mới tạo được.
	Create(ctx context.Context, actorID uuid.UUID, isAdmin bool, req dto.CreateAssignmentDTO) (*model.Assignment, error)
	GetByID(ctx context.Context, id uuid.UUID, includeHidden bool) (*model.Assignment, error)
	// GetBySession (S3): chỉ thành viên phiên hoặc chủ phiên mới liệt kê được; thành viên chỉ thấy bản đã publish.
	GetBySession(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID uuid.UUID, page, pageSize int) (*dto.AssignmentListDTO, error)
	// GetByClass (R4): bài tập của một lớp. Quản lý lớp (GV, chủ khoá, người tạo, admin, chủ/quản trị tổ chức) thấy cả
	// bản nháp, học viên đang học chỉ thấy bản đã công bố; người ngoài (hoặc lớp không tồn tại) nhận ErrClassNotFound.
	GetByClass(ctx context.Context, actorID uuid.UUID, isAdmin bool, classID uuid.UUID, page, pageSize int) (*dto.AssignmentListDTO, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateAssignmentDTO) (*model.Assignment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Publish(ctx context.Context, id uuid.UUID, livekitSvc LivekitServiceInterface) (*model.Assignment, error)
	Unpublish(ctx context.Context, id uuid.UUID) (*model.Assignment, error)
	AddTestCase(ctx context.Context, assignmentID uuid.UUID, req dto.CreateTestCaseDTO) (*model.TestCase, error)
	DeleteTestCase(ctx context.Context, assignmentID, testCaseID uuid.UUID) error
	ImportTestCases(ctx context.Context, assignmentID uuid.UUID, req dto.ImportTestCasesDTO) ([]model.TestCase, error)
	GetTestCases(ctx context.Context, assignmentID uuid.UUID, includeHidden bool) ([]model.TestCase, error)
	GetSandbox(ctx context.Context, assignmentID uuid.UUID, userID uuid.UUID) (*dto.SandboxResponseDTO, error)
	// CanManage (S2): người được xem test case ẩn và sửa test case — chủ assignment hoặc admin.
	CanManage(ctx context.Context, assignmentID, userID uuid.UUID, isAdmin bool) (bool, error)
	// CanView (S3): chủ/admin, hoặc thành viên phiên/lớp của assignment ĐÃ publish.
	CanView(ctx context.Context, assignmentID, userID uuid.UUID, isAdmin bool) (bool, error)
}

type AssignmentService struct {
	repo           repository.AssignmentRepositoryInterface
	testCaseRepo   repository.TestCaseRepositoryInterface
	submissionRepo repository.SubmissionRepositoryInterface
	classAccess    assignmentClassAccess
	sessionGate    assignmentSessionGate
	orgAccess      assignmentOrgAccess
}

// SetOrgAccess nối kiểm tra quyền tổ chức (tuỳ chọn; nil = không ai được nâng quyền).
func (s *AssignmentService) SetOrgAccess(a assignmentOrgAccess) { s.orgAccess = a }

func NewAssignmentService(
	repo repository.AssignmentRepositoryInterface,
	testCaseRepo repository.TestCaseRepositoryInterface,
	submissionRepo repository.SubmissionRepositoryInterface,
	classAccess assignmentClassAccess,
	sessionGate assignmentSessionGate,
) *AssignmentService {
	return &AssignmentService{
		repo:           repo,
		testCaseRepo:   testCaseRepo,
		submissionRepo: submissionRepo,
		classAccess:    classAccess,
		sessionGate:    sessionGate,
	}
}

func (s *AssignmentService) Create(ctx context.Context, actorID uuid.UUID, isAdmin bool, req dto.CreateAssignmentDTO) (*model.Assignment, error) {
	var sessionID *uuid.UUID
	if req.SessionID != "" {
		parsed, err := uuid.Parse(req.SessionID)
		if err != nil {
			return nil, errors.New("invalid session_id")
		}
		sessionID = &parsed
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

	// Parse ClassID if provided
	var classID *uuid.UUID
	if req.ClassID != "" {
		parsed, err := uuid.Parse(req.ClassID)
		if err != nil {
			return nil, errors.New("invalid class_id")
		}
		classID = &parsed
	}

	// S3: assignment không có cột người tạo — "chủ" được suy ra từ phiên/lớp nó gắn vào. Không gắn
	// vào đâu thì sẽ không ai (ngoài admin) quản lý được, nên không cho non-admin tạo. Mỗi nơi được
	// nêu phải qua kiểm riêng, không gộp: nếu gộp, lớp của chính mình che được phiên của người khác.
	if !isAdmin {
		if sessionID == nil && classID == nil {
			return nil, ErrAssignmentTargetRequired
		}
		for _, target := range []struct{ session, class *uuid.UUID }{{sessionID, nil}, {nil, classID}} {
			if target.session == nil && target.class == nil {
				continue
			}
			ok, err := s.repo.CanManageTarget(ctx, target.session, target.class, actorID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, ErrAssignmentForbidden
			}
		}
	}

	// Default type
	assignmentType := req.Type
	if assignmentType == "" {
		assignmentType = "live_coding"
	}

	assignment := &model.Assignment{
		SessionID:           sessionID,
		ClassID:             classID,
		Type:                assignmentType,
		Title:               req.Title,
		Description:         req.Description,
		Difficulty:          difficulty,
		Language:            req.Language,
		StarterCode:         req.StarterCode,
		TimeLimit:           timeLimit,
		MemoryLimit:         memoryLimit,
		AllowLateSubmission: req.AllowLateSubmission,
		LatePenaltyPercent:  req.LatePenaltyPercent,
		MaxLateDays:         req.MaxLateDays,
		GracePeriodMinutes:  req.GracePeriodMinutes,
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

func (s *AssignmentService) GetBySession(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID uuid.UUID, page, pageSize int) (*dto.AssignmentListDTO, error) {
	// Chủ phiên/admin thấy cả bản nháp; thành viên phiên chỉ thấy bản đã publish; người ngoài
	// nhận 404 (không lộ phiên có tồn tại hay không).
	canManage := isAdmin
	if !canManage {
		var err error
		if canManage, err = s.repo.CanManageTarget(ctx, &sessionID, nil, actorID); err != nil {
			return nil, err
		}
	}
	if !canManage {
		member, err := s.isSessionMember(ctx, sessionID, actorID)
		if err != nil {
			return nil, err
		}
		if !member {
			return nil, ErrAssignmentNotFound
		}
	}

	assignments, total, err := s.repo.GetBySession(ctx, sessionID, page, pageSize, !canManage)
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

func (s *AssignmentService) GetByClass(ctx context.Context, actorID uuid.UUID, isAdmin bool, classID uuid.UUID, page, pageSize int) (*dto.AssignmentListDTO, error) {
	// Thiếu checker thì fail-closed: không có căn cứ nào để nói người gọi xem được lớp.
	if s.orgAccess == nil {
		return nil, ErrClassNotFound
	}
	audience, err := s.orgAccess.ClassAudience(ctx, classID, actorID, isAdmin)
	if err != nil {
		return nil, err
	}
	if audience == ClassAudienceNone {
		return nil, ErrClassNotFound
	}

	assignments, total, err := s.repo.GetByClass(ctx, classID, page, pageSize, audience != ClassAudienceManager)
	if err != nil {
		return nil, err
	}
	data := make([]dto.AssignmentResponseDTO, 0, len(assignments))
	for _, a := range assignments {
		data = append(data, s.toResponseDTO(a))
	}
	return &dto.AssignmentListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
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

func (s *AssignmentService) DeleteTestCase(ctx context.Context, assignmentID, testCaseID uuid.UUID) error {
	return s.testCaseRepo.DeleteFromAssignment(ctx, assignmentID, testCaseID)
}

// CanManage (S2): admin luôn được; còn lại phải là chủ assignment (host phiên / giảng viên lớp /
// giảng viên chủ khoá) — xem AssignmentRepository.CanManage.
func (s *AssignmentService) CanManage(ctx context.Context, assignmentID, userID uuid.UUID, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	return s.repo.CanManage(ctx, assignmentID, userID)
}

// isSessionMember: true khi userID là thành viên phiên theo đúng định nghĩa của chat/bảng trắng.
// Không phải thành viên, bị kick hay phiên không còn thì là "không"; lỗi hạ tầng thì trả lỗi để
// handler báo 500 thay vì âm thầm từ chối. Thiếu dependency thì từ chối (fail-closed).
func (s *AssignmentService) isSessionMember(ctx context.Context, sessionID, userID uuid.UUID) (bool, error) {
	if s.sessionGate == nil {
		return false, nil
	}
	err := s.sessionGate.EnsureSessionMember(ctx, sessionID, userID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNotSessionMember), errors.Is(err, ErrParticipantKicked), errors.Is(err, ErrSessionNotFound):
		return false, nil
	default:
		return false, err
	}
}

// CanView (S3): admin và chủ luôn xem được (kể cả bản nháp). Người khác chỉ xem được assignment
// ĐÃ publish và chỉ khi là thành viên lớp hoặc phiên mà nó gắn vào — trước đây mọi tài khoản đăng
// nhập đọc được đề, starter_code và test mẫu của bất kỳ assignment nào biết id.
func (s *AssignmentService) CanView(ctx context.Context, assignmentID, userID uuid.UUID, isAdmin bool) (bool, error) {
	assignment, err := s.repo.GetByID(ctx, assignmentID)
	if err != nil {
		return false, err
	}
	if assignment == nil {
		return false, nil
	}
	if isAdmin {
		return true, nil
	}
	if manage, err := s.repo.CanManage(ctx, assignmentID, userID); err != nil || manage {
		return manage, err
	}
	// R4: chủ/quản trị tổ chức của lớp xem được đề (kể cả bản nháp) để chấm bài; chỉ đọc, không nâng CanManage.
	if org, err := orgManagesAssignmentClass(ctx, s.orgAccess, assignment, userID); err != nil || org {
		return org, err
	}
	if !assignment.IsPublished {
		return false, nil
	}
	if assignment.ClassID != nil && s.classAccess != nil {
		related, err := s.classAccess.IsUserRelatedToClass(ctx, *assignment.ClassID, userID)
		if err != nil {
			return false, err
		}
		if related {
			return true, nil
		}
	}
	if assignment.SessionID != nil {
		return s.isSessionMember(ctx, *assignment.SessionID, userID)
	}
	return false, nil
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
		ID:                  a.ID,
		SessionID:           a.SessionID,
		ClassID:             a.ClassID,
		Type:                a.Type,
		Title:               a.Title,
		Description:         a.Description,
		Difficulty:          string(a.Difficulty),
		Language:            []string(a.Language),
		StarterCode:         a.StarterCode,
		TimeLimit:           a.TimeLimit,
		MemoryLimit:         a.MemoryLimit,
		DurationMins:        a.DurationMins,
		IsPublished:         a.IsPublished,
		PublishedAt:         publishedAt,
		StartTime:           startTime,
		EndTime:             endTime,
		ShowInRecap:         a.ShowInRecap,
		AllowLateSubmission: a.AllowLateSubmission,
		LatePenaltyPercent:  a.LatePenaltyPercent,
		MaxLateDays:         a.MaxLateDays,
		GracePeriodMinutes:  a.GracePeriodMinutes,
		CreatedAt:           a.CreatedAt.Format(time.RFC3339),
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
