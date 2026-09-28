package handler

// QA vòng 2, lane D — nối dây lỗi service -> HTTP qua HANDLER THẬT (service fake):
//   - ErrCourseLockedForReview -> 409 {"code":"COURSE_PENDING_REVIEW"} ở mọi route ghi khoá/chương/
//     bài/nội dung bài (Q5);
//   - ErrCourseHidden -> 404 ở các route đọc (D4; GET sections/lessons trước đây trả 500).

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type lockedCourseSvc struct{ service.CourseServiceInterface }

func (lockedCourseSvc) UpdateCourse(context.Context, uuid.UUID, uuid.UUID, bool, dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error) {
	return nil, service.ErrCourseLockedForReview
}
func (lockedCourseSvc) GetCourseByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.CourseDetailDTO, error) {
	return nil, service.ErrCourseHidden
}

type lockedSectionSvc struct{ service.SectionServiceInterface }

func (lockedSectionSvc) CreateSection(context.Context, uuid.UUID, uuid.UUID, dto.CreateSectionDTO) (*dto.SectionResponseDTO, error) {
	return nil, service.ErrCourseLockedForReview
}
func (lockedSectionSvc) UpdateSection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, bool, dto.UpdateSectionDTO) (*dto.SectionResponseDTO, error) {
	return nil, service.ErrCourseLockedForReview
}
func (lockedSectionSvc) DeleteSection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, bool) error {
	return service.ErrCourseLockedForReview
}
func (lockedSectionSvc) ReorderSections(context.Context, uuid.UUID, uuid.UUID, bool, dto.ReorderDTO) error {
	return service.ErrCourseLockedForReview
}
func (lockedSectionSvc) GetAllSections(context.Context, uuid.UUID, uuid.UUID, bool) ([]dto.SectionResponseDTO, error) {
	return nil, service.ErrCourseHidden
}
func (lockedSectionSvc) GetSectionByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.SectionResponseDTO, error) {
	return nil, service.ErrCourseHidden
}

type lockedLessonSvc struct{ service.LessonServiceInterface }

func (lockedLessonSvc) CreateLesson(context.Context, uuid.UUID, uuid.UUID, dto.CreateLessonDTO) (*dto.LessonResponseDTO, error) {
	return nil, service.ErrCourseLockedForReview
}
func (lockedLessonSvc) UpdateLesson(context.Context, uuid.UUID, uuid.UUID, bool, dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error) {
	return nil, service.ErrCourseLockedForReview
}
func (lockedLessonSvc) DeleteLesson(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return service.ErrCourseLockedForReview
}
func (lockedLessonSvc) ReorderLessons(context.Context, uuid.UUID, uuid.UUID, bool, dto.ReorderDTO) error {
	return service.ErrCourseLockedForReview
}
func (lockedLessonSvc) GetAllLessons(context.Context, uuid.UUID, uuid.UUID, bool) ([]dto.LessonResponseDTO, error) {
	return nil, service.ErrCourseHidden
}

type lockedContentSvc struct{ service.LessonContentServiceInterface }

func (lockedContentSvc) CreateContent(context.Context, uuid.UUID, uuid.UUID, bool, dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	return nil, service.ErrCourseLockedForReview
}

func TestCourseEditLockHandlers_StatusMapping(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	})
	course := NewCourseHandler(lockedCourseSvc{}, nil)
	section := NewSectionHandler(lockedSectionSvc{}, nil)
	lesson := NewLessonHandler(lockedLessonSvc{}, nil)
	content := NewLessonContentHandler(lockedContentSvc{}, nil)
	app.Put("/courses/:id", course.UpdateCourse)
	app.Get("/courses/:id", course.GetCourseByID)
	app.Post("/courses/:course_id/sections", section.CreateSection)
	app.Get("/courses/:course_id/sections", section.GetAllSections)
	app.Put("/courses/:course_id/sections/reorder", section.ReorderSections)
	app.Get("/courses/:course_id/sections/:id", section.GetSectionByID)
	app.Put("/courses/:course_id/sections/:id", section.UpdateSection)
	app.Delete("/courses/:course_id/sections/:id", section.DeleteSection)
	app.Post("/sections/:section_id/lessons", lesson.CreateLesson)
	app.Get("/sections/:section_id/lessons", lesson.GetAllLessons)
	app.Put("/sections/:section_id/lessons/reorder", lesson.ReorderLessons)
	app.Put("/lessons/:id", lesson.UpdateLesson)
	app.Delete("/lessons/:id", lesson.DeleteLesson)
	app.Post("/lessons/:lesson_id/contents", content.CreateContent)

	cid, sid, lid := uuid.NewString(), uuid.NewString(), uuid.NewString()
	reorder := `{"items":[{"id":"` + uuid.NewString() + `","display_order":1}]}`
	cases := []struct {
		method, path, body string
		want               int
		code               string
	}{
		{"PUT", "/courses/" + cid, `{"description":"QA"}`, 409, CourseLockedCode},
		{"POST", "/courses/" + cid + "/sections", `{"title":"QA-chuong"}`, 409, CourseLockedCode},
		{"PUT", "/courses/" + cid + "/sections/" + sid, `{"title":"QA"}`, 409, CourseLockedCode},
		{"DELETE", "/courses/" + cid + "/sections/" + sid, "", 409, CourseLockedCode},
		{"PUT", "/courses/" + cid + "/sections/reorder", reorder, 409, CourseLockedCode},
		{"POST", "/sections/" + sid + "/lessons", `{"title":"QA-bai"}`, 409, CourseLockedCode},
		{"PUT", "/lessons/" + lid, `{"title":"QA"}`, 409, CourseLockedCode},
		{"DELETE", "/lessons/" + lid, "", 409, CourseLockedCode},
		{"PUT", "/sections/" + sid + "/lessons/reorder", reorder, 409, CourseLockedCode},
		{"POST", "/lessons/" + lid + "/contents", `{"type":"exercise","title":"QA"}`, 409, CourseLockedCode},
		{"GET", "/courses/" + cid, "", 404, ""},
		{"GET", "/courses/" + cid + "/sections", "", 404, ""},
		{"GET", "/courses/" + cid + "/sections/" + sid, "", 404, ""},
		{"GET", "/sections/" + sid + "/lessons", "", 404, ""},
	}
	for _, tc := range cases {
		var body io.Reader
		if tc.body != "" {
			body = strings.NewReader(tc.body)
		}
		req := httptest.NewRequest(tc.method, tc.path, body)
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		raw, _ := io.ReadAll(res.Body)
		var out map[string]interface{}
		_ = json.Unmarshal(raw, &out)
		if res.StatusCode != tc.want || (tc.code != "" && out["code"] != tc.code) {
			t.Errorf("%s %s = %d %s, muốn %d code=%q", tc.method, tc.path, res.StatusCode, raw, tc.want, tc.code)
		}
	}
}
