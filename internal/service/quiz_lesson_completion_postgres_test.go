package service

// H1 (review QA follow-up 261009): bài học mà nội dung chính là một quiz không có chỗ nào để hoàn thành — web
// không có nút, SubmitQuiz không ghi tiến độ — nên khoá tuần tự bị khoá vĩnh viễn từ bài đó, tiến độ khoá không
// bao giờ tới 100% và không có chứng chỉ. Server chốt: một lần làm quiz CHÍNH THỨC ĐỖ của quiz gắn bài đó (quiz.lesson_id,
// cũng là dòng lesson_contents type=quiz) đánh dấu bài completed. Đỗ trượt/luyện tập/người không ghi danh không chốt gì.
//
// Mutation đã thử (mỗi dòng làm ít nhất một test ĐỎ): bỏ lời gọi lessonCompleter trong SubmitQuiz; bỏ điều kiện
// mode official; bỏ điều kiện isPassed; bỏ `progress.Status == "completed"` (idempotent); bỏ recalculateProgress.

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type quizCompletionWorld struct {
	t       *testing.T
	db      *gorm.DB
	quiz    *QuizService
	enroll  *EnrollmentService
	enrollR repository.EnrollmentRepositoryInterface
	teacher model.User
	student model.User
	course  model.Course
	lessons []model.Lesson
	// quiz gắn lessons[0]
	quizID  uuid.UUID
	qID     uuid.UUID
	correct uuid.UUID
	wrong   uuid.UUID
}

// newQuizCompletionWorld dựng khoá (sequential nếu được yêu cầu) với lessonCount bài bắt buộc; quiz 1 câu gắn bài đầu
// (cả quiz.lesson_id lẫn dòng lesson_contents type=quiz, đúng như luồng soạn bài và backfill tạo ra), học viên đã ghi danh.
func newQuizCompletionWorld(t *testing.T, lessonCount int, sequential bool) *quizCompletionWorld {
	t.Helper()
	db := isolatedAPISchema(t)
	f := &s2Fixture{t: t, db: db}
	w := &quizCompletionWorld{t: t, db: db, teacher: f.user("teacher"), student: f.user("student")}
	courseRepo, sectionRepo, lessonRepo := repository.NewCourseRepository(db), repository.NewSectionRepository(db), repository.NewLessonRepository(db)
	w.enrollR = repository.NewEnrollmentRepository(db)
	w.quiz = NewQuizService(repository.NewQuizRepository(db), nil, courseRepo, sectionRepo, lessonRepo, nil, w.enrollR)
	w.enroll = NewEnrollmentService(w.enrollR, courseRepo, lessonRepo, nil)
	w.quiz.SetLessonCompleter(w.enroll)

	w.course = f.course(w.teacher)
	if sequential {
		w.course.Sequential = true
		if err := db.Model(&w.course).Update("sequential", true).Error; err != nil {
			t.Fatalf("bật sequential: %v", err)
		}
	}
	sec := model.Section{CourseID: w.course.ID, Title: "Chương", DisplayOrder: 1}
	if err := db.Create(&sec).Error; err != nil {
		t.Fatalf("tạo chương: %v", err)
	}
	for i := 1; i <= lessonCount; i++ {
		l := model.Lesson{SectionID: sec.ID, Title: "Bài", DisplayOrder: i}
		if err := db.Create(&l).Error; err != nil {
			t.Fatalf("tạo bài: %v", err)
		}
		w.lessons = append(w.lessons, l)
	}
	if err := db.Create(&model.Enrollment{UserID: w.student.ID, CourseID: w.course.ID}).Error; err != nil {
		t.Fatalf("ghi danh: %v", err)
	}

	lessonID := w.lessons[0].ID
	q := model.Quiz{LessonID: &lessonID, Title: "QA quiz bài", TriggerType: "manual", CreatedBy: &w.teacher.ID,
		PassPercentage: decimal.NewFromInt(70)}
	if err := db.Create(&q).Error; err != nil {
		t.Fatalf("tạo quiz: %v", err)
	}
	w.quizID = q.ID
	question := model.Question{QuizID: q.ID, QuestionText: "1+1?", QuestionType: "single_choice", Points: decimal.NewFromInt(1), DisplayOrder: 1}
	if err := db.Create(&question).Error; err != nil {
		t.Fatalf("tạo câu hỏi: %v", err)
	}
	w.qID = question.ID
	answers := []model.QuestionAnswer{
		{QuestionID: question.ID, AnswerText: "2", IsCorrect: true, DisplayOrder: 1},
		{QuestionID: question.ID, AnswerText: "3", DisplayOrder: 2},
	}
	if err := db.Create(&answers).Error; err != nil {
		t.Fatalf("tạo đáp án: %v", err)
	}
	w.correct, w.wrong = answers[0].ID, answers[1].ID
	title := "Kiểm tra"
	if err := db.Create(&model.LessonContent{LessonID: lessonID, Type: "quiz", Title: &title, QuizID: &q.ID}).Error; err != nil {
		t.Fatalf("tạo nội dung quiz: %v", err)
	}
	return w
}

// attempt bắt đầu rồi nộp một lần làm quiz với mode và đáp án đã chọn; trả kết quả chấm.
func (w *quizCompletionWorld) attempt(userID uuid.UUID, mode string, answer uuid.UUID) *dto.QuizAttemptResponseDTO {
	w.t.Helper()
	ctx := context.Background()
	started, err := w.quiz.StartQuiz(ctx, w.quizID, userID, false, dto.StartQuizDTO{Mode: mode})
	if err != nil {
		w.t.Fatalf("StartQuiz(%s): %v", mode, err)
	}
	attemptID := started.AttemptID.String()
	res, err := w.quiz.SubmitQuiz(ctx, w.quizID, userID, false, dto.SubmitQuizDTO{
		AttemptID: &attemptID,
		Answers:   []dto.SubmitAnswerDTO{{QuestionID: w.qID.String(), SelectedAnswerIDs: []string{answer.String()}}},
	})
	if err != nil {
		w.t.Fatalf("SubmitQuiz(%s): %v", mode, err)
	}
	return res
}

func (w *quizCompletionWorld) progressOf(lessonID uuid.UUID) *model.LessonProgress {
	w.t.Helper()
	var rows []model.LessonProgress
	if err := w.db.Where("user_id = ? AND lesson_id = ?", w.student.ID, lessonID).Find(&rows).Error; err != nil {
		w.t.Fatalf("đọc lesson_progress: %v", err)
	}
	if len(rows) > 1 {
		w.t.Fatalf("lesson_progress có %d dòng cho cùng (user, bài), muốn tối đa 1", len(rows))
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func describeProgress(p *model.LessonProgress) string {
	if p == nil {
		return "không có dòng lesson_progress"
	}
	return fmt.Sprintf("status=%s completed_at=%v", p.Status, p.CompletedAt)
}

func (w *quizCompletionWorld) lessonLocked(lessonID uuid.UUID) bool {
	w.t.Helper()
	in, err := gatherLessonLockInput(context.Background(), w.enrollR, w.student.ID, w.course.ID, w.course.Sequential, false)
	if err != nil {
		w.t.Fatalf("gatherLessonLockInput: %v", err)
	}
	locked, _, _ := ResolveLessonLock(lessonID, in)
	return locked
}

func (w *quizCompletionWorld) enrollment() model.Enrollment {
	w.t.Helper()
	var e model.Enrollment
	if err := w.db.First(&e, "user_id = ? AND course_id = ?", w.student.ID, w.course.ID).Error; err != nil {
		w.t.Fatalf("đọc enrollment: %v", err)
	}
	return e
}

// Đỗ quiz chính thức => bài completed, bài kế của khoá tuần tự mở khoá, tiến độ khoá tính lại (1/2 bài bắt buộc = 50%).
func TestQuizPass_Official_CompletesLesson_UnlocksNextAndUpdatesCourseProgress(t *testing.T) {
	w := newQuizCompletionWorld(t, 2, true)
	if !w.lessonLocked(w.lessons[1].ID) {
		t.Fatal("tiền điều kiện: bài 2 của khoá tuần tự phải đang khoá khi bài 1 chưa xong")
	}

	res := w.attempt(w.student.ID, "official", w.correct)
	if res.IsPassed == nil || !*res.IsPassed {
		t.Fatalf("tiền điều kiện: lần làm phải đỗ, nhận %+v", res)
	}

	p := w.progressOf(w.lessons[0].ID)
	if p == nil || p.Status != "completed" || p.CompletedAt == nil {
		t.Fatalf("đỗ quiz chính thức phải chốt bài 1 completed (có completed_at), nhận %s", describeProgress(p))
	}
	if w.lessonLocked(w.lessons[1].ID) {
		t.Error("bài 2 phải mở khoá sau khi bài 1 completed")
	}
	if e := w.enrollment(); !e.ProgressPercent.Equal(decimal.NewFromInt(50)) || e.CompletedLessons != 1 || e.TotalLessons != 2 {
		t.Errorf("tiến độ khoá muốn 50%% (1/2), nhận %s%% (%d/%d)", e.ProgressPercent, e.CompletedLessons, e.TotalLessons)
	}
}

// Khoá chỉ có bài quiz: đỗ => 100% và enrollment.completed_at được chốt (điều kiện để cấp chứng chỉ).
func TestQuizPass_Official_LastLesson_CompletesCourse(t *testing.T) {
	w := newQuizCompletionWorld(t, 1, false)
	w.attempt(w.student.ID, "official", w.correct)

	e := w.enrollment()
	if !e.ProgressPercent.Equal(decimal.NewFromInt(100)) || e.CompletedAt == nil {
		t.Fatalf("khoá 1 bài quiz đỗ phải 100%% và có completed_at, nhận %s%% completed_at=%v", e.ProgressPercent, e.CompletedAt)
	}
}

// Trượt, hoặc luyện tập (kể cả đạt điểm đỗ) không chốt bài.
func TestQuizPass_FailedOrPractice_DoesNotCompleteLesson(t *testing.T) {
	w := newQuizCompletionWorld(t, 2, true)

	if res := w.attempt(w.student.ID, "official", w.wrong); res.IsPassed == nil || *res.IsPassed {
		t.Fatalf("tiền điều kiện: đáp án sai phải trượt, nhận %+v", res)
	}
	if p := w.progressOf(w.lessons[0].ID); p != nil && p.Status == "completed" {
		t.Error("lần làm chính thức TRƯỢT không được chốt bài completed")
	}

	if res := w.attempt(w.student.ID, "practice", w.correct); res.IsPassed == nil || !*res.IsPassed {
		t.Fatalf("tiền điều kiện: luyện tập đáp án đúng phải đạt điểm đỗ, nhận %+v", res)
	}
	if p := w.progressOf(w.lessons[0].ID); p != nil && p.Status == "completed" {
		t.Error("lần làm LUYỆN TẬP không được chốt bài completed dù đạt điểm đỗ")
	}
	if !w.lessonLocked(w.lessons[1].ID) {
		t.Error("bài 2 vẫn phải khoá khi bài 1 chưa completed")
	}
}

// Làm đỗ lần nữa không ghi trùng dòng và không dời completed_at.
func TestQuizPass_Official_IsIdempotent(t *testing.T) {
	w := newQuizCompletionWorld(t, 2, true)
	w.attempt(w.student.ID, "official", w.correct)
	first := w.progressOf(w.lessons[0].ID)
	if first == nil || first.CompletedAt == nil {
		t.Fatal("tiền điều kiện: lần đỗ đầu phải chốt bài")
	}

	w.attempt(w.student.ID, "official", w.correct)
	second := w.progressOf(w.lessons[0].ID) // fail nếu >1 dòng
	if second.Status != "completed" || !second.CompletedAt.Equal(*first.CompletedAt) {
		t.Errorf("đỗ lần hai phải giữ nguyên completed_at=%v, nhận status=%s completed_at=%v", first.CompletedAt, second.Status, second.CompletedAt)
	}
}

// Bài đã có dòng tiến độ in_progress (đã mở bài) được nâng lên completed, không tạo dòng mới.
func TestQuizPass_Official_UpgradesExistingInProgressRow(t *testing.T) {
	w := newQuizCompletionWorld(t, 2, true)
	enr := w.enrollment()
	if err := w.db.Create(&model.LessonProgress{UserID: w.student.ID, LessonID: w.lessons[0].ID, EnrollmentID: enr.ID, Status: "in_progress"}).Error; err != nil {
		t.Fatalf("tạo tiến độ in_progress: %v", err)
	}

	w.attempt(w.student.ID, "official", w.correct)
	p := w.progressOf(w.lessons[0].ID)
	if p == nil || p.Status != "completed" || p.CompletedAt == nil {
		t.Fatalf("dòng in_progress phải thành completed (có completed_at), nhận %s", describeProgress(p))
	}
}

// Chủ khoá làm thử quiz của chính mình (không ghi danh) đỗ: không lỗi và không sinh tiến độ.
func TestQuizPass_Official_NotEnrolled_IsNoOp(t *testing.T) {
	w := newQuizCompletionWorld(t, 1, false)
	w.attempt(w.teacher.ID, "official", w.correct) // fail nếu SubmitQuiz trả lỗi
	var n int64
	if err := w.db.Model(&model.LessonProgress{}).Where("user_id = ?", w.teacher.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("người không ghi danh không được có lesson_progress, nhận %d dòng", n)
	}
}
