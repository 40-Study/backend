package service

import (
	"context"

	"study.com/v1/internal/model"
)

// Review QA2 H1: bài/bình luận diễn đàn bị kiểm duyệt ẩn (is_hidden) trả 404 cho mọi người trừ người kiểm duyệt. Cờ này đi
// theo context (handler đặt sau khi kiểm REPORTS_MODERATE) để không đổi chữ ký DiscussionServiceInterface dùng chung.
type moderatorViewKey struct{}

// WithModeratorView đánh dấu request này được xem nội dung diễn đàn đã ẩn.
func WithModeratorView(ctx context.Context) context.Context {
	return context.WithValue(ctx, moderatorViewKey{}, true)
}

// IsModeratorView báo request có được xem nội dung diễn đàn đã ẩn hay không.
func IsModeratorView(ctx context.Context) bool {
	v, _ := ctx.Value(moderatorViewKey{}).(bool)
	return v
}

// visibleComments bỏ các bình luận đã ẩn với người không phải kiểm duyệt. Bình luận con của một bình luận đã ẩn tự biến
// mất theo vì buildCommentTree chỉ gắn con vào cha còn hiện.
func visibleComments(ctx context.Context, comments []model.Discussion) []model.Discussion {
	if IsModeratorView(ctx) {
		return comments
	}
	out := make([]model.Discussion, 0, len(comments))
	for _, c := range comments {
		if !c.IsHidden {
			out = append(out, c)
		}
	}
	return out
}
