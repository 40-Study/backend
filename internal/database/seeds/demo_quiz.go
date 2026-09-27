package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// SeedDemoQuiz (S-P1-3, QA 260927): seed cũ (scripts/seed-local-demo-260915.sql) trỏ course_id/
// lesson_id bằng UUID CỐ ĐỊNH ghi cứng trong file SQL. Khi khoá học được re-seed (Go seeder tạo
// lại courses/lessons), UUID đổi nhưng file SQL không đổi theo -> quiz trỏ vào course/lesson
// KHÔNG CÒN TỒN TẠI (404 khi gọi API thật — verify-260927-student-admin.md, mục S-P1-3). Seed
// này tra course theo SLUG ("git-github-cho-nguoi-moi-bat-dau" — khoá cả student1 và student2 đều
// đã enroll, xem demoEnrollments) và LẤY BÀI HỌC ĐẦU TIÊN theo thứ tự thật của khoá đó (dùng lại
// courseLessonsInOrder — cùng helper mà seedLessonProgress dùng), nên luôn trỏ đúng dù course/
// lesson UUID có đổi qua các lần seed khác nhau.
func (s *Seeder) SeedDemoQuiz(courses map[string]model.Course) error {
	log.Println("Seeding demo quiz...")

	course, ok := courses["git-github-cho-nguoi-moi-bat-dau"]
	if !ok {
		return fmt.Errorf("course git-github-cho-nguoi-moi-bat-dau not found for quiz seed")
	}

	lessons, err := s.courseLessonsInOrder(course.ID)
	if err != nil {
		return err
	}
	if len(lessons) == 0 {
		log.Println("Warning: khoá Git & GitHub chưa có bài học nào, bỏ qua seed quiz")
		return nil
	}
	lesson := lessons[0]

	quiz := model.Quiz{
		LessonID:           &lesson.ID,
		CourseID:           &course.ID,
		Title:               "Kiểm tra: Cài đặt và cấu hình Git",
		Description:         ptr("Ba câu hỏi về cấu hình Git lần đầu."),
		TimeLimitMins:       ptr(15),
		PassPercentage:      rating(70),
		MaxAttempts:         ptr(5),
		TriggerType:         "manual",
		ShuffleQuestions:    true,
		ShuffleAnswers:      true,
		ShowCorrectAnswers:  true,
	}

	// Tra theo (course_id, lesson_id, title) — khoá tự nhiên — thay vì UUID cố định như seed cũ,
	// để idempotent bất kể lần seed nào sinh ra UUID nào cho course/lesson.
	if err := s.db.Where("course_id = ? AND lesson_id = ? AND title = ?", course.ID, lesson.ID, quiz.Title).
		Attrs(quiz).
		FirstOrCreate(&quiz).Error; err != nil {
		return fmt.Errorf("failed to seed quiz: %w", err)
	}

	if err := s.seedQuizQuestions(quiz.ID); err != nil {
		return err
	}

	log.Println("Seeded demo quiz cho khoá Git & GitHub")
	return nil
}

type demoAnswerSpec struct {
	Text      string
	IsCorrect bool
	Order     int
}

type demoQuestionSpec struct {
	Text    string
	Type    string // phải khớp CHECK constraint question_type (internal/model/quiz.go)
	Order   int
	Answers []demoAnswerSpec
}

// demoQuizQuestions phủ 3 loại câu hỏi hệ thống hỗ trợ, nội dung bám chủ đề "cài đặt & cấu hình
// Git" khớp tên quiz — cùng ý tưởng với seed SQL cũ (scripts/seed-local-demo-260915.sql) nhưng
// không còn phụ thuộc UUID cố định.
var demoQuizQuestions = []demoQuestionSpec{
	{
		Text:  "Lệnh nào dùng để khởi tạo một Git repository mới trong thư mục hiện tại?",
		Type:  "single_choice",
		Order: 1,
		Answers: []demoAnswerSpec{
			{Text: "git init", IsCorrect: true, Order: 1},
			{Text: "git start", IsCorrect: false, Order: 2},
			{Text: "git new", IsCorrect: false, Order: 3},
			{Text: "git create", IsCorrect: false, Order: 4},
		},
	},
	{
		Text:  "`git clone` tải về TOÀN BỘ lịch sử commit của repository, không chỉ phiên bản mới nhất.",
		Type:  "true_false",
		Order: 2,
		Answers: []demoAnswerSpec{
			{Text: "Đúng", IsCorrect: true, Order: 1},
			{Text: "Sai", IsCorrect: false, Order: 2},
		},
	},
	{
		Text:  "Những lệnh nào dưới đây liên quan tới việc đưa thay đổi lên remote repository? (chọn nhiều đáp án)",
		Type:  "multiple_choice",
		Order: 3,
		Answers: []demoAnswerSpec{
			{Text: "git push", IsCorrect: true, Order: 1},
			{Text: "git commit", IsCorrect: false, Order: 2},
			{Text: "git remote add origin <url>", IsCorrect: false, Order: 3},
			{Text: "git push origin main", IsCorrect: true, Order: 4},
		},
	},
}

func (s *Seeder) seedQuizQuestions(quizID uuid.UUID) error {
	for _, q := range demoQuizQuestions {
		question := model.Question{
			QuizID:       quizID,
			QuestionText: q.Text,
			QuestionType: q.Type,
			Points:       rating(1),
			DisplayOrder: q.Order,
		}
		if err := s.db.Where("quiz_id = ? AND display_order = ?", quizID, q.Order).
			Attrs(question).
			FirstOrCreate(&question).Error; err != nil {
			return fmt.Errorf("failed to seed question %d for quiz %s: %w", q.Order, quizID, err)
		}

		for _, a := range q.Answers {
			answer := model.QuestionAnswer{
				QuestionID:   question.ID,
				AnswerText:   a.Text,
				IsCorrect:    a.IsCorrect,
				DisplayOrder: a.Order,
			}
			if err := s.db.Where("question_id = ? AND display_order = ?", question.ID, a.Order).
				Attrs(answer).
				FirstOrCreate(&answer).Error; err != nil {
				return fmt.Errorf("failed to seed answer %d for question %s: %w", a.Order, question.ID, err)
			}
		}
	}
	return nil
}
