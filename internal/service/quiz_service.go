package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrQuizAttemptAlreadySubmitted (R8, review 260919): attempt đã completed_at != nil khi
// SubmitQuiz cố ghi kết quả — do double-click, retry sau timeout, hoặc client gửi lại request
// cũ. Handler ánh xạ sang 409 Conflict (xem quiz_handler.go), khác 400 chung chung của các lỗi
// validate khác — khớp quy ước sentinel error của package này (auth_service.go, order_service.go,
// payment_service.go).
var ErrQuizAttemptAlreadySubmitted = errors.New("quiz attempt already submitted")

// Khoá quiz theo cuộc thi (contract "Cuộc thi" §3.2): mọi method đọc/ghi dưới đây nhận thêm
// userID/isAdmin để hỏi ContestQuizGate. Quiz gắn cuộc thi trả ErrQuizLockedByContest cho người
// không phải chủ cuộc thi/admin (403), và ErrQuizEditLockedByContest cho MỌI thao tác sửa khi cuộc
// thi đang chờ duyệt/đã công bố/đã huỷ (409). Gate nil (trước khi lane B1 nối ContestService) =
// không quiz nào bị khoá.
type QuizServiceInterface interface {
	// CreateQuiz: userID là người gọi, ghi vào quizzes.created_by.
	CreateQuiz(ctx context.Context, userID uuid.UUID, req dto.CreateQuizDTO) (*dto.QuizResponseDTO, error)
	// GetAllQuizzes (SEC-1, vá lộ nội dung quiz): thêm userID/isAdmin — khi lọc ra quiz gắn với
	// một bài học (LessonID != nil), quiz của bài đang khoá đối với CHÍNH người gọi (chưa enroll,
	// hoặc sequential mà bài trước chưa xong) không được liệt kê, xem checkLessonQuizAccess.
	GetAllQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.QuizListDTO, error)
	// GetQuizByID (B-3, review vòng 2): userID/isAdmin quyết định is_correct/explanation có bị
	// giấu hay không — xem canViewQuizAnswerKey.
	GetQuizByID(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizDetailDTO, error)
	UpdateQuiz(ctx context.Context, id, userID uuid.UUID, isAdmin bool, req dto.UpdateQuizDTO) (*dto.QuizResponseDTO, error)
	DeleteQuiz(ctx context.Context, id, userID uuid.UUID, isAdmin bool) error
	// DuplicateQuiz: bản sao thuộc về người gọi (created_by = userID).
	DuplicateQuiz(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizResponseDTO, error)

	// Questions
	CreateQuestion(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.CreateQuestionDTO) (*dto.QuestionResponseDTO, error)
	// GetQuestionsByQuiz (B-3, review vòng 2): userID/isAdmin — xem GetQuizByID.
	GetQuestionsByQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuestionResponseDTO, error)
	UpdateQuestion(ctx context.Context, quizID, questionID, userID uuid.UUID, isAdmin bool, req dto.UpdateQuestionDTO) (*dto.QuestionResponseDTO, error)
	DeleteQuestion(ctx context.Context, quizID, questionID, userID uuid.UUID, isAdmin bool) error
	ReorderQuestions(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.ReorderQuestionsDTO) error
	BulkCreateQuestions(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.BulkCreateQuestionsDTO) ([]dto.QuestionResponseDTO, error)

	// Attempts
	// StartQuiz (Phase 1 §6): req.Mode "official" (mặc định) hoặc "practice" — practice không
	// tính vào quiz_max_attempts (xem CountAttemptsByUserAndQuiz, chỉ đếm attempt "official").
	StartQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.StartQuizDTO) (*dto.StartQuizResponseDTO, error)
	SubmitQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.SubmitQuizDTO) (*dto.QuizAttemptResponseDTO, error)
	GetMyAttempts(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuizAttemptResponseDTO, error)
	GetAttemptByID(ctx context.Context, attemptID, userID uuid.UUID, isAdmin bool) (*dto.QuizAttemptDetailDTO, error)
	GetQuizResults(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) (*dto.QuizResultsDTO, error)
	GetQuizStatistics(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) (*dto.QuizStatisticsDTO, error)
	SaveAnswer(ctx context.Context, attemptID, userID uuid.UUID, isAdmin bool, req dto.SaveAnswerDTO) error
	GetAttemptProgress(ctx context.Context, attemptID, userID uuid.UUID, isAdmin bool) (*dto.QuizAttemptDetailDTO, error)
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
	// contestGate (contract "Cuộc thi" §3.2): ContestService của lane B1, nối qua SetContestGate.
	contestGate ContestQuizGate
	// parentLinks (S2): cho phụ huynh đã liên kết active xem bài làm của con. nil = không phụ huynh
	// nào được xem (fail-closed) — nối qua SetParentLinkChecker.
	parentLinks AttemptParentLinkChecker
}

// AttemptParentLinkChecker trả true khi parentID đang là phụ huynh có liên kết ACTIVE của studentID.
type AttemptParentLinkChecker interface {
	HasActiveParent(ctx context.Context, parentID, studentID uuid.UUID) (bool, error)
}

// SetParentLinkChecker nối kiểm tra liên kết phụ huynh-học viên (S2).
func (s *QuizService) SetParentLinkChecker(c AttemptParentLinkChecker) {
	s.parentLinks = c
}

// ErrQuizAttemptNotFound gộp "bài làm không tồn tại" và "người gọi không được xem" thành MỘT lỗi
// để handler trả 404 cho cả hai — không cho dò được sự tồn tại của bài làm người khác (S2).
var ErrQuizAttemptNotFound = errors.New("attempt not found")

// ErrQuizResultsNotFound: người gọi không quản lý quiz nên không được xem kết quả/thống kê của cả
// quiz; handler trả 404 để không lộ quiz có bài làm hay không (S2).
var ErrQuizResultsNotFound = errors.New("quiz not found")

// ErrQuizNotFound: quiz không tồn tại (repo trả nil, nil). Phải là sentinel để handler (qua
// respondQuizGateError) ánh xạ 404 — errors.New trần rơi vào nhánh 500 ở GET /quizzes/:id/attempts (S7).
var ErrQuizNotFound = errors.New("quiz not found")

// canViewOthersAttempt (S2): ai được xem bài làm CỦA NGƯỜI KHÁC — admin, người quản lý quiz (người
// tạo hoặc giảng viên chủ khoá chứa quiz), hoặc phụ huynh đã liên kết active với chủ bài làm.
// Chính chủ bài làm không đi qua hàm này. Lỗi tra cứu coi như không có quyền (fail-closed).
func (s *QuizService) canViewOthersAttempt(ctx context.Context, quizID, ownerID, viewerID uuid.UUID, isAdmin bool) bool {
	if isAdmin {
		return true
	}
	if err := s.checkQuizOwner(ctx, quizID, viewerID, false); err == nil {
		return true
	}
	if s.parentLinks != nil {
		linked, err := s.parentLinks.HasActiveParent(ctx, viewerID, ownerID)
		return err == nil && linked
	}
	return false
}

// checkQuizResultsManager: kết quả/thống kê của CẢ quiz chỉ dành cho người quản lý quiz và admin.
func (s *QuizService) checkQuizResultsManager(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if err := s.checkQuizOwner(ctx, quizID, userID, isAdmin); err != nil {
		return ErrQuizResultsNotFound
	}
	return nil
}

// SetContestGate nối cổng khoá quiz theo cuộc thi. Setter thay vì tham số constructor vì
// ContestService lại cần chính QuizService (ContestQuizEngine) — hai phía phụ thuộc lẫn nhau nên
// phải tạo QuizService trước rồi mới gắn gate. Gate nil chỉ hợp lệ trước khi lane B1 merge.
func (s *QuizService) SetContestGate(g ContestQuizGate) {
	s.contestGate = g
}

// checkContestAccess: quiz gắn cuộc thi chỉ người tạo cuộc thi và admin được đụng tới (§3.2).
func (s *QuizService) checkContestAccess(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if s.contestGate == nil {
		return nil
	}
	return s.contestGate.CheckQuizAccess(ctx, quizID, userID, isAdmin)
}

// ErrQuizNotOwner (review PR #80, F2): sửa/xoá/nhân bản quiz hoặc câu hỏi của quiz mà người gọi
// không tạo ra. Handler ánh xạ sang 403 QUIZ_FORBIDDEN.
var ErrQuizNotOwner = errors.New("only the quiz creator or an admin can modify this quiz")

// checkQuizOwner: chỉ người tạo quiz (created_by), giảng viên chủ khoá chứa quiz, hoặc admin được
// sửa, xoá, nhân bản quiz và câu hỏi của nó. Trước bản vá, mọi tài khoản đăng nhập đều sửa được quiz của người khác, và nhân bản
// quiz của giảng viên khác rồi làm bài trên bản sao là đọc được đáp án trước khi quiz gốc được gắn
// vào cuộc thi. Quiz tạo trước khi có cột created_by (NULL) không có chủ xác định nên chỉ admin
// được sửa — chủ dự án chốt, không để thành "vô chủ ai cũng sửa".
func (s *QuizService) checkQuizOwner(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil {
		return err
	}
	if quiz == nil {
		return ErrQuizNotFound
	}
	if quiz.CreatedBy != nil && *quiz.CreatedBy == userID {
		return nil
	}
	// Re-review vòng 2, R2-C (chủ dự án chốt 29/09): giảng viên chủ khoá sửa được MỌI quiz thuộc
	// khoá của mình (suy từ bài học hoặc khoá học), kể cả quiz do admin hay người khác tạo.
	isInstructor, err := s.isQuizCourseInstructor(ctx, quiz, userID)
	if err != nil {
		return err
	}
	if isInstructor {
		return nil
	}
	return ErrQuizNotOwner
}

// checkQuizMutable: thứ tự kiểm cho mọi thao tác sửa. (1) Khoá cuộc thi: người ngoài nhận 403 như
// mọi route khác, không để lộ trạng thái cuộc thi qua mã 409. (2) Chủ sở hữu. (3) Cuộc thi đang
// chờ duyệt/đã công bố/đã huỷ thì không ai sửa được, kể cả admin (409).
func (s *QuizService) checkQuizMutable(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if err := s.CheckQuizEditAccess(ctx, quizID, userID, isAdmin); err != nil {
		return err
	}
	if s.contestGate == nil {
		return nil
	}
	return s.contestGate.CheckQuizEditable(ctx, quizID)
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
	// Review PR #80, F3: người tạo quiz luôn xem được đáp án của chính mình. Quiz standalone (loại
	// gắn vào cuộc thi) không lần ra khoá học nào nên trước đây bị giấu cả với người tạo. Không mở
	// thêm đường lộ: thí sinh cuộc thi đã bị gate cuộc thi chặn trước khi tới đây.
	if quiz.CreatedBy != nil && *quiz.CreatedBy == userID {
		return true, nil
	}

	var courseID *uuid.UUID
	switch {
	case quiz.CourseID != nil || quiz.LessonID != nil:
		id, err := s.quizCourseID(ctx, quiz)
		if err != nil {
			return false, err
		}
		courseID = id
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

// quizCourseID: khoá học chứa quiz, suy từ course_id hoặc lesson → section → course (cùng thứ tự
// canViewQuizAnswerKey vẫn dùng). nil = quiz không thuộc khoá nào (standalone, live) hoặc dữ liệu
// hỏng.
func (s *QuizService) quizCourseID(ctx context.Context, quiz *model.Quiz) (*uuid.UUID, error) {
	if quiz.CourseID != nil {
		return quiz.CourseID, nil
	}
	if quiz.LessonID == nil {
		return nil, nil
	}
	lesson, err := s.lessonRepo.GetByID(ctx, *quiz.LessonID)
	if err != nil || lesson == nil {
		return nil, err
	}
	section, err := s.sectionRepo.GetByID(ctx, lesson.SectionID)
	if err != nil || section == nil {
		return nil, err
	}
	return &section.CourseID, nil
}

// isQuizCourseInstructor (re-review vòng 2, R2-C): userID là giảng viên của khoá chứa quiz. Khoá
// "chuẩn" theo đúng luật #79 (quiz_course_guard.go): khoá suy từ lesson_id nếu có, ngược lại
// course_id — để luật sửa quiz không lệch với guard tạo quiz khi dữ liệu cũ có hai khoá khác nhau.
func (s *QuizService) isQuizCourseInstructor(ctx context.Context, quiz *model.Quiz, userID uuid.UUID) (bool, error) {
	course, err := s.lessonCourse(ctx, quiz.LessonID)
	if err != nil {
		return false, err
	}
	if course == nil && quiz.CourseID != nil {
		if course, err = s.courseRepo.GetByID(ctx, *quiz.CourseID); err != nil {
			return false, err
		}
	}
	return course != nil && course.InstructorID == userID, nil
}

// CheckQuizEditAccess: phần "ai được sửa" của checkQuizMutable (khoá cuộc thi 403, rồi chủ sở hữu
// 403), không gồm các khoá theo trạng thái (409). Middleware CourseEditLock của #79 gọi hàm này
// TRƯỚC khoá "khoá học đang chờ duyệt", để người ngoài luôn nhận 403 như mọi route quiz khác, không
// biết được trạng thái khoá học qua mã 409.
func (s *QuizService) CheckQuizEditAccess(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return err
	}
	return s.checkQuizOwner(ctx, quizID, userID, isAdmin)
}

// isStandaloneQuiz: quiz không gắn bài học, khoá học hay buổi live — loại quiz dùng cho cuộc thi.
func isStandaloneQuiz(quiz *model.Quiz) bool {
	return quiz.LessonID == nil && quiz.CourseID == nil && quiz.SessionID == nil
}

// checkStandaloneQuizReader (re-review vòng 2, R2-A): quiz standalone chỉ người tạo hoặc admin được
// đọc, làm bài, nộp và xem attempt. Trước bản vá, quiz standalone CHƯA gắn cuộc thi không có kiểm
// quyền nào: người lạ start + submit rồi đọc attempt là thấy correct_answer_ids, và attempt của họ
// khiến quiz không gắn được vào cuộc thi nữa (§3.3 yêu cầu chưa có attempt). Quiz ĐÃ gắn cuộc thi
// vẫn do gate cuộc thi quyết định (gọi trước hàm này). Quiz bài học/khoá học/live giữ luật cũ.
func (s *QuizService) checkStandaloneQuizReader(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil {
		return err
	}
	if quiz == nil {
		return ErrQuizNotFound
	}
	if !isStandaloneQuiz(quiz) || (quiz.CreatedBy != nil && *quiz.CreatedBy == userID) {
		return nil
	}
	return ErrQuizNotOwner
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

// checkCourseQuizAccess (R4, review 260919 — "Gate quiz #65 chỉ áp cho quiz có lesson_id"): quiz
// gắn THẲNG course_id (không qua lesson) — không có khái niệm "bài trước" nên không áp luật
// sequential/lesson-order như checkLessonQuizAccess, chỉ cần đã enroll khoá học chứa quiz.
// canView=true (chủ khoá học/giảng viên/admin, xem canViewQuizAnswerKey) bỏ qua hoàn toàn, khớp
// đúng các gate quiz khác trong file này. Trả ErrLessonLocked (không phải lỗi mới) để handler
// dùng NGUYÊN respondLessonLockError hiện có — 403 {message:"LESSON_LOCKED"}, cùng format với
// gate lesson.
func (s *QuizService) checkCourseQuizAccess(ctx context.Context, courseID, userID uuid.UUID, canView bool) error {
	if canView {
		return nil
	}
	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return ErrLessonLocked
	}
	return nil
}

// checkSessionQuizAccess (R4, review 260919): quiz gắn THẲNG session_id (quiz live trong buổi
// học trực tuyến) — quyền xem là "người tham dự (host/participant) buổi live NÀY, hoặc đã enroll
// khoá học chứa buổi live", đúng yêu cầu review R4. KHÔNG tái dùng
// LivestreamService.EnsureSessionMember: hàm đó còn xét quan hệ LỚP qua classRepo/participantRepo
// cho chat/bảng trắng (phạm vi rộng hơn nhu cầu ở đây) và QuizService không giữ 2 repo đó.
// canView=true bỏ qua hoàn toàn, cùng lý do với hai gate trên. Trả ErrLessonLocked — cùng format
// 403 với gate lesson/course.
func (s *QuizService) checkSessionQuizAccess(ctx context.Context, sessionID, userID uuid.UUID, canView bool) error {
	if canView {
		return nil
	}
	session, err := s.livestreamRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}
	if session.HostID == userID {
		return nil
	}
	for _, p := range session.Participants {
		if p.UserID == userID {
			return nil
		}
	}
	if session.CourseID != nil {
		enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, *session.CourseID)
		if err != nil {
			return err
		}
		if enrollment != nil {
			return nil
		}
	}
	return ErrLessonLocked
}

// checkQuizAccess (R4, review 260919): gate DUY NHẤT cho MỌI quiz bất kể gắn vào lesson/course/
// session — tránh viết lặp lại nhánh if ở GetQuizByID/GetQuestionsByQuiz/StartQuiz/
// GetAllQuizzes. Trước bản vá R4, cả 4 nơi trên CHỈ kiểm tra khi quiz.LessonID != nil, để lộ
// toàn bộ quiz gắn course_id/session_id cho người chưa enroll (xem review 260919, mục R4).
// canView=true (chủ khoá học/giảng viên/admin) bỏ qua hoàn toàn ở TẤT CẢ nhánh — giữ đúng hành
// vi hiện có. Quiz không gắn lesson/course/session nào (standalone) chỉ người tạo/admin được đọc.
func (s *QuizService) checkQuizAccess(ctx context.Context, quiz *model.Quiz, userID uuid.UUID, canView bool) error {
	// D4 (review PR #79): quiz của khoá chưa xuất bản ẩn với người ngoài — xem quiz_course_guard.go.
	if !canView {
		if err := s.ensureQuizCourseVisible(ctx, quiz, userID); err != nil {
			return err
		}
	}
	switch {
	case quiz.LessonID != nil:
		return s.checkLessonQuizAccess(ctx, *quiz.LessonID, userID, canView)
	case quiz.CourseID != nil:
		return s.checkCourseQuizAccess(ctx, *quiz.CourseID, userID, canView)
	case quiz.SessionID != nil:
		return s.checkSessionQuizAccess(ctx, *quiz.SessionID, userID, canView)
	default:
		// Re-review vòng 2, R2-A: quiz standalone (loại dùng cho cuộc thi) chỉ người tạo và admin
		// được đụng tới — canView đã đúng bằng "người tạo hoặc admin" với quiz không thuộc khoá nào.
		// Trước đây nhánh này trả nil cho mọi người, xem checkStandaloneQuizReader.
		if canView {
			return nil
		}
		return ErrQuizNotOwner
	}
}

// stripAnswerKey (B-3, review vòng 2): xoá is_correct/explanation khỏi MỘT bản sao của
// QuestionResponseDTO trước khi trả cho người xem không đủ quyền — gọi SAU khi map từ model,
// không sửa dữ liệu cache (xem GetQuizByID: cache lưu bản ĐẦY ĐỦ, strip áp dụng trên response
// cho TỪNG người xem, để một request của instructor sau đó vẫn đọc được cache đầy đủ).
func stripAnswerKey(q *dto.QuestionResponseDTO) dto.QuestionResponseDTO {
	out := *q
	out.Explanation = nil
	// Review PR #80, F1: với câu mà "lựa chọn" chính là đáp án (fill_blank, essay) thì giấu
	// is_correct là chưa đủ, answer_text đã là đáp án — bỏ cả danh sách.
	if answerOptionsRevealKey(q.QuestionType) {
		out.Answers = []dto.AnswerResponseDTO{}
		return out
	}
	out.Answers = make([]dto.AnswerResponseDTO, len(q.Answers))
	for i, a := range q.Answers {
		a.IsCorrect = nil
		out.Answers[i] = a
	}
	return out
}

// answerOptionsRevealKey (review PR #80, F1): fill_blank chấm bằng cách so text_answer với
// answer_text của các lựa chọn is_correct (checkAnswer), nên danh sách lựa chọn CHÍNH LÀ đáp án;
// essay nếu có lựa chọn thì đó là đáp án mẫu. Với trắc nghiệm (single/multiple/true_false) các lựa
// chọn là phương án, trả ra được miễn là giấu is_correct.
func answerOptionsRevealKey(questionType string) bool {
	return questionType == "fill_blank" || questionType == "essay"
}

// toAttemptQuestion map câu hỏi sang dạng đề làm bài, KHÔNG kèm đáp án: dùng chung cho StartQuiz và
// đề thi (GetContestAttemptQuestions) để hai đường không lệch nhau.
func toAttemptQuestion(q *model.Question) dto.AttemptQuestionDTO {
	answers := []dto.AttemptAnswerDTO{}
	if !answerOptionsRevealKey(q.QuestionType) {
		answers = make([]dto.AttemptAnswerDTO, len(q.Answers))
		for j, a := range q.Answers {
			answers[j] = dto.AttemptAnswerDTO{ID: a.ID, AnswerText: a.AnswerText, DisplayOrder: a.DisplayOrder}
		}
	}
	return dto.AttemptQuestionDTO{
		ID:           q.ID,
		QuestionText: q.QuestionText,
		QuestionType: q.QuestionType,
		Points:       q.Points,
		DisplayOrder: q.DisplayOrder,
		ImageURL:     q.ImageURL,
		Answers:      answers,
	}
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

func (s *QuizService) CreateQuiz(ctx context.Context, userID uuid.UUID, req dto.CreateQuizDTO) (*dto.QuizResponseDTO, error) {
	quiz := &model.Quiz{
		Title:       req.Title,
		TriggerType: "manual",
		CreatedBy:   &userID,
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

	// Quiz vừa tạo chưa có câu hỏi nào (câu hỏi thêm qua endpoint riêng): 0 là số THẬT, không phải mặc định.
	return s.mapQuizToDTO(quiz, 0), nil
}

// questionCounts (QA T10): số câu hỏi chưa xoá của từng quiz, một truy vấn cho cả danh sách. Quiz không có
// câu hỏi vắng trong map nên map[id] = 0 là đúng; lỗi truy vấn trả ra, KHÔNG lặng lẽ rơi về 0 (đúng cái lỗi T10).
func (s *QuizService) questionCounts(ctx context.Context, quizzes []*model.Quiz) (map[uuid.UUID]int, error) {
	ids := make([]uuid.UUID, len(quizzes))
	for i, q := range quizzes {
		ids[i] = q.ID
	}
	return s.repo.CountQuestionsByQuizIDs(ctx, ids)
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

	// SEC-1 (vá lộ nội dung quiz) + R4 (review 260919): MỌI quiz — gắn lesson_id, course_id, hay
	// session_id — phải qua đúng luật khoá tương ứng (checkQuizAccess) như
	// GetQuizByID/GetQuestionsByQuiz/StartQuiz — quiz đang khoá đối với CHÍNH người gọi bị LOẠI
	// KHỎI danh sách (không phải lỗi cả request), khớp yêu cầu "người không có quyền xem khoá đó
	// thì không liệt kê quiz của nó". Trước bản vá R4, nhánh này CHỈ chạy khi q.LessonID != nil —
	// quiz gắn course_id/session_id bỏ qua hoàn toàn, lộ cho người chưa enroll. `total` vẫn là số
	// đếm THÔ từ repo (không trừ phần bị lọc) — chấp nhận được vì GetQuizzesByLesson (đường web
	// thật sự dùng) chỉ đọc `data`, không đọc `total`; đây là giới hạn đã biết, không phải bug ẩn.
	visible := make([]*model.Quiz, 0, len(quizzes))
	for i := range quizzes {
		q := &quizzes[i]
		// Contract "Cuộc thi" §3.2: quiz gắn cuộc thi bị loại khỏi danh sách với người không phải
		// chủ cuộc thi/admin — cùng cách xử lý quiz của bài đang khoá bên dưới.
		if err := s.checkContestAccess(ctx, q.ID, userID, isAdmin); err != nil {
			if errors.Is(err, ErrQuizLockedByContest) {
				continue
			}
			return nil, err
		}
		canView, err := s.canViewQuizAnswerKey(ctx, q, userID, isAdmin)
		if err != nil {
			return nil, err
		}
		if err := s.checkQuizAccess(ctx, q, userID, canView); err != nil {
			// ErrCourseHidden: quiz của khoá chưa xuất bản (#79, D4). ErrQuizNotOwner: quiz standalone
			// của người khác (PR #80, R2-A). Cả hai đều loại khỏi danh sách, không làm hỏng cả trang.
			if err == ErrLessonLocked || err == ErrLessonNotInCourse || err == ErrCourseHidden || err == ErrQuizNotOwner {
				continue
			}
			return nil, err
		}
		visible = append(visible, q)
	}

	counts, err := s.questionCounts(ctx, visible)
	if err != nil {
		return nil, err
	}
	data := make([]dto.QuizResponseDTO, len(visible))
	for i, q := range visible {
		data[i] = *s.mapQuizToDTO(q, counts[q.ID])
	}

	return &dto.QuizListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *QuizService) GetQuizByID(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizDetailDTO, error) {
	// Kiểm TRƯỚC cache: bản cache chứa đầy đủ đáp án, không được trả cho thí sinh cuộc thi.
	if err := s.checkContestAccess(ctx, id, userID, isAdmin); err != nil {
		return nil, err
	}
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
		// SEC-1 (vá lộ nội dung quiz) + R4 (review 260919): trước bản vá SEC-1, canView=false chỉ
		// dẫn tới STRIP đáp án đúng bên dưới — toàn bộ tiêu đề/mô tả/text câu hỏi/phương án vẫn
		// trả về 200 cho người chưa enroll (hoặc bài trước chưa xong ở khoá sequential).
		// checkQuizAccess trả ErrLessonLocked cho trường hợp đó (bất kể quiz gắn lesson/course/
		// session) — handler ánh xạ sang 403 {message:"LESSON_LOCKED"}, giống hệt
		// LessonContentHandler.GetContent. Trước bản vá R4, gate này CHỈ chạy khi
		// quiz.LessonID != nil.
		if err := s.checkQuizAccess(ctx, quiz, userID, canView); err != nil {
			return nil, err
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

func (s *QuizService) UpdateQuiz(ctx context.Context, id, userID uuid.UUID, isAdmin bool, req dto.UpdateQuizDTO) (*dto.QuizResponseDTO, error) {
	if err := s.checkQuizMutable(ctx, id, userID, isAdmin); err != nil {
		return nil, err
	}
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
	counts, err := s.questionCounts(ctx, []*model.Quiz{quiz})
	if err != nil {
		return nil, err
	}
	return s.mapQuizToDTO(quiz, counts[quiz.ID]), nil
}

func (s *QuizService) DeleteQuiz(ctx context.Context, id, userID uuid.UUID, isAdmin bool) error {
	if err := s.checkQuizMutable(ctx, id, userID, isAdmin); err != nil {
		return err
	}
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

func (s *QuizService) DuplicateQuiz(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizResponseDTO, error) {
	// Nhân bản = chép toàn bộ câu hỏi + đáp án đúng sang quiz của người gọi, nên chịu cùng khoá đọc
	// của cuộc thi VÀ chỉ chủ quiz/admin được làm (review PR #80, F2: nhân bản quiz người khác rồi
	// làm bài trên bản sao là đọc được đáp án).
	if err := s.checkContestAccess(ctx, id, userID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.checkQuizOwner(ctx, id, userID, isAdmin); err != nil {
		return nil, err
	}
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
		CreatedBy:          &userID,
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

func (s *QuizService) CreateQuestion(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.CreateQuestionDTO) (*dto.QuestionResponseDTO, error) {
	if err := s.checkQuizMutable(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}
	// QA T3: từ chối TRƯỚC khi ghi bất cứ thứ gì (câu hỏi + đáp án ghi hai bước, không transaction).
	if err := validateQuestionAnswers(req.QuestionType, req.Answers); err != nil {
		return nil, err
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
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
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
	// SEC-1 (vá lộ nội dung quiz) + R4 (review 260919): xem chú thích tại GetQuizByID — trước
	// bản vá SEC-1, hàm này CHỈ strip is_correct/explanation, không hề kiểm bài có đang khoá với
	// userID hay không; trước bản vá R4, gate CHỈ chạy khi quiz.LessonID != nil.
	if err := s.checkQuizAccess(ctx, quiz, userID, canView); err != nil {
		return nil, err
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

func (s *QuizService) UpdateQuestion(ctx context.Context, quizID, questionID, userID uuid.UUID, isAdmin bool, req dto.UpdateQuestionDTO) (*dto.QuestionResponseDTO, error) {
	// Kiểm theo quizID trên route; câu hỏi thuộc quiz khác bị chặn ở kiểm "belong" bên dưới, nên
	// không thể mượn một quiz không khoá để sửa câu hỏi của quiz đang thi.
	if err := s.checkQuizMutable(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	question, err := s.repo.GetQuestionByID(ctx, questionID)
	if err != nil || question == nil {
		return nil, errors.New("question not found")
	}
	if question.QuizID != quizID {
		return nil, errors.New("question does not belong to this quiz")
	}

	// QA T3: chỉ kiểm khi loại hoặc đáp án thay đổi, để sửa riêng text của câu cũ (đã hỏng) không bị chặn.
	// Đáp án hiệu lực = đáp án gửi lên, hoặc đáp án đang lưu nếu chỉ đổi loại.
	if req.QuestionType != nil || req.Answers != nil {
		effType := question.QuestionType
		if req.QuestionType != nil {
			effType = *req.QuestionType
		}
		effAnswers := req.Answers
		if req.Answers == nil {
			effAnswers = make([]dto.CreateAnswerDTO, len(question.Answers))
			for i, a := range question.Answers {
				effAnswers[i] = dto.CreateAnswerDTO{AnswerText: a.AnswerText, IsCorrect: a.IsCorrect, DisplayOrder: a.DisplayOrder}
			}
		}
		if err := validateQuestionAnswers(effType, effAnswers); err != nil {
			return nil, err
		}
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

func (s *QuizService) DeleteQuestion(ctx context.Context, quizID, questionID, userID uuid.UUID, isAdmin bool) error {
	if err := s.checkQuizMutable(ctx, quizID, userID, isAdmin); err != nil {
		return err
	}
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

func (s *QuizService) ReorderQuestions(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.ReorderQuestionsDTO) error {
	if err := s.checkQuizMutable(ctx, quizID, userID, isAdmin); err != nil {
		return err
	}
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

func (s *QuizService) BulkCreateQuestions(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.BulkCreateQuestionsDTO) ([]dto.QuestionResponseDTO, error) {
	// QA T3: kiểm cả lô trước, để một câu sai không để lại các câu đứng trước đã ghi dở.
	for i, qReq := range req.Questions {
		if err := validateQuestionAnswers(qReq.QuestionType, qReq.Answers); err != nil {
			return nil, fmt.Errorf("question %d: %w", i+1, err)
		}
	}
	var results []dto.QuestionResponseDTO
	for _, qReq := range req.Questions {
		result, err := s.CreateQuestion(ctx, quizID, userID, isAdmin, qReq)
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
	// Thí sinh chỉ được làm bài thi qua POST /contests/:id/start (attempt mode "contest", có hạn
	// giờ server). Đường /quizzes/:id/start không có hạn giờ đó nên phải bị khoá.
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil || quiz == nil {
		return nil, errors.New("quiz not found")
	}

	// SEC-1 + R4 (review 260919): /start trả về toàn bộ text câu hỏi + phương án, nên phải qua
	// CÙNG cổng khoá với GetQuizByID/GetQuestionsByQuiz — nếu không, đây là lối vòng qua bản vá
	// của hai endpoint đọc kia. Chặn trước CreateAttempt để không sinh attempt rác cho quiz đang
	// khoá. Trước bản vá R4, gate CHỈ chạy khi quiz.LessonID != nil — quiz gắn course_id/
	// session_id bỏ qua hoàn toàn, người chưa enroll vẫn /start được và nhận toàn bộ câu hỏi.
	canView, err := s.canViewQuizAnswerKey(ctx, quiz, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	if err := s.checkQuizAccess(ctx, quiz, userID, canView); err != nil {
		return nil, err
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
	for i := range quiz.Questions {
		questions[i] = toAttemptQuestion(&quiz.Questions[i])
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

func (s *QuizService) SubmitQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.SubmitQuizDTO) (*dto.QuizAttemptResponseDTO, error) {
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.checkStandaloneQuizReader(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	// Find the in-progress attempt to submit.
	all, err := s.repo.GetAttemptsByUserAndQuiz(ctx, userID, quizID)
	if err != nil {
		return nil, err
	}
	// Attempt "contest" chỉ được nộp qua SubmitContestAttempt (kiểm hạn giờ cuộc thi, giới hạn
	// time_spent) — đường nộp thường coi như không thấy nó.
	attempts := make([]model.QuizAttempt, 0, len(all))
	for _, a := range all {
		if a.Mode != QuizAttemptModeContest {
			attempts = append(attempts, a)
		}
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
			return nil, ErrQuizAttemptAlreadySubmitted
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

	earnedPoints, totalPoints, attemptAnswers := s.gradeSubmission(quiz, attempt.ID, req.Answers)

	// Calculate result
	now := time.Now()
	timeSpent := int(now.Sub(attempt.StartedAt).Seconds())
	percentage := gradePercentage(earnedPoints, totalPoints)
	isPassed := percentage.GreaterThanOrEqual(quiz.PassPercentage)

	attempt.Score = &earnedPoints
	attempt.TotalPoints = &totalPoints
	attempt.Percentage = &percentage
	attempt.IsPassed = &isPassed
	attempt.TimeSpentSecs = &timeSpent
	attempt.CompletedAt = &now

	// R8 (review 260919, IMPORTANT): trước bản vá này, UpdateAttempt (= Save, không điều kiện)
	// chạy sau CreateAttemptAnswers, KHÔNG transaction và KHÔNG kiểm completed_at IS NULL — hai
	// request nộp cùng lúc (double-click, retry sau timeout) đều đọc thấy CompletedAt == nil ở
	// bước tìm attempt phía trên (TOCTOU: giữa lúc đọc đó và lúc ghi ở đây, request kia có thể
	// đã nộp xong), nên cả 2 đều insert answers + update attempt => answers bị ghi trùng cho
	// cùng một attempt. CompleteAttemptIfPending gộp UPDATE có điều kiện completed_at IS NULL và
	// CreateAttemptAnswers vào CÙNG MỘT transaction — 0 dòng bị ảnh hưởng nghĩa là request khác
	// đã nộp xong trước; trả lỗi rõ ràng thay vì âm thầm ghi trùng.
	updated, err := s.repo.CompleteAttemptIfPending(ctx, attempt, attemptAnswers)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrQuizAttemptAlreadySubmitted
	}

	return s.mapAttemptToDTO(attempt), nil
}

// gradeSubmission chấm một lần nộp — dùng chung cho SubmitQuiz và SubmitContestAttempt để bài thi
// được chấm bằng ĐÚNG luật của quiz thường (contract "Cuộc thi" §4.2).
//
// R1 (review 260919, CRITICAL): mẫu số (total) PHẢI là tổng điểm CỦA TOÀN BỘ câu hỏi thuộc quiz
// (quiz.Questions), KHÔNG PHẢI chỉ những câu client gửi. Trước bản vá đó, quiz 10 câu mà học viên
// chỉ trả lời (hoặc bỏ trống rồi hết giờ tự nộp) đúng 1 câu sẽ có total = điểm đúng 1 câu đó =>
// percentage 100% nếu câu đó đúng. Lặp qua quiz.Questions (không phải answers) còn tự nhiên
// "dedupe": client gửi trùng question_id nhiều lần không cộng dồn điểm (map ghi đè, bản cuối
// được chấm). Câu không trả lời = 0 điểm và không tạo quiz_attempt_answers.
func (s *QuizService) gradeSubmission(quiz *model.Quiz, attemptID uuid.UUID, answers []dto.SubmitAnswerDTO) (earned, total decimal.Decimal, rows []model.QuizAttemptAnswer) {
	answersByQuestion := make(map[uuid.UUID]dto.SubmitAnswerDTO, len(answers))
	for _, ans := range answers {
		questionID, parseErr := uuid.Parse(ans.QuestionID)
		if parseErr != nil {
			continue // question_id sai định dạng — bỏ qua như câu không khớp.
		}
		answersByQuestion[questionID] = ans
	}

	earned, total = decimal.Zero, decimal.Zero
	for i := range quiz.Questions {
		question := &quiz.Questions[i]
		total = total.Add(question.Points)

		ans, answered := answersByQuestion[question.ID]
		if !answered {
			continue
		}
		correct := s.checkAnswer(question, ans)
		points := decimal.Zero
		if correct {
			points = question.Points
		}
		earned = earned.Add(points)

		row := model.QuizAttemptAnswer{
			AttemptID:         attemptID,
			QuestionID:        question.ID,
			SelectedAnswerIDs: pq.StringArray(ans.SelectedAnswerIDs),
			IsCorrect:         &correct,
			PointsEarned:      points,
		}
		if ans.TextAnswer != "" {
			text := ans.TextAnswer
			row.TextAnswer = &text
		}
		rows = append(rows, row)
	}
	return earned, total, rows
}

// gradePercentage: earned/total*100, quiz không có điểm nào thì 0.
func gradePercentage(earned, total decimal.Decimal) decimal.Decimal {
	if total.IsZero() {
		return decimal.Zero
	}
	return earned.Div(total).Mul(decimal.NewFromInt(100))
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
		// So khớp sau chuẩn hoá (fill_blank_answer.go): khoảng trắng, hoa thường, NFC; giữ dấu.
		accepted := []string{}
		for _, a := range question.Answers {
			if a.IsCorrect {
				accepted = append(accepted, a.AnswerText)
			}
		}
		return fillBlankMatches(accepted, answer.TextAnswer)

	case "essay":
		// Essays need manual grading
		return false
	}
	return false
}

func (s *QuizService) GetMyAttempts(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuizAttemptResponseDTO, error) {
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.checkStandaloneQuizReader(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
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

func (s *QuizService) GetAttemptByID(ctx context.Context, attemptID, userID uuid.UUID, isAdmin bool) (*dto.QuizAttemptDetailDTO, error) {
	attempt, err := s.repo.GetAttemptWithAnswers(ctx, attemptID)
	if err != nil || attempt == nil {
		return nil, ErrQuizAttemptNotFound
	}
	if attempt.UserID != userID {
		// S2: bài làm của người khác. "Không tồn tại" và "không được xem" cho CÙNG một lỗi (404), kể cả
		// quiz gắn cuộc thi: nếu trả 403 cho attempt thật mà 404 cho id bịa thì dò được attempt có
		// tồn tại hay không. 403 QUIZ_LOCKED_BY_CONTEST chỉ còn cho CHÍNH CHỦ attempt (nhánh else).
		if !s.canViewOthersAttempt(ctx, attempt.QuizID, attempt.UserID, userID, isAdmin) {
			return nil, ErrQuizAttemptNotFound
		}
		// Người được phép xem (giảng viên chủ khoá, phụ huynh, admin) vẫn qua khoá cuộc thi để đáp án
		// không lộ trước giờ mở.
		if err := s.checkContestAccess(ctx, attempt.QuizID, userID, isAdmin); err != nil {
			return nil, err
		}
	} else {
		// Bài thi đã nộp sẽ có correct_answer_ids/explanation bên dưới — thí sinh chỉ được xem qua
		// GET /contests/:id/my-result sau khi cuộc thi đóng (contract §4.3), không phải ở đây.
		if err := s.checkContestAccess(ctx, attempt.QuizID, userID, isAdmin); err != nil {
			return nil, err
		}
		if err := s.checkStandaloneQuizReader(ctx, attempt.QuizID, userID, isAdmin); err != nil {
			return nil, err
		}
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

func (s *QuizService) GetQuizResults(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) (*dto.QuizResultsDTO, error) {
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.checkStandaloneQuizReader(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	// S2: trước đây bất kỳ tài khoản nào cũng đọc được điểm của MỌI học viên qua route này.
	if err := s.checkQuizResultsManager(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
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

func (s *QuizService) GetQuizStatistics(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) (*dto.QuizStatisticsDTO, error) {
	if err := s.checkContestAccess(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.checkStandaloneQuizReader(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.checkQuizResultsManager(ctx, quizID, userID, isAdmin); err != nil {
		return nil, err
	}
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

func (s *QuizService) SaveAnswer(ctx context.Context, attemptID, userID uuid.UUID, isAdmin bool, req dto.SaveAnswerDTO) error {
	// Route chỉ có attemptId, nên phải tra attempt để biết quiz nào mà hỏi khoá cuộc thi.
	attempt, err := s.repo.GetAttemptByID(ctx, attemptID)
	if err != nil || attempt == nil {
		return ErrQuizAttemptNotFound
	}
	// S2/m4: lưu đáp án là việc của CHÍNH CHỦ attempt; attempt của người khác trả cùng lỗi với id bịa.
	if attempt.UserID != userID {
		return ErrQuizAttemptNotFound
	}
	if err := s.checkContestAccess(ctx, attempt.QuizID, userID, isAdmin); err != nil {
		return err
	}
	if err := s.checkStandaloneQuizReader(ctx, attempt.QuizID, userID, isAdmin); err != nil {
		return err
	}
	// Save in-progress answer to Redis for auto-save
	if s.redis == nil {
		return nil
	}

	key := attemptCachePrefix + attemptID.String()
	data, _ := json.Marshal(req)
	return s.redis.HSet(ctx, key, req.QuestionID, data).Err()
}

func (s *QuizService) GetAttemptProgress(ctx context.Context, attemptID, userID uuid.UUID, isAdmin bool) (*dto.QuizAttemptDetailDTO, error) {
	return s.GetAttemptByID(ctx, attemptID, userID, isAdmin)
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

	ptrs := make([]*model.Quiz, len(quizzes))
	for i := range quizzes {
		ptrs[i] = &quizzes[i]
	}
	counts, err := s.questionCounts(ctx, ptrs)
	if err != nil {
		return nil, err
	}
	data := make([]dto.QuizResponseDTO, len(quizzes))
	for i := range quizzes {
		data[i] = *s.mapQuizToDTO(&quizzes[i], counts[quizzes[i].ID])
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
		Mode:          a.Mode,
		Score:         a.Score,
		TotalPoints:   a.TotalPoints,
		Percentage:    a.Percentage,
		IsPassed:      a.IsPassed,
		TimeSpentSecs: a.TimeSpentSecs,
		StartedAt:     a.StartedAt,
		CompletedAt:   a.CompletedAt,
	}
}
