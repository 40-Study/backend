package service

// Lane B2 "Cuộc thi" — contract §9 B2 (a), (b), (f): khoá quiz gắn cuộc thi và cột quizzes.created_by.
// Postgres thật; gate là fakeContestGate (ContestService thật thuộc lane B1).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// (f) created_by được ghi khi tạo/nhân bản, và GET /me/quizzes chỉ trả quiz của chính người gọi.
// Trước bản vá, GetQuizzesByCreator trả MỌI quiz của hệ thống cho bất kỳ ai (IDOR).
func TestQuizCreatedBy_GhiNguoiTao_MyQuizzesChiTraCuaMinh(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	teacherA, teacherB := f.user("teacher-a"), f.user("teacher-b")

	created, err := f.quiz.CreateQuiz(ctx, teacherA, dto.CreateQuizDTO{Title: "Quiz của A"})
	if err != nil {
		t.Fatalf("CreateQuiz: %v", err)
	}
	quizB := f.standaloneQuiz(teacherB, false)
	// A nhân bản quiz của chính A: bản sao cũng thuộc A. (A nhân bản quiz của B bị chặn, xem
	// TestQuizOwnership_NguoiLaKhongSuaXoaNhanBan.)
	copied, err := f.quiz.DuplicateQuiz(ctx, created.ID, teacherA, false)
	if err != nil {
		t.Fatalf("DuplicateQuiz: %v", err)
	}

	for _, id := range []uuid.UUID{created.ID, copied.ID} {
		var q model.Quiz
		if err := f.db.First(&q, "id = ?", id).Error; err != nil {
			t.Fatalf("đọc quiz: %v", err)
		}
		if q.CreatedBy == nil || *q.CreatedBy != teacherA {
			t.Errorf("quiz %s: created_by = %v, muốn %s", id, q.CreatedBy, teacherA)
		}
	}

	mine, err := f.quiz.GetMyCreatedQuizzes(ctx, teacherA, 1, 50)
	if err != nil {
		t.Fatalf("GetMyCreatedQuizzes: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, q := range mine.Data {
		got[q.ID] = true
	}
	if len(mine.Data) != 2 || !got[created.ID] || !got[copied.ID] || mine.Total != 2 {
		t.Errorf("quiz của A phải đúng 2 (tạo + nhân bản), nhận %d (total %d): %v", len(mine.Data), mine.Total, got)
	}
	if got[quizB.ID] {
		t.Errorf("GET /me/quizzes của A lộ quiz của B (IDOR)")
	}
}

// (a) Mọi đường ĐỌC quiz gắn cuộc thi trả ErrQuizLockedByContest cho học viên; người tạo cuộc thi và
// admin vẫn dùng được; GET /quizzes loại quiz đó khỏi danh sách với học viên.
func TestContestGate_DocQuiz_HocVienBiKhoa_ChuVaAdminDuoc(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, student, admin := f.user("owner"), f.user("student"), f.user("admin")
	cq := f.standaloneQuiz(owner, false)
	f.quiz.SetContestGate(&fakeContestGate{locked: map[uuid.UUID]fakeContestLock{cq.ID: {creator: owner}}})

	// Attempt thường của người tạo — để thử các đường đọc theo attempt_id.
	started, err := f.quiz.StartQuiz(ctx, cq.ID, owner, false, dto.StartQuizDTO{Mode: "practice"})
	if err != nil {
		t.Fatalf("người tạo cuộc thi phải start được: %v", err)
	}
	attemptID := started.AttemptID

	reads := map[string]func(uid uuid.UUID, isAdmin bool) error{
		"GetQuizByID": func(uid uuid.UUID, a bool) error { _, err := f.quiz.GetQuizByID(ctx, cq.ID, uid, a); return err },
		"GetQuestionsByQuiz": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.GetQuestionsByQuiz(ctx, cq.ID, uid, a)
			return err
		},
		"StartQuiz": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.StartQuiz(ctx, cq.ID, uid, a, dto.StartQuizDTO{Mode: "practice"})
			return err
		},
		"GetMyAttempts":     func(uid uuid.UUID, a bool) error { _, err := f.quiz.GetMyAttempts(ctx, cq.ID, uid, a); return err },
		"GetQuizResults":    func(uid uuid.UUID, a bool) error { _, err := f.quiz.GetQuizResults(ctx, cq.ID, uid, a); return err },
		"GetQuizStatistics": func(uid uuid.UUID, a bool) error { _, err := f.quiz.GetQuizStatistics(ctx, cq.ID, uid, a); return err },
		"DuplicateQuiz":     func(uid uuid.UUID, a bool) error { _, err := f.quiz.DuplicateQuiz(ctx, cq.ID, uid, a); return err },
		"SubmitQuiz": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.SubmitQuiz(ctx, cq.ID, uid, a, dto.SubmitQuizDTO{Answers: []dto.SubmitAnswerDTO{}})
			return err
		},
	}
	for name, call := range reads {
		if err := call(student, false); !errors.Is(err, ErrQuizLockedByContest) {
			t.Errorf("%s với học viên: muốn ErrQuizLockedByContest, nhận %v", name, err)
		}
		if name == "SubmitQuiz" {
			continue // chủ/admin không có attempt dang dở ở đây — chỉ cần chắc học viên bị chặn.
		}
		if err := call(owner, false); err != nil {
			t.Errorf("%s với người tạo cuộc thi: muốn thành công, nhận %v", name, err)
		}
		if err := call(admin, true); err != nil {
			t.Errorf("%s với admin: muốn thành công, nhận %v", name, err)
		}
	}

	// Các đường theo attempt_id (GET /quizzes/:id/attempts/:attemptId, /attempts/:id/progress,
	// /attempts/:id/save-answer): học viên nhận 404 (không lộ attempt có tồn tại, S2/m4).
	if _, err := f.quiz.GetAttemptByID(ctx, attemptID, student, false); !errors.Is(err, ErrQuizAttemptNotFound) {
		t.Errorf("GetAttemptByID với học viên: muốn ErrQuizAttemptNotFound (S2/m4), nhận %v", err)
	}
	if _, err := f.quiz.GetAttemptProgress(ctx, attemptID, student, false); !errors.Is(err, ErrQuizAttemptNotFound) {
		t.Errorf("GetAttemptProgress với học viên: muốn ErrQuizAttemptNotFound (S2/m4), nhận %v", err)
	}
	save := dto.SaveAnswerDTO{QuestionID: cq.SingleQ.String()}
	if err := f.quiz.SaveAnswer(ctx, attemptID, student, false, save); !errors.Is(err, ErrQuizAttemptNotFound) {
		t.Errorf("SaveAnswer với học viên: muốn ErrQuizAttemptNotFound, nhận %v", err)
	}
	if _, err := f.quiz.GetAttemptByID(ctx, attemptID, owner, false); err != nil {
		t.Errorf("GetAttemptByID với người tạo: %v", err)
	}

	listed := func(uid uuid.UUID, isAdmin bool) bool {
		list, err := f.quiz.GetAllQuizzes(ctx, nil, nil, nil, uid, isAdmin, 1, 50)
		if err != nil {
			t.Fatalf("GetAllQuizzes: %v", err)
		}
		for _, q := range list.Data {
			if q.ID == cq.ID {
				return true
			}
		}
		return false
	}
	if listed(student, false) {
		t.Errorf("GET /quizzes của học viên vẫn liệt kê quiz gắn cuộc thi")
	}
	if !listed(owner, false) || !listed(admin, true) {
		t.Errorf("GET /quizzes của người tạo/admin phải liệt kê quiz gắn cuộc thi")
	}
}

// (b) Cuộc thi đang PENDING_REVIEW/PUBLISHED/CANCELLED: MỌI thao tác sửa quiz/câu hỏi trả
// ErrQuizEditLockedByContest với cả người tạo lẫn admin; học viên vẫn nhận 403 trước.
// Khi cuộc thi còn DRAFT (editable) thì người tạo sửa được bình thường.
func TestContestGate_SuaQuizKhiCuocThiDaCongBo_BiChan(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, student, admin := f.user("owner"), f.user("student"), f.user("admin")
	cq := f.standaloneQuiz(owner, false)
	gate := &fakeContestGate{locked: map[uuid.UUID]fakeContestLock{cq.ID: {creator: owner}}}
	f.quiz.SetContestGate(gate)

	title := "Tên mới"
	text := "Câu đã sửa"
	edits := map[string]func(uid uuid.UUID, isAdmin bool) error{
		"UpdateQuiz": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.UpdateQuiz(ctx, cq.ID, uid, a, dto.UpdateQuizDTO{Title: &title})
			return err
		},
		"CreateQuestion": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.CreateQuestion(ctx, cq.ID, uid, a, dto.CreateQuestionDTO{QuestionText: "Mới", QuestionType: "essay"})
			return err
		},
		"BulkCreateQuestions": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.BulkCreateQuestions(ctx, cq.ID, uid, a, dto.BulkCreateQuestionsDTO{
				Questions: []dto.CreateQuestionDTO{{QuestionText: "Mới", QuestionType: "essay"}}})
			return err
		},
		"UpdateQuestion": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.UpdateQuestion(ctx, cq.ID, cq.SingleQ, uid, a, dto.UpdateQuestionDTO{QuestionText: &text})
			return err
		},
		"ReorderQuestions": func(uid uuid.UUID, a bool) error {
			return f.quiz.ReorderQuestions(ctx, cq.ID, uid, a, dto.ReorderQuestionsDTO{QuestionIDs: []string{cq.FillQ.String(), cq.SingleQ.String()}})
		},
		"DeleteQuestion": func(uid uuid.UUID, a bool) error { return f.quiz.DeleteQuestion(ctx, cq.ID, cq.MultiQ, uid, a) },
		"DeleteQuiz":     func(uid uuid.UUID, a bool) error { return f.quiz.DeleteQuiz(ctx, cq.ID, uid, a) },
	}
	for name, call := range edits {
		if err := call(student, false); !errors.Is(err, ErrQuizLockedByContest) {
			t.Errorf("%s với học viên: muốn ErrQuizLockedByContest, nhận %v", name, err)
		}
		if err := call(owner, false); !errors.Is(err, ErrQuizEditLockedByContest) {
			t.Errorf("%s với người tạo khi cuộc thi đã công bố: muốn ErrQuizEditLockedByContest, nhận %v", name, err)
		}
		if err := call(admin, true); !errors.Is(err, ErrQuizEditLockedByContest) {
			t.Errorf("%s với admin khi cuộc thi đã công bố: muốn ErrQuizEditLockedByContest, nhận %v", name, err)
		}
	}
	if n := f.count("questions", "quiz_id = ?", cq.ID); n != 4 {
		t.Errorf("quiz bị khoá mà số câu hỏi đổi thành %d (muốn 4)", n)
	}
	if n := f.count("quizzes", "id = ?", cq.ID); n != 1 {
		t.Errorf("quiz bị khoá mà đã bị xoá")
	}

	// Cuộc thi còn DRAFT: người tạo sửa được.
	gate.locked[cq.ID] = fakeContestLock{creator: owner, editable: true}
	if _, err := f.quiz.UpdateQuiz(ctx, cq.ID, owner, false, dto.UpdateQuizDTO{Title: &title}); err != nil {
		t.Errorf("cuộc thi DRAFT: người tạo phải sửa được quiz, nhận %v", err)
	}
}

// Đường nộp thường (/quizzes/:id/submit) không được "nhặt" attempt contest: bài thi chỉ nộp qua
// SubmitContestAttempt (có hạn giờ cuộc thi). Không gate (trước B1) cũng phải đúng. Dùng chính người
// tạo quiz để đi qua được kiểm quyền quiz standalone (R2-A) và chạm tới bộ lọc attempt contest.
func TestSubmitQuiz_BoQuaAttemptContest(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner := f.user("owner")
	cq := f.standaloneQuiz(owner, false)
	attemptID := f.startContestAttempt(cq.ID, owner)

	if _, err := f.quiz.SubmitQuiz(ctx, cq.ID, owner, false, dto.SubmitQuizDTO{Answers: []dto.SubmitAnswerDTO{}}); err == nil || errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("nộp thường không attempt_id: phải bị từ chối vì không có attempt thường, nhận %v", err)
	}
	id := attemptID.String()
	if _, err := f.quiz.SubmitQuiz(ctx, cq.ID, owner, false, dto.SubmitQuizDTO{AttemptID: &id, Answers: []dto.SubmitAnswerDTO{}}); err == nil || errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("nộp thường kèm attempt_id contest: phải bị từ chối")
	}
	if n := f.count("quiz_attempts", "id = ? AND completed_at IS NOT NULL", attemptID); n != 0 {
		t.Errorf("attempt contest bị đánh dấu đã nộp qua đường thường")
	}
}
