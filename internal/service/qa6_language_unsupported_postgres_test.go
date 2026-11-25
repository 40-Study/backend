package service

// QA6 S6-01 (C11): ngôn ngữ máy chấm không chạy được (Dart) không bao giờ bị chạy ngầm như Python.
//  - Chạy thử / chạy tuỳ ý -> 422 ERR_LANGUAGE_UNSUPPORTED, máy chấm KHÔNG được gọi.
//  - Nộp bài -> lưu "pending" (chấm tay), máy chấm KHÔNG được gọi, không bao giờ runtime_error.
//  - DTO bài tập có auto_grade=false.
// Mutation đã kiểm: đưa lại `return 71` (mặc định Python) cho ngôn ngữ lạ làm ca Dart ĐỎ; ca python vẫn phải đi qua máy chấm.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestQA6_NgonNguMayChamKhongChay_KhongBaoGioChayNhuPython(t *testing.T) {
	judge := &s3CountingJudge{}
	e := newLiveJudgeEnv(t, judge)
	ctx := context.Background()

	dart := model.Assignment{ClassID: e.assignment.ClassID, Type: "homework", Title: "Dart", Description: "d", Language: []string{"dart"}, IsPublished: true, TimeLimit: 2, MemoryLimit: 128}
	if err := e.f.db.Create(&dart).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.f.db.Create(&model.TestCase{AssignmentID: dart.ID, Input: "1", ExpectedOutput: "1"}).Error; err != nil {
		t.Fatal(err)
	}

	wantUnsupported := func(name string, err error) {
		t.Helper()
		var ce *ContestError
		if !errors.As(err, &ce) || ce.Status != 422 || ce.Code != "ERR_LANGUAGE_UNSUPPORTED" {
			t.Errorf("%s: err=%v, muốn 422 ERR_LANGUAGE_UNSUPPORTED", name, err)
		}
	}
	_, err := e.svc.RunCode(ctx, e.student.ID, false, dto.RunCodeDTO{AssignmentID: dart.ID.String(), Language: "dart", Code: "void main() { print(2 + 2); }"})
	wantUnsupported("RunCode", err)
	_, err = e.svc.RunCustomCode(ctx, e.student.ID, false, dto.RunCustomCodeDTO{AssignmentID: dart.ID.String(), Language: "dart", Code: "void main() {}", CustomInput: "1"})
	wantUnsupported("RunCustomCode", err)

	sub, err := e.svc.Submit(ctx, false, dto.CreateSubmissionDTO{AssignmentID: dart.ID.String(), UserID: e.student.ID.String(), Language: "dart", Code: "void main() {}"})
	if err != nil || sub == nil || sub.Verdict != model.VerdictPending {
		t.Fatalf("Submit dart: %+v err=%v, muốn lưu pending", sub, err)
	}
	time.Sleep(300 * time.Millisecond) // chấm nền (nếu có) sẽ đổi verdict trong khoảng này
	var row model.Submission
	if err := e.f.db.First(&row, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.Verdict != model.VerdictPending || row.TestCasesPassed != 0 {
		t.Errorf("bài Dart sau khi nộp: verdict=%s passed=%d, muốn pending", row.Verdict, row.TestCasesPassed)
	}
	// ProcessSubmission gọi trực tiếp (đường chấm lại) cũng không được ghi verdict.
	if err := e.svc.ProcessSubmission(ctx, sub.ID); err != nil {
		t.Fatalf("ProcessSubmission dart: %v", err)
	}
	if err := e.f.db.First(&row, "id = ?", sub.ID).Error; err != nil || row.Verdict != model.VerdictPending {
		t.Errorf("sau ProcessSubmission: verdict=%s err=%v, muốn pending", row.Verdict, err)
	}
	if n := judge.calls.Load(); n != 0 {
		t.Errorf("máy chấm bị gọi %d lần cho ngôn ngữ không hỗ trợ", n)
	}

	// Ngôn ngữ hỗ trợ vẫn đi qua máy chấm (đối chứng: test không chặn nhầm mọi thứ).
	py, err := e.svc.Submit(ctx, false, dto.CreateSubmissionDTO{AssignmentID: e.assignment.ID.String(), UserID: e.student.ID.String(), Language: "python", Code: "print(1)"})
	if err != nil || py == nil {
		t.Fatalf("Submit python: %+v err=%v", py, err)
	}
	waitForGradedSubmission(t, e.f.db, py.ID)
	if judge.calls.Load() == 0 {
		t.Error("bài python không đi qua máy chấm")
	}
}

func TestQA6_AssignmentAutoGrade(t *testing.T) {
	for _, tc := range []struct {
		name  string
		langs []string
		want  bool
	}{
		{"python", []string{"python"}, true},
		{"dart thuần", []string{"dart"}, false},
		{"Dart viết hoa + flutter", []string{"Dart", "flutter"}, false},
		{"dart + python: còn chạy được", []string{"dart", "python"}, true},
		{"trắc nghiệm json", []string{"json"}, true},
		{"tự luận text", []string{"text"}, true},
		{"rỗng giữ hành vi cũ", nil, true},
	} {
		if got := assignmentAutoGrade(tc.langs); got != tc.want {
			t.Errorf("%s: assignmentAutoGrade(%v)=%v, muốn %v", tc.name, tc.langs, got, tc.want)
		}
		a := model.Assignment{Language: tc.langs}
		if got := (&AssignmentService{}).toResponseDTO(a).AutoGrade; got != tc.want {
			t.Errorf("%s: DTO auto_grade=%v, muốn %v", tc.name, got, tc.want)
		}
	}
}
