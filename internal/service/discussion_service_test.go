package service

// R7 (code-reviewer-260919-1557): hoi dap theo bai hoc (LessonID != nil) truoc ban va nay
// KHONG kiem enroll, khong kiem lesson_id ton tai/thuoc khoa nao — bat ky ai da dang nhap deu
// doc/ghi duoc Q&A cua bat ky bai tra phi nao. Test o day khang dinh ca hai duong (doc va ghi)
// bi chan dung, va bai dang dien dan chung (khong LessonID) khong bi anh huong.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeDiscussionRepoR7 cai dat toi thieu DiscussionRepositoryInterface — du de mot bai dang
// dien dan CHUNG (khong lesson_id) tao thanh cong; cac test enroll/lesson-not-found khong cham
// toi repo nay (kiem tra chay TRUOC khi goi repo), nen khong can cai dat gi them.
type fakeDiscussionRepoR7 struct {
	repository.DiscussionRepositoryInterface
	created *model.Discussion
}

func (f *fakeDiscussionRepoR7) SlugExists(ctx context.Context, slug string) (bool, error) {
	return false, nil
}

func (f *fakeDiscussionRepoR7) CreatePost(ctx context.Context, post *model.Discussion) error {
	if post.ID == uuid.Nil {
		post.ID = uuid.New()
	}
	f.created = post
	return nil
}

func (f *fakeDiscussionRepoR7) GetPostByID(ctx context.Context, id uuid.UUID) (*model.Discussion, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, errors.New("not found")
}

// fakeEnrollmentRepoForDiscussion: rieng cho package nay, khong dung chung fakeEnrollmentRepoWatched
// vi can gia lap DUOC ca truong hop GetCourseIDByLessonID tra loi (lesson khong ton tai/section
// da xoa) — fakeEnrollmentRepoWatched (enrollment_watched_seconds_test.go) luon tra nil err.
type fakeEnrollmentRepoForDiscussion struct {
	repository.EnrollmentRepositoryInterface
	courseID   uuid.UUID
	courseErr  error
	enrollment *model.Enrollment
	// gotLessonID/gotUserID ghi lai tham so THAT SU nhan duoc — dam bao service truyen dung
	// lessonID/userID xuong repo, khong phai mot gia tri rong/sai lang.
	gotLessonID uuid.UUID
	gotUserID   uuid.UUID
}

func (f *fakeEnrollmentRepoForDiscussion) GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error) {
	f.gotLessonID = lessonID
	if f.courseErr != nil {
		return uuid.Nil, f.courseErr
	}
	return f.courseID, nil
}

func (f *fakeEnrollmentRepoForDiscussion) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	f.gotUserID = userID
	return f.enrollment, nil
}

func TestCreatePost_LessonIDKhongTonTai_TraLoiLessonNotFound(t *testing.T) {
	lessonID := uuid.New()
	enrollRepo := &fakeEnrollmentRepoForDiscussion{courseErr: errors.New("lesson not found in any course")}
	svc := NewDiscussionService(&fakeDiscussionRepoR7{}, enrollRepo)

	lid := lessonID.String()
	_, err := svc.CreatePost(context.Background(), uuid.New(), dto.CreateForumPostDTO{
		Title: "hoi bai nay", Content: "noi dung", Category: "qna", LessonID: &lid,
	})
	if !errors.Is(err, ErrDiscussionLessonNotFound) {
		t.Fatalf("err = %v, muon ErrDiscussionLessonNotFound (lesson_id khong ton tai/thuoc khoa nao)", err)
	}
}

func TestCreatePost_ChuaEnroll_BiChan(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	enrollRepo := &fakeEnrollmentRepoForDiscussion{courseID: courseID, enrollment: nil}
	svc := NewDiscussionService(&fakeDiscussionRepoR7{}, enrollRepo)

	lid := lessonID.String()
	_, err := svc.CreatePost(context.Background(), uuid.New(), dto.CreateForumPostDTO{
		Title: "hoi bai nay", Content: "noi dung", Category: "qna", LessonID: &lid,
	})
	if !errors.Is(err, ErrDiscussionNotEnrolled) {
		t.Fatalf("err = %v, muon ErrDiscussionNotEnrolled — chua enroll khong duoc tao bai hoi dap theo bai", err)
	}
	if enrollRepo.gotLessonID != lessonID {
		t.Fatalf("GetCourseIDByLessonID nhan lessonID = %s, muon %s", enrollRepo.gotLessonID, lessonID)
	}
}

func TestCreatePost_DaEnroll_TaoThanhCong(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	userID := uuid.New()
	enrollRepo := &fakeEnrollmentRepoForDiscussion{
		courseID:   courseID,
		enrollment: &model.Enrollment{UserID: userID, CourseID: courseID},
	}
	repo := &fakeDiscussionRepoR7{}
	svc := NewDiscussionService(repo, enrollRepo)

	lid := lessonID.String()
	post, err := svc.CreatePost(context.Background(), userID, dto.CreateForumPostDTO{
		Title: "hoi bai nay", Content: "noi dung", Category: "qna", LessonID: &lid,
	})
	if err != nil {
		t.Fatalf("da enroll van bi chan: %v", err)
	}
	if post == nil {
		t.Fatal("post tra ve nil du khong co loi")
	}
	if repo.created == nil || repo.created.LessonID == nil || *repo.created.LessonID != lessonID {
		t.Fatalf("ban ghi Discussion khong gan dung LessonID — created=%+v", repo.created)
	}
}

func TestCreatePost_KhongCoLessonID_KhongCanEnroll(t *testing.T) {
	// Bai dang dien dan CHUNG (khong gan bai hoc nao) khong duoc doi hoi enroll — enrollRepo
	// KHONG duoc goi toi (courseErr set de test do NGAY neu service lo goi nham).
	enrollRepo := &fakeEnrollmentRepoForDiscussion{courseErr: errors.New("khong duoc goi toi")}
	svc := NewDiscussionService(&fakeDiscussionRepoR7{}, enrollRepo)

	post, err := svc.CreatePost(context.Background(), uuid.New(), dto.CreateForumPostDTO{
		Title: "hoi chung", Content: "noi dung", Category: "programming",
	})
	if err != nil {
		t.Fatalf("bai dang dien dan chung (khong lesson_id) khong duoc bi chan: %v", err)
	}
	if post == nil {
		t.Fatal("post tra ve nil du khong co loi")
	}
	if enrollRepo.gotLessonID != uuid.Nil {
		t.Fatal("enrollRepo.GetCourseIDByLessonID bi goi toi du request khong co lesson_id")
	}
}

func TestListPostsByLesson_ChuaEnroll_BiChan(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	userID := uuid.New()
	enrollRepo := &fakeEnrollmentRepoForDiscussion{courseID: courseID, enrollment: nil}
	svc := NewDiscussionService(&fakeDiscussionRepoR7{}, enrollRepo)

	_, err := svc.ListPostsByLesson(context.Background(), lessonID, 1, 10, &userID)
	if !errors.Is(err, ErrDiscussionNotEnrolled) {
		t.Fatalf("err = %v, muon ErrDiscussionNotEnrolled — nguoi chua enroll khong duoc doc Q&A cua bai tra phi", err)
	}
}

func TestListPostsByLesson_KhongCoUserID_BiChan(t *testing.T) {
	lessonID := uuid.New()
	enrollRepo := &fakeEnrollmentRepoForDiscussion{courseErr: errors.New("khong duoc goi toi")}
	svc := NewDiscussionService(&fakeDiscussionRepoR7{}, enrollRepo)

	_, err := svc.ListPostsByLesson(context.Background(), lessonID, 1, 10, nil)
	if !errors.Is(err, ErrDiscussionNotEnrolled) {
		t.Fatalf("err = %v, muon ErrDiscussionNotEnrolled khi khong co userID", err)
	}
}
