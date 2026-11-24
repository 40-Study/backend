package service

// QA6 review M3 + L1 (C11 follow-up).
//  - M3: nộp / chạy thử / chạy tuỳ ý một ngôn ngữ NGOÀI danh sách của bài -> 400 ERR_LANGUAGE_NOT_ALLOWED, kiểm TRƯỚC khi lưu bài
//    và trước kiểm ngôn ngữ-không-hỗ-trợ; không có dòng submission nào được tạo, máy chấm không được gọi. Khớp không phân biệt
//    hoa thường/khoảng trắng. Bỏ assignmentAllowsLanguage khỏi Submit/RunCode/RunCustomCode thì ca "dart vào bài python" ĐỎ.
//  - L1: bài tự luận nộp "TEXT" / " text " vẫn đi nhánh tự luận (accepted), không kẹt "pending". Đưa lại so khớp `== "text"` thì ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func wantLanguageNotAllowed(t *testing.T, name string, err error) {
	t.Helper()
	var ce *ContestError
	if !errors.As(err, &ce) || ce.Status != 400 || ce.Code != "ERR_LANGUAGE_NOT_ALLOWED" {
		t.Errorf("%s: err=%v, muốn 400 ERR_LANGUAGE_NOT_ALLOWED", name, err)
	}
}

func TestQA6_NgonNguNgoaiDanhSachCuaBai_Tu400_Postgres(t *testing.T) {
	judge := &s3CountingJudge{}
	e := newLiveJudgeEnv(t, judge) // bài python
	ctx := context.Background()
	count := func() int64 {
		var n int64
		if err := e.f.db.Model(&model.Submission{}).Where("assignment_id = ?", e.assignment.ID).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	for _, lang := range []string{"dart", "java", "no-such-lang"} {
		_, err := e.svc.Submit(ctx, false, dto.CreateSubmissionDTO{AssignmentID: e.assignment.ID.String(), UserID: e.student.ID.String(), Language: lang, Code: "x"})
		wantLanguageNotAllowed(t, "Submit "+lang, err)
	}
	_, err := e.svc.RunCode(ctx, e.student.ID, false, dto.RunCodeDTO{AssignmentID: e.assignment.ID.String(), Language: "dart", Code: "x"})
	wantLanguageNotAllowed(t, "RunCode dart", err)
	_, err = e.svc.RunCustomCode(ctx, e.student.ID, false, dto.RunCustomCodeDTO{AssignmentID: e.assignment.ID.String(), Language: "java", Code: "x", CustomInput: "1"})
	wantLanguageNotAllowed(t, "RunCustomCode java", err)
	if n := count(); n != 0 {
		t.Errorf("bài bị từ chối vẫn tạo %d dòng submission", n)
	}
	if n := judge.calls.Load(); n != 0 {
		t.Errorf("máy chấm bị gọi %d lần cho ngôn ngữ ngoài danh sách", n)
	}

	// Đối chứng: đúng ngôn ngữ của bài (kể cả viết hoa/thừa khoảng trắng) vẫn nộp được và đi qua máy chấm.
	ok, err := e.svc.Submit(ctx, false, dto.CreateSubmissionDTO{AssignmentID: e.assignment.ID.String(), UserID: e.student.ID.String(), Language: " Python ", Code: "print(1)"})
	if err != nil || ok == nil {
		t.Fatalf("Submit ' Python ': %+v err=%v", ok, err)
	}
	waitForGradedSubmission(t, e.f.db, ok.ID)
	if judge.calls.Load() == 0 {
		t.Error("bài python không đi qua máy chấm")
	}
}

func TestQA6_AssignmentAllowsLanguage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed []string
		lang    string
		want    bool
	}{
		{"khớp đúng", []string{"python"}, "python", true},
		{"hoa thường + khoảng trắng", []string{"python"}, "  PyThon ", true},
		{"danh sách viết hoa", []string{"Python"}, "python", true},
		{"ngoài danh sách", []string{"python"}, "dart", false},
		{"bí danh cùng máy chấm", []string{"c++"}, "cpp", true},
		{"python3 là python", []string{"python"}, "python3", true},
		{"khác ngôn ngữ máy chấm", []string{"python"}, "java", false},
		{"nhiều ngôn ngữ", []string{"dart", "python"}, "dart", true},
		{"json trắc nghiệm", []string{"json"}, "JSON", true},
		{"json không phải python", []string{"json"}, "python", false},
		{"danh sách rỗng giữ hành vi cũ", nil, "anything", true},
	} {
		if got := assignmentAllowsLanguage(tc.allowed, tc.lang); got != tc.want {
			t.Errorf("%s: assignmentAllowsLanguage(%v, %q)=%v, muốn %v", tc.name, tc.allowed, tc.lang, got, tc.want)
		}
	}
}

func TestQA6_BaiTuLuan_NgonNguTextVietHoa_VanAccepted_Postgres(t *testing.T) {
	e := newLiveJudgeEnv(t, &s3CountingJudge{})
	ctx := context.Background()
	essay := model.Assignment{ClassID: e.assignment.ClassID, Type: "homework", Title: "Tự luận", Description: "d", Language: []string{"text"}, IsPublished: true, TimeLimit: 2, MemoryLimit: 128}
	if err := e.f.db.Create(&essay).Error; err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"TEXT", " text "} {
		sub, err := e.svc.Submit(ctx, false, dto.CreateSubmissionDTO{AssignmentID: essay.ID.String(), UserID: e.student.ID.String(), Language: lang, Code: "bài làm"})
		if err != nil || sub == nil {
			t.Fatalf("Submit %q: %+v err=%v", lang, sub, err)
		}
		var row model.Submission
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if err := e.f.db.First(&row, "id = ?", sub.ID).Error; err != nil {
				t.Fatal(err)
			}
			if row.Verdict != model.VerdictPending {
				break
			}
		}
		if row.Verdict != model.VerdictAccepted {
			t.Errorf("tự luận %q: verdict=%s, muốn accepted (nhánh tự luận)", lang, row.Verdict)
		}
	}
}
