package service

// Test cho review 260919 — 3 loi trong quiz_service.go:
//   - R1 (CRITICAL): SubmitQuiz truoc day tinh mau so (totalPoints) chi tu req.Answers (nhung
//     cau client GUI len), khong phai tu TOAN BO cau hoi cua quiz — quiz 10 cau chi tra loi
//     (hoac chi con lai) 1 cau van co the ra 100% neu cau do dung.
//   - R4: gate truy cap quiz (#65, checkLessonQuizAccess) truoc day CHI ap dung cho quiz gan
//     lesson_id — quiz gan thang course_id/session_id khong qua gate nao, nguoi chua enroll van
//     doc/start duoc.
//   - R8: SubmitQuiz truoc day update attempt (Save) + insert answers KHONG transaction, KHONG
//     dieu kien completed_at IS NULL — nop 2 lan (double-click/retry) ghi trung answers.
//
// Mutation muon bat:
//   - R1: doi vong lap tinh totalPoints tu quiz.Questions ve lai req.Answers (hoac bo dedupe map)
//     phai lam TestSubmitQuiz_CauKhongTraLoi_TinhTrenTatCaCauHoi DO.
//   - R4: bo nhanh checkCourseQuizAccess/checkSessionQuizAccess trong checkQuizAccess (hoac bo
//     goi checkQuizAccess o GetQuizByID/GetQuestionsByQuiz/StartQuiz/GetAllQuizzes) phai lam cac
//     test *_GanCourseID_*/*_GanSessionID_* DO.
//   - R8: bo dieu kien completed_at IS NULL trong CompleteAttemptIfPending (hoac goi
//     UpdateAttempt/CreateAttemptAnswers rieng le nhu cu) phai lam
//     TestSubmitQuiz_NopHaiLan_LanHaiBiTuChoi DO.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ----------------------------------------------------------------------------
// R1 — SubmitQuiz phai tinh diem tren TOAN BO cau hoi cua quiz
// ----------------------------------------------------------------------------

// fakeQuizRepoForSubmit: du method QuizService.SubmitQuiz thuc su goi. CompleteAttemptIfPending
// mo phong DUNG hop dong cua ban that (interface, R8): chi "hoan tat" duoc MOT LAN cho moi
// attemptID — trang thai "da hoan tat" luu O RIENG (committed map), TACH BACH voi attempt truyen
// vao (attempt truyen vao la ban ghi CUC BO cua tung request, da bi service tu gan CompletedAt
// truoc khi goi ham nay — neu kiem tren chinh no thi lan goi DAU TIEN cung sai).
type fakeQuizRepoForSubmit struct {
	repository.QuizRepositoryInterface
	quiz    *model.Quiz
	attempt model.QuizAttempt // attempt goc, CompletedAt luon nil — moi lan GetAttemptsByUserAndQuiz tra ban SAO

	committed      map[uuid.UUID]bool
	completeCalls  int
	lastAnswers    []model.QuizAttemptAnswer
	allAnswersSeen [][]model.QuizAttemptAnswer
}

func (f *fakeQuizRepoForSubmit) GetAttemptsByUserAndQuiz(ctx context.Context, userID, quizID uuid.UUID) ([]model.QuizAttempt, error) {
	a := f.attempt // ban sao — moi "request" doc lai tu dau, khong chia se con tro voi lan truoc
	return []model.QuizAttempt{a}, nil
}

func (f *fakeQuizRepoForSubmit) GetQuizWithQuestions(ctx context.Context, quizID uuid.UUID) (*model.Quiz, error) {
	return f.quiz, nil
}

func (f *fakeQuizRepoForSubmit) CompleteAttemptIfPending(ctx context.Context, attempt *model.QuizAttempt, answers []model.QuizAttemptAnswer) (bool, error) {
	f.completeCalls++
	f.allAnswersSeen = append(f.allAnswersSeen, answers)
	if f.committed == nil {
		f.committed = map[uuid.UUID]bool{}
	}
	if f.committed[attempt.ID] {
		return false, nil
	}
	f.committed[attempt.ID] = true
	f.lastAnswers = answers
	return true, nil
}

// quizMuoiCauMotDiem: quiz 10 cau single_choice, moi cau 1 diem, cau i co dap an dung la
// answers[i][0] va dap an sai la answers[i][1].
func quizMuoiCauMotDiem() *model.Quiz {
	q := &model.Quiz{Title: "Quiz 10 cau", PassPercentage: decimal.NewFromInt(50)}
	q.ID = uuid.New()
	q.Questions = make([]model.Question, 10)
	for i := 0; i < 10; i++ {
		correctID := uuid.New()
		wrongID := uuid.New()
		q.Questions[i] = model.Question{
			QuestionText: "cau hoi",
			QuestionType: "single_choice",
			Points:       decimal.NewFromInt(1),
			Answers: []model.QuestionAnswer{
				{AnswerText: "dung", IsCorrect: true},
				{AnswerText: "sai", IsCorrect: false},
			},
		}
		q.Questions[i].ID = uuid.New()
		q.Questions[i].Answers[0].ID = correctID
		q.Questions[i].Answers[1].ID = wrongID
	}
	return q
}

// TestSubmitQuiz_CauKhongTraLoi_TinhTrenTatCaCauHoi (R1): quiz 10 cau 1 diem/cau, client CHI
// gui dung 1 cau (dung). Truoc ban va, totalPoints = 1 (chi cau da gui) => percentage = 100%.
// Sau ban va, totalPoints PHAI = 10 (tong diem CA quiz) => percentage = 10%, is_passed = false
// (PassPercentage = 50%).
func TestSubmitQuiz_CauKhongTraLoi_TinhTrenTatCaCauHoi(t *testing.T) {
	quiz := quizMuoiCauMotDiem()
	attemptID := uuid.New()
	userID := uuid.New()

	repo := &fakeQuizRepoForSubmit{
		quiz: quiz,
		attempt: model.QuizAttempt{
			ID:        attemptID,
			UserID:    userID,
			QuizID:    quiz.ID,
			StartedAt: time.Now().Add(-1 * time.Minute),
		},
	}
	s := NewQuizService(repo, nil, nil, nil, nil, nil, nil)

	// Chi tra loi CAU DAU TIEN, va tra loi DUNG.
	req := dto.SubmitQuizDTO{
		Answers: []dto.SubmitAnswerDTO{
			{
				QuestionID:        quiz.Questions[0].ID.String(),
				SelectedAnswerIDs: []string{quiz.Questions[0].Answers[0].ID.String()},
			},
		},
	}

	result, err := s.SubmitQuiz(context.Background(), quiz.ID, userID, req)
	if err != nil {
		t.Fatalf("err = %v, muon nil", err)
	}
	if result.TotalPoints == nil || !result.TotalPoints.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("TotalPoints = %v, muon 10 (tong diem CA 10 cau, khong phai chi cau da tra loi)", result.TotalPoints)
	}
	if result.Score == nil || !result.Score.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("Score = %v, muon 1 (dung 1/10 cau)", result.Score)
	}
	if result.Percentage == nil || !result.Percentage.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("Percentage = %v, muon 10 (1/10*100) — day chinh la loi R1: bo trong 9 cau khong duoc phep ra 100%%", result.Percentage)
	}
	if result.IsPassed == nil || *result.IsPassed {
		t.Fatal("IsPassed = true, muon false (10%% < PassPercentage 50%%)")
	}
	if len(repo.lastAnswers) != 1 {
		t.Fatalf("len(lastAnswers) = %d, muon 1 (chi cau da tra loi moi tao attempt_answer)", len(repo.lastAnswers))
	}
}

// TestSubmitQuiz_TraLoiTrungQuestionID_KhongCongDonDiem (R1, he qua cua viec doc tu
// quiz.Questions thay vi req.Answers): client gui TRUNG mot question_id 2 lan (cung dung) —
// khong duoc cong diem 2 lan cho cung 1 cau.
func TestSubmitQuiz_TraLoiTrungQuestionID_KhongCongDonDiem(t *testing.T) {
	quiz := quizMuoiCauMotDiem()
	attemptID := uuid.New()
	userID := uuid.New()

	repo := &fakeQuizRepoForSubmit{
		quiz: quiz,
		attempt: model.QuizAttempt{
			ID:        attemptID,
			UserID:    userID,
			QuizID:    quiz.ID,
			StartedAt: time.Now(),
		},
	}
	s := NewQuizService(repo, nil, nil, nil, nil, nil, nil)

	req := dto.SubmitQuizDTO{
		Answers: []dto.SubmitAnswerDTO{
			{QuestionID: quiz.Questions[0].ID.String(), SelectedAnswerIDs: []string{quiz.Questions[0].Answers[0].ID.String()}},
			{QuestionID: quiz.Questions[0].ID.String(), SelectedAnswerIDs: []string{quiz.Questions[0].Answers[0].ID.String()}},
		},
	}

	result, err := s.SubmitQuiz(context.Background(), quiz.ID, userID, req)
	if err != nil {
		t.Fatalf("err = %v, muon nil", err)
	}
	if !result.Score.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("Score = %v, muon 1 — gui trung question_id 2 lan khong duoc cong diem 2 lan", result.Score)
	}
	if len(repo.lastAnswers) != 1 {
		t.Fatalf("len(lastAnswers) = %d, muon 1 (dedupe theo question_id)", len(repo.lastAnswers))
	}
}

// ----------------------------------------------------------------------------
// R8 — nop quiz hai lan
// ----------------------------------------------------------------------------

// TestSubmitQuiz_NopHaiLan_LanHaiBiTuChoi (R8): goi SubmitQuiz 2 LAN LIEN TIEP cho CUNG mot
// attempt (mo phong double-click/retry — moi lan doc lai attempt tu "DB" deu thay CompletedAt =
// nil, dung nhu tinh huong TOCTOU that). Lan 1 phai thanh cong; lan 2 PHAI bi tu choi
// (ErrQuizAttemptAlreadySubmitted) va KHONG duoc goi insert answers lan thu 2.
func TestSubmitQuiz_NopHaiLan_LanHaiBiTuChoi(t *testing.T) {
	quiz := quizMuoiCauMotDiem()
	attemptID := uuid.New()
	userID := uuid.New()

	repo := &fakeQuizRepoForSubmit{
		quiz: quiz,
		attempt: model.QuizAttempt{
			ID:        attemptID,
			UserID:    userID,
			QuizID:    quiz.ID,
			StartedAt: time.Now(),
		},
	}
	s := NewQuizService(repo, nil, nil, nil, nil, nil, nil)

	req := dto.SubmitQuizDTO{
		Answers: []dto.SubmitAnswerDTO{
			{QuestionID: quiz.Questions[0].ID.String(), SelectedAnswerIDs: []string{quiz.Questions[0].Answers[0].ID.String()}},
		},
	}

	result1, err1 := s.SubmitQuiz(context.Background(), quiz.ID, userID, req)
	if err1 != nil {
		t.Fatalf("lan 1: err = %v, muon nil (lan dau phai thanh cong)", err1)
	}
	if result1 == nil {
		t.Fatal("lan 1: result = nil, muon co du lieu")
	}

	result2, err2 := s.SubmitQuiz(context.Background(), quiz.ID, userID, req)
	if !errors.Is(err2, ErrQuizAttemptAlreadySubmitted) {
		t.Fatalf("lan 2: err = %v, muon ErrQuizAttemptAlreadySubmitted — day chinh la loi R8: nop 2 lan phai bi tu choi o lan 2", err2)
	}
	if result2 != nil {
		t.Fatalf("lan 2: result = %+v, muon nil khi da nop roi", result2)
	}

	if repo.completeCalls != 2 {
		t.Fatalf("completeCalls = %d, muon 2 (ca 2 lan goi deu phai di qua CompleteAttemptIfPending)", repo.completeCalls)
	}
	// Tong so answers duoc GHI THUC SU (khong tinh lan bi tu choi) phai la 1, khong phai 2 —
	// day la bang chung "khong ghi trung": lan 2 bi tu choi truoc khi insert.
	totalWritten := 0
	for i, answers := range repo.allAnswersSeen {
		if i == 0 {
			totalWritten += len(answers) // lan 1: da commit that
		}
		// lan 2 (i==1): CompleteAttemptIfPending nhan answers nhung PHAI tra updated=false va
		// KHONG duoc coi la da ghi — ham fake da tra false truoc khi "ghi" (committed check truoc),
		// nen answers cua lan 2 khong duoc tinh vao totalWritten.
	}
	if totalWritten != 1 {
		t.Fatalf("totalWritten (lan 1) = %d, muon 1 — nop 2 lan khong duoc de DB co 2 attempt_answer cho cung 1 cau", totalWritten)
	}
}

// ----------------------------------------------------------------------------
// R4 — gate quiz gan course_id / session_id
// ----------------------------------------------------------------------------

func quizGanCourseID(courseID uuid.UUID) *model.Quiz {
	q := &model.Quiz{Title: "Quiz cap khoa", CourseID: &courseID}
	q.ID = uuid.New()
	q.Questions = []model.Question{{QuestionText: "q", QuestionType: "single_choice", Points: decimal.NewFromInt(1)}}
	q.Questions[0].ID = uuid.New()
	return q
}

// TestGetQuizByID_QuizGanCourseID_ChuaEnroll_TraErrLessonLocked (R4): quiz gan THANG course_id
// (khong qua lesson) — nguoi CHUA enroll khoa do phai bi chan, dung 403/ErrLessonLocked nhu gate
// lesson. Truoc ban va R4, checkLessonQuizAccess chi chay khi quiz.LessonID != nil nen nhanh nay
// bi bo qua hoan toan — ai cung doc duoc.
func TestGetQuizByID_QuizGanCourseID_ChuaEnroll_TraErrLessonLocked(t *testing.T) {
	courseID := uuid.New()
	quiz := quizGanCourseID(courseID)

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID, enrollment: nil}

	s := NewQuizService(quizRepo, nil, courseRepo, nil, nil, nil, enrollmentRepo)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, uuid.New(), false)
	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil khi chua enroll", result)
	}
}

// TestGetQuizByID_QuizGanCourseID_DaEnroll_TraDuLieu (R4): da enroll khoa chua quiz => phai tra
// du lieu binh thuong (khong bi khoa boi gate moi).
func TestGetQuizByID_QuizGanCourseID_DaEnroll_TraDuLieu(t *testing.T) {
	courseID := uuid.New()
	quiz := quizGanCourseID(courseID)

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID, enrollment: &model.Enrollment{}}

	s := NewQuizService(quizRepo, nil, courseRepo, nil, nil, nil, enrollmentRepo)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("err = %v, muon nil (da enroll)", err)
	}
	if result == nil {
		t.Fatal("result = nil, muon co du lieu quiz")
	}
}

// TestGetQuestionsByQuiz_QuizGanCourseID_ChuaEnroll_TraErrLessonLocked (R4): cung gate, qua
// duong GetQuestionsByQuiz.
func TestGetQuestionsByQuiz_QuizGanCourseID_ChuaEnroll_TraErrLessonLocked(t *testing.T) {
	courseID := uuid.New()
	quiz := quizGanCourseID(courseID)

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID, enrollment: nil}

	s := NewQuizService(quizRepo, nil, courseRepo, nil, nil, nil, enrollmentRepo)

	result, err := s.GetQuestionsByQuiz(context.Background(), quiz.ID, uuid.New(), false)
	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil", result)
	}
	if quizRepo.gotQuestionsCall {
		t.Fatal("cau hoi THAT da bi nap du chua enroll")
	}
}

// TestStartQuiz_QuizGanCourseID_ChuaEnroll_TraErrLessonLocked_KhongTaoAttempt (R4): qua duong
// StartQuiz — fake repo khong cai CreateAttempt (nil, embed interface): neu cong khoa bi go,
// StartQuiz se goi CreateAttempt va panic.
func TestStartQuiz_QuizGanCourseID_ChuaEnroll_TraErrLessonLocked_KhongTaoAttempt(t *testing.T) {
	courseID := uuid.New()
	quiz := quizGanCourseID(courseID)

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID, enrollment: nil}

	s := NewQuizService(quizRepo, nil, courseRepo, nil, nil, nil, enrollmentRepo)

	result, err := s.StartQuiz(context.Background(), quiz.ID, uuid.New(), false, dto.StartQuizDTO{})
	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil", result)
	}
}

// TestGetAllQuizzes_LocQuizGanCourseID_ChuaEnroll (R4): GetAllQuizzes phai LOAI quiz gan
// course_id ra khoi danh sach doi voi nguoi CHUA enroll, giong het hanh vi da co san cho quiz
// gan lesson_id.
func TestGetAllQuizzes_LocQuizGanCourseID_ChuaEnroll(t *testing.T) {
	courseID := uuid.New()
	lockedQuiz := *quizGanCourseID(courseID)
	lockedQuiz.Title = "Quiz cap khoa chua enroll"

	openQuiz := model.Quiz{Title: "Quiz mo coi"}
	openQuiz.ID = uuid.New()

	quizRepo := &fakeQuizRepoForAccessLock{quizzes: []model.Quiz{lockedQuiz, openQuiz}, total: 2}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID, enrollment: nil}

	s := NewQuizService(quizRepo, nil, courseRepo, nil, nil, nil, enrollmentRepo)

	list, err := s.GetAllQuizzes(context.Background(), nil, nil, nil, uuid.New(), false, 1, 10)
	if err != nil {
		t.Fatalf("err = %v, muon nil (quiz bi khoa phai bi LOC, khong phai LOI)", err)
	}
	if len(list.Data) != 1 {
		t.Fatalf("len(Data) = %d, muon 1 (chi con quiz mo coi) — Data: %+v", len(list.Data), list.Data)
	}
	if list.Data[0].Title != "Quiz mo coi" {
		t.Fatalf("Data[0].Title = %q, muon %q", list.Data[0].Title, "Quiz mo coi")
	}
}

// ----------------------------------------------------------------------------
// R4 — gate quiz gan session_id
// ----------------------------------------------------------------------------

// fakeLivestreamRepoForQuizLock: GetByID tra ve session cau hinh san.
type fakeLivestreamRepoForQuizLock struct {
	repository.LivestreamRepositoryInterface
	session *model.LivestreamSession
}

func (f *fakeLivestreamRepoForQuizLock) GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	return f.session, nil
}

func quizGanSessionID(sessionID uuid.UUID) *model.Quiz {
	q := &model.Quiz{Title: "Quiz live", SessionID: &sessionID}
	q.ID = uuid.New()
	q.Questions = []model.Question{{QuestionText: "q", QuestionType: "single_choice", Points: decimal.NewFromInt(1)}}
	q.Questions[0].ID = uuid.New()
	return q
}

// TestGetQuizByID_QuizGanSessionID_KhongPhaiThanhVien_TraErrLessonLocked (R4): nguoi KHONG phai
// host, KHONG phai participant cua session, VA session khong gan course (hoac chua enroll khoa
// do) => phai bi chan.
func TestGetQuizByID_QuizGanSessionID_KhongPhaiThanhVien_TraErrLessonLocked(t *testing.T) {
	sessionID := uuid.New()
	quiz := quizGanSessionID(sessionID)
	session := &model.LivestreamSession{HostID: uuid.New()}
	session.ID = sessionID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	livestreamRepo := &fakeLivestreamRepoForQuizLock{session: session}

	s := NewQuizService(quizRepo, nil, nil, nil, nil, livestreamRepo, nil)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, uuid.New(), false)
	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil", result)
	}
}

// TestGetQuizByID_QuizGanSessionID_LaHost_TraDuLieu (R4): host cua chinh session do khong bao
// gio bi khoa.
func TestGetQuizByID_QuizGanSessionID_LaHost_TraDuLieu(t *testing.T) {
	sessionID := uuid.New()
	hostID := uuid.New()
	quiz := quizGanSessionID(sessionID)
	session := &model.LivestreamSession{HostID: hostID}
	session.ID = sessionID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	livestreamRepo := &fakeLivestreamRepoForQuizLock{session: session}

	s := NewQuizService(quizRepo, nil, nil, nil, nil, livestreamRepo, nil)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, hostID, false)
	if err != nil {
		t.Fatalf("err = %v, muon nil (host)", err)
	}
	if result == nil {
		t.Fatal("result = nil, muon co du lieu")
	}
}

// TestGetQuizByID_QuizGanSessionID_LaParticipant_TraDuLieu (R4): nguoi da tham gia session
// (co trong Participants) duoc xem, du khong phai host va khong enroll khoa nao.
func TestGetQuizByID_QuizGanSessionID_LaParticipant_TraDuLieu(t *testing.T) {
	sessionID := uuid.New()
	participantID := uuid.New()
	quiz := quizGanSessionID(sessionID)
	session := &model.LivestreamSession{
		HostID:       uuid.New(),
		Participants: []model.Participant{{UserID: participantID}},
	}
	session.ID = sessionID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	livestreamRepo := &fakeLivestreamRepoForQuizLock{session: session}

	s := NewQuizService(quizRepo, nil, nil, nil, nil, livestreamRepo, nil)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, participantID, false)
	if err != nil {
		t.Fatalf("err = %v, muon nil (da la participant)", err)
	}
	if result == nil {
		t.Fatal("result = nil, muon co du lieu")
	}
}

// TestGetQuizByID_QuizGanSessionID_DaEnrollKhoaChuaSession_TraDuLieu (R4): khong phai host,
// khong phai participant, nhung DA ENROLL khoa hoc chua session do => van duoc xem.
func TestGetQuizByID_QuizGanSessionID_DaEnrollKhoaChuaSession_TraDuLieu(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	userID := uuid.New()
	quiz := quizGanSessionID(sessionID)
	session := &model.LivestreamSession{HostID: uuid.New(), CourseID: &courseID}
	session.ID = sessionID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	livestreamRepo := &fakeLivestreamRepoForQuizLock{session: session}
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID, enrollment: &model.Enrollment{}}
	// canViewQuizAnswerKey (duong doc, khong phai gate) cung lan qua session.CourseID de tinh
	// "co phai instructor khong" — can courseRepo du la khong instructor.
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}}

	s := NewQuizService(quizRepo, nil, courseRepo, nil, nil, livestreamRepo, enrollmentRepo)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, userID, false)
	if err != nil {
		t.Fatalf("err = %v, muon nil (da enroll khoa chua session)", err)
	}
	if result == nil {
		t.Fatal("result = nil, muon co du lieu")
	}
}
