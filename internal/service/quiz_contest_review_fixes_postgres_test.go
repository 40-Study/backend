package service

// Test cho các finding của review đối kháng PR #80 (plans/reports/review-260928-contest-pr80.md):
// F1 lộ đáp án fill_blank, F2 không kiểm chủ sở hữu quiz, F3 người tạo bị giấu đáp án, F4 phát
// voucher hết lượt + phát trùng dòng ví, M5 (mutation M33) engine không đối chiếu quiz của attempt.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// F1: đáp án câu fill_blank ("Hà Nội") không được xuất hiện ở bất kỳ đường nào trả đề cho người
// không có quyền xem đáp án: đề thi, /start, và GET quiz/câu hỏi. Lựa chọn của câu trắc nghiệm vẫn
// phải còn (thí sinh cần chúng để chọn).
//
// Từ re-review vòng 2 (R2-A) quiz standalone chỉ người tạo/admin đọc được qua đường thường — cả hai
// đều có quyền xem đáp án — nên các đường thường được thử trên quiz gắn khoá học với học viên đã
// enroll (người đọc hợp lệ nhưng không có quyền xem đáp án). Đề thi cuộc thi vẫn thử trên quiz
// standalone.
func TestFillBlank_KhongLoDapAn_MoiDuongTraDe(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, student := f.user("owner"), f.user("student")
	cq := f.standaloneQuiz(owner, false)
	courseID, _ := f.course(owner)
	f.enroll(student, courseID)
	lq := f.standaloneQuiz(owner, false)
	f.attachQuiz(lq.ID, "course_id", courseID)

	contest, err := f.quiz.GetContestAttemptQuestions(ctx, cq.ID, f.startContestAttempt(cq.ID, student))
	if err != nil {
		t.Fatalf("GetContestAttemptQuestions: %v", err)
	}
	started, err := f.quiz.StartQuiz(ctx, lq.ID, student, false, dto.StartQuizDTO{Mode: "practice"})
	if err != nil {
		t.Fatalf("StartQuiz: %v", err)
	}
	detail, err := f.quiz.GetQuizByID(ctx, lq.ID, student, false)
	if err != nil {
		t.Fatalf("GetQuizByID: %v", err)
	}
	questions, err := f.quiz.GetQuestionsByQuiz(ctx, lq.ID, student, false)
	if err != nil {
		t.Fatalf("GetQuestionsByQuiz: %v", err)
	}
	for name, v := range map[string]interface{}{"đề thi": contest, "/start": started, "GET quiz": detail, "GET questions": questions} {
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), "Hà Nội") {
			t.Errorf("%s lộ đáp án fill_blank: %s", name, raw)
		}
		if !strings.Contains(string(raw), `"answer_text":"X"`) {
			t.Errorf("%s mất lựa chọn của câu multiple_choice: %s", name, raw)
		}
	}
}

// F2: chỉ người tạo quiz hoặc admin được sửa, xoá, nhân bản quiz và thêm/sửa/xoá/sắp xếp câu hỏi.
// Quiz có created_by NULL (tạo trước khi có cột) chỉ admin được sửa.
func TestQuizOwnership_NguoiLaKhongSuaXoaNhanBan(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, stranger, admin := f.user("owner"), f.user("stranger"), f.user("admin")
	cq := f.standaloneQuiz(owner, false)
	legacy := f.standaloneQuiz(owner, false)
	if err := f.db.Exec("UPDATE quizzes SET created_by = NULL WHERE id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("đặt created_by NULL: %v", err)
	}

	title := "Đổi tên"
	text := "Câu đã sửa"
	ops := func(q contestQuiz) map[string]func(uid uuid.UUID, isAdmin bool) error {
		return map[string]func(uuid.UUID, bool) error{
			"UpdateQuiz": func(uid uuid.UUID, a bool) error {
				_, err := f.quiz.UpdateQuiz(ctx, q.ID, uid, a, dto.UpdateQuizDTO{Title: &title})
				return err
			},
			"DuplicateQuiz": func(uid uuid.UUID, a bool) error { _, err := f.quiz.DuplicateQuiz(ctx, q.ID, uid, a); return err },
			"CreateQuestion": func(uid uuid.UUID, a bool) error {
				_, err := f.quiz.CreateQuestion(ctx, q.ID, uid, a, dto.CreateQuestionDTO{QuestionText: "Mới", QuestionType: "essay"})
				return err
			},
			"BulkCreateQuestions": func(uid uuid.UUID, a bool) error {
				_, err := f.quiz.BulkCreateQuestions(ctx, q.ID, uid, a, dto.BulkCreateQuestionsDTO{
					Questions: []dto.CreateQuestionDTO{{QuestionText: "Mới", QuestionType: "essay"}}})
				return err
			},
			"UpdateQuestion": func(uid uuid.UUID, a bool) error {
				_, err := f.quiz.UpdateQuestion(ctx, q.ID, q.SingleQ, uid, a, dto.UpdateQuestionDTO{QuestionText: &text})
				return err
			},
			"ReorderQuestions": func(uid uuid.UUID, a bool) error {
				return f.quiz.ReorderQuestions(ctx, q.ID, uid, a, dto.ReorderQuestionsDTO{QuestionIDs: []string{q.FillQ.String(), q.SingleQ.String()}})
			},
			"DeleteQuestion": func(uid uuid.UUID, a bool) error { return f.quiz.DeleteQuestion(ctx, q.ID, q.MultiQ, uid, a) },
			"DeleteQuiz":     func(uid uuid.UUID, a bool) error { return f.quiz.DeleteQuiz(ctx, q.ID, uid, a) },
		}
	}

	for name, call := range ops(cq) {
		if err := call(stranger, false); !errors.Is(err, ErrQuizNotOwner) {
			t.Errorf("%s bởi người lạ: muốn ErrQuizNotOwner, nhận %v", name, err)
		}
	}
	for name, call := range ops(legacy) {
		if err := call(owner, false); !errors.Is(err, ErrQuizNotOwner) {
			t.Errorf("%s trên quiz created_by NULL bởi người không phải admin: muốn ErrQuizNotOwner, nhận %v", name, err)
		}
	}
	if n := f.count("questions", "quiz_id IN ?", []uuid.UUID{cq.ID, legacy.ID}); n != 8 {
		t.Errorf("người lạ làm đổi số câu hỏi thành %d (muốn 8)", n)
	}
	if n := f.count("quizzes", "created_by = ?", stranger); n != 0 {
		t.Errorf("người lạ nhân bản được %d quiz", n)
	}

	// Chủ quiz và admin làm được. Xoá quiz để cuối vì các thao tác khác cần quiz còn tồn tại.
	order := []string{"UpdateQuiz", "DuplicateQuiz", "CreateQuestion", "BulkCreateQuestions", "UpdateQuestion", "ReorderQuestions", "DeleteQuestion", "DeleteQuiz"}
	for _, name := range order {
		if err := ops(cq)[name](owner, false); err != nil {
			t.Errorf("%s bởi chủ quiz: %v", name, err)
		}
		if err := ops(legacy)[name](admin, true); err != nil {
			t.Errorf("%s trên quiz created_by NULL bởi admin: %v", name, err)
		}
	}
}

// F3: người tạo quiz standalone xem được is_correct/explanation của quiz mình; người lạ thì không.
func TestCanViewAnswerKey_NguoiTaoXemDuocQuizCuaMinh(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, stranger := f.user("owner"), f.user("stranger")
	cq := f.standaloneQuiz(owner, false)

	check := func(uid uuid.UUID, wantVisible bool) {
		t.Helper()
		detail, err := f.quiz.GetQuizByID(ctx, cq.ID, uid, false)
		if err != nil {
			t.Fatalf("GetQuizByID: %v", err)
		}
		questions, err := f.quiz.GetQuestionsByQuiz(ctx, cq.ID, uid, false)
		if err != nil {
			t.Fatalf("GetQuestionsByQuiz: %v", err)
		}
		for _, list := range [][]dto.QuestionResponseDTO{detail.Questions, questions} {
			for _, q := range list {
				if (q.Explanation != nil) != wantVisible {
					t.Errorf("user %s câu %s: explanation hiện=%v, muốn %v", uid, q.QuestionType, q.Explanation != nil, wantVisible)
				}
				for _, a := range q.Answers {
					if (a.IsCorrect != nil) != wantVisible {
						t.Errorf("user %s câu %s: is_correct hiện=%v, muốn %v", uid, q.QuestionType, a.IsCorrect != nil, wantVisible)
					}
				}
			}
		}
	}
	check(owner, true)
	// Re-review vòng 2 (R2-A): người lạ không còn đọc được quiz standalone, nên không có đường nào để
	// "giấu đáp án" — bị chặn hẳn. Nhánh giấu đáp án cho người đọc hợp lệ: TestFillBlank_... .
	if _, err := f.quiz.GetQuizByID(ctx, cq.ID, stranger, false); !errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("người lạ đọc quiz standalone: muốn ErrQuizNotOwner, nhận %v", err)
	}
	if _, err := f.quiz.GetQuestionsByQuiz(ctx, cq.ID, stranger, false); !errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("người lạ đọc câu hỏi quiz standalone: muốn ErrQuizNotOwner, nhận %v", err)
	}
}

// F4: voucher đã hết tổng lượt dùng bị từ chối; học viên đã tự lưu voucher thì phát lại dòng cũ,
// không tạo dòng trùng trong ví.
func TestGrantVoucherTx_HetLuot_VaIdempotent(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	student := f.user("student")

	exhausted := f.voucher(true, nil)
	if err := f.db.Model(&model.Voucher{}).Where("id = ?", exhausted).
		Updates(map[string]interface{}{"usage_limit": 1, "used_count": 1}).Error; err != nil {
		t.Fatalf("đặt usage_limit: %v", err)
	}
	err := f.db.Transaction(func(tx *gorm.DB) error {
		_, err := vs.GrantVoucherTx(ctx, tx, student, exhausted, model.UserVoucherSourceContestReward, "")
		return err
	})
	if !errors.Is(err, ErrVoucherUnavailableForGrant) {
		t.Errorf("voucher hết lượt: muốn ErrVoucherUnavailableForGrant, nhận %v", err)
	}

	saved := f.voucher(true, nil)
	own := model.UserVoucher{UserID: student, VoucherID: saved, Source: "manual", SavedAt: time.Now()}
	if err := f.db.Create(&own).Error; err != nil {
		t.Fatalf("học viên tự lưu voucher: %v", err)
	}
	for i := 0; i < 2; i++ {
		var got *model.UserVoucher
		err := f.db.Transaction(func(tx *gorm.DB) error {
			var err error
			got, err = vs.GrantVoucherTx(ctx, tx, student, saved, model.UserVoucherSourceContestReward, "Giải")
			return err
		})
		if err != nil {
			t.Fatalf("lần %d GrantVoucherTx: %v", i+1, err)
		}
		if got.ID != own.ID {
			t.Errorf("lần %d: phải dùng lại dòng ví đã có %s, nhận %s", i+1, own.ID, got.ID)
		}
	}
	if n := f.count("user_vouchers", "user_id = ? AND voucher_id = ?", student, saved); n != 1 {
		t.Errorf("ví có %d dòng cho cùng voucher, muốn 1", n)
	}
}

// Chủ dự án chốt 28/09: voucher chưa tới start_date, hoặc user đã hết lượt usage_per_user của voucher
// đó, bị từ chối; voucher đã tới ngày và user khác chưa dùng thì phát được.
func TestGrantVoucherTx_ChuaToiNgay_HetLuotCuaUser(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	used, fresh := f.user("used"), f.user("fresh")
	grant := func(userID, voucherID uuid.UUID) error {
		return f.db.Transaction(func(tx *gorm.DB) error {
			_, err := vs.GrantVoucherTx(ctx, tx, userID, voucherID, model.UserVoucherSourceContestReward, "")
			return err
		})
	}
	setStart := func(voucherID uuid.UUID, start time.Time) {
		if err := f.db.Model(&model.Voucher{}).Where("id = ?", voucherID).Update("start_date", start).Error; err != nil {
			t.Fatalf("đặt start_date: %v", err)
		}
	}

	future := f.voucher(true, nil)
	setStart(future, time.Now().Add(48*time.Hour))
	if err := grant(fresh, future); !errors.Is(err, ErrVoucherUnavailableForGrant) {
		t.Errorf("voucher chưa tới start_date: muốn ErrVoucherUnavailableForGrant, nhận %v", err)
	}
	started := f.voucher(true, nil)
	setStart(started, time.Now().Add(-48*time.Hour))
	if err := grant(fresh, started); err != nil {
		t.Errorf("voucher đã tới start_date phải phát được: %v", err)
	}

	perUser := f.voucher(true, nil)
	if err := f.db.Model(&model.Voucher{}).Where("id = ?", perUser).Update("usage_per_user", 1).Error; err != nil {
		t.Fatalf("đặt usage_per_user: %v", err)
	}
	if err := f.db.Exec(`INSERT INTO voucher_logs (id, user_id, voucher_id, voucher_code, order_id, action, amount, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'QA', gen_random_uuid(), 'used', 0, now())`, used, perUser).Error; err != nil {
		t.Fatalf("ghi lượt đã dùng: %v", err)
	}
	if err := grant(used, perUser); !errors.Is(err, ErrVoucherUnavailableForGrant) {
		t.Errorf("user đã hết usage_per_user: muốn ErrVoucherUnavailableForGrant, nhận %v", err)
	}
	if err := grant(fresh, perUser); err != nil {
		t.Errorf("user khác chưa dùng voucher phải nhận được: %v", err)
	}
	if n := f.count("user_vouchers", "user_id = ? AND voucher_id IN ?", used, []uuid.UUID{perUser}); n != 0 {
		t.Errorf("user hết lượt vẫn được ghi %d dòng ví", n)
	}
}

// M5 (mutation M33 của review): engine phải đối chiếu attempt với ĐÚNG quiz được hỏi. Dùng một quiz
// THẬT khác (không phải uuid ngẫu nhiên, vốn rơi vào nhánh "quiz not found" và che mất lỗi).
func TestContestEngine_AttemptKhacQuiz_BiTuChoi(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, student := f.user("owner"), f.user("student")
	quizA := f.standaloneQuiz(owner, false)
	quizB := f.standaloneQuiz(owner, false)
	attemptA := f.startContestAttempt(quizA.ID, student)

	if _, err := f.quiz.GetContestAttemptQuestions(ctx, quizB.ID, attemptA); !errors.Is(err, errContestAttemptInvalid) {
		t.Errorf("lấy đề quiz B bằng attempt của quiz A: muốn errContestAttemptInvalid, nhận %v", err)
	}
	if _, err := f.quiz.SubmitContestAttempt(ctx, quizB.ID, student, attemptA, nil, 600); !errors.Is(err, errContestAttemptInvalid) {
		t.Errorf("nộp attempt của quiz A vào quiz B: muốn errContestAttemptInvalid, nhận %v", err)
	}
	if n := f.count("quiz_attempts", "id = ? AND completed_at IS NOT NULL", attemptA); n != 0 {
		t.Errorf("attempt bị nộp nhầm quiz")
	}
}
