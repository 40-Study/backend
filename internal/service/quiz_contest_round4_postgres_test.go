package service

// Test cho re-review vòng 2 của PR #80 (plans/reports/review-260929-contest-be-round2.md): R2-A quiz
// standalone chưa gắn cuộc thi ai cũng làm bài được, R2-C giảng viên chủ khoá mất quyền sửa quiz
// trong khoá của mình, lỗi voucher lúc chốt phải nêu người thắng + voucher, và mutation MU1 (đơn
// đang chờ giữ voucher phải tính vào usage_per_user).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// course tạo một khoá học của instructor kèm chương + bài học đầu tiên.
func (f *contestFixture) course(instructor uuid.UUID) (courseID, lessonID uuid.UUID) {
	f.t.Helper()
	c := model.Course{InstructorID: instructor, Title: "Khoá QA contest", Slug: "qa-contest-" + uuid.NewString(), Price: decimal.NewFromInt(1)}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo khoá: %v", err)
	}
	s := model.Section{CourseID: c.ID, Title: "C1", DisplayOrder: 1}
	if err := f.db.Create(&s).Error; err != nil {
		f.t.Fatalf("tạo chương: %v", err)
	}
	l := model.Lesson{SectionID: s.ID, Title: "B1", DisplayOrder: 1}
	if err := f.db.Create(&l).Error; err != nil {
		f.t.Fatalf("tạo bài: %v", err)
	}
	return c.ID, l.ID
}

func (f *contestFixture) enroll(userID, courseID uuid.UUID) {
	f.t.Helper()
	if err := f.db.Create(&model.Enrollment{UserID: userID, CourseID: courseID}).Error; err != nil {
		f.t.Fatalf("enroll: %v", err)
	}
}

// attachQuiz gắn quiz vào khoá học hoặc bài học bằng SQL (bỏ qua guard tạo quiz, chỉ dựng dữ liệu).
func (f *contestFixture) attachQuiz(quizID uuid.UUID, column string, target uuid.UUID) {
	f.t.Helper()
	if err := f.db.Exec("UPDATE quizzes SET "+column+" = ? WHERE id = ?", target, quizID).Error; err != nil {
		f.t.Fatalf("gắn quiz vào %s: %v", column, err)
	}
}

// R2-A: đúng kịch bản probe của reviewer. Người lạ không xem, không start, không nộp, không đọc attempt
// của quiz standalone chưa gắn cuộc thi, và quiz vẫn chưa có attempt nào (còn gắn được, §3.3). Người
// tạo và admin vẫn dùng bình thường.
func TestStandaloneQuiz_NguoiLaKhongLamBai_QuizVanGanDuoc(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, stranger, admin := f.user("owner"), f.user("stranger"), f.user("admin")
	cq := f.standaloneQuiz(owner, false)

	calls := map[string]func(uid uuid.UUID, isAdmin bool) error{
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
		"SubmitQuiz": func(uid uuid.UUID, a bool) error {
			_, err := f.quiz.SubmitQuiz(ctx, cq.ID, uid, a, dto.SubmitQuizDTO{
				Answers: []dto.SubmitAnswerDTO{{QuestionID: cq.SingleQ.String(), SelectedAnswerIDs: []string{cq.SingleWrong.String()}}}})
			return err
		},
	}
	for name, call := range calls {
		if err := call(stranger, false); !errors.Is(err, ErrQuizNotOwner) {
			t.Errorf("%s bởi người lạ trên quiz standalone: muốn ErrQuizNotOwner, nhận %v", name, err)
		}
	}
	if n := f.count("quiz_attempts", "quiz_id = ?", cq.ID); n != 0 {
		t.Fatalf("người lạ tạo được %d attempt: quiz không còn gắn được vào cuộc thi (§3.3)", n)
	}

	// Người tạo làm thử: người lạ không đọc được attempt đó (đường lộ correct_answer_ids của probe).
	started, err := f.quiz.StartQuiz(ctx, cq.ID, owner, false, dto.StartQuizDTO{Mode: "practice"})
	if err != nil {
		t.Fatalf("người tạo phải start được: %v", err)
	}
	if _, err := f.quiz.GetAttemptByID(ctx, started.AttemptID, stranger, false); !errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("GetAttemptByID bởi người lạ: muốn ErrQuizNotOwner, nhận %v", err)
	}
	if _, err := f.quiz.GetAttemptProgress(ctx, started.AttemptID, stranger, false); !errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("GetAttemptProgress bởi người lạ: muốn ErrQuizNotOwner, nhận %v", err)
	}
	if err := f.quiz.SaveAnswer(ctx, started.AttemptID, stranger, false, dto.SaveAnswerDTO{QuestionID: cq.SingleQ.String()}); !errors.Is(err, ErrQuizNotOwner) {
		t.Errorf("SaveAnswer bởi người lạ: muốn ErrQuizNotOwner, nhận %v", err)
	}
	aid := started.AttemptID.String()
	if _, err := f.quiz.SubmitQuiz(ctx, cq.ID, owner, false, dto.SubmitQuizDTO{AttemptID: &aid,
		Answers: []dto.SubmitAnswerDTO{{QuestionID: cq.SingleQ.String(), SelectedAnswerIDs: []string{cq.SingleCorrect.String()}}}}); err != nil {
		t.Fatalf("người tạo nộp bài: %v", err)
	}
	if _, err := f.quiz.GetAttemptByID(ctx, started.AttemptID, owner, false); err != nil {
		t.Errorf("người tạo đọc attempt của mình: %v", err)
	}

	for name, call := range calls {
		if name == "SubmitQuiz" {
			continue // admin không có attempt dang dở ở đây.
		}
		if err := call(admin, true); err != nil {
			t.Errorf("%s bởi admin: %v", name, err)
		}
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
	if listed(stranger, false) {
		t.Errorf("GET /quizzes của người lạ vẫn liệt kê quiz standalone của người khác")
	}
	if !listed(owner, false) || !listed(admin, true) {
		t.Errorf("GET /quizzes của người tạo/admin phải liệt kê quiz standalone")
	}
}

// R2-C: giảng viên chủ khoá sửa/xoá được quiz trong khoá của mình (qua bài học hoặc gắn thẳng khoá
// học) kể cả khi admin tạo; giảng viên khoá khác thì không.
func TestQuizOwnership_GiangVienChuKhoaSuaDuocQuizAdminTao(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	teacher, otherTeacher, admin := f.user("teacher"), f.user("other-teacher"), f.user("admin")
	courseID, lessonID := f.course(teacher)
	f.course(otherTeacher)

	lessonQuiz := f.standaloneQuiz(admin, false)
	f.attachQuiz(lessonQuiz.ID, "lesson_id", lessonID)
	courseQuiz := f.standaloneQuiz(admin, false)
	f.attachQuiz(courseQuiz.ID, "course_id", courseID)

	title := "GV chủ khoá sửa"
	for name, q := range map[string]contestQuiz{"quiz bài học": lessonQuiz, "quiz khoá học": courseQuiz} {
		if _, err := f.quiz.UpdateQuiz(ctx, q.ID, otherTeacher, false, dto.UpdateQuizDTO{Title: &title}); !errors.Is(err, ErrQuizNotOwner) {
			t.Errorf("%s: GV khoá khác sửa được, muốn ErrQuizNotOwner, nhận %v", name, err)
		}
		if err := f.quiz.DeleteQuiz(ctx, q.ID, otherTeacher, false); !errors.Is(err, ErrQuizNotOwner) {
			t.Errorf("%s: GV khoá khác xoá được, muốn ErrQuizNotOwner, nhận %v", name, err)
		}
		if _, err := f.quiz.UpdateQuiz(ctx, q.ID, teacher, false, dto.UpdateQuizDTO{Title: &title}); err != nil {
			t.Errorf("%s: GV chủ khoá phải sửa được quiz admin tạo, nhận %v", name, err)
		}
		if _, err := f.quiz.CreateQuestion(ctx, q.ID, teacher, false, dto.CreateQuestionDTO{QuestionText: "Mới", QuestionType: "essay"}); err != nil {
			t.Errorf("%s: GV chủ khoá phải thêm câu hỏi được, nhận %v", name, err)
		}
		if err := f.quiz.DeleteQuiz(ctx, q.ID, teacher, false); err != nil {
			t.Errorf("%s: GV chủ khoá phải xoá được, nhận %v", name, err)
		}
	}
}

// Chủ dự án chốt 29/09: voucher hỏng vẫn chặn cả lần chốt, nhưng lỗi phải nói rõ người thắng (tên,
// hạng) và voucher (mã, lý do) để admin sửa rồi chốt lại. errors.Is cũ vẫn phải đúng.
func TestIssueAwardTx_VoucherHong_LoiNeuNguoiThangVaVoucher(t *testing.T) {
	f := newContestFixture(t)
	issuer := NewContestRewardService(newTestVoucherService(f), &recordingNotifier{})
	ctx := context.Background()
	winner := f.user("winner")
	if err := f.db.Model(&model.User{}).Where("id = ?", winner).Update("full_name", "Nguyễn Thị Hạng Hai").Error; err != nil {
		t.Fatalf("đặt full_name: %v", err)
	}
	exhausted := f.voucher(true, nil)
	if err := f.db.Model(&model.Voucher{}).Where("id = ?", exhausted).
		Updates(map[string]interface{}{"usage_limit": 1, "used_count": 1}).Error; err != nil {
		t.Fatalf("đặt usage_limit: %v", err)
	}
	var code string
	if err := f.db.Model(&model.Voucher{}).Where("id = ?", exhausted).Pluck("code", &code).Error; err != nil {
		t.Fatalf("đọc mã voucher: %v", err)
	}
	rank := 2

	err := f.db.Transaction(func(tx *gorm.DB) error {
		_, err := issuer.IssueAwardTx(ctx, tx, ContestAwardGrant{UserID: winner, ContestTitle: "Olympic", Rank: &rank, VoucherID: &exhausted})
		return err
	})
	if !errors.Is(err, ErrVoucherUnavailableForGrant) {
		t.Fatalf("muốn ErrVoucherUnavailableForGrant, nhận %v", err)
	}
	var ge *VoucherGrantError
	if !errors.As(err, &ge) {
		t.Fatalf("lỗi không mang chi tiết *VoucherGrantError: %v", err)
	}
	if ge.UserID != winner || ge.VoucherID != exhausted || ge.VoucherCode != code || ge.Reason != VoucherGrantReasonUsageLimit {
		t.Errorf("chi tiết sai: %+v (muốn user %s, voucher %s/%s, lý do hết tổng lượt)", ge, winner, exhausted, code)
	}
	if ge.Rank == nil || *ge.Rank != 2 || ge.UserName != "Nguyễn Thị Hạng Hai" {
		t.Errorf("thiếu hạng/tên người thắng: rank=%v name=%q", ge.Rank, ge.UserName)
	}
	for _, want := range []string{code, "Nguyễn Thị Hạng Hai", "hạng 2", VoucherGrantReasonUsageLimit} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("thông điệp lỗi thiếu %q: %s", want, err.Error())
		}
	}
	if n := f.count("user_vouchers", "user_id = ?", winner); n != 0 {
		t.Errorf("chốt bị chặn mà vẫn ghi %d dòng ví", n)
	}
}

// MU1 (mutation của re-review): đơn đang chờ thanh toán giữ voucher cũng tính vào usage_per_user.
// Không có voucher_logs "used" nào, chỉ có đơn pending, vẫn phải từ chối.
func TestGrantVoucherTx_DonDangChoGiuVoucher_TinhVaoLuotCuaUser(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	holder, fresh := f.user("holder"), f.user("fresh")
	perUser := f.voucher(true, nil)
	if err := f.db.Model(&model.Voucher{}).Where("id = ?", perUser).Update("usage_per_user", 1).Error; err != nil {
		t.Fatalf("đặt usage_per_user: %v", err)
	}
	o := model.Order{UserID: holder, OrderNumber: "QA-CT-" + uuid.NewString(), Currency: "VND", Status: "pending",
		Subtotal: decimal.NewFromInt(1000), TotalAmount: decimal.NewFromInt(1000)}
	if err := f.db.Create(&o).Error; err != nil {
		t.Fatalf("tạo đơn: %v", err)
	}
	if err := f.db.Exec("UPDATE orders SET voucher_id = ? WHERE id = ?", perUser, o.ID).Error; err != nil {
		t.Fatalf("gắn voucher vào đơn: %v", err)
	}
	grant := func(userID uuid.UUID) error {
		return f.db.Transaction(func(tx *gorm.DB) error {
			_, err := vs.GrantVoucherTx(ctx, tx, userID, perUser, model.UserVoucherSourceContestReward, "")
			return err
		})
	}
	err := grant(holder)
	var ge *VoucherGrantError
	if !errors.As(err, &ge) || ge.Reason != VoucherGrantReasonPerUserLimit {
		t.Errorf("user đang giữ voucher trong đơn chờ: muốn lý do hết lượt của user, nhận %v", err)
	}
	if err := grant(fresh); err != nil {
		t.Errorf("user khác chưa giữ voucher phải nhận được: %v", err)
	}
}
