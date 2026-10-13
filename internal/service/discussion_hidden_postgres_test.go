package service

// Review QA2 H1 (+web #3): `hide_discussion` (ADM-N4) phải thật sự ẩn bài. Trước đây chỉ hai truy vấn DANH SÁCH lọc
// is_hidden; trang chi tiết theo slug, danh sách bình luận và việc thêm bình luận vẫn chạy trên bài đã ẩn, còn
// báo cáo về một bình luận (reply) thì ẩn xong không có tác dụng gì. Postgres thật.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type hiddenForumWorld struct {
	t   *testing.T
	f   *s2Fixture
	svc *DiscussionService
	u   model.User
}

func newHiddenForumWorld(t *testing.T) *hiddenForumWorld {
	t.Helper()
	db := isolatedAPISchema(t)
	return &hiddenForumWorld{t: t, f: &s2Fixture{t: t, db: db}, u: (&s2Fixture{t: t, db: db}).user("author"),
		svc: NewDiscussionService(repository.NewDiscussionRepository(db), repository.NewEnrollmentRepository(db))}
}

func (w *hiddenForumWorld) post(hidden bool) model.Discussion {
	w.t.Helper()
	slug := "hidden-" + uuid.NewString()[:8]
	title := "Bai " + slug
	d := model.Discussion{UserID: w.u.ID, Title: &title, Slug: &slug, Content: "noi dung"}
	if err := w.f.db.Create(&d).Error; err != nil {
		w.t.Fatalf("tao bai: %v", err)
	}
	if hidden {
		w.f.db.Model(&d).Update("is_hidden", true)
	}
	return d
}

func (w *hiddenForumWorld) reply(parent model.Discussion, hidden bool) model.Discussion {
	w.t.Helper()
	r := model.Discussion{UserID: w.u.ID, Content: "reply " + uuid.NewString()[:6], ParentID: &parent.ID}
	if err := w.f.db.Create(&r).Error; err != nil {
		w.t.Fatalf("tao reply: %v", err)
	}
	if hidden {
		w.f.db.Model(&r).Update("is_hidden", true)
	}
	return r
}

func TestDiscussion_BaiDaAn_404ChoNguoiThuongTrenChiTietVaBinhLuan(t *testing.T) {
	w := newHiddenForumWorld(t)
	ctx := context.Background()
	hidden := w.post(true)

	if _, err := w.svc.GetPostBySlug(ctx, *hidden.Slug, nil); err == nil || err.Error() != "post not found" {
		t.Fatalf("bai da an phai 'post not found' voi khach, duoc %v", err)
	}
	uid := w.u.ID
	if _, err := w.svc.GetPostBySlug(ctx, *hidden.Slug, &uid); err == nil || err.Error() != "post not found" {
		t.Fatalf("bai da an phai 'post not found' voi ca tac gia (chi nguoi kiem duyet duoc xem): %v", err)
	}
	if _, err := w.svc.AddComment(ctx, *hidden.Slug, w.u.ID, dto.CreateForumCommentDTO{Content: "tra loi"}); err == nil || err.Error() != "post not found" {
		t.Fatalf("khong duoc binh luan vao bai da an: %v", err)
	}
	var n int64
	w.f.db.Model(&model.Discussion{}).Where("parent_id = ?", hidden.ID).Count(&n)
	if n != 0 {
		t.Fatalf("binh luan vao bai da an van duoc ghi (%d dong)", n)
	}
}

func TestDiscussion_ReplyDaAnBiLoaiKhoiChiTiet(t *testing.T) {
	w := newHiddenForumWorld(t)
	ctx := context.Background()
	p := w.post(false)
	shown := w.reply(p, false)
	gone := w.reply(p, true)

	got, err := w.svc.GetPostBySlug(ctx, *p.Slug, nil)
	if err != nil {
		t.Fatalf("GetPostBySlug: %v", err)
	}
	ids := map[uuid.UUID]bool{}
	for _, c := range got.Comments {
		ids[c.ID] = true
	}
	if !ids[shown.ID] || ids[gone.ID] || len(got.Comments) != 1 {
		t.Fatalf("muon chi reply hien, duoc %v", ids)
	}
}

func TestDiscussion_NguoiKiemDuyetXemDuocBaiVaReplyDaAn(t *testing.T) {
	w := newHiddenForumWorld(t)
	ctx := WithModeratorView(context.Background())
	hiddenPost := w.post(true)
	hiddenReply := w.reply(hiddenPost, true)

	got, err := w.svc.GetPostBySlug(ctx, *hiddenPost.Slug, nil)
	if err != nil {
		t.Fatalf("kiem duyet phai xem duoc bai da an: %v", err)
	}
	if len(got.Comments) != 1 || got.Comments[0].ID != hiddenReply.ID {
		t.Fatalf("kiem duyet phai thay reply da an: %+v", got.Comments)
	}
	// Nhung van khong the tra loi vao bai dang an.
	if _, err := w.svc.AddComment(ctx, *hiddenPost.Slug, w.u.ID, dto.CreateForumCommentDTO{Content: "x"}); err == nil {
		t.Fatal("khong duoc binh luan vao bai da an, ke ca kiem duyet")
	}
}
