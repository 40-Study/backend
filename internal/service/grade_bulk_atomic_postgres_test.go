package service

// W2-B (review R4, 04/10/2026): hai lỗ ở đường chấm điểm.
//   - body thiếu `score` ghi âm thầm thành điểm 0 (DTO để float64 + gte=0);
//   - chấm hàng loạt kiểm khoảng điểm giữa vòng ghi, nên một dòng vượt thang ở giữa lô để lại các dòng trước.
// Postgres thật, schema tạm riêng.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// gradeF64 trả con trỏ tới điểm: Score của CreateGradeDTO là *float64.
func gradeF64(v float64) *float64 { return &v }

func TestGradeDTO_MissingScoreIsRejectedButZeroIsValid(t *testing.T) {
	parse := func(body string) dto.CreateGradeDTO {
		var req dto.CreateGradeDTO
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatalf("unmarshal %s: %v", body, err)
		}
		return req
	}
	const head = `{"student_id":"3f2504e0-4f89-41d3-9a0c-0305e82c3301","grade_type":"assignment","title":"Bài tập","max_score":10`

	if errs := utils.ValidateStruct(parse(head + `}`)); errs == nil {
		t.Error("body thiếu score lọt qua validate (sẽ ghi thầm thành 0)")
	}
	if errs := utils.ValidateStruct(parse(head + `,"score":0}`)); errs != nil {
		t.Errorf("score 0 phải hợp lệ: %v", errs)
	}
	if errs := utils.ValidateStruct(parse(head + `,"score":-1}`)); errs == nil {
		t.Error("score âm lọt qua validate")
	}
}

func TestBulkCreateGrades_InvalidRowWritesNothing(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := NewGradeService(repository.NewGradeRepository(e.f.db), repository.NewClassRepository(e.f.db), repository.NewCourseRepository(e.f.db), nil, nil)
	row := func(score float64) dto.CreateGradeDTO {
		return dto.CreateGradeDTO{StudentID: e.student.ID.String(), GradeType: "assignment", Title: "Bài tập", Score: gradeF64(score), MaxScore: 10}
	}

	// Dòng 1 hợp lệ, dòng 2 vượt thang: không dòng nào được ghi.
	_, err := svc.BulkCreateGrades(ctx, e.class.ID, e.coTeacher.ID, dto.BulkCreateGradesDTO{Grades: []dto.CreateGradeDTO{row(7), row(15)}})
	if !errors.Is(err, ErrGradeScoreOutOfRange) {
		t.Fatalf("lô có dòng 15/10: err=%v, muốn ErrGradeScoreOutOfRange", err)
	}
	if mine, err := svc.GetMyGrades(ctx, e.student.ID); err != nil || len(mine) != 0 {
		t.Fatalf("lô bị từ chối vẫn để lại %d dòng điểm (err=%v)", len(mine), err)
	}

	// Thiếu score ở một dòng cũng là lỗi cả lô.
	missing := row(5)
	missing.Score = nil
	if _, err := svc.BulkCreateGrades(ctx, e.class.ID, e.coTeacher.ID, dto.BulkCreateGradesDTO{Grades: []dto.CreateGradeDTO{row(7), missing}}); err == nil {
		t.Fatal("lô có dòng thiếu score được chấp nhận")
	}
	if mine, _ := svc.GetMyGrades(ctx, e.student.ID); len(mine) != 0 {
		t.Fatalf("lô thiếu score vẫn để lại %d dòng điểm", len(mine))
	}

	// Lô hợp lệ ghi đủ, kèm người chấm.
	out, err := svc.BulkCreateGrades(ctx, e.class.ID, e.coTeacher.ID, dto.BulkCreateGradesDTO{Grades: []dto.CreateGradeDTO{row(0), row(9)}})
	if err != nil || len(out) != 2 {
		t.Fatalf("lô hợp lệ: %v err=%v", out, err)
	}
	if out[0].GradedBy != e.coTeacher.ID {
		t.Errorf("graded_by=%v, muốn người gọi %v", out[0].GradedBy, e.coTeacher.ID)
	}
}
