package service

// Test cho SEC-1 (vá lộ nội dung quiz): checkLessonQuizAccess, áp dụng ở GetQuizByID,
// GetQuestionsByQuiz (trả ErrLessonLocked) và GetAllQuizzes (loại quiz bị khoá khỏi danh sách,
// không lỗi cả request) — xem chú thích tại quiz_service.go.
//
// Mutation muốn bắt: xoá dòng `return ErrLessonLocked` trong checkLessonQuizAccess (hoặc bỏ hẳn
// lệnh gọi checkLessonQuizAccess ở ba nơi trên) phải làm
// TestGetQuizByID_BaiKhoa_TraErrLessonLocked và TestGetQuestionsByQuiz_BaiKhoa_TraErrLessonLocked
// ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ----------------------------------------------------------------------------
// Fakes riêng cho file này. fakeLessonRepoForLock/fakeCourseRepoForLock/
// fakeEnrollmentRepoForLock (lesson_content_lock_test.go, cùng package) được TÁI SỬ DỤNG thẳng ở
// nơi chỉ cần MỘT lesson/course cố định — không viết lại.
// ----------------------------------------------------------------------------

// fakeLessonRepoByID: khác fakeLessonRepoForLock ở chỗ tra theo TỪNG id — cần cho
// TestGetAllQuizzes_LocQuizCuaBaiKhoa, nơi nhiều quiz trỏ tới nhiều lesson khác nhau trong CÙNG
// một lần gọi.
type fakeLessonRepoByID struct {
	repository.LessonRepositoryInterface
	lessons map[uuid.UUID]*model.Lesson
}

func (f *fakeLessonRepoByID) GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error) {
	return f.lessons[id], nil
}

// fakeSectionRepoForQuizLock: trả về MỘT section cố định (đủ dùng vì canViewQuizAnswerKey chỉ
// cần section.CourseID để lần ra course, và fakeCourseRepoForLock.GetByID bỏ qua id được truyền).
type fakeSectionRepoForQuizLock struct {
	repository.SectionRepositoryInterface
	section *model.Section
}

func (f *fakeSectionRepoForQuizLock) GetByID(ctx context.Context, id uuid.UUID) (*model.Section, error) {
	return f.section, nil
}

// fakeQuizRepoForAccessLock: đủ 4 method QuizService thật sự gọi qua các hàm đang test
// (GetQuizByID, GetQuestionsByQuiz, GetAllQuizzes) — method nào không override mà bị gọi sẽ
// panic (embed interface nil), tự lộ ngay nếu test đang chạm vào đường ngoài phạm vi.
type fakeQuizRepoForAccessLock struct {
	repository.QuizRepositoryInterface
	quiz             *model.Quiz
	quizzes          []model.Quiz
	total            int64
	gotQuestionsCall bool
}

func (f *fakeQuizRepoForAccessLock) GetQuizByID(ctx context.Context, id uuid.UUID) (*model.Quiz, error) {
	return f.quiz, nil
}

func (f *fakeQuizRepoForAccessLock) GetQuizWithQuestions(ctx context.Context, id uuid.UUID) (*model.Quiz, error) {
	return f.quiz, nil
}

func (f *fakeQuizRepoForAccessLock) GetQuestionsByQuizID(ctx context.Context, quizID uuid.UUID) ([]model.Question, error) {
	f.gotQuestionsCall = true
	if f.quiz == nil {
		return nil, nil
	}
	return f.quiz.Questions, nil
}

func (f *fakeQuizRepoForAccessLock) ListQuizzes(ctx context.Context, lessonID, courseID, sessionID *uuid.UUID, page, pageSize int) ([]model.Quiz, int64, error) {
	return f.quizzes, f.total, nil
}

// quizVoiMotCauHoi: quiz gắn LessonID, có đúng 1 câu hỏi với 1 đáp án đúng/1 đáp án sai + giải
// thích — đủ để khẳng định stripAnswerKey vẫn hoạt động (is_correct/explanation bị ẩn) SAU khi
// đi qua checkLessonQuizAccess.
func quizVoiMotCauHoi(lessonID uuid.UUID) *model.Quiz {
	explanation := "vi 1+1=2"
	q := &model.Quiz{
		Title:    "Quiz co dap an",
		LessonID: &lessonID,
		Questions: []model.Question{
			{
				QuestionText: "1+1=?",
				QuestionType: "single_choice",
				Explanation:  &explanation,
				Answers: []model.QuestionAnswer{
					{AnswerText: "2", IsCorrect: true},
					{AnswerText: "3", IsCorrect: false},
				},
			},
		},
	}
	q.ID = uuid.New()
	q.Questions[0].ID = uuid.New()
	return q
}

// TestGetQuizByID_BaiKhoa_TraErrLessonLocked (case 1): nguoi dung CHUA enroll, bai khong preview
// => GetQuizByID phai tra ErrLessonLocked, KHONG duoc tra du lieu quiz that.
func TestGetQuizByID_BaiKhoa_TraErrLessonLocked(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: sectionID, IsPreview: false}
	lesson.ID = lessonID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true, InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil, // chua enroll
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, uuid.New(), false)

	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil khi bai dang khoa", result)
	}
}

// TestGetQuestionsByQuiz_BaiKhoa_TraErrLessonLocked (case 1, mutation M3): giong test tren nhung
// qua GetQuestionsByQuiz — phai tra ErrLessonLocked va KHONG duoc nap cau hoi that
// (gotQuestionsCall phai van false, giong duong lo GetContentsByLessonID ma
// lesson_content_lock_test.go da chan).
func TestGetQuestionsByQuiz_BaiKhoa_TraErrLessonLocked(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: sectionID, IsPreview: false}
	lesson.ID = lessonID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true, InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil,
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	result, err := s.GetQuestionsByQuiz(context.Background(), quiz.ID, uuid.New(), false)

	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil khi bai dang khoa", result)
	}
	if quizRepo.gotQuestionsCall {
		t.Fatal("cau hoi THAT da bi nap du bai dang khoa — day chinh la duong lo SEC-1 dong lai")
	}
}

// TestGetQuestionsByQuiz_DaEnrollBaiMo_TraDuLieu_AnDapAn (case 2): da enroll, khoa KHONG
// sequential => bai mo, GetQuestionsByQuiz phai tra du lieu, VA is_correct/explanation van bi an
// (canView=false, hanh vi B-3 cu khong doi).
func TestGetQuestionsByQuiz_DaEnrollBaiMo_TraDuLieu_AnDapAn(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: sectionID, IsPreview: false}
	lesson.ID = lessonID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: false, InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: &model.Enrollment{}, // da enroll
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	result, err := s.GetQuestionsByQuiz(context.Background(), quiz.ID, uuid.New(), false)

	if err != nil {
		t.Fatalf("err = %v, muon nil (bai da enroll, khoa khong sequential nen khong khoa)", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, muon 1", len(result))
	}
	if len(result[0].Answers) != 2 {
		t.Fatalf("len(Answers) = %d, muon 2", len(result[0].Answers))
	}
	for _, a := range result[0].Answers {
		if a.IsCorrect != nil {
			t.Fatal("Answers[i].IsCorrect khong duoc nil — dap an dung bi lo cho nguoi khong co quyen")
		}
	}
	if result[0].Explanation != nil {
		t.Fatal("Explanation khong duoc nil — giai thich bi lo cho nguoi khong co quyen")
	}
}

// TestGetQuizByID_ChuKhoaHoc_KhongBiKhoa (case 3): giang vien so huu khoa hoc chua bai nay, CHUA
// enroll (dieu ma not_enrolled dang le khoa moi bai) — van phai xem duoc du lieu THAT (khong bi
// strip is_correct/explanation).
func TestGetQuizByID_ChuKhoaHoc_KhongBiKhoa(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	instructorID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: sectionID, IsPreview: false}
	lesson.ID = lessonID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true, InstructorID: instructorID}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil, // giang vien khong can enroll khoa cua chinh minh
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, instructorID, false)

	if err != nil {
		t.Fatalf("err = %v, muon nil (chu khoa hoc khong bi khoa)", err)
	}
	if len(result.Questions) != 1 || len(result.Questions[0].Answers) != 2 {
		t.Fatalf("result = %+v, muon 1 cau hoi 2 dap an", result)
	}
	if result.Questions[0].Answers[0].IsCorrect == nil {
		t.Fatal("IsCorrect bi an du la chu khoa hoc — chu khoa hoc phai xem duoc dap an dung")
	}
}

// TestGetQuizByID_Admin_KhongBiKhoa (case 3): admin he thong khong bi khoa du chua enroll va
// khong so huu khoa hoc.
func TestGetQuizByID_Admin_KhongBiKhoa(t *testing.T) {
	lessonID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	// isAdmin=true lam canViewQuizAnswerKey tra true NGAY, khong cham toi lessonRepo/sectionRepo/
	// courseRepo/enrollmentRepo — de map lesson RONG cung phai qua duoc, tu do khang dinh dung
	// nhanh bypass.
	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{}}

	s := NewQuizService(quizRepo, nil, nil, nil, lessonRepo, nil, nil)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, uuid.New(), true)

	if err != nil {
		t.Fatalf("err = %v, muon nil (admin khong bi khoa)", err)
	}
	if result.Questions[0].Answers[0].IsCorrect == nil {
		t.Fatal("IsCorrect bi an du la admin — admin phai xem duoc dap an dung")
	}
}

// TestGetQuestionsByQuiz_BaiPreview_TraDuLieu_DuChuaEnroll (case 4): bai preview khong bao gio bi
// khoa, du chua enroll.
func TestGetQuestionsByQuiz_BaiPreview_TraDuLieu_DuChuaEnroll(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: sectionID, IsPreview: true}
	lesson.ID = lessonID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true, InstructorID: uuid.New()}}
	// enrollmentRepo KHONG duoc goi toi khi bai la preview (checkLessonQuizAccess tra som) —
	// enrollment de nil/order rong van phai qua duoc neu dung luat.
	enrollmentRepo := &fakeEnrollmentRepoForLock{courseID: courseID}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	result, err := s.GetQuestionsByQuiz(context.Background(), quiz.ID, uuid.New(), false)

	if err != nil {
		t.Fatalf("err = %v, muon nil (bai preview khong bao gio bi khoa)", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, muon 1", len(result))
	}
}

// TestGetQuizByID_KhoaSequential_BaiTruocChuaXong_TraErrLessonLocked (case 6): da enroll nhung
// khoa bat sequential va bai TRUOC chua completed => van phai khoa (previous_incomplete).
func TestGetQuizByID_KhoaSequential_BaiTruocChuaXong_TraErrLessonLocked(t *testing.T) {
	prevLessonID := uuid.New()
	lessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: sectionID, IsPreview: false}
	lesson.ID = lessonID

	quizRepo := &fakeQuizRepoForAccessLock{quiz: quiz}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true, InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: &model.Enrollment{}, // da enroll
		order: []repository.LessonOrderInfo{
			{ID: prevLessonID, IsPreview: false},
			{ID: lessonID, IsPreview: false},
		},
		progress: map[uuid.UUID]*model.LessonProgress{}, // prevLessonID chua co progress -> chua completed
	}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	result, err := s.GetQuizByID(context.Background(), quiz.ID, uuid.New(), false)

	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked (bai truoc chua completed o khoa sequential)", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil", result)
	}
}

// TestGetAllQuizzes_LocQuizCuaBaiKhoa (case 5): GetAllQuizzes phai LOAI quiz cua bai dang khoa
// doi voi nguoi goi ra khoi danh sach — KHONG tra loi ca request (khac han GetQuizByID/
// GetQuestionsByQuiz).
func TestGetAllQuizzes_LocQuizCuaBaiKhoa(t *testing.T) {
	lockedLessonID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()

	lockedLesson := &model.Lesson{SectionID: sectionID, IsPreview: false}
	lockedLesson.ID = lockedLessonID

	lockedQuiz := *quizVoiMotCauHoi(lockedLessonID)
	lockedQuiz.Title = "Quiz cua bai khoa"

	// openQuiz: khong gan LessonID (gan CourseID) — checkLessonQuizAccess hoan toan bo qua
	// (chi ap dung cho quiz co LessonID), nen luon xuat hien trong danh sach.
	openQuiz := model.Quiz{Title: "Quiz mo", CourseID: &courseID}
	openQuiz.ID = uuid.New()

	quizRepo := &fakeQuizRepoForAccessLock{
		quizzes: []model.Quiz{lockedQuiz, openQuiz},
		total:   2,
	}
	lessonRepo := &fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lockedLessonID: lockedLesson}}
	sectionRepo := &fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: false, InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil, // chua enroll -> lockedQuiz bi khoa
		order:      []repository.LessonOrderInfo{{ID: lockedLessonID, IsPreview: false}},
	}

	s := NewQuizService(quizRepo, nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo)

	list, err := s.GetAllQuizzes(context.Background(), nil, nil, nil, uuid.New(), false, 1, 10)

	if err != nil {
		t.Fatalf("err = %v, muon nil (quiz bi khoa phai bi LOC, khong phai LOI)", err)
	}
	if len(list.Data) != 1 {
		t.Fatalf("len(Data) = %d, muon 1 (chi con quiz mo) — Data: %+v", len(list.Data), list.Data)
	}
	if list.Data[0].Title != "Quiz mo" {
		t.Fatalf("Data[0].Title = %q, muon %q — quiz cua bai khoa khong duoc lot qua", list.Data[0].Title, "Quiz mo")
	}
}

// TestStartQuiz_BaiKhoa_TraErrLessonLocked_KhongTaoAttempt: POST /quizzes/:id/start tra ve toan bo
// text cau hoi + phuong an, nen la loi vong qua ban va GetQuizByID/GetQuestionsByQuiz neu khong
// qua cung cong khoa. Fake repo khong cai CreateAttempt (interface nhung nil): neu cong khoa bi go,
// StartQuiz se goi CreateAttempt va panic — test khong the xanh gia.
func TestStartQuiz_BaiKhoa_TraErrLessonLocked_KhongTaoAttempt(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	quiz := quizVoiMotCauHoi(lessonID)

	lesson := &model.Lesson{SectionID: uuid.New(), IsPreview: false}
	lesson.ID = lessonID

	s := NewQuizService(
		&fakeQuizRepoForAccessLock{quiz: quiz},
		nil,
		&fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New()}},
		&fakeSectionRepoForQuizLock{section: &model.Section{CourseID: courseID}},
		&fakeLessonRepoByID{lessons: map[uuid.UUID]*model.Lesson{lessonID: lesson}},
		nil,
		&fakeEnrollmentRepoForLock{
			courseID:   courseID,
			enrollment: nil, // chua enroll
			order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
		},
	)

	result, err := s.StartQuiz(context.Background(), quiz.ID, uuid.New(), false, dto.StartQuizDTO{})
	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, muon nil khi bai dang khoa", result)
	}
}
