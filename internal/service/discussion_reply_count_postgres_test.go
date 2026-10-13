package service

// QA2 F-05: reply_count của bài diễn đàn chỉ được cộng khi thêm trả lời (AddComment) mà không bao giờ
// trừ khi xoá trả lời (DeletePost), nên danh sách hiện "N bình luận" lớn hơn số trả lời còn sống.
// Postgres thật (schema tạm). Mutation đã kiểm: bỏ bước tính lại trong DeletePost làm các test dưới ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type discussionCountFixture struct {
	t    *testing.T
	f    *orderFixture
	svc  *DiscussionService
	post *dto.ForumPostResponseDTO
	user uuid.UUID
}

func newDiscussionCountFixture(t *testing.T) *discussionCountFixture {
	t.Helper()
	f := newOrderFixture(t)
	svc := NewDiscussionService(repository.NewDiscussionRepository(f.db), nil)
	author := f.user()
	post, err := svc.CreatePost(context.Background(), author, dto.CreateForumPostDTO{
		Title: "Bài đếm trả lời", Content: "nội dung", Category: "qna",
	})
	if err != nil {
		t.Fatalf("tạo bài: %v", err)
	}
	return &discussionCountFixture{t: t, f: f, svc: svc, post: post, user: author}
}

func (d *discussionCountFixture) reply(parentID *string) uuid.UUID {
	d.t.Helper()
	c, err := d.svc.AddComment(context.Background(), d.post.Slug, d.user, dto.CreateForumCommentDTO{Content: "trả lời", ParentID: parentID})
	if err != nil {
		d.t.Fatalf("thêm trả lời: %v", err)
	}
	return c.ID
}

func (d *discussionCountFixture) storedCount() int {
	d.t.Helper()
	var p model.Discussion
	if err := d.f.db.Where("id = ?", d.post.ID).First(&p).Error; err != nil {
		d.t.Fatalf("đọc bài: %v", err)
	}
	return p.ReplyCount
}

func TestDeletePost_Reply_DecrementsThreadReplyCount(t *testing.T) {
	d := newDiscussionCountFixture(t)
	r1, _, _ := d.reply(nil), d.reply(nil), d.reply(nil)
	if got := d.storedCount(); got != 3 {
		t.Fatalf("sau 3 trả lời reply_count = %d, muốn 3", got)
	}

	if err := d.svc.DeletePost(context.Background(), r1, d.user); err != nil {
		t.Fatalf("xoá trả lời: %v", err)
	}
	if got := d.storedCount(); got != 2 {
		t.Errorf("xoá 1 trong 3 trả lời: reply_count = %d, muốn 2", got)
	}
}

// Trả lời lồng (parent_id là một trả lời khác): AddComment cộng vào BÀI GỐC, nên xoá nó cũng phải trừ ở bài gốc.
func TestDeletePost_NestedReply_DecrementsRootThread(t *testing.T) {
	d := newDiscussionCountFixture(t)
	top := d.reply(nil)
	topStr := top.String()
	nested := d.reply(&topStr)
	if got := d.storedCount(); got != 2 {
		t.Fatalf("sau 1 trả lời + 1 trả lời lồng reply_count = %d, muốn 2", got)
	}

	if err := d.svc.DeletePost(context.Background(), nested, d.user); err != nil {
		t.Fatalf("xoá trả lời lồng: %v", err)
	}
	if got := d.storedCount(); got != 1 {
		t.Errorf("xoá trả lời lồng: reply_count = %d, muốn 1", got)
	}
}

// Xoá bài gốc không lỗi, và đếm lại không bao giờ làm bài khác lệch.
func TestDeletePost_Root_DoesNotTouchOtherThreads(t *testing.T) {
	d := newDiscussionCountFixture(t)
	d.reply(nil)
	other, err := d.svc.CreatePost(context.Background(), d.user, dto.CreateForumPostDTO{Title: "Bài khác", Content: "x", Category: "qna"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.AddComment(context.Background(), other.Slug, d.user, dto.CreateForumCommentDTO{Content: "r"}); err != nil {
		t.Fatal(err)
	}

	if err := d.svc.DeletePost(context.Background(), d.post.ID, d.user); err != nil {
		t.Fatalf("xoá bài gốc: %v", err)
	}
	var o model.Discussion
	if err := d.f.db.Where("id = ?", other.ID).First(&o).Error; err != nil {
		t.Fatal(err)
	}
	if o.ReplyCount != 1 {
		t.Errorf("bài khác reply_count = %d, muốn 1", o.ReplyCount)
	}
}

// Số đã lệch sẵn (dữ liệu cũ) được sửa luôn theo số thật ở lần xoá kế tiếp, không trừ mù thành số âm.
func TestDeletePost_Reply_SelfHealsDriftedCounter(t *testing.T) {
	d := newDiscussionCountFixture(t)
	r1, _ := d.reply(nil), d.reply(nil)
	d.f.exec("UPDATE discussions SET reply_count = 0 WHERE id = ?", d.post.ID) // dữ liệu cũ: lệch xuống 0

	if err := d.svc.DeletePost(context.Background(), r1, d.user); err != nil {
		t.Fatal(err)
	}
	if got := d.storedCount(); got != 1 {
		t.Errorf("reply_count = %d, muốn 1 (số trả lời còn sống), không được âm hay giữ số lệch", got)
	}
}

// Review L2: AddComment chèn trả lời rồi cộng +1 ở hai câu lệnh riêng; một lần đếm lại của DeletePost chen vào giữa thì
// +1 cộng lên số đã có trả lời mới (lệch 1). Thêm trả lời nay cũng đếm lại theo số thật trong cùng giao dịch, nên số
// lệch sẵn được sửa ở ngay lần thêm.
func TestAddComment_RecountsFromTruth_SelfHealsDrift(t *testing.T) {
	d := newDiscussionCountFixture(t)
	d.reply(nil)
	d.reply(nil)
	d.f.exec("UPDATE discussions SET reply_count = 7 WHERE id = ?", d.post.ID) // lệch cao

	d.reply(nil)
	if got := d.storedCount(); got != 3 {
		t.Errorf("reply_count = %d, muốn 3 (số trả lời còn sống), không phải 8 (số lệch + 1)", got)
	}
}

// Thêm và xoá trả lời chạy ĐỒNG THỜI trên cùng một bài: số lưu phải bằng số trả lời còn sống. Cả hai đường khoá dòng bài
// gốc trước khi đếm nên không có lần đếm nào dựa trên ảnh chụp cũ của đường kia.
func TestReplyCount_ConcurrentAddAndDelete_MatchesLiveReplies(t *testing.T) {
	d := newDiscussionCountFixture(t)
	const seeded, adders = 10, 15
	var toDelete []uuid.UUID
	for i := 0; i < seeded; i++ {
		toDelete = append(toDelete, d.reply(nil))
	}

	errs := make(chan error, seeded+adders)
	for i := 0; i < adders; i++ {
		go func() {
			_, err := d.svc.AddComment(context.Background(), d.post.Slug, d.user, dto.CreateForumCommentDTO{Content: "đồng thời"})
			errs <- err
		}()
	}
	for _, id := range toDelete {
		go func(id uuid.UUID) { errs <- d.svc.DeletePost(context.Background(), id, d.user) }(id)
	}
	for i := 0; i < seeded+adders; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("thao tác đồng thời lỗi: %v", err)
		}
	}

	var live int64
	if err := d.f.db.Model(&model.Discussion{}).Where("parent_id = ?", d.post.ID).Count(&live).Error; err != nil {
		t.Fatal(err)
	}
	if live != adders {
		t.Fatalf("còn %d trả lời sống, muốn %d", live, adders)
	}
	if got := d.storedCount(); got != int(live) {
		t.Errorf("reply_count = %d, muốn %d (số trả lời còn sống)", got, live)
	}
}
