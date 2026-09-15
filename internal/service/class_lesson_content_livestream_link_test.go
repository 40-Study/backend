package service

// Test cho N10 (review vong 2, tu review web): lesson content type "livestream" truoc day tao
// phien xong roi bo qua (`_, _ = s.livestreamSvc.Create(...)`), khong co cach nao tu content lay
// lai duoc id phien — web mo phong theo id lesson_content (`/rooms/<lesson_content_id>`) nen join
// luon hong vi do khong phai RoomName/session id that. Test nay chot chan hanh vi ghi lai
// content.LivestreamSessionID khi tao phien thanh cong, dung truc tiep ham private
// createLivestreamSession (cung package) de khong phai fake toan bo be mat cua
// AssignClassToContent.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeLessonRepoForLivestreamLink: chi UpdateContent duoc dung trong pham vi test nay — cac
// method khac cua repository.LessonRepositoryInterface se panic neu bi goi (embed interface that,
// khong tu khai gia tri zero).
type fakeLessonRepoForLivestreamLink struct {
	repository.LessonRepositoryInterface
	updateContentCalls int
	lastContent        *model.LessonContent
}

func (f *fakeLessonRepoForLivestreamLink) UpdateContent(ctx context.Context, content *model.LessonContent) error {
	f.updateContentCalls++
	f.lastContent = content
	return nil
}

// fakeLivestreamSvcForLink: chi Create duoc dung trong pham vi test nay.
type fakeLivestreamSvcForLink struct {
	LivestreamServiceInterface
	session *model.LivestreamSession
	err     error
}

func (f *fakeLivestreamSvcForLink) Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error) {
	return f.session, f.err
}

// TestCreateLivestreamSession_GhiLaiLivestreamSessionIDVaoContent (N10): tao phien thanh cong ->
// content.LivestreamSessionID phai duoc set dung bang session.ID, va UpdateContent phai duoc goi
// dung 1 lan.
func TestCreateLivestreamSession_GhiLaiLivestreamSessionIDVaoContent(t *testing.T) {
	sessionID := uuid.New()
	fakeLesson := &fakeLessonRepoForLivestreamLink{}
	fakeLivestream := &fakeLivestreamSvcForLink{
		session: &model.LivestreamSession{BaseModel: model.BaseModel{ID: sessionID}},
	}

	s := &ClassLessonContentService{
		lessonRepo:    fakeLesson,
		livestreamSvc: fakeLivestream,
	}

	title := "Buoi hoc truc tiep"
	content := &model.LessonContent{ID: uuid.New(), Type: "livestream", Title: &title}
	scheduledAt := time.Now().Add(24 * time.Hour)
	clc := &model.ClassLessonContent{
		ClassID:         uuid.New(),
		LessonContentID: content.ID,
		ScheduledAt:     &scheduledAt,
	}
	class := &model.Class{Name: "Lop A"}
	courseID := uuid.New()
	userID := uuid.New()

	s.createLivestreamSession(context.Background(), clc, content, class, courseID, userID)

	if fakeLesson.updateContentCalls != 1 {
		t.Fatalf("UpdateContent duoc goi %d lan, muon 1", fakeLesson.updateContentCalls)
	}
	if fakeLesson.lastContent == nil || fakeLesson.lastContent.LivestreamSessionID == nil {
		t.Fatal("content.LivestreamSessionID van la nil sau khi tao phien thanh cong")
	}
	if *fakeLesson.lastContent.LivestreamSessionID != sessionID {
		t.Errorf("LivestreamSessionID = %s, muon %s (id phien vua tao)", *fakeLesson.lastContent.LivestreamSessionID, sessionID)
	}
	if content.LivestreamSessionID == nil || *content.LivestreamSessionID != sessionID {
		t.Error("bien content goc (con tro truyen vao) khong duoc cap nhat LivestreamSessionID")
	}
}

// TestCreateLivestreamSession_TaoPhienLoi_KhongGoiUpdateContent: livestreamSvc.Create tra loi ->
// khong duoc goi UpdateContent (khong co session id nao de ghi).
func TestCreateLivestreamSession_TaoPhienLoi_KhongGoiUpdateContent(t *testing.T) {
	fakeLesson := &fakeLessonRepoForLivestreamLink{}
	fakeLivestream := &fakeLivestreamSvcForLink{err: errors.New("loi ha tang gia lap")}

	s := &ClassLessonContentService{
		lessonRepo:    fakeLesson,
		livestreamSvc: fakeLivestream,
	}

	content := &model.LessonContent{ID: uuid.New(), Type: "livestream"}
	clc := &model.ClassLessonContent{ClassID: uuid.New(), LessonContentID: content.ID}
	class := &model.Class{Name: "Lop A"}

	s.createLivestreamSession(context.Background(), clc, content, class, uuid.New(), uuid.New())

	if fakeLesson.updateContentCalls != 0 {
		t.Errorf("UpdateContent bi goi %d lan du tao phien loi, muon 0", fakeLesson.updateContentCalls)
	}
	if content.LivestreamSessionID != nil {
		t.Error("content.LivestreamSessionID khong con nil du tao phien that bai")
	}
}

// TestToResponseDTO_ContentLivestream_TraDungLivestreamSessionID (N10, bo sung theo yeu cau mo
// rong cua team-lead sang mapper "class lesson content"): ClassLessonContentResponseDTO — dung
// cho GET /lesson-contents/:id/classes va GET /classes/:id/content-schedule — phai lo dung
// livestream_session_id lay tu LessonContent da preload, khong chi LessonContentResponseDTO.
func TestToResponseDTO_ContentLivestream_TraDungLivestreamSessionID(t *testing.T) {
	sessionID := uuid.New()
	s := &ClassLessonContentService{}

	clc := &model.ClassLessonContent{
		ID:      uuid.New(),
		ClassID: uuid.New(),
		Class:   model.Class{Name: "Lop A"},
		LessonContent: model.LessonContent{
			Type:                "livestream",
			LivestreamSessionID: &sessionID,
		},
	}

	resp := s.toResponseDTO(clc)

	if resp.LivestreamSessionID == nil {
		t.Fatal("response.LivestreamSessionID la nil du LessonContent da co gia tri")
	}
	if *resp.LivestreamSessionID != sessionID {
		t.Errorf("LivestreamSessionID = %s, muon %s", *resp.LivestreamSessionID, sessionID)
	}
}

// TestToResponseDTO_ContentVideo_LivestreamSessionIDLaNull: content khong phai livestream (vd
// video) -> LivestreamSessionID phai la null, khong duoc bay ra gia tri rac.
func TestToResponseDTO_ContentVideo_LivestreamSessionIDLaNull(t *testing.T) {
	s := &ClassLessonContentService{}

	clc := &model.ClassLessonContent{
		ID:            uuid.New(),
		ClassID:       uuid.New(),
		Class:         model.Class{Name: "Lop A"},
		LessonContent: model.LessonContent{Type: "video"},
	}

	resp := s.toResponseDTO(clc)

	if resp.LivestreamSessionID != nil {
		t.Errorf("LivestreamSessionID = %v, muon nil cho content type=video", *resp.LivestreamSessionID)
	}
}
