package service

// Test cho N10 (review vong 2): LessonContentResponseDTO phai tra dung
// livestream_session_id da luu tren model.LessonContent (chieu doc, doi xung voi chieu ghi da
// chot chan o class_lesson_content_livestream_link_test.go). Dung GetContentByID vi day la duong
// don gian nhat di qua toContentResponseDTO — ham dung chung cho ca GetContentsByLessonID
// (endpoint web that su goi: /lessons/:lessonId/contents).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type fakeLessonRepoForContentRead struct {
	repository.LessonRepositoryInterface
	content *model.LessonContent
}

func (f *fakeLessonRepoForContentRead) GetContentByID(ctx context.Context, id uuid.UUID) (*model.LessonContent, error) {
	return f.content, nil
}

// TestGetContentByID_TraDungLivestreamSessionID (N10): content da co LivestreamSessionID (do
// createLivestreamSession ghi lai truoc do) -> response DTO phai mang dung gia tri nay, khong
// duoc bo qua hay tra null oan.
func TestGetContentByID_TraDungLivestreamSessionID(t *testing.T) {
	sessionID := uuid.New()
	contentID := uuid.New()
	fake := &fakeLessonRepoForContentRead{
		content: &model.LessonContent{
			ID:                  contentID,
			Type:                "livestream",
			LivestreamSessionID: &sessionID,
		},
	}
	s := NewLessonContentService(fake, nil, nil, nil, nil, nil)

	resp, err := s.GetContentByID(context.Background(), contentID)
	if err != nil {
		t.Fatalf("GetContentByID loi: %v", err)
	}
	if resp.LivestreamSessionID == nil {
		t.Fatal("response.LivestreamSessionID la nil du content model da co gia tri")
	}
	if *resp.LivestreamSessionID != sessionID {
		t.Errorf("LivestreamSessionID = %s, muon %s", *resp.LivestreamSessionID, sessionID)
	}
}

// TestGetContentByID_ChuaCoPhien_TraNull: content chua tung tao phien (LivestreamSessionID nil
// trong model) -> response phai la null, khong duoc bay ra gia tri rac (vd uuid.Nil).
func TestGetContentByID_ChuaCoPhien_TraNull(t *testing.T) {
	contentID := uuid.New()
	fake := &fakeLessonRepoForContentRead{
		content: &model.LessonContent{ID: contentID, Type: "video"},
	}
	s := NewLessonContentService(fake, nil, nil, nil, nil, nil)

	resp, err := s.GetContentByID(context.Background(), contentID)
	if err != nil {
		t.Fatalf("GetContentByID loi: %v", err)
	}
	if resp.LivestreamSessionID != nil {
		t.Errorf("LivestreamSessionID = %v, muon nil (chua tung tao phien)", *resp.LivestreamSessionID)
	}
}
