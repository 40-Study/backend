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
	// GetAllQuizzes (SEC-1, vá lộ nội dung quiz): thêm userID/isAdmin — khi lọc ra quiz gắn với
	// một bài học (LessonID != nil), quiz của bài đang khoá đối với CHÍNH người gọi (chưa enroll,
	// hoặc sequential mà bài trước chưa xong) không được liệt kê, xem checkLessonQuizAccess.
	GetAllQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.QuizListDTO, error)
	// GetQuizByID (B-3, review vòng 2): userID/isAdmin quyết định is_correct/explanation có bị
	// giấu hay không — xem canViewQuizAnswerKey.
	GetQuizByID(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizDetailDTO, error)
	UpdateQuiz(ctx context.Context, id uuid.UUID, req dto.UpdateQuizDTO) (*dto.QuizResponseDTO, error)
	DeleteQuiz(ctx context.Context, id uuid.UUID) error
	DuplicateQuiz(ctx context.Context, id uuid.UUID) (*dto.QuizResponseDTO, error)

	// Questions
	CreateQuestion(ctx context.Context, quizID uuid.UUID, req dto.CreateQuestionDTO) (*dto.QuestionResponseDTO, error)
	// GetQuestionsByQuiz (B-3, review vòng 2): userID/isAdmin — xem GetQuizByID.
	GetQuestionsByQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuestionResponseDTO, error)
	UpdateQuestion(ctx context.Context, quizID, questionID uuid.UUID, req dto.UpdateQuestionDTO) (*dto.QuestionResponseDTO, error)
	DeleteQuestion(ctx context.Context, quizID, questionID uuid.UUID) error
	ReorderQuestions(ctx context.Context, quizID uuid.UUID, req dto.ReorderQuestionsDTO) error
	BulkCreateQuestions(ctx context.Context, quizID uuid.UUID, req dto.BulkCreateQuestionsDTO) ([]dto.QuestionResponseDTO, error)

	// Attempts
	// StartQuiz (Phase 1 §6): req.Mode "official" (mặc định) hoặc "practice" — practice không
	// tính vào quiz_max_attempts (xem CountAttemptsByUserAndQuiz, chỉ đếm attempt "official").
	StartQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.StartQuizDTO) (*dto.StartQuizResponseDTO, error)
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
	// courseRepo/sectionRepo/lessonRepo/livestreamRepo (B-3, review vòng 2): dùng để lần từ
	// quiz -> khoá học -> instructor, tính "ai được xem đáp án đúng trước khi nộp bài".
	courseRepo     repository.CourseRepositoryInterface
	sectionRepo    repository.SectionRepositoryInterface
	lessonRepo     repository.LessonRepositoryInterface
	livestreamRepo repository.LivestreamRepositoryInterface
	// enrollmentRepo (SEC-1, vá lộ nội dung quiz): dùng lại ĐÚNG helper khoá bài học mà
	// LessonContentService đang dùng (gatherLessonLockInput, lesson_lock.go) — xem
	// checkLessonQuizAccess.
	enrollmentRepo repository.EnrollmentRepositoryInterface
}

func NewQuizService(
	repo repository.QuizRepositoryInterface,
	redis *redis.Client,
	courseRepo repository.CourseRepositoryInterface,
	sectionRepo repository.SectionRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
	livestreamRepo repository.LivestreamRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
) *QuizService {
	return &QuizService{
		repo:           repo,
		redis:          redis,
		courseRepo:     courseRepo,
		sectionRepo:    sectionRepo,
		lessonRepo:     lessonRepo,
		livestreamRepo: livestreamRepo,
		enrollmentRepo: enrollmentRepo,
	}
}

// canViewQuizAnswerKey (B-3, review vòng 2): trả true khi userID là instructor sở hữu khoá học
// chứa quiz này, hoặc isAdmin — ba đường quiz có thể gắn vào (CourseID/LessonID/SessionID, xem
// model.Quiz), thử LẦN LƯỢT, dừng ở đường ĐẦU TIÊN xác định được course. Quiz không lần ra được
// course nào (dữ liệu hỏng/quiz mồ côi) mặc định GIẤU — fail-closed, không mở rộng quyền khi
// không chắc chắn.
func (s *QuizService) canViewQuizAnswerKey(ctx context.Context, quiz *model.Quiz, userID uuid.UUID, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}

	var courseID *uuid.UUID
	switch {
	case quiz.CourseID != nil:
		courseID = quiz.CourseID
	case quiz.LessonID != nil:
		lesson, err := s.lessonRepo.GetByID(ctx, *quiz.LessonID)
		if err != nil {
			return false, err
		}
		if lesson != nil {
			section, err := s.sectionRepo.GetByID(ctx, lesson.SectionID)
			if err != nil {
				return false, err
			}
			if section != nil {
				courseID = &section.CourseID
			}
		}
	case quiz.SessionID != nil:
		session, err := s.livestreamRepo.GetByID(ctx, *quiz.SessionID)
		if err != nil {
			return false, err
		}
		if session != nil {
			if session.HostID == userID {
				// Host cua chinh phien live nay — khong can tra courseRepo, quyet dinh luon.
				return true, nil
			}
			courseID = session.CourseID
		}
	}

	if courseID == nil {
		return false, nil
	}
	course, err := s.courseRepo.GetByID(ctx, *courseID)
	if err != nil {
		return false, err
	}
	return course != nil && course.InstructorID == userID, nil
}

// checkLessonQuizAccess (SEC-1, vá lộ nội dung quiz): quiz gắn LessonID mà người gọi KHÔNG phải
// chủ khoá học/giảng viên/admin (canView=false, xem canViewQuizAnswerKey) phải qua ĐÚNG luật khoá
// bài học mà LessonContentService.GetContentsByLessonID đang dùng (ResolveLessonLock +
// gatherLessonLockInput, lesson_lock.go) — KHÔNG viết lại luật quyền lần thứ hai. Trước bản vá
// này, GetQuizByID/GetQuestionsByQuiz chỉ ẩn is_correct/explanation qua canViewQuizAnswerKey,
// không hề kiểm enroll/lock — học viên chưa enroll (hoặc bài trước chưa hoàn thành ở khoá
// sequential) vẫn đọc được toàn bộ tiêu đề + text câu hỏi + phương án của bài đang khoá.
//
// canView=true (đã xác định là chủ khoá học/giảng viên/admin ở tầng gọi) bỏ qua hoàn toàn, khớp
// đúng hành vi hiện tại của canViewQuizAnswerKey (không bao giờ bị khoá) — không truy vấn
// enrollment/lesson-order thêm cho nhóm này.
func (s *QuizService) checkLessonQuizAccess(ctx context.Context, lessonID, userID uuid.UUID, canView bool) error {
	if canView {
		return nil
	}

	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return err
	}
	if lesson == nil {
		return errors.New("lesson not found")
	}
	// Luật 1 (contract Phase 1 §2, giống ResolveLessonLock): bài preview/miễn phí không bao giờ
	// bị khoá.
	if lesson.IsPreview {
		return nil
	}

	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lessonID)
	if err != nil {
		return err
	}
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return err
	}
	sequential := course != nil && course.Sequential

	// bypassLock=false: canView=true đã return sớm ở trên, nên tới đây chắc chắn người gọi không
	// phải chủ khoá học/admin.
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, courseID, sequential, false)
	if err != nil {
		return err
	}
	if err := EnsureLessonInCourse(lessonID, lockInput.LessonOrder); err != nil {
		return err
	}
	if locked, _, _ := ResolveLessonLock(lessonID, lockInput); locked {
		return ErrLessonLocked
	}
	return nil
}

// stripAnswerKey (B-3, review vòng 2): xoá is_correct/explanation khỏi MỘT bản sao của
// QuestionResponseDTO trước khi trả cho người xem không đủ quyền — gọi SAU khi map từ model,
// không sửa dữ liệu cache (xem GetQuizByID: cache lưu bản ĐẦY ĐỦ, strip áp dụng trên response
// cho TỪNG người xem, để một request của instructor sau đó vẫn đọc được cache đầy đủ).
func stripAnswerKey(q *dto.QuestionResponseDTO) dto.QuestionResponseDTO {
	out := *q
	out.Explanation = nil
	out.Answers = make([]dto.AnswerResponseDTO, len(q.Answers))
	for i, a := range q.Answers {
		a.IsCorrect = nil
		out.Answers[i] = a
	}
	return out
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

func (s *QuizService) GetAllQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.QuizListDTO, error) {
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

	// SEC-1 (vá lộ nội dung quiz): mỗi quiz gắn LessonID phải qua đúng luật khoá bài học
	// (checkLessonQuizAccess) như GetQuizByID/GetQuestionsByQuiz — quiz của bài đang khoá đối với
	// CHÍNH người gọi bị LOẠI KHỎI danh sách (không phải lỗi cả request), khớp yêu cầu "người
	// không có quyền xem khoá đó thì không liệt kê quiz của nó". `total` vẫn là số đếm THÔ từ
	// repo (không trừ phần bị lọc) — chấp nhận được vì GetQuizzesByLesson (đường web thật sự
	// dùng) chỉ đọc `data`, không đọc `total`; đây là giới hạn đã biết, không phải bug ẩn.
	data := make([]dto.QuizResponseDTO, 0, len(quizzes))
	for i := range quizzes {
		q := &quizzes[i]
		if q.LessonID != nil {
			canView, err := s.canViewQuizAnswerKey(ctx, q, userID, isAdmin)
			if err != nil {
				return nil, err
			}
			if err := s.checkLessonQuizAccess(ctx, *q.LessonID, userID, canView); err != nil {
				if err == ErrLessonLocked || err == ErrLessonNotInCourse {
					continue
				}
				return nil, err
			}
		}
		data = append(data, *s.mapQuizToDTO(q, 0))
	}

	return &dto.QuizListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *QuizService) GetQuizByID(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizDetailDTO, error) {
	var quiz *model.Quiz
	var result *dto.QuizDetailDTO

	// Check cache — B-3 (review vòng 2): cache CHỨA BẢN ĐẦY ĐỦ (kể cả is_correct/explanation),
	// chưa lọc theo người xem. Strip luôn diễn ra SAU đoạn cache/DB này, trên một BẢN SAO, nên
	// một request của instructor sau đó vẫn đọc được đúng dữ liệu đầy đủ từ cache.
	if s.redis != nil {
		cacheKey := quizCachePrefix + id.String()
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedResult dto.QuizDetailDTO
			if json.Unmarshal([]byte(cached), &cachedResult) == nil {
				result = &cachedResult
			}
		}
	}

	if result == nil {
		var err error
		quiz, err = s.repo.GetQuizWithQuestions(ctx, id)
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

		result = &dto.QuizDetailDTO{
			QuizResponseDTO: *s.mapQuizToDTO(quiz, len(quiz.Questions)),
			Questions:       questions,
		}

		// Cache — luôn cache bản ĐẦY ĐỦ (chưa strip).
		if s.redis != nil {
			if data, err := json.Marshal(result); err == nil {
				s.redis.Set(ctx, quizCachePrefix+id.String(), data, quizCacheTTL)
			}
		}
	} else {
		// Đường cache-hit không nạp lại model.Quiz — cần nạp riêng để canViewQuizAnswerKey lần
		// ra course (CourseID/LessonID/SessionID không đổi theo cache nên tra DB nhẹ, không phá
		// mục đích cache — mục đích cache ở đây là tránh JOIN questions+answers, không phải
		// tránh MỌI truy vấn).
		fetched, err := s.repo.GetQuizByID(ctx, id)
		if err != nil {
			return nil, err
		}
		quiz = fetched
	}

	canView := isAdmin
	if quiz != nil {
		var err error
		canView, err = s.canViewQuizAnswerKey(ctx, quiz, userID, isAdmin)
		if err != nil {
			return nil, err
		}
		// SEC-1 (vá lộ nội dung quiz): trước bản vá này, canView=false chỉ dẫn tới STRIP đáp án
		// đúng bên dưới — toàn bộ tiêu đề/mô tả/text câu hỏi/phương án vẫn trả về 200 cho người
		// chưa enroll (hoặc bài trước chưa xong ở khoá sequential). checkLessonQuizAccess trả
		// ErrLessonLocked cho trường hợp đó — handler ánh xạ sang 403 {message:"LESSON_LOCKED"},
		// giống hệt LessonContentHandler.GetContent.
		if quiz.LessonID != nil {
			if err := s.checkLessonQuizAccess(ctx, *quiz.LessonID, userID, canView); err != nil {
				return nil, err
			}
		}
	}
	if !canView {
		strippedQuestions := make([]dto.QuestionResponseDTO, len(result.Questions))
		for i, q := range result.Questions {
			strippedQuestions[i] = stripAnswerKey(&q)
		}
		strippedResult := *result
		strippedResult.Questions = strippedQuestions
		return &strippedResult, nil
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

func (s *QuizService) GetQuestionsByQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuestionResponseDTO, error) {
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}
	canView, err := s.canViewQuizAnswerKey(ctx, quiz, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	// SEC-1 (vá lộ nội dung quiz): xem chú thích tại GetQuizByID — trước bản vá này hàm này CHỈ
	// strip is_correct/explanation, không hề kiểm bài có đang khoá với userID hay không.
	if quiz.LessonID != nil {
		if err := s.checkLessonQuizAccess(ctx, *quiz.LessonID, userID, canView); err != nil {
			return nil, err
		}
	}

	questions, err := s.repo.GetQuestionsByQuizID(ctx, quizID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.QuestionResponseDTO, len(questions))
	for i, q := range questions {
		mapped := *s.mapQuestionToDTO(&q)
		if !canView {
			mapped = stripAnswerKey(&mapped)
		}
		result[i] = mapped
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

func (s *QuizService) StartQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.StartQuizDTO) (*dto.StartQuizResponseDTO, error) {
	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	// SEC-1: /start trả về toàn bộ text câu hỏi + phương án, nên phải qua CÙNG cổng khoá bài học
	// với GetQuizByID/GetQuestionsByQuiz — nếu không, đây là lối vòng qua bản vá của hai endpoint
	// đọc kia. Chặn trước CreateAttempt để không sinh attempt rác cho bài đang khoá.
	if quiz.LessonID != nil {
		canView, err := s.canViewQuizAnswerKey(ctx, quiz, userID, isAdmin)
		if err != nil {
			return nil, err
		}
		if err := s.checkLessonQuizAccess(ctx, *quiz.LessonID, userID, canView); err != nil {
			return nil, err
		}
	}

	mode := req.Mode
	if mode == "" {
		mode = "official"
	}

	// Check max attempts — CHỈ đếm attempt "official" (contract §6: practice "không đếm vào
	// quiz_max_attempts"). CountAttemptsByUserAndQuiz đã tự lọc mode='official' ở tầng SQL.
	if quiz.MaxAttempts != nil && mode == "official" {
		count, _ := s.repo.CountAttemptsByUserAndQuiz(ctx, userID, quizID)
		if count >= int64(*quiz.MaxAttempts) {
			return nil, errors.New("max attempts reached")
		}
	}

	attempt := &model.QuizAttempt{
		UserID:    userID,
		QuizID:    quizID,
		Mode:      mode,
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
		Mode:          attempt.Mode,
	}, nil
}

func (s *QuizService) SubmitQuiz(ctx context.Context, quizID, userID uuid.UUID, req dto.SubmitQuizDTO) (*dto.QuizAttemptResponseDTO, error) {
	// Find the in-progress attempt to submit.
	attempts, err := s.repo.GetAttemptsByUserAndQuiz(ctx, userID, quizID)
	if err != nil {
		return nil, err
	}

	var attempt *model.QuizAttempt
	if req.AttemptID != nil && *req.AttemptID != "" {
		// CAO-6 (review vòng 2): client gửi kèm attempt_id (nhận từ POST /start) → nộp ĐÚNG
		// attempt đó, tránh mơ hồ khi có cả attempt "official" VÀ "practice" cùng dang dở cho
		// cùng quiz. attempt_id sai định dạng, không thuộc user này, không thuộc quiz này, hoặc
		// đã nộp rồi đều là lỗi rõ ràng — KHÔNG âm thầm rơi về "chọn đại một attempt khác".
		attemptID, parseErr := uuid.Parse(*req.AttemptID)
		if parseErr != nil {
			return nil, errors.New("invalid attempt_id")
		}
		for i := range attempts {
			if attempts[i].ID == attemptID {
				attempt = &attempts[i]
				break
			}
		}
		if attempt == nil {
			return nil, errors.New("attempt not found for this user/quiz")
		}
		if attempt.CompletedAt != nil {
			return nil, errors.New("attempt already submitted")
		}
	} else {
		// Client cũ (chưa gửi attempt_id): giữ hành vi cũ — chọn attempt DANG DỞ mới nhất
		// (attempts đã sắp created_at DESC), bất kể mode.
		for i := range attempts {
			if attempts[i].CompletedAt == nil {
				attempt = &attempts[i]
				break
			}
		}
		if attempt == nil {
			return nil, errors.New("no active attempt found")
		}
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

	// Phase 1 §6: "explanation chỉ trả sau khi nộp" — gate CẢ correct_answer_ids theo cùng điều
	// kiện cho nhất quán (contract không nói rõ, nhưng để lộ đáp án đúng trong lúc attempt còn
	// đang làm dở thì cũng phá gate y hệt explanation).
	submitted := attempt.CompletedAt != nil

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
		}
		if submitted {
			answers[i].Explanation = aa.Question.Explanation
			var correctIDs []string
			for _, qa := range aa.Question.Answers {
				if qa.IsCorrect {
					correctIDs = append(correctIDs, qa.ID.String())
				}
			}
			answers[i].CorrectAnswerIDs = correctIDs
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
		isCorrect := a.IsCorrect
		answers[i] = dto.AnswerResponseDTO{
			ID:           a.ID,
			AnswerText:   a.AnswerText,
			IsCorrect:    &isCorrect,
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
