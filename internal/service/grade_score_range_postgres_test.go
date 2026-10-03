package service

// R4 (QA hồi quy 03/10/2026, B-06): màn chấm bài của web gửi điểm tới POST/PUT grades. Hai lỗ hổng ở đường
// chấm làm UI không thể dùng đúng:
//   - điểm 0 bị DTO coi là "thiếu" (validate:"required" trên float không-con-trỏ) → 400 cho bài làm sai hết;
//   - điểm 15/10 vẫn được ghi, kéo điểm trung bình có trọng số của cả lớp lên.
// Và trang "Điểm của tôi" của học viên cần tên lớp đi kèm điểm (class_name) để nhóm theo lớp.
// Postgres thật, schema tạm riêng.

import (
	"context"
	"errors"
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

func TestGradeScoreRange_ZeroAllowedOverMaxRejected(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := NewGradeService(repository.NewGradeRepository(e.f.db), repository.NewClassRepository(e.f.db), repository.NewCourseRepository(e.f.db), nil, nil)
	teacher := e.coTeacher.ID

	req := func(score, max float64) dto.CreateGradeDTO {
		return dto.CreateGradeDTO{StudentID: e.student.ID.String(), GradeType: "assignment", Title: "Bài tập", Score: score, MaxScore: max}
	}

	// DTO: 0 điểm hợp lệ, âm và max_score 0 thì không.
	if errs := utils.ValidateStruct(req(0, 10)); errs != nil {
		t.Fatalf("điểm 0 bị validate chặn: %v", errs)
	}
	if errs := utils.ValidateStruct(req(-1, 10)); errs == nil {
		t.Fatal("điểm âm lọt qua validate")
	}
	if errs := utils.ValidateStruct(req(5, 0)); errs == nil {
		t.Fatal("max_score 0 lọt qua validate")
	}

	// Service: điểm 0 ghi được; điểm vượt thang bị từ chối và không để lại dòng điểm.
	zero, err := svc.CreateGrade(ctx, e.class.ID, teacher, req(0, 10))
	if err != nil {
		t.Fatalf("ghi điểm 0: %v", err)
	}
	if _, err := svc.CreateGrade(ctx, e.class.ID, teacher, req(15, 10)); !errors.Is(err, ErrGradeScoreOutOfRange) {
		t.Fatalf("điểm 15/10: err=%v, muốn ErrGradeScoreOutOfRange", err)
	}
	if mine, err := svc.GetMyGrades(ctx, e.student.ID); err != nil || len(mine) != 1 {
		t.Fatalf("sau khi bị từ chối còn %d dòng điểm (err=%v), muốn đúng 1 dòng điểm 0", len(mine), err)
	}

	// Sửa: vượt thang bị chặn, trong thang thì được; hạ max_score xuống dưới điểm đang có cũng bị chặn.
	over := 11.0
	if _, err := svc.UpdateGrade(ctx, zero.ID, teacher, dto.UpdateGradeDTO{Score: &over}); !errors.Is(err, ErrGradeScoreOutOfRange) {
		t.Fatalf("sửa lên 11/10: err=%v, muốn ErrGradeScoreOutOfRange", err)
	}
	ok := 9.5
	up, err := svc.UpdateGrade(ctx, zero.ID, teacher, dto.UpdateGradeDTO{Score: &ok})
	if err != nil || up.Score.String() != "9.5" {
		t.Fatalf("sửa lên 9.5: %+v err=%v", up, err)
	}
	lowMax := 5.0
	if _, err := svc.UpdateGrade(ctx, zero.ID, teacher, dto.UpdateGradeDTO{MaxScore: &lowMax}); !errors.Is(err, ErrGradeScoreOutOfRange) {
		t.Fatalf("hạ max_score xuống 5 khi điểm là 9.5: err=%v, muốn ErrGradeScoreOutOfRange", err)
	}
}

func TestGradeResponse_MyGradesCarryClassName(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := NewGradeService(repository.NewGradeRepository(e.f.db), repository.NewClassRepository(e.f.db), repository.NewCourseRepository(e.f.db), nil, nil)

	if _, err := svc.CreateGrade(ctx, e.class.ID, e.coTeacher.ID, dto.CreateGradeDTO{
		StudentID: e.student.ID.String(), GradeType: "assignment", Title: "Bài tập", Score: 8, MaxScore: 10,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := svc.GetMyGrades(ctx, e.student.ID)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetMyGrades: %v err=%v", all, err)
	}
	if all[0].ClassName != e.class.Name || all[0].ClassName == "" {
		t.Errorf("GetMyGrades class_name=%q, muốn %q", all[0].ClassName, e.class.Name)
	}

	inClass, err := svc.GetMyGradesByClass(ctx, e.student.ID, e.class.ID)
	if err != nil || len(inClass) != 1 {
		t.Fatalf("GetMyGradesByClass: %v err=%v", inClass, err)
	}
	if inClass[0].ClassName != e.class.Name {
		t.Errorf("GetMyGradesByClass class_name=%q, muốn %q", inClass[0].ClassName, e.class.Name)
	}
}
