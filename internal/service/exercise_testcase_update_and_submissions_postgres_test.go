package service

// Lane BE-2 (code-editor gaps #1, #2, #4), Postgres THẬT:
//   - UpdateTestCase sửa từng phần, chỉ trong đúng bài tập, và làm mất hiệu lực cache Redis của bài tập.
//   - GetSubmissions kèm danh tính học viên (student: id, full_name, email); danh sách rỗng là [] không null.
//   - GetLatestSubmission ("bài nộp gần nhất" của bài học) bỏ qua bài chưa chấm: pending / system_error.
// Bỏ invalidateExerciseCache, bỏ điều kiện exercise_id, bỏ submissionStudent hoặc bỏ lọc verdict thì ĐỎ.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func be2Exercise(t *testing.T, w *s2ExerciseWorld) model.ExerciseTestCase {
	t.Helper()
	var tc model.ExerciseTestCase
	if err := w.db.Where("exercise_id = ? AND input = ?", w.exercise.ID, "VIS_IN").First(&tc).Error; err != nil {
		t.Fatalf("đọc test case mẫu: %v", err)
	}
	return tc
}

func TestBE2_UpdateTestCase_PartialScopedAndInvalidatesCache(t *testing.T) {
	w := newS2ExerciseWorld(t)
	ctx := context.Background()
	mr := miniredis.RunT(t)
	w.svc = NewExerciseService(repository.NewExerciseRepository(w.db), redis.NewClient(&redis.Options{Addr: mr.Addr()}), nil)
	tc := be2Exercise(t, w)

	// Nạp cache trước khi sửa.
	if _, err := w.svc.GetExerciseByID(ctx, w.exercise.ID, true); err != nil {
		t.Fatal(err)
	}

	newOut, hidden := "NEW_OUT", true
	got, err := w.svc.UpdateTestCase(ctx, w.exercise.ID, tc.ID, dto.UpdateExerciseTestCaseDTO{ExpectedOutput: &newOut, IsHidden: &hidden})
	if err != nil {
		t.Fatalf("UpdateTestCase: %v", err)
	}
	if got.ID != tc.ID || got.ExerciseID != w.exercise.ID || got.Input != "VIS_IN" || got.ExpectedOutput != "NEW_OUT" || !got.IsHidden || got.DisplayOrder != 1 {
		t.Fatalf("response sai (input/display_order phải giữ nguyên): %+v", got)
	}
	var stored model.ExerciseTestCase
	if err := w.db.First(&stored, "id = ?", tc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Input != "VIS_IN" || stored.ExpectedOutput != "NEW_OUT" || !stored.IsHidden || stored.DisplayOrder != 1 {
		t.Fatalf("DB sau khi sửa: %+v", stored)
	}

	detail, err := w.svc.GetExerciseByID(ctx, w.exercise.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range detail.TestCases {
		if d.ID == tc.ID && (d.ExpectedOutput != "NEW_OUT" || !d.IsHidden) {
			t.Fatalf("cache bài tập chưa bị xoá sau khi sửa test case: %+v", d)
		}
	}
	student, err := w.svc.GetExerciseByID(ctx, w.exercise.ID, false)
	if err != nil || len(student.TestCases) != 0 {
		t.Fatalf("test vừa chuyển sang ẩn không được trả cho học viên: %+v (err=%v)", student.TestCases, err)
	}

	// Test case thuộc bài tập KHÁC: 404, không sửa chéo.
	other := be2Exercise(t, w)
	in := "HACK"
	if _, err := w.svc.UpdateTestCase(ctx, w.orphan.ID, other.ID, dto.UpdateExerciseTestCaseDTO{Input: &in}); !errors.Is(err, ErrExerciseTestCaseNotFound) {
		t.Fatalf("sửa test case qua bài tập khác phải trả ErrExerciseTestCaseNotFound, nhận %v", err)
	}
	if _, err := w.svc.UpdateTestCase(ctx, w.exercise.ID, uuid.New(), dto.UpdateExerciseTestCaseDTO{Input: &in}); !errors.Is(err, ErrExerciseTestCaseNotFound) {
		t.Fatalf("test case không tồn tại phải trả ErrExerciseTestCaseNotFound, nhận %v", err)
	}
	if err := w.db.First(&stored, "id = ?", other.ID).Error; err != nil || stored.Input != "VIS_IN" {
		t.Fatalf("test case bị sửa chéo: %+v (err=%v)", stored, err)
	}
}

func TestBE2_UpdateExercise_PersistsJudgeLimits(t *testing.T) {
	w := newS2ExerciseWorld(t)
	tl, ml, pp := 3, 128, 60
	out, err := w.svc.UpdateExercise(context.Background(), w.exercise.ID, dto.UpdateExerciseDTO{TimeLimit: &tl, MemoryLimit: &ml, PassPercentage: &pp})
	if err != nil {
		t.Fatal(err)
	}
	var stored model.CourseExercise
	if err := w.db.First(&stored, "id = ?", w.exercise.ID).Error; err != nil {
		t.Fatal(err)
	}
	if out.TimeLimit != 3 || out.MemoryLimit != 128 || out.PassPercentage != 60 ||
		stored.TimeLimit != 3 || stored.MemoryLimit != 128 || stored.PassPercentage != 60 || stored.Title != w.exercise.Title {
		t.Fatalf("giới hạn chưa lưu: response=%+v db=%+v", out, stored)
	}
}

func TestBE2_GetSubmissions_IncludesStudentIdentity(t *testing.T) {
	w := newS2ExerciseWorld(t)
	ctx := context.Background()

	empty, err := w.svc.GetSubmissions(ctx, w.exercise.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(empty)
	var shape struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil || string(shape.Data) != "[]" {
		t.Fatalf("danh sách rỗng phải là [] (không null), nhận %s", raw)
	}

	fullName := "Nguyễn Văn Học"
	named := w.stranger
	if err := w.db.Model(&model.User{}).Where("id = ?", named.ID).Update("full_name", fullName).Error; err != nil {
		t.Fatal(err)
	}
	noName := w.creatorOnly // full_name NULL -> rơi về user_name
	for i, u := range []model.User{named, noName} {
		sub := model.ExerciseSubmission{ExerciseID: w.exercise.ID, UserID: u.ID, Language: "python", Code: "print(1)", Verdict: "accepted",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second)}
		if err := w.db.Create(&sub).Error; err != nil {
			t.Fatal(err)
		}
	}

	list, err := w.svc.GetSubmissions(ctx, w.exercise.ID, 1, 20)
	if err != nil || len(list.Data) != 2 {
		t.Fatalf("GetSubmissions: %+v (err=%v)", list, err)
	}
	want := map[uuid.UUID]dto.ExerciseSubmissionStudentDTO{
		named.ID:  {ID: named.ID, FullName: fullName, Email: named.Email},
		noName.ID: {ID: noName.ID, FullName: noName.UserName, Email: noName.Email},
	}
	for _, item := range list.Data {
		if item.Student == nil || *item.Student != want[item.UserID] {
			t.Errorf("bài nộp của %s: student = %+v, muốn %+v", item.UserID, item.Student, want[item.UserID])
		}
	}
	b, _ := json.Marshal(list.Data[0])
	var wire map[string]any
	_ = json.Unmarshal(b, &wire)
	st, ok := wire["student"].(map[string]any)
	if !ok || st["id"] == nil || st["full_name"] == nil || st["email"] == nil {
		t.Fatalf("JSON phải có student{id,full_name,email}: %s", b)
	}

	// Danh sách của chính học viên (my-submissions) không kèm student.
	mine, err := w.svc.GetMySubmissions(ctx, w.exercise.ID, named.ID, 1, 20)
	if err != nil || len(mine.Data) != 1 || mine.Data[0].Student != nil {
		t.Fatalf("my-submissions không được thêm student: %+v (err=%v)", mine, err)
	}
}

func TestBE2_GetLatestSubmission_SkipsUnjudgedLegacyRows(t *testing.T) {
	w := newS2ExerciseWorld(t)
	repo := repository.NewExerciseRepository(w.db)
	u := w.stranger
	base := time.Now().Add(-time.Hour)
	mk := func(verdict string, offset time.Duration) uuid.UUID {
		t.Helper()
		sub := model.ExerciseSubmission{ExerciseID: w.exercise.ID, UserID: u.ID, Language: "python", Code: "x", Verdict: verdict, CreatedAt: base.Add(offset)}
		if err := w.db.Create(&sub).Error; err != nil {
			t.Fatal(err)
		}
		return sub.ID
	}

	if got, err := repo.GetLatestSubmission(context.Background(), w.exercise.ID, u.ID); err != nil || got != nil {
		t.Fatalf("chưa nộp bài: muốn nil, nhận %+v (err=%v)", got, err)
	}
	mk("pending", 0)
	if got, err := repo.GetLatestSubmission(context.Background(), w.exercise.ID, u.ID); err != nil || got != nil {
		t.Fatalf("chỉ có bài pending: không có bài nộp gần nhất, nhận %+v (err=%v)", got, err)
	}
	judged := mk("wrong_answer", time.Minute)
	mk("pending", 2*time.Minute)
	mk("system_error", 3*time.Minute)

	got, err := repo.GetLatestSubmission(context.Background(), w.exercise.ID, u.ID)
	if err != nil || got == nil || got.ID != judged {
		t.Fatalf("bài nộp gần nhất phải là bài đã chấm %s, nhận %+v (err=%v)", judged, got, err)
	}
}
