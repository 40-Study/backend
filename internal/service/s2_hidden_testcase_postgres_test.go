package service

// Lane S2, lỗi 1: test case ẩn của bài tập lập trình chỉ trả cho người quản lý (người tạo,
// giảng viên chủ khoá gắn bài tập, admin). Bỏ lọc IsHidden ở GetExerciseByID/GetTestCases hoặc bỏ
// điều kiện sở hữu ở CanManage thì các test này ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type s2ExerciseWorld struct {
	db                                  *gorm.DB
	svc                                 *ExerciseService
	exercise                            model.CourseExercise
	owner, creatorOnly, stranger, admin model.User
	orphan                              model.CourseExercise // chưa gắn khoá nào, chỉ có created_by
}

func newS2ExerciseWorld(t *testing.T) *s2ExerciseWorld {
	t.Helper()
	f := newS2Fixture(t)
	w := &s2ExerciseWorld{db: f.db, owner: f.user("teacher"), creatorOnly: f.user("creator"), stranger: f.user("stranger"), admin: f.user("admin")}

	mkExercise := func(createdBy *uuid.UUID) model.CourseExercise {
		ex := model.CourseExercise{Title: "QA-s2 ex", Description: "d", Language: []string{"python"}, CreatedBy: createdBy}
		if err := f.db.Create(&ex).Error; err != nil {
			t.Fatalf("tạo exercise: %v", err)
		}
		for _, tc := range []model.ExerciseTestCase{
			{ExerciseID: ex.ID, Input: "VIS_IN", ExpectedOutput: "VIS_OUT", IsHidden: false, DisplayOrder: 1},
			{ExerciseID: ex.ID, Input: "SECRET_IN", ExpectedOutput: "SECRET_OUT", IsHidden: true, DisplayOrder: 2},
		} {
			if err := f.db.Create(&tc).Error; err != nil {
				t.Fatalf("tạo test case: %v", err)
			}
		}
		return ex
	}
	// exercise gắn vào bài học của khoá do owner dạy (không có created_by => sở hữu suy từ khoá)
	w.exercise = mkExercise(nil)
	lesson := f.lessonOf(f.course(w.owner))
	content := model.LessonContent{LessonID: lesson.ID, Type: "exercise", ExerciseID: &w.exercise.ID}
	if err := f.db.Create(&content).Error; err != nil {
		t.Fatalf("tạo lesson content: %v", err)
	}
	w.orphan = mkExercise(&w.creatorOnly.ID)
	w.svc = NewExerciseService(repository.NewExerciseRepository(f.db), nil, nil)
	return w
}

func TestS2_Exercise_CanManage(t *testing.T) {
	w := newS2ExerciseWorld(t)
	ctx := context.Background()
	check := func(name string, ex model.CourseExercise, u model.User, isAdmin, want bool) {
		t.Helper()
		got, err := w.svc.CanManage(ctx, ex.ID, u.ID, isAdmin)
		if err != nil || got != want {
			t.Errorf("%s: CanManage=%v (err=%v), muốn %v", name, got, err, want)
		}
	}
	check("giảng viên chủ khoá gắn bài tập", w.exercise, w.owner, false, true)
	check("người lạ với bài tập gắn khoá", w.exercise, w.stranger, false, false)
	check("giảng viên khác (không phải chủ khoá)", w.exercise, w.creatorOnly, false, false)
	check("admin", w.exercise, w.stranger, true, true)
	check("người tạo, bài tập chưa gắn khoá", w.orphan, w.creatorOnly, false, true)
	check("người lạ, bài tập chưa gắn khoá", w.orphan, w.owner, false, false)
}

func hasSecret(tcs []dto.ExerciseTestCaseResponseDTO) bool {
	for _, tc := range tcs {
		if tc.IsHidden || tc.Input == "SECRET_IN" || tc.ExpectedOutput == "SECRET_OUT" {
			return true
		}
	}
	return false
}

func TestS2_Exercise_TestCaseAnChiTraChoNguoiQuanLy(t *testing.T) {
	w := newS2ExerciseWorld(t)
	ctx := context.Background()

	detail, err := w.svc.GetExerciseByID(ctx, w.exercise.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if hasSecret(detail.TestCases) || len(detail.TestCases) != 1 {
		t.Errorf("người không quản lý chỉ được thấy test mẫu, nhận %+v", detail.TestCases)
	}
	if detail.TestCaseCount != 2 {
		t.Errorf("test_case_count vẫn phải là tổng số (2), nhận %d", detail.TestCaseCount)
	}
	tcs, err := w.svc.GetTestCases(ctx, w.exercise.ID, false)
	if err != nil || hasSecret(tcs) || len(tcs) != 1 {
		t.Errorf("GetTestCases(includeHidden=false) lộ test ẩn: %+v (err=%v)", tcs, err)
	}

	full, err := w.svc.GetExerciseByID(ctx, w.exercise.ID, true)
	if err != nil || len(full.TestCases) != 2 || !hasSecret(full.TestCases) {
		t.Errorf("người quản lý phải thấy đủ 2 test kể cả test ẩn: %+v (err=%v)", full, err)
	}
	all, err := w.svc.GetTestCases(ctx, w.exercise.ID, true)
	if err != nil || len(all) != 2 {
		t.Errorf("GetTestCases(includeHidden=true) phải trả đủ: %+v (err=%v)", all, err)
	}
}

// Cache Redis lưu bản đầy đủ; lọc phải diễn ra SAU khi đọc cache, không được để bản của giảng viên
// (đủ test ẩn) lọt sang học viên gọi ngay sau đó.
func TestS2_WithoutHiddenTests_KhongSuaBanDauVaLocDung(t *testing.T) {
	full := &dto.ExerciseDetailDTO{TestCases: []dto.ExerciseTestCaseResponseDTO{
		{Input: "a", IsHidden: false}, {Input: "SECRET_IN", ExpectedOutput: "SECRET_OUT", IsHidden: true},
	}}
	got := withoutHiddenTests(full)
	if hasSecret(got.TestCases) || len(got.TestCases) != 1 {
		t.Fatalf("bản đã lọc còn test ẩn: %+v", got.TestCases)
	}
	if len(full.TestCases) != 2 {
		t.Fatalf("withoutHiddenTests không được sửa bản gốc (bản cache): %+v", full.TestCases)
	}
}

// Bài nộp bài tập khoá học: chủ bài nộp và người quản lý bài tập được xem; người khác nhận đúng
// một lỗi "không tìm thấy" giống bài nộp không tồn tại (không dò được sự tồn tại).
func TestS2_ExerciseSubmission_ChiChuBaiNopVaNguoiQuanLy(t *testing.T) {
	w := newS2ExerciseWorld(t)
	ctx := context.Background()
	author := w.stranger // học viên nộp bài
	sub := model.ExerciseSubmission{ExerciseID: w.exercise.ID, UserID: author.ID, Language: "python", Code: "print(1)"}
	if err := w.db.Create(&sub).Error; err != nil {
		t.Fatalf("tạo bài nộp: %v", err)
	}

	for name, who := range map[string]struct {
		u       model.User
		isAdmin bool
	}{"chủ bài nộp": {author, false}, "giảng viên chủ khoá": {w.owner, false}, "admin": {w.admin, true}} {
		if _, err := w.svc.GetSubmissionByID(ctx, sub.ID, who.u.ID, who.isAdmin); err != nil {
			t.Errorf("%s phải xem được bài nộp: %v", name, err)
		}
	}
	_, errForeign := w.svc.GetSubmissionByID(ctx, sub.ID, w.creatorOnly.ID, false)
	_, errMissing := w.svc.GetSubmissionByID(ctx, uuid.New(), w.creatorOnly.ID, false)
	if !errors.Is(errForeign, ErrExerciseSubmissionNotFound) || !errors.Is(errMissing, ErrExerciseSubmissionNotFound) {
		t.Fatalf("người khác và bài nộp không tồn tại phải cùng ErrExerciseSubmissionNotFound, nhận %v / %v", errForeign, errMissing)
	}
}
