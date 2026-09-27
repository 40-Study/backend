package service

// Test cho P2 QA 260927 teacher: total_lessons/total_duration_minutes trên courses không cập
// nhật sau khi thêm/sửa/xoá bài — khoá đã có nội dung thật vẫn hiện "0 bài/0 phút". Bản vá gọi
// CourseRepository.RecalculateLessonStats ngay sau khi ghi lesson; test này kiểm WIRING (đúng
// courseID được gọi lại) chứ không kiểm câu SQL thật (nằm ở internal/repository, không chạy
// được trên máy này do Smart App Control chặn binary test — xem báo cáo cuối).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type recalcTrackingCourseRepo struct {
	repository.CourseRepositoryInterface
	course        *model.Course
	recalcCalls   []uuid.UUID
}

func (r *recalcTrackingCourseRepo) GetByID(_ context.Context, _ uuid.UUID) (*model.Course, error) {
	return r.course, nil
}

func (r *recalcTrackingCourseRepo) RecalculateLessonStats(_ context.Context, courseID uuid.UUID) error {
	r.recalcCalls = append(r.recalcCalls, courseID)
	return nil
}

type fixedSectionRepoForStats struct {
	repository.SectionRepositoryInterface
	section *model.Section
}

func (r *fixedSectionRepoForStats) GetByID(_ context.Context, _ uuid.UUID) (*model.Section, error) {
	return r.section, nil
}

type recordingLessonRepoForStats struct {
	repository.LessonRepositoryInterface
	created *model.Lesson
	lesson  *model.Lesson
}

func (r *recordingLessonRepoForStats) Create(_ context.Context, lesson *model.Lesson) error {
	r.created = lesson
	return nil
}

func (r *recordingLessonRepoForStats) GetMaxDisplayOrder(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (r *recordingLessonRepoForStats) GetByID(_ context.Context, _ uuid.UUID) (*model.Lesson, error) {
	return r.lesson, nil
}

func (r *recordingLessonRepoForStats) Update(_ context.Context, lesson *model.Lesson) error {
	r.lesson = lesson
	return nil
}

func (r *recordingLessonRepoForStats) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (r *recordingLessonRepoForStats) GetContentsByLessonID(_ context.Context, _ uuid.UUID) ([]model.LessonContent, error) {
	return nil, nil
}

func TestCreateLesson_GhiLaiTongSoBaiCuaCourse(t *testing.T) {
	instructorID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()

	sectionRepo := &fixedSectionRepoForStats{section: &model.Section{
		BaseModel: model.BaseModel{ID: sectionID},
		CourseID:  courseID,
	}}
	courseRepo := &recalcTrackingCourseRepo{course: &model.Course{
		BaseModel:    model.BaseModel{ID: courseID},
		InstructorID: instructorID,
	}}
	lessonRepo := &recordingLessonRepoForStats{}

	svc := NewLessonService(lessonRepo, sectionRepo, courseRepo, nil)

	_, err := svc.CreateLesson(context.Background(), sectionID, instructorID, dto.CreateLessonDTO{Title: "Bai moi"})
	if err != nil {
		t.Fatalf("CreateLesson loi: %v", err)
	}

	if len(courseRepo.recalcCalls) != 1 || courseRepo.recalcCalls[0] != courseID {
		t.Fatalf("RecalculateLessonStats calls = %v, muon duoc goi dung 1 lan voi courseID=%s — thieu wiring nay la nguyen nhan total_lessons dung yen o 0 sau khi them bai",
			courseRepo.recalcCalls, courseID)
	}
}

func TestDeleteLesson_GhiLaiTongSoBaiCuaCourse(t *testing.T) {
	instructorID := uuid.New()
	courseID := uuid.New()
	sectionID := uuid.New()
	lessonID := uuid.New()

	sectionRepo := &fixedSectionRepoForStats{section: &model.Section{
		BaseModel: model.BaseModel{ID: sectionID},
		CourseID:  courseID,
	}}
	courseRepo := &recalcTrackingCourseRepo{course: &model.Course{
		BaseModel:    model.BaseModel{ID: courseID},
		InstructorID: instructorID,
	}}
	lessonRepo := &recordingLessonRepoForStats{lesson: &model.Lesson{
		ID:        lessonID,
		SectionID: sectionID,
	}}

	svc := NewLessonService(lessonRepo, sectionRepo, courseRepo, nil)

	if err := svc.DeleteLesson(context.Background(), lessonID, instructorID, false); err != nil {
		t.Fatalf("DeleteLesson loi: %v", err)
	}

	if len(courseRepo.recalcCalls) != 1 || courseRepo.recalcCalls[0] != courseID {
		t.Fatalf("RecalculateLessonStats calls = %v, muon duoc goi dung 1 lan voi courseID=%s sau khi xoa bai",
			courseRepo.recalcCalls, courseID)
	}
}
