package router

// Test cho CAO-5 (review vòng 2, PR #60): GET /lessons/:lessonId/discussions trước bản vá đăng
// ký thẳng trên `api` KHÔNG qua auth, nên c.Locals("user_id") không bao giờ được điền —
// user_vote luôn sai/rỗng ngay cả với chính người đang đăng nhập. Bản vá chuyển route này vào
// nhóm auth (discussion_router.go). Test khẳng định ĐÚNG hành vi ĐÓ: request không kèm token bị
// 401 (route giờ yêu cầu đăng nhập), thay vì lọt thẳng tới handler như trước.

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
)

type fakeDiscussionServiceForAuthTest struct {
	gotUserID *uuid.UUID
	called    bool
}

func (f *fakeDiscussionServiceForAuthTest) CreatePost(ctx context.Context, userID uuid.UUID, req dto.CreateForumPostDTO) (*dto.ForumPostResponseDTO, error) {
	return nil, nil
}
func (f *fakeDiscussionServiceForAuthTest) GetPostBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.ForumPostDetailResponseDTO, error) {
	return nil, nil
}
func (f *fakeDiscussionServiceForAuthTest) ListPosts(ctx context.Context, category string, page, pageSize int, userID *uuid.UUID) (*dto.ForumPostListResponseDTO, error) {
	return nil, nil
}
func (f *fakeDiscussionServiceForAuthTest) ListPostsByLesson(ctx context.Context, lessonID uuid.UUID, page, pageSize int, userID *uuid.UUID) (*dto.ForumPostListResponseDTO, error) {
	f.called = true
	f.gotUserID = userID
	return &dto.ForumPostListResponseDTO{}, nil
}
func (f *fakeDiscussionServiceForAuthTest) AddComment(ctx context.Context, postSlug string, userID uuid.UUID, req dto.CreateForumCommentDTO) (*dto.ForumCommentResponseDTO, error) {
	return nil, nil
}
func (f *fakeDiscussionServiceForAuthTest) VoteDiscussion(ctx context.Context, discussionID, userID uuid.UUID, voteType string) error {
	return nil
}
func (f *fakeDiscussionServiceForAuthTest) RemoveVote(ctx context.Context, discussionID, userID uuid.UUID) error {
	return nil
}
func (f *fakeDiscussionServiceForAuthTest) DeletePost(ctx context.Context, postID, userID uuid.UUID) error {
	return nil
}

// TestLessonDiscussionsRoute_YeuCauAuth (CAO-5): khong kem Authorization/cookie phai bi 401 —
// truoc ban va, route nay khong co auth nen se lot toi handler va tra 200.
func TestLessonDiscussionsRoute_YeuCauAuth(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	fake := &fakeDiscussionServiceForAuthTest{}

	// cfg/redis nil an toan: AuthMiddleware tra 401 ngay khi khong co token, truoc khi cham
	// toi cfg/redis (xem comment tuong tu trong progress_quiz_routes_test.go).
	SetupDiscussionRoutes(api, nil, handler.NewDiscussionHandler(fake), nil)

	req := httptest.NewRequest("GET", "/api/lessons/"+uuid.New().String()+"/discussions", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, muon 401 (route phai yeu cau dang nhap sau CAO-5)", resp.StatusCode)
	}
	if fake.called {
		t.Fatal("service.ListPostsByLesson bi goi du request khong co token — auth middleware khong chan request nay")
	}
}
