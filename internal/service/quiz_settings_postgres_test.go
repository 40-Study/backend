package service

// Q2-04 (QA vòng 2 261013): pass_percentage và max_attempts do giáo viên đặt phải được LƯU và trả lại
// đúng (qua Postgres thật, không chỉ DTO). Bẫy cần pin: cột pass_percentage có `default:70.00` — một ngưỡng
// 0 (cho qua mọi điểm) không được bị GORM nuốt thành 70 như bài học is_mandatory=false từng bị.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

func TestQuizSettings_LuuVaTraLaiDungGiaTriGiaoVienDat(t *testing.T) {
	db := isolatedAPISchema(t)
	f := &s2Fixture{t: t, db: db}
	teacher := f.user("teacher")
	course := f.course(teacher)
	svc := NewQuizService(repository.NewQuizRepository(db), nil, repository.NewCourseRepository(db),
		repository.NewSectionRepository(db), repository.NewLessonRepository(db), nil, repository.NewEnrollmentRepository(db))
	ctx := context.Background()
	cid := course.ID.String()

	pass, attempts := 85.5, 2
	created, err := svc.CreateQuiz(ctx, teacher.ID, dto.CreateQuizDTO{CourseID: cid, Title: "Kiem tra giua ky", PassPercentage: &pass, MaxAttempts: &attempts})
	if err != nil {
		t.Fatalf("CreateQuiz: %v", err)
	}
	assertQuizSettings(t, "create (echo)", created.PassPercentage, created.MaxAttempts, 85.5, 2)
	stored, err := svc.GetQuizByID(ctx, created.ID, teacher.ID, true)
	if err != nil {
		t.Fatalf("GetQuizByID: %v", err)
	}
	assertQuizSettings(t, "create (doc lai)", stored.PassPercentage, stored.MaxAttempts, 85.5, 2)

	zero, four := 0.0, 4
	updated, err := svc.UpdateQuiz(ctx, created.ID, teacher.ID, false, dto.UpdateQuizDTO{PassPercentage: &zero, MaxAttempts: &four})
	if err != nil {
		t.Fatalf("UpdateQuiz: %v", err)
	}
	assertQuizSettings(t, "update (echo)", updated.PassPercentage, updated.MaxAttempts, 0, 4)
	stored, _ = svc.GetQuizByID(ctx, created.ID, teacher.ID, true)
	assertQuizSettings(t, "update (doc lai)", stored.PassPercentage, stored.MaxAttempts, 0, 4)

	// Tạo mới với pass 0 ngay từ đầu cũng không được bị đổi về 70.
	zeroNew, err := svc.CreateQuiz(ctx, teacher.ID, dto.CreateQuizDTO{CourseID: cid, Title: "Cho qua moi diem", PassPercentage: &zero})
	if err != nil {
		t.Fatalf("CreateQuiz pass=0: %v", err)
	}
	stored, _ = svc.GetQuizByID(ctx, zeroNew.ID, teacher.ID, true)
	if !stored.PassPercentage.Equal(decimal.Zero) {
		t.Fatalf("pass_percentage=0 bi doi thanh %s", stored.PassPercentage)
	}

	// Bỏ trống cả hai khi tạo: mặc định của cột (pass 70, tối đa 5 lần) — hợp đồng cho web hiển thị placeholder.
	def, err := svc.CreateQuiz(ctx, teacher.ID, dto.CreateQuizDTO{CourseID: cid, Title: "Mac dinh"})
	if err != nil {
		t.Fatalf("CreateQuiz mac dinh: %v", err)
	}
	stored, _ = svc.GetQuizByID(ctx, def.ID, teacher.ID, true)
	assertQuizSettings(t, "mac dinh", stored.PassPercentage, stored.MaxAttempts, 70, 5)
}

func assertQuizSettings(t *testing.T, label string, pass decimal.Decimal, attempts *int, wantPass float64, wantAttempts int) {
	t.Helper()
	if !pass.Equal(decimal.NewFromFloat(wantPass)) {
		t.Errorf("%s: pass_percentage = %s, muon %v", label, pass, wantPass)
	}
	if attempts == nil || *attempts != wantAttempts {
		t.Errorf("%s: max_attempts = %v, muon %d", label, attempts, wantAttempts)
	}
}
