package handler

// Test cho QA T6 (261008), tầng handler: lỗi "còn bài học chưa có nội dung" của luồng nộp duyệt
// phải thành 422 COURSE_LESSON_NO_CONTENT, message nêu tên từng bài và body mang danh sách bài —
// không rơi xuống nhánh 500 chung.

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/repository"
)

func TestCourseReviewError_BaiRongNeuTenTungBai(t *testing.T) {
	app := fiber.New()
	idA, idB := uuid.New(), uuid.New()
	app.Get("/x", func(c *fiber.Ctx) error {
		return courseReviewError(c, &repository.CourseLessonsWithoutContentError{Lessons: []repository.LessonWithoutContent{
			{ID: idA, Title: "Bai rong A"}, {ID: idB, Title: "Bai rong B"},
		}}, "unused")
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 422 {
		t.Fatalf("muốn 422, nhận %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Lessons []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"lessons"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("body không phải JSON: %s", raw)
	}
	if body.Code != "COURSE_LESSON_NO_CONTENT" {
		t.Errorf("code=%q", body.Code)
	}
	if !strings.Contains(body.Message, "Bai rong A") || !strings.Contains(body.Message, "Bai rong B") {
		t.Errorf("message phải nêu tên các bài: %q", body.Message)
	}
	if len(body.Lessons) != 2 || body.Lessons[0].ID != idA.String() || body.Lessons[1].Title != "Bai rong B" {
		t.Errorf("lessons=%+v", body.Lessons)
	}
}

// QA2 Q2-05: thông báo nộp duyệt bị chặn vì thiếu nội dung phải bằng tiếng Việt (web hiện nguyên văn khi
// không dịch được theo code) và kèm mã máy đọc được — cả khi còn bài trống lẫn khi khoá chưa có bài nào.
func TestCourseReviewError_ThongBaoTiengViet(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  []string
	}{
		{"bài trống", &repository.CourseLessonsWithoutContentError{Lessons: []repository.LessonWithoutContent{{ID: uuid.New(), Title: "Bài 2"}}},
			"COURSE_LESSON_NO_CONTENT", []string{"Mỗi bài học phải có nội dung", "Bài 2"}},
		{"khoá rỗng", repository.ErrCourseEmptyContent, "COURSE_EMPTY", []string{"ít nhất một bài học"}},
	}
	for _, tc := range cases {
		app := fiber.New()
		app.Get("/x", func(c *fiber.Ctx) error { return courseReviewError(c, tc.err, "unused") })
		resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		var body struct{ Message, Code string }
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("%s: body không phải JSON: %s", tc.name, raw)
		}
		if resp.StatusCode != 422 || body.Code != tc.wantCode {
			t.Errorf("%s: status=%d code=%q, muốn 422 %s", tc.name, resp.StatusCode, body.Code, tc.wantCode)
		}
		for _, want := range tc.wantMsg {
			if !strings.Contains(body.Message, want) {
				t.Errorf("%s: message %q thiếu %q", tc.name, body.Message, want)
			}
		}
	}
}
