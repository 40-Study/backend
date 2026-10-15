package service

// Hợp đồng code editor (plans/261015-student-code-editor/contract.md, Phase 0): khoá CHÍNH XÁC tập key JSON của
// mọi DTO và message RabbitMQ mà các lane BE-A/BE-B/BE-C và web dùng chung, bảng ngôn ngữ chạy được (sec. 3) và
// các chuỗi verdict (sec. 2). Không cần DB. Thêm/bớt/đổi tên một key ở phía Go mà không sửa contract.md (và
// fixture web) thì test này ĐỎ.

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func contractKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%T không phải JSON object: %s", v, raw)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func assertContractKeys(t *testing.T, name string, v any, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := contractKeys(t, v)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s: key JSON = %v, contract.md muốn %v", name, got, want)
	}
}

func ptrInt(v int) *int          { return &v }
func ptrString(v string) *string { return &v }

func fullSubmissionResponse() dto.ExerciseSubmissionResponseDTO {
	return dto.ExerciseSubmissionResponseDTO{
		ID: uuid.New(), ExerciseID: uuid.New(), UserID: uuid.New(), Language: "python", Code: "print(1)",
		Verdict: string(model.VerdictAccepted), TestCasesPassed: 1, TotalTestCases: 1,
		ExecutionTimeMs: ptrInt(12), MemoryUsedKB: ptrInt(3000), ErrorMessage: ptrString("x"), CreatedAt: time.Now(),
		User: &dto.SubmissionUserSummaryDTO{ID: uuid.New(), UserName: "u", FullName: "Nguyễn Văn A"},
	}
}

var submissionResponseKeys = []string{"id", "exercise_id", "user_id", "language", "code", "verdict", "test_cases_passed",
	"total_test_cases", "execution_time_ms", "memory_used_kb", "error_message", "created_at", "user"}

func TestContract_DTOKeySets(t *testing.T) {
	assertContractKeys(t, "ExerciseSubmissionResponseDTO", fullSubmissionResponse(), submissionResponseKeys...)

	// user (sec. 5) chỉ có trên E7: omitempty, và không bao giờ lộ email.
	noUser := fullSubmissionResponse()
	noUser.User = nil
	if keys := strings.Join(contractKeys(t, noUser), ","); strings.Contains(keys, "user,") || strings.HasSuffix(keys, ",user") {
		t.Errorf("ExerciseSubmissionResponseDTO không có User vẫn có key user: %s", keys)
	}
	assertContractKeys(t, "SubmissionUserSummaryDTO", dto.SubmissionUserSummaryDTO{ID: uuid.New()}, "id", "user_name", "full_name")

	tcID := uuid.New()
	fullResult := dto.ExerciseSubmissionResultDTO{
		TestCaseID: &tcID, DisplayOrder: 1, IsHidden: false, Verdict: string(model.VerdictWrongAnswer),
		Input: ptrString("1"), ExpectedOutput: ptrString("2"), ActualOutput: ptrString("3"), ErrorOutput: ptrString(""),
		ExecutionTimeMs: ptrInt(5), MemoryUsedKB: ptrInt(100),
	}
	resultKeys := []string{"test_case_id", "display_order", "is_hidden", "verdict", "input", "expected_output",
		"actual_output", "error_output", "execution_time_ms", "memory_used_kb"}
	assertContractKeys(t, "ExerciseSubmissionResultDTO", fullResult, resultKeys...)

	// Ca ẩn với người không quản lý: bỏ input/expected/actual/error, test_case_id vẫn có (null khi ca đã xoá).
	hidden := dto.ExerciseSubmissionResultDTO{DisplayOrder: 2, IsHidden: true, Verdict: string(model.VerdictAccepted)}
	assertContractKeys(t, "ExerciseSubmissionResultDTO (ẩn, đã lược)", hidden, "test_case_id", "display_order", "is_hidden", "verdict")
	if raw, _ := json.Marshal(hidden); !strings.Contains(string(raw), `"test_case_id":null`) {
		t.Errorf("test_case_id của ca đã xoá phải là null, không bị bỏ: %s", raw)
	}

	detail := dto.ExerciseSubmissionDetailDTO{ExerciseSubmissionResponseDTO: fullSubmissionResponse(), Results: []dto.ExerciseSubmissionResultDTO{fullResult}}
	assertContractKeys(t, "ExerciseSubmissionDetailDTO", detail, append(append([]string{}, submissionResponseKeys...), "results")...)
	emptyDetail := dto.ExerciseSubmissionDetailDTO{ExerciseSubmissionResponseDTO: fullSubmissionResponse(), Results: []dto.ExerciseSubmissionResultDTO{}}
	if raw, _ := json.Marshal(emptyDetail); !strings.Contains(string(raw), `"results":[]`) {
		t.Errorf("results rỗng phải là [], không bị bỏ: %s", raw)
	}

	assertContractKeys(t, "UpdateExerciseTestCaseDTO",
		dto.UpdateExerciseTestCaseDTO{Input: ptrString(""), ExpectedOutput: ptrString(""), IsHidden: new(bool), DisplayOrder: ptrInt(0)},
		"input", "expected_output", "is_hidden", "display_order")
	assertContractKeys(t, "CodeDraftDTO", dto.CodeDraftDTO{Scope: "sandbox:a.py", UpdatedAt: time.Now()}, "scope", "language", "code", "updated_at")
	assertContractKeys(t, "CodeDraftSummaryDTO", dto.CodeDraftSummaryDTO{Scope: "sandbox:a.py"}, "scope", "language", "size", "updated_at")
	assertContractKeys(t, "SaveCodeDraftDTO", dto.SaveCodeDraftDTO{}, "scope", "language", "code")
	assertContractKeys(t, "RunnableLanguageDTO", dto.RunnableLanguageDTO{}, "key", "label")
	assertContractKeys(t, "APIErrorDTO", dto.APIErrorDTO{}, "code", "error", "message")
	assertContractKeys(t, "ExecuteCodeResponseDTO (E1)",
		dto.ExecuteCodeResponseDTO{Error: "e", Status: string(model.VerdictAccepted), StatusID: 3},
		"stdout", "stderr", "compile_output", "error", "exit_code", "time", "memory", "status", "status_id")
}

func TestContract_ExerciseSubmissionMessageShape(t *testing.T) {
	msg := ExerciseSubmissionMessage{
		SubmissionID: uuid.New(), ExerciseID: uuid.New(), UserID: uuid.New(), Language: "python", Code: "x",
		TimeLimit: 2, MemoryLimit: 256, PassPercentage: 100,
		TestCases: []ExerciseSubmissionTestCase{{TestCaseID: uuid.New(), DisplayOrder: 0, IsHidden: true, Input: "1", ExpectedOutput: "1"}},
	}
	assertContractKeys(t, "ExerciseSubmissionMessage", msg, "submission_id", "exercise_id", "user_id", "language", "code",
		"time_limit", "memory_limit", "pass_percentage", "test_cases")
	assertContractKeys(t, "ExerciseSubmissionMessage.test_cases[]", msg.TestCases[0],
		"test_case_id", "display_order", "is_hidden", "input", "expected_output")

	if ExerciseExchange != "exercise" || ExerciseSubmitQueue != "exercise.submit" || ExerciseResultQueue != "exercise.result" {
		t.Errorf("exchange/queue = %q/%q/%q, contract sec. 7 muốn exercise/exercise.submit/exercise.result",
			ExerciseExchange, ExerciseSubmitQueue, ExerciseResultQueue)
	}
	if MaxSourceCodeBytes != 65536 || MaxStoredOutputBytes != 8192 || MaxDraftsPerUser != 200 {
		t.Errorf("giới hạn = %d/%d/%d, contract muốn 65536/8192/200", MaxSourceCodeBytes, MaxStoredOutputBytes, MaxDraftsPerUser)
	}
}

func TestContract_RunnableLanguages(t *testing.T) {
	want := map[string]int{"python": 71, "javascript": 63, "cpp": 54, "c": 50, "java": 62, "sql": 82}
	got := RunnableLanguages()
	if len(got) != len(want) {
		t.Fatalf("RunnableLanguages() = %v, muốn đúng 6 khoá %v", got, want)
	}
	for _, l := range got {
		if _, ok := want[l.Key]; !ok {
			t.Errorf("khoá thừa %q (html KHÔNG chạy được)", l.Key)
		}
		if l.Label == "" {
			t.Errorf("khoá %q thiếu label", l.Key)
		}
		lang, err := LookupLanguage(l.Key)
		if err != nil || lang.Judge0ID != want[l.Key] {
			t.Errorf("LookupLanguage(%q) = %+v, %v; muốn Judge0 id %d", l.Key, lang, err, want[l.Key])
		}
	}

	for alias, key := range map[string]string{"python3": "python", "js": "javascript", "c++": "cpp", " Python ": "python"} {
		lang, err := LookupLanguage(alias)
		if err != nil || lang.Key != key {
			t.Errorf("LookupLanguage(%q) = %+v, %v; muốn khoá %q", alias, lang, err, key)
		}
	}
	for _, bad := range []string{"html", "typescript", "go", "", "brainfuck"} {
		if _, err := LookupLanguage(bad); !errors.Is(err, ErrUnsupportedLanguage) {
			t.Errorf("LookupLanguage(%q) err = %v, muốn ErrUnsupportedLanguage (không mặc định về Python 71)", bad, err)
		}
	}
	// Bài live cũ vẫn chạy các khoá ngoài bảng (BE-A chuyển nơi gọi sang map riêng này).
	for key, id := range map[string]int{"typescript": 74, "go": 60, "python": 71} {
		if got, ok := LegacyLiveAssignmentLanguageID(key); !ok || got != id {
			t.Errorf("LegacyLiveAssignmentLanguageID(%q) = %d, %v; muốn %d", key, got, ok, id)
		}
	}
	if _, ok := LegacyLiveAssignmentLanguageID("html"); ok {
		t.Error("html không có Judge0 id, kể cả ở bài live")
	}
}

func TestContract_VerdictEnum(t *testing.T) {
	want := map[model.SubmissionVerdict]bool{ // verdict -> terminal?
		"pending": false, "running": false, "accepted": true, "wrong_answer": true, "compilation_error": true,
		"runtime_error": true, "time_limit_exceeded": true, "memory_limit_exceeded": true, "system_error": true,
		"judge_unavailable": true,
	}
	consts := []model.SubmissionVerdict{model.VerdictPending, model.VerdictRunning, model.VerdictAccepted,
		model.VerdictWrongAnswer, model.VerdictCompilationError, model.VerdictRuntimeError, model.VerdictTimeLimitExceed,
		model.VerdictMemoryLimitExceed, model.VerdictSystemError, model.VerdictJudgeUnavailable}
	if len(consts) != len(want) {
		t.Fatalf("số verdict = %d, muốn %d", len(consts), len(want))
	}
	for _, v := range consts {
		terminal, ok := want[v]
		if !ok {
			t.Errorf("verdict lạ %q", v)
			continue
		}
		if v.IsTerminal() != terminal {
			t.Errorf("%q.IsTerminal() = %v, muốn %v", v, v.IsTerminal(), terminal)
		}
		if len(v) > 32 {
			t.Errorf("%q dài hơn varchar(32)", v)
		}
	}
	if model.SubmissionVerdict("bogus").IsTerminal() {
		t.Error("verdict lạ không được coi là terminal (web sẽ ngừng poll sai)")
	}
}
