package app

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// participantChecker và groupMemberChecker là phần repository mà WebSocket authorizer cần; tách thành interface
// để test không cần dựng cả app. Cài đặt thật là ConversationParticipantRepository / GroupMemberRepository, cùng
// những repository mà ConversationService/GroupService dùng để kiểm quyền đọc/gửi tin, nên WebSocket và REST luôn
// đồng ý về "ai còn là thành viên" (cùng bộ lọc left_at IS NULL và status = ACTIVE).
type participantChecker interface {
	IsActiveParticipant(ctx context.Context, convID, userID uuid.UUID) (bool, error)
}

type groupMemberChecker interface {
	GetActiveByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error)
}

// wsChannelAuthorizer quyết định user nào được đăng ký (subscribe) kênh WebSocket nào (S6).
//
// Trước đây defaultAuthorizer trả true cho MỌI "conversation:<id>" và "group:<id>": bất kỳ tài khoản nào biết id
// hội thoại (lộ qua GET /groups/:slug) đều nghe được toàn bộ tin nhắn nhóm private. Nay:
//   - user:<id>          chỉ chính chủ
//   - notifications      mọi tài khoản đã đăng nhập (kênh chung, không mang dữ liệu riêng)
//   - conversation:<id>  participant CÒN HIỆU LỰC (chưa rời/bị kick/ban) của hội thoại
//   - group:<id>         thành viên ACTIVE của nhóm
//   - kênh khác / id sai định dạng: từ chối
//
// Lỗi hạ tầng trả ra ngoài và bị coi là từ chối (fail-closed) ở socket.subscribe.
type wsChannelAuthorizer struct {
	participants participantChecker
	groupMembers groupMemberChecker
	// directBlocks nối sau khi dựng services (authorizer được tạo trước services trong app.New). nil = chưa nối:
	// mọi "đang gõ" của người đã đăng ký kênh được chuyển tiếp như trước.
	directBlocks directBlockChecker
}

// directBlockChecker — DM 1-1 giữa userID và người kia có chặn ở bất kỳ chiều nào không. Cài đặt thật là
// ConversationService.IsDirectBlocked, cùng quy tắc khoá gửi tin nên "đang gõ" và "gửi" không bao giờ lệch.
type directBlockChecker interface {
	IsDirectBlocked(ctx context.Context, convID, userID uuid.UUID) (bool, error)
}

func newWSChannelAuthorizer(p participantChecker, g groupMemberChecker) *wsChannelAuthorizer {
	return &wsChannelAuthorizer{participants: p, groupMembers: g}
}

// SetDirectBlockChecker nối kiểm tra chặn cho sự kiện "đang gõ" (socket.TypingGuard).
func (a *wsChannelAuthorizer) SetDirectBlockChecker(c directBlockChecker) { a.directBlocks = c }

// CanBroadcastTyping cài đặt socket.TypingGuard: DM 1-1 bị chặn thì không phát "đang gõ".
func (a *wsChannelAuthorizer) CanBroadcastTyping(userID, conversationID uuid.UUID) (bool, error) {
	if a.directBlocks == nil {
		return true, nil
	}
	blocked, err := a.directBlocks.IsDirectBlocked(context.Background(), conversationID, userID)
	return !blocked, err
}

func (a *wsChannelAuthorizer) CanSubscribe(userID uuid.UUID, channel string) (bool, error) {
	if channel == "notifications" {
		return true, nil
	}
	prefix, rawID, found := strings.Cut(channel, ":")
	if !found {
		return false, nil
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		return false, nil
	}

	ctx := context.Background()
	switch prefix {
	case "user":
		return id == userID, nil
	case "conversation":
		return a.participants.IsActiveParticipant(ctx, id, userID)
	case "group":
		member, err := a.groupMembers.GetActiveByGroupAndUser(ctx, id, userID)
		return member != nil, err
	default:
		return false, nil
	}
}
