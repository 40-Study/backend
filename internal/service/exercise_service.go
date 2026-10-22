package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/repository"
)

type ExerciseServiceInterface interface {
	CreateExercise(ctx context.Context, creatorID uuid.UUID, req dto.CreateExerciseDTO) (*dto.ExerciseResponseDTO, error)
	// includeHidden=false (mặc định cho học viên) loại test case ẩn khỏi kết quả — S2.
	GetExerciseByID(ctx context.Context, id uuid.UUID, includeHidden bool) (*dto.ExerciseDetailDTO, error)
	// CanManage: người tạo / giảng viên chủ khoá gắn bài tập / admin (S2).
	CanManage(ctx context.Context, exerciseID, userID uuid.UUID, isAdmin bool) (bool, error)
	ListExercises(ctx context.Context, page, pageSize int) (*dto.ExerciseListDTO, error)
	UpdateExercise(ctx context.Context, id uuid.UUID, req dto.UpdateExerciseDTO) (*dto.ExerciseResponseDTO, error)
	DeleteExercise(ctx context.Context, id uuid.UUID) error

	// TestCase
	CreateTestCase(ctx context.Context, exerciseID uuid.UUID, req dto.CreateExerciseTestCaseDTO) (*dto.ExerciseTestCaseResponseDTO, error)
	GetTestCases(ctx context.Context, exerciseID uuid.UUID, includeHidden bool) ([]dto.ExerciseTestCaseResponseDTO, error)
	ImportTestCases(ctx context.Context, exerciseID uuid.UUID, req dto.ImportExerciseTestCasesDTO) ([]dto.ExerciseTestCaseResponseDTO, error)
	DeleteTestCase(ctx context.Context, exerciseID uuid.UUID, id uuid.UUID) error
	// UpdateTestCase sửa từng phần một test case của đúng bài tập; ErrExerciseTestCaseNotFound khi
	// test case không tồn tại hoặc thuộc bài tập khác.
	UpdateTestCase(ctx context.Context, exerciseID, id uuid.UUID, req dto.UpdateExerciseTestCaseDTO) (*dto.ExerciseTestCaseResponseDTO, error)

	// Submission — chấm đồng bộ qua POST /lessons/:lessonId/exercises/:id/submit (lesson_code_service.go).
	// GetSubmissions kèm danh tính học viên (student) cho người quản lý bài tập.
	GetSubmissions(ctx context.Context, exerciseID uuid.UUID, page, pageSize int) (*dto.ExerciseSubmissionListDTO, error)
	GetMySubmissions(ctx context.Context, exerciseID, userID uuid.UUID, page, pageSize int) (*dto.ExerciseSubmissionListDTO, error)
	// GetSubmissionByID trả ErrExerciseSubmissionNotFound cả khi bài nộp không thuộc người gọi và
	// người gọi không quản lý bài tập, để không dò được sự tồn tại của bài nộp (S2).
	GetSubmissionByID(ctx context.Context, id, requesterID uuid.UUID, isAdmin bool) (*dto.ExerciseSubmissionResponseDTO, error)

	// Progress
	UpdateContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID, req dto.UpdateContentProgressDTO) (*dto.ContentProgressResponseDTO, error)
	GetContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID) (*dto.ContentProgressResponseDTO, error)
}

type ExerciseService struct {
	judge        Judge0Client
	lessonAccess LessonContentServiceInterface
	repo         repository.ExerciseRepositoryInterface
	redis        *redis.Client
}

// NewExerciseService: tham số RabbitMQ không còn dùng — đường nộp cũ POST /exercises/:id/submit
// (đẩy vào hàng exercise.submit không có consumer nào) đã bị gỡ. Giữ chữ ký để các lane song song
// đang gọi NewExerciseService(repo, nil, nil) vẫn biên dịch được; bỏ tham số ở một PR dọn sau.
func NewExerciseService(
	repo repository.ExerciseRepositoryInterface,
	redis *redis.Client,
	_ *rabbitmq_queue.RabbitMQService,
) *ExerciseService {
	return &ExerciseService{repo: repo, redis: redis}
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

// ErrExerciseSubmissionNotFound: bài nộp không tồn tại HOẶC người gọi không được xem (gộp một lỗi).
var ErrExerciseSubmissionNotFound = errors.New("submission not found")

// ErrExerciseTestCaseNotFound: test case không tồn tại hoặc không thuộc bài tập trong đường dẫn.
var ErrExerciseTestCaseNotFound = errors.New("test case not found")

func (s *ExerciseService) CreateExercise(ctx context.Context, creatorID uuid.UUID, req dto.CreateExerciseDTO) (*dto.ExerciseResponseDTO, error) {
	exercise := &model.CourseExercise{
		CreatedBy:      &creatorID,
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

// CanManage: admin luôn được; còn lại xem ExerciseRepository.CanManage.
func (s *ExerciseService) CanManage(ctx context.Context, exerciseID, userID uuid.UUID, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	return s.repo.CanManage(ctx, exerciseID, userID)
}

// withoutHiddenTests trả bản sao của detail đã bỏ test case ẩn. Cache Redis lưu bản ĐẦY ĐỦ (chỉ
// server đọc), việc lọc theo người gọi diễn ra sau khi lấy cache — nếu lọc trước khi cache thì
// giảng viên sẽ nhận bản đã lọc từ cache của học viên.
func withoutHiddenTests(d *dto.ExerciseDetailDTO) *dto.ExerciseDetailDTO {
	visible := make([]dto.ExerciseTestCaseResponseDTO, 0, len(d.TestCases))
	for _, tc := range d.TestCases {
		if !tc.IsHidden {
			visible = append(visible, tc)
		}
	}
	out := *d
	out.TestCases = visible
	return &out
}

func (s *ExerciseService) GetExerciseByID(ctx context.Context, id uuid.UUID, includeHidden bool) (*dto.ExerciseDetailDTO, error) {
	full, err := s.getExerciseDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if includeHidden {
		return full, nil
	}
	return withoutHiddenTests(full), nil
}

func (s *ExerciseService) getExerciseDetail(ctx context.Context, id uuid.UUID) (*dto.ExerciseDetailDTO, error) {
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

func (s *ExerciseService) GetTestCases(ctx context.Context, exerciseID uuid.UUID, includeHidden bool) ([]dto.ExerciseTestCaseResponseDTO, error) {
	tcs, err := s.repo.GetTestCasesByExerciseID(ctx, exerciseID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.ExerciseTestCaseResponseDTO, 0, len(tcs))
	for i := range tcs {
		if tcs[i].IsHidden && !includeHidden {
			continue
		}
		result = append(result, *s.mapTestCaseToDTO(&tcs[i]))
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
	if err := s.repo.DeleteTestCase(ctx, exerciseID, id); err != nil {
		return err
	}
	s.invalidateExerciseCache(ctx, exerciseID)
	return nil
}

func (s *ExerciseService) UpdateTestCase(ctx context.Context, exerciseID, id uuid.UUID, req dto.UpdateExerciseTestCaseDTO) (*dto.ExerciseTestCaseResponseDTO, error) {
	tc, err := s.repo.GetTestCase(ctx, exerciseID, id)
	if err != nil {
		return nil, err
	}
	if tc == nil {
		return nil, ErrExerciseTestCaseNotFound
	}
	if req.Input != nil {
		tc.Input = *req.Input
	}
	if req.ExpectedOutput != nil {
		tc.ExpectedOutput = *req.ExpectedOutput
	}
	if req.IsHidden != nil {
		tc.IsHidden = *req.IsHidden
	}
	if req.DisplayOrder != nil {
		tc.DisplayOrder = *req.DisplayOrder
	}
	if err := s.repo.UpdateTestCase(ctx, tc); err != nil {
		return nil, err
	}
	s.invalidateExerciseCache(ctx, exerciseID)
	return s.mapTestCaseToDTO(tc), nil
}

// ============================================================================
// SUBMISSION
// ============================================================================

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
		data[i].Student = submissionStudent(&sub)
	}

	return &dto.ExerciseSubmissionListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

// submissionStudent: danh tính người nộp từ quan hệ User đã preload. full_name rơi về user_name khi
// tài khoản chưa đặt họ tên. Tài khoản đã xoá mềm (preload không nạp) vẫn trả id, tên/email rỗng.
func submissionStudent(sub *model.ExerciseSubmission) *dto.ExerciseSubmissionStudentDTO {
	st := &dto.ExerciseSubmissionStudentDTO{ID: sub.UserID}
	if sub.User == nil {
		return st
	}
	st.Email = sub.User.Email
	st.FullName = sub.User.UserName
	if sub.User.FullName != nil && *sub.User.FullName != "" {
		st.FullName = *sub.User.FullName
	}
	return st
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

func (s *ExerciseService) GetSubmissionByID(ctx context.Context, id, requesterID uuid.UUID, isAdmin bool) (*dto.ExerciseSubmissionResponseDTO, error) {
	sub, err := s.repo.GetSubmissionByID(ctx, id)
	if err != nil || sub == nil {
		return nil, ErrExerciseSubmissionNotFound
	}
	if sub.UserID != requesterID {
		allowed, err := s.CanManage(ctx, sub.ExerciseID, requesterID, isAdmin)
		if err != nil || !allowed {
			return nil, ErrExerciseSubmissionNotFound
		}
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
