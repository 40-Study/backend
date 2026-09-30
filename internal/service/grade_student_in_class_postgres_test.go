package service

// Lane P (câu hỏi mở 7 của rà soát phân quyền, chỉ phần đang CHO PHÉP sai): giảng viên của lớp ghi điểm
// cho student_id bất kỳ, kể cả người không học lớp đó, và điểm hiện ra ở bảng điểm cá nhân của người ấy.
// Hướng từ chối của bảng điểm (chỉ teacher_classes) giữ nguyên theo S4. Postgres thật, schema tạm riêng.

import (
	"context"
	"errors"
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestGrade_OnlyForStudentsOfTheClass(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := NewGradeService(repository.NewGradeRepository(e.f.db), repository.NewClassRepository(e.f.db), nil)
	teacher := e.coTeacher.ID

	grade := func(student model.User) dto.CreateGradeDTO {
		return dto.CreateGradeDTO{StudentID: student.ID.String(), GradeType: "midterm", Title: "Giữa kỳ", Score: 8, MaxScore: 10}
	}
	rows := func(student model.User) int64 {
		var n int64
		e.f.db.Model(&model.Grade{}).Where("student_id = ?", student.ID).Count(&n)
		return n
	}

	// Học viên trong lớp: ghi được.
	if _, err := svc.CreateGrade(ctx, e.class.ID, teacher, grade(e.student)); err != nil {
		t.Fatalf("ghi điểm cho học viên của lớp bị chặn nhầm: %v", err)
	}

	// Người ngoài lớp: bị từ chối, không có dòng điểm nào, bảng điểm cá nhân của họ vẫn trống.
	if _, err := svc.CreateGrade(ctx, e.class.ID, teacher, grade(e.stranger)); !errors.Is(err, ErrStudentNotInClass) {
		t.Fatalf("ghi điểm cho người ngoài lớp: err=%v, muốn ErrStudentNotInClass", err)
	}
	if n := rows(e.stranger); n != 0 {
		t.Fatalf("người ngoài lớp có %d dòng điểm", n)
	}
	if mine, err := svc.GetMyGrades(ctx, e.stranger.ID); err != nil || len(mine) != 0 {
		t.Fatalf("bảng điểm cá nhân của người ngoài lớp: %v err=%v, muốn rỗng", mine, err)
	}

	// Học viên đã rời lớp (không còn active) cũng không ghi được.
	if err := e.f.db.Model(&model.StudentClass{}).Where("student_id = ? AND class_id = ?", e.student.ID, e.class.ID).Update("status", "dropped").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateGrade(ctx, e.class.ID, teacher, grade(e.student)); !errors.Is(err, ErrStudentNotInClass) {
		t.Fatalf("ghi điểm cho học viên đã rời lớp: err=%v, muốn ErrStudentNotInClass", err)
	}
	if err := e.f.db.Model(&model.StudentClass{}).Where("student_id = ? AND class_id = ?", e.student.ID, e.class.ID).Update("status", "active").Error; err != nil {
		t.Fatal(err)
	}

	// Lô có một người ngoài lớp: cả lô bị từ chối, không ghi dòng nào (kể cả dòng hợp lệ đứng trước).
	before := rows(e.student)
	_, err := svc.BulkCreateGrades(ctx, e.class.ID, teacher, dto.BulkCreateGradesDTO{Grades: []dto.CreateGradeDTO{grade(e.student), grade(e.stranger)}})
	if !errors.Is(err, ErrStudentNotInClass) {
		t.Fatalf("lô có người ngoài lớp: err=%v, muốn ErrStudentNotInClass", err)
	}
	if after := rows(e.student); after != before {
		t.Fatalf("lô bị từ chối vẫn ghi %d dòng cho học viên hợp lệ", after-before)
	}
}
