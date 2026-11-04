package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/repository"
)

// Q3-09 (QA3): nộp duyệt thiếu ảnh bìa / mô tả / giá -> 422 COURSE_INCOMPLETE, kèm key và câu tiếng Việt nêu từng mục.
func TestCourseReviewError_IncompleteCourseListsWhatIsMissing(t *testing.T) {
	app := fiber.New()
	err := &repository.CourseIncompleteError{Missing: []string{repository.CourseMissingThumbnail, repository.CourseMissingPrice}}
	app.Post("/x", func(c *fiber.Ctx) error { return courseReviewError(c, err, "x") })
	res, e := app.Test(httptest.NewRequest("POST", "/x", nil), -1)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := io.ReadAll(res.Body)
	var body struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Missing []string `json:"missing"`
	}
	_ = json.Unmarshal(raw, &body)
	if res.StatusCode != fiber.StatusUnprocessableEntity || body.Code != "COURSE_INCOMPLETE" {
		t.Fatalf("got %d %s", res.StatusCode, raw)
	}
	if len(body.Missing) != 2 || body.Missing[0] != "thumbnail_url" || body.Missing[1] != "price" {
		t.Errorf("missing = %v", body.Missing)
	}
	if !strings.Contains(body.Message, "ảnh bìa") || !strings.Contains(body.Message, "giá") {
		t.Errorf("thông báo phải nêu từng mục còn thiếu bằng tiếng Việt: %q", body.Message)
	}
}
