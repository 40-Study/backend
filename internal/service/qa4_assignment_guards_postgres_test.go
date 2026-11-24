package service

// QA4-T01 / QA4-T08 (Postgres thật, schema tạm): xoá bài tập đã có bài nộp bị từ chối; hạn nộp đã qua bị từ chối khi
// tạo, đổi hạn và công bố. Bỏ kiểm ở AssignmentService.Delete / ensureDueInFuture thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func newQA4AssignmentFixture(t *testing.T) (*s2Fixture, *AssignmentService, model.Class) {
	t.Helper()
	f := newS2Fixture(t)
	teacher := f.user("teacher")
	course := f.course(teacher)
	class := model.Class{Name: "QA4-assign", CourseID: &course.ID, Status: "active"}
	if err := f.db.Create(&class).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAssignmentService(repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db),
		repository.NewSubmissionRepository(f.db), nil, nil)
	return f, svc, class
}

func TestQA4_DeleteAssignment_RefusedWhenSubmissionsExist(t *testing.T) {
	f, svc, class := newQA4AssignmentFixture(t)
	ctx := context.Background()
	student := f.user("student")
	withSub := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "có bài nộp", Description: "d", Language: []string{"go"}}
	empty := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "chưa ai nộp", Description: "d", Language: []string{"go"}}
	for _, a := range []*model.Assignment{&withSub, &empty} {
		if err := f.db.Create(a).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.db.Create(&model.Submission{AssignmentID: withSub.ID, UserID: student.ID, Language: "go", Code: "package main"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, withSub.ID); !errors.Is(err, ErrAssignmentHasSubmissions) {
		t.Fatalf("xoá bài đã có bài nộp: err=%v, muốn ErrAssignmentHasSubmissions", err)
	}
	if n := countAssignments(t, f, withSub.ID); n != 1 {
		t.Fatalf("bài đã có bài nộp bị xoá (còn %d dòng)", n)
	}
	if err := svc.Delete(ctx, empty.ID); err != nil {
		t.Fatalf("xoá bài chưa ai nộp: %v", err)
	}
	if n := countAssignments(t, f, empty.ID); n != 0 {
		t.Fatalf("bài chưa ai nộp chưa bị xoá (còn %d dòng)", n)
	}
}

func countAssignments(t *testing.T, f *s2Fixture, id interface{}) int64 {
	t.Helper()
	var n int64
	if err := f.db.Model(&model.Assignment{}).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestQA4_AssignmentDueDate_MustBeInFuture(t *testing.T) {
	f, svc, class := newQA4AssignmentFixture(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	future := time.Now().AddDate(0, 0, 3).Format(time.RFC3339)
	create := func(end string) (*model.Assignment, error) {
		return svc.Create(ctx, f.user("admin").ID, true, dto.CreateAssignmentDTO{ClassID: class.ID.String(), Type: "homework",
			Title: "bài " + end, Description: "d", Difficulty: "easy", Language: []string{"go"}, EndTime: &end})
	}

	if _, err := create(past); !errors.Is(err, ErrDueDateInPast) {
		t.Fatalf("tạo với hạn đã qua: err=%v, muốn ErrDueDateInPast", err)
	}
	a, err := create(future)
	if err != nil {
		t.Fatalf("tạo với hạn ở tương lai: %v", err)
	}
	if _, err := svc.Update(ctx, a.ID, dto.UpdateAssignmentDTO{EndTime: &past}); !errors.Is(err, ErrDueDateInPast) {
		t.Fatalf("đổi hạn về quá khứ: err=%v, muốn ErrDueDateInPast", err)
	}

	// Bài đã quá hạn từ trước (dữ liệu cũ): sửa tiêu đề kèm end_time cũ không bị chặn, nhưng không công bố được.
	old := time.Now().AddDate(0, 0, -2).Truncate(time.Second)
	overdue := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "đã quá hạn", Description: "d", Language: []string{"go"}, EndTime: &old}
	if err := f.db.Create(&overdue).Error; err != nil {
		t.Fatal(err)
	}
	title, same := "đã quá hạn (sửa)", old.Format(time.RFC3339)
	if _, err := svc.Update(ctx, overdue.ID, dto.UpdateAssignmentDTO{Title: &title, EndTime: &same}); err != nil {
		t.Fatalf("sửa tiêu đề bài đã quá hạn (gửi lại end_time cũ): %v", err)
	}
	if _, err := svc.Publish(ctx, overdue.ID, nil); !errors.Is(err, ErrDueDateInPast) {
		t.Fatalf("công bố bài đã quá hạn: err=%v, muốn ErrDueDateInPast", err)
	}
	var got model.Assignment
	if err := f.db.First(&got, "id = ?", overdue.ID).Error; err != nil || got.IsPublished {
		t.Fatalf("bài quá hạn vẫn được công bố (err=%v, published=%v)", err, got.IsPublished)
	}
	if _, err := svc.Publish(ctx, a.ID, nil); err != nil {
		t.Fatalf("công bố bài còn hạn: %v", err)
	}
}
