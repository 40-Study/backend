package service

// L2 (review 261009, contract C1): bốn nơi dựng LessonContentResponseDTO (LessonContentService, SectionService,
// CourseService, LessonService) phải cho CÙNG một kết quả cho cùng một nội dung. Trước đây ba nơi sau bỏ
// article_body/reading_time_minutes/quiz_id (cùng với exercise_id/is_mandatory/subtitle_url), nên bất kỳ client nào đọc
// nội dung qua curriculum/chi tiết bài sẽ nhận bài đọc không có thân bài, quiz không có quiz_id.
//
// Mutation đã thử: trả lại một trong ba mapper về bản dựng tay cũ (thiếu trường) làm test này ĐỎ.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func parityContents(lessonID uuid.UUID) []model.LessonContent {
	body, title := "<p>"+"từ "+"</p>", "Tiêu đề"
	quizID, exerciseID := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	base := func(typ string, order int) model.LessonContent {
		return model.LessonContent{ID: uuid.New(), LessonID: lessonID, Type: typ, Title: &title, DisplayOrder: order,
			IsMandatory: true, CreatedAt: now, UpdatedAt: now}
	}
	article, quiz, exercise, video := base("article", 1), base("quiz", 2), base("exercise", 3), base("video", 4)
	article.ArticleBody = &body
	quiz.QuizID = &quizID
	exercise.ExerciseID = &exerciseID
	video.Duration = 90
	return []model.LessonContent{article, quiz, exercise, video}
}

func marshalContents(t *testing.T, items []dto.LessonContentResponseDTO) string {
	t.Helper()
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestLessonContentMappers_AllFourBuildersAgree(t *testing.T) {
	lesson := &model.Lesson{ID: uuid.New(), SectionID: uuid.New(), Title: "Bài", IsMandatory: true}
	contents := parityContents(lesson.ID)

	canonical := make([]dto.LessonContentResponseDTO, len(contents))
	lc := &LessonContentService{}
	for i := range contents {
		canonical[i] = *lc.toContentResponseDTO(&contents[i], withheldVideoViewer)
	}
	want := marshalContents(t, canonical)

	// Tiền điều kiện: bản chuẩn thật sự mang các trường C1 (nếu không, so sánh bằng nhau không chứng minh gì).
	if canonical[0].ArticleBody == nil || canonical[0].ReadingTimeMinutes == nil || canonical[1].QuizID == nil ||
		canonical[2].ExerciseID == nil || !canonical[2].IsMandatory {
		t.Fatalf("bản chuẩn thiếu trường C1: %+v", canonical)
	}

	cases := map[string][]dto.LessonContentResponseDTO{
		"SectionService": (&SectionService{}).toLessonResponseDTO(lesson, contents).Contents,
		"CourseService":  (&CourseService{}).toLessonResponseDTO(lesson, contents, withheldVideoViewer).Contents,
		"LessonService":  (&LessonService{}).toLessonResponseDTO(lesson, contents, withheldVideoViewer).Contents,
	}
	for name, got := range cases {
		if g := marshalContents(t, got); g != want {
			t.Errorf("%s dựng nội dung khác bản chuẩn của LessonContentService:\n got: %s\nwant: %s", name, g, want)
		}
	}
}
