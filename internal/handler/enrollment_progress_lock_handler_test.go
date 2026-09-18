package handler

// Test cho F3 (review 260917), tang handler: ghi tien do vao bai dang khoa tuan tu phai la 403
// {message:"LESSON_LOCKED"} o CA HAI loi vao (PUT /lessons/:lessonId/progress va beacon
// POST /progress), cung shape voi LessonContentHandler.GetContent — khong phai 400 chung.
//
// mountWithCaller/doJSON dung lai tu livestream_authz_handler_test.go (cung package).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubLockedEnrollmentService struct {
	service.EnrollmentServiceInterface
	err error
}

func (s *stubLockedEnrollmentService) UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO, isAdmin bool) (*dto.LessonProgressStateDTO, error) {
	return nil, s.err
}

func TestUpdateLessonProgress_BaiKhoa_403(t *testing.T) {
	h := NewEnrollmentHandler(&stubLockedEnrollmentService{err: service.ErrLessonLocked}, nil)
	lessonID := uuid.NewString()

	app := mountWithCaller("PUT", "/lessons/:lessonId/progress", uuid.New(), h.UpdateLessonProgress)
	if code := doJSON(t, app, "PUT", "/lessons/"+lessonID+"/progress", `{"status":"in_progress"}`); code != 403 {
		t.Errorf("PUT progress vao bai khoa phai tra 403, nhan %d", code)
	}

	app = mountWithCaller("POST", "/progress", uuid.New(), h.TrackProgressBeacon)
	if code := doJSON(t, app, "POST", "/progress", `{"lessonId":"`+lessonID+`","status":"in_progress","videoWatchedSeconds":5}`); code != 403 {
		t.Errorf("beacon vao bai khoa phai tra 403, nhan %d", code)
	}
}
