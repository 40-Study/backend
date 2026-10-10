package service

// Test cho QA T3 (261008): câu choice/true_false lưu được với 0 đáp án đúng.
//
// Mutation muốn bắt:
//   - bỏ lời gọi validateQuestionAnswers trong CreateQuestion / UpdateQuestion / BulkCreateQuestions
//     phải làm test tương ứng ĐỎ (repo giả đếm số lần ghi);
//   - đổi `correct != 1` thành `correct < 1` làm TestValidateQuestionAnswers_Bang đỏ ở ca 2 đáp án đúng.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func answers(correctFlags ...bool) []dto.CreateAnswerDTO {
	out := make([]dto.CreateAnswerDTO, len(correctFlags))
	for i, c := range correctFlags {
		out[i] = dto.CreateAnswerDTO{AnswerText: "a", IsCorrect: c, DisplayOrder: i}
	}
	return out
}

func TestValidateQuestionAnswers_Bang(t *testing.T) {
	cases := []struct {
		name    string
		qType   string
		answers []dto.CreateAnswerDTO
		wantErr bool
	}{
		{"single 0 đúng", "single_choice", answers(false, false, false, false), true},
		{"single 1 đúng", "single_choice", answers(true, false), false},
		{"single 2 đúng", "single_choice", answers(true, true), true},
		{"single không đáp án", "single_choice", nil, true},
		{"true_false 0 đúng", "true_false", answers(false, false), true},
		{"true_false 1 đúng", "true_false", answers(false, true), false},
		{"true_false 2 đúng", "true_false", answers(true, true), true},
		{"multiple 0 đúng", "multiple_choice", answers(false, false), true},
		{"multiple 1 đúng", "multiple_choice", answers(true, false), false},
		{"multiple 3 đúng", "multiple_choice", answers(true, true, true), false},
		{"fill_blank không bị kiểm", "fill_blank", answers(false), false},
		{"essay không bị kiểm", "essay", nil, false},
	}
	for _, tc := range cases {
		err := validateQuestionAnswers(tc.qType, tc.answers)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v, wantErr=%v", tc.name, err, tc.wantErr)
		}
		if err != nil && !errors.Is(err, ErrInvalidQuestionAnswers) {
			t.Errorf("%s: lỗi phải bọc ErrInvalidQuestionAnswers, nhận %v", tc.name, err)
		}
	}
}

// repoQuestionWrites: repo giả đếm số lần GHI để chứng minh yêu cầu bị từ chối TRƯỚC khi chạm DB.
type repoQuestionWrites struct {
	repository.QuizRepositoryInterface
	quiz            *model.Quiz
	existing        *model.Question
	createQuestions int
	updateQuestions int
	createAnswers   int
}

func (r *repoQuestionWrites) GetQuizByID(ctx context.Context, id uuid.UUID) (*model.Quiz, error) {
	return r.quiz, nil
}
func (r *repoQuestionWrites) GetQuestionByID(ctx context.Context, id uuid.UUID) (*model.Question, error) {
	return r.existing, nil
}
func (r *repoQuestionWrites) CreateQuestion(ctx context.Context, q *model.Question) error {
	r.createQuestions++
	return nil
}
func (r *repoQuestionWrites) UpdateQuestion(ctx context.Context, q *model.Question) error {
	r.updateQuestions++
	return nil
}
func (r *repoQuestionWrites) CreateAnswers(ctx context.Context, a []model.QuestionAnswer) error {
	r.createAnswers++
	return nil
}
func (r *repoQuestionWrites) DeleteAnswersByQuestionID(ctx context.Context, id uuid.UUID) error {
	return nil
}

func newQuestionFixture() (*QuizService, *repoQuestionWrites, uuid.UUID) {
	quizID := uuid.New()
	repo := &repoQuestionWrites{quiz: &model.Quiz{}}
	repo.quiz.ID = quizID
	return NewQuizService(repo, nil, nil, nil, nil, nil, nil), repo, quizID
}

func TestCreateQuestion_KhongCoDapAnDung_BiTuChoiVaKhongGhi(t *testing.T) {
	s, repo, quizID := newQuestionFixture()
	_, err := s.CreateQuestion(context.Background(), quizID, uuid.New(), true, dto.CreateQuestionDTO{
		QuestionText: "q", QuestionType: "single_choice", Answers: answers(false, false, false, false),
	})
	if !errors.Is(err, ErrInvalidQuestionAnswers) {
		t.Fatalf("muốn ErrInvalidQuestionAnswers, nhận %v", err)
	}
	if repo.createQuestions != 0 || repo.createAnswers != 0 {
		t.Errorf("không được ghi DB khi bị từ chối: createQuestions=%d createAnswers=%d", repo.createQuestions, repo.createAnswers)
	}
}

func TestCreateQuestion_MotDapAnDung_DuocLuu(t *testing.T) {
	s, repo, quizID := newQuestionFixture()
	if _, err := s.CreateQuestion(context.Background(), quizID, uuid.New(), true, dto.CreateQuestionDTO{
		QuestionText: "q", QuestionType: "single_choice", Answers: answers(false, true),
	}); err != nil {
		t.Fatalf("câu hợp lệ bị từ chối: %v", err)
	}
	if repo.createQuestions != 1 {
		t.Errorf("createQuestions=%d, muốn 1", repo.createQuestions)
	}
}

func TestBulkCreateQuestions_CauThuHaiSai_KhongGhiCauNaoCa(t *testing.T) {
	s, repo, quizID := newQuestionFixture()
	_, err := s.BulkCreateQuestions(context.Background(), quizID, uuid.New(), true, dto.BulkCreateQuestionsDTO{
		Questions: []dto.CreateQuestionDTO{
			{QuestionText: "ok", QuestionType: "single_choice", Answers: answers(true, false)},
			{QuestionText: "bad", QuestionType: "true_false", Answers: answers(false, false)},
		},
	})
	if !errors.Is(err, ErrInvalidQuestionAnswers) {
		t.Fatalf("muốn ErrInvalidQuestionAnswers, nhận %v", err)
	}
	if repo.createQuestions != 0 {
		t.Errorf("bulk phải kiểm hết TRƯỚC khi ghi: createQuestions=%d, muốn 0", repo.createQuestions)
	}
}

func existingQuestion(quizID uuid.UUID, qType string, flags ...bool) *model.Question {
	q := &model.Question{QuizID: quizID, QuestionType: qType}
	for i, f := range flags {
		q.Answers = append(q.Answers, model.QuestionAnswer{AnswerText: "a", IsCorrect: f, DisplayOrder: i})
	}
	return q
}

func TestUpdateQuestion_GhiDeDapAnKhongCoDung_BiTuChoi(t *testing.T) {
	s, repo, quizID := newQuestionFixture()
	repo.existing = existingQuestion(quizID, "single_choice", true, false)
	_, err := s.UpdateQuestion(context.Background(), quizID, uuid.New(), uuid.New(), true, dto.UpdateQuestionDTO{
		Answers: answers(false, false),
	})
	if !errors.Is(err, ErrInvalidQuestionAnswers) {
		t.Fatalf("muốn ErrInvalidQuestionAnswers, nhận %v", err)
	}
	if repo.updateQuestions != 0 || repo.createAnswers != 0 {
		t.Errorf("không được ghi DB: updateQuestions=%d createAnswers=%d", repo.updateQuestions, repo.createAnswers)
	}
}

func TestUpdateQuestion_DoiLoaiSangTrueFalse_VoiDapAnCuNhieuDung_BiTuChoi(t *testing.T) {
	s, repo, quizID := newQuestionFixture()
	repo.existing = existingQuestion(quizID, "multiple_choice", true, true)
	tf := "true_false"
	_, err := s.UpdateQuestion(context.Background(), quizID, uuid.New(), uuid.New(), true, dto.UpdateQuestionDTO{QuestionType: &tf})
	if !errors.Is(err, ErrInvalidQuestionAnswers) {
		t.Fatalf("đổi loại mà đáp án cũ có 2 đúng phải bị từ chối, nhận %v", err)
	}
}

func TestUpdateQuestion_ChiSuaChu_CauCuKhongCoDapAnDung_VanDuocSua(t *testing.T) {
	s, repo, quizID := newQuestionFixture()
	repo.existing = existingQuestion(quizID, "single_choice", false, false) // dữ liệu cũ đã hỏng
	text := "new text"
	if _, err := s.UpdateQuestion(context.Background(), quizID, uuid.New(), uuid.New(), true, dto.UpdateQuestionDTO{QuestionText: &text}); err != nil {
		t.Fatalf("sửa riêng text không được bị chặn bởi dữ liệu cũ: %v", err)
	}
	if repo.updateQuestions != 1 {
		t.Errorf("updateQuestions=%d, muốn 1", repo.updateQuestions)
	}
}

// CountQuestionsByQuizIDs (QA T10): danh sách quiz giờ đếm câu hỏi qua repo; fake này không có câu hỏi nào.
func (r *repoQuestionWrites) CountQuestionsByQuizIDs(context.Context, []uuid.UUID) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}
