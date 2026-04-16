package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/socket"
)

type ConversationServiceInterface interface {
	// Conversations
	ListConversations(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.ConversationListResponse, error)
	CreateDirectConversation(ctx context.Context, userID, targetUserID uuid.UUID) (*dto.ConversationResponse, error)
	GetConversation(ctx context.Context, userID, convID uuid.UUID) (*dto.ConversationResponse, error)
	MarkAsRead(ctx context.Context, userID, convID uuid.UUID) error
	MuteConversation(ctx context.Context, userID, convID uuid.UUID, muted bool) error
	PinConversation(ctx context.Context, userID, convID uuid.UUID, pinned bool) error

	// Messages
	SendMessage(ctx context.Context, userID, convID uuid.UUID, req dto.SendMessageRequest) (*dto.MessageResponse, error)
	EditMessage(ctx context.Context, userID, convID, messageID uuid.UUID, req dto.EditMessageRequest) (*dto.MessageResponse, error)
	DeleteMessage(ctx context.Context, userID, convID, messageID uuid.UUID) error
	ListMessages(ctx context.Context, userID, convID uuid.UUID, page, pageSize int) (*dto.MessageListResponse, error)
	PinMessage(ctx context.Context, userID, convID, messageID uuid.UUID) error
	UnpinMessage(ctx context.Context, userID, convID, messageID uuid.UUID) error

	// Reactions
	AddReaction(ctx context.Context, userID, convID, messageID uuid.UUID, emoji string) error
	RemoveReaction(ctx context.Context, userID, convID, messageID uuid.UUID, emoji string) error

	// Search & Unread
	SearchMessages(ctx context.Context, userID uuid.UUID, keyword string, convID *uuid.UUID, page, pageSize int) (*dto.MessageListResponse, error)
	GetUnreadCount(ctx context.Context, userID uuid.UUID) (*dto.UnreadCountResponse, error)
}

type ConversationService struct {
	convRepo        *repository.ConversationRepository
	participantRepo *repository.ConversationParticipantRepository
	messageRepo     *repository.MessageRepository
	reactionRepo    *repository.MessageReactionRepository
	notifier        *socket.Notifier
}

func NewConversationService(
	convRepo *repository.ConversationRepository,
	participantRepo *repository.ConversationParticipantRepository,
	messageRepo *repository.MessageRepository,
	reactionRepo *repository.MessageReactionRepository,
	notifier *socket.Notifier,
) *ConversationService {
	return &ConversationService{
		convRepo:        convRepo,
		participantRepo: participantRepo,
		messageRepo:     messageRepo,
		reactionRepo:    reactionRepo,
		notifier:        notifier,
	}
}

// ============================================================================
// CONVERSATIONS
// ============================================================================

func (s *ConversationService) ListConversations(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.ConversationListResponse, error) {
	conversations, total, err := s.convRepo.ListByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ConversationResponse, len(conversations))
	for i, conv := range conversations {
		responses[i] = s.toConversationResponse(&conv, userID)
	}

	return &dto.ConversationListResponse{
		Conversations: responses,
		TotalCount:    total,
		Page:          page,
		Limit:         pageSize,
	}, nil
}

func (s *ConversationService) CreateDirectConversation(ctx context.Context, userID, targetUserID uuid.UUID) (*dto.ConversationResponse, error) {
	if userID == targetUserID {
		return nil, errors.New("cannot create conversation with yourself")
	}

	// Check if direct conversation already exists
	existing, err := s.convRepo.GetDirectBetweenUsers(ctx, userID, targetUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		resp := s.toConversationResponse(existing, userID)
		return &resp, nil
	}

	conv := &model.Conversation{
		Type: model.ConversationTypeDirect,
	}
	if err := s.convRepo.Create(ctx, conv); err != nil {
		return nil, err
	}

	now := time.Now()
	participants := []model.ConversationParticipant{
		{ConversationID: conv.ID, UserID: userID, JoinedAt: now},
		{ConversationID: conv.ID, UserID: targetUserID, JoinedAt: now},
	}
	if err := s.participantRepo.CreateBatch(ctx, participants); err != nil {
		return nil, err
	}

	// Re-fetch with participants loaded
	conv, err = s.convRepo.GetByID(ctx, conv.ID)
	if err != nil {
		return nil, err
	}

	resp := s.toConversationResponse(conv, userID)
	return &resp, nil
}

func (s *ConversationService) GetConversation(ctx context.Context, userID, convID uuid.UUID) (*dto.ConversationResponse, error) {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return nil, err
	}

	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("conversation not found")
	}

	resp := s.toConversationResponse(conv, userID)
	return &resp, nil
}

func (s *ConversationService) MarkAsRead(ctx context.Context, userID, convID uuid.UUID) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}

	lastMsg, err := s.messageRepo.GetLastMessage(ctx, convID)
	if err != nil {
		return err
	}
	if lastMsg == nil {
		return nil
	}

	return s.participantRepo.MarkAsRead(ctx, convID, userID, lastMsg.ID)
}

func (s *ConversationService) MuteConversation(ctx context.Context, userID, convID uuid.UUID, muted bool) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}
	return s.participantRepo.UpdateMuted(ctx, convID, userID, muted)
}

func (s *ConversationService) PinConversation(ctx context.Context, userID, convID uuid.UUID, pinned bool) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}
	return s.participantRepo.UpdatePinned(ctx, convID, userID, pinned)
}

// ============================================================================
// MESSAGES
// ============================================================================

func (s *ConversationService) SendMessage(ctx context.Context, userID, convID uuid.UUID, req dto.SendMessageRequest) (*dto.MessageResponse, error) {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return nil, err
	}

	msgType := model.MessageTypeText
	if req.Type != "" {
		msgType = model.MessageType(req.Type)
	}

	var metadata []byte
	if req.Metadata != nil {
		metadata, _ = json.Marshal(req.Metadata)
	}

	msg := &model.Message{
		ConversationID: convID,
		SenderID:       &userID,
		Type:           msgType,
		Content:        req.Content,
		ReplyToID:      req.ReplyToID,
		Status:         model.MessageStatusSent,
	}
	if metadata != nil {
		msg.Metadata = metadata
	}

	if err := s.messageRepo.Create(ctx, msg); err != nil {
		return nil, err
	}

	// Update conversation last message
	_ = s.convRepo.UpdateLastMessage(ctx, convID, msg.ID)
	_ = s.convRepo.IncrementMessageCount(ctx, convID)

	// Increment unread for other participants
	_ = s.participantRepo.IncrementUnreadExcept(ctx, convID, userID)

	// Re-fetch message with relations
	msg, err := s.messageRepo.GetByID(ctx, msg.ID)
	if err != nil {
		return nil, err
	}

	resp := s.toMessageResponse(msg)

	// Broadcast via WebSocket to conversation channel
	channelName := "conversation:" + convID.String()
	s.notifier.SendToChannel(channelName, socket.EventConversationMessage, resp)

	return &resp, nil
}

func (s *ConversationService) EditMessage(ctx context.Context, userID, convID, messageID uuid.UUID, req dto.EditMessageRequest) (*dto.MessageResponse, error) {
	msg, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errors.New("message not found")
	}
	if msg.ConversationID != convID {
		return nil, errors.New("message does not belong to this conversation")
	}
	if msg.SenderID == nil || *msg.SenderID != userID {
		return nil, errors.New("can only edit your own messages")
	}

	now := time.Now()
	msg.Content = &req.Content
	msg.IsEdited = true
	msg.EditedAt = &now

	if err := s.messageRepo.Update(ctx, msg); err != nil {
		return nil, err
	}

	resp := s.toMessageResponse(msg)

	channelName := "conversation:" + convID.String()
	s.notifier.SendToChannel(channelName, socket.EventMessageEdited, resp)

	return &resp, nil
}

func (s *ConversationService) DeleteMessage(ctx context.Context, userID, convID, messageID uuid.UUID) error {
	msg, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return errors.New("message not found")
	}
	if msg.ConversationID != convID {
		return errors.New("message does not belong to this conversation")
	}
	if msg.SenderID == nil || *msg.SenderID != userID {
		return errors.New("can only delete your own messages")
	}

	if err := s.messageRepo.Delete(ctx, messageID); err != nil {
		return err
	}

	channelName := "conversation:" + convID.String()
	s.notifier.SendToChannel(channelName, socket.EventMessageDeleted, map[string]interface{}{
		"message_id":      messageID,
		"conversation_id": convID,
	})

	return nil
}

func (s *ConversationService) ListMessages(ctx context.Context, userID, convID uuid.UUID, page, pageSize int) (*dto.MessageListResponse, error) {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return nil, err
	}

	messages, total, err := s.messageRepo.ListByConversationID(ctx, convID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.MessageResponse, len(messages))
	for i, m := range messages {
		responses[i] = s.toMessageResponse(&m)
	}

	return &dto.MessageListResponse{
		Messages:   responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *ConversationService) PinMessage(ctx context.Context, userID, convID, messageID uuid.UUID) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}

	msg, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return errors.New("message not found")
	}
	if msg.ConversationID != convID {
		return errors.New("message does not belong to this conversation")
	}

	now := time.Now()
	msg.IsPinned = true
	msg.PinnedBy = &userID
	msg.PinnedAt = &now
	return s.messageRepo.Update(ctx, msg)
}

func (s *ConversationService) UnpinMessage(ctx context.Context, userID, convID, messageID uuid.UUID) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}

	msg, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return errors.New("message not found")
	}

	msg.IsPinned = false
	msg.PinnedBy = nil
	msg.PinnedAt = nil
	return s.messageRepo.Update(ctx, msg)
}

// ============================================================================
// REACTIONS
// ============================================================================

func (s *ConversationService) AddReaction(ctx context.Context, userID, convID, messageID uuid.UUID, emoji string) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}

	reaction := &model.MessageReaction{
		MessageID: messageID,
		UserID:    userID,
		Emoji:     emoji,
	}
	if err := s.reactionRepo.Create(ctx, reaction); err != nil {
		return err
	}

	channelName := "conversation:" + convID.String()
	s.notifier.SendToChannel(channelName, socket.EventMessageReaction, map[string]interface{}{
		"message_id":      messageID,
		"conversation_id": convID,
		"user_id":         userID,
		"emoji":           emoji,
		"action":          "add",
	})

	return nil
}

func (s *ConversationService) RemoveReaction(ctx context.Context, userID, convID, messageID uuid.UUID, emoji string) error {
	if err := s.requireParticipant(ctx, convID, userID); err != nil {
		return err
	}

	if err := s.reactionRepo.Delete(ctx, messageID, userID, emoji); err != nil {
		return err
	}

	channelName := "conversation:" + convID.String()
	s.notifier.SendToChannel(channelName, socket.EventMessageReaction, map[string]interface{}{
		"message_id":      messageID,
		"conversation_id": convID,
		"user_id":         userID,
		"emoji":           emoji,
		"action":          "remove",
	})

	return nil
}

// ============================================================================
// SEARCH & UNREAD
// ============================================================================

func (s *ConversationService) SearchMessages(ctx context.Context, userID uuid.UUID, keyword string, convID *uuid.UUID, page, pageSize int) (*dto.MessageListResponse, error) {
	messages, total, err := s.messageRepo.Search(ctx, userID, keyword, convID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.MessageResponse, len(messages))
	for i, m := range messages {
		responses[i] = s.toMessageResponse(&m)
	}

	return &dto.MessageListResponse{
		Messages:   responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *ConversationService) GetUnreadCount(ctx context.Context, userID uuid.UUID) (*dto.UnreadCountResponse, error) {
	totalUnread, err := s.participantRepo.GetTotalUnread(ctx, userID)
	if err != nil {
		return nil, err
	}

	counts, err := s.participantRepo.GetUnreadCounts(ctx, userID)
	if err != nil {
		return nil, err
	}

	convCounts := make([]dto.ConversationUnreadCount, len(counts))
	for i, c := range counts {
		convCounts[i] = dto.ConversationUnreadCount{
			ConversationID: c.ConversationID,
			UnreadCount:    c.UnreadCount,
		}
	}

	return &dto.UnreadCountResponse{
		TotalUnread:   totalUnread,
		Conversations: convCounts,
	}, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func (s *ConversationService) requireParticipant(ctx context.Context, convID, userID uuid.UUID) error {
	p, err := s.participantRepo.GetByConvAndUser(ctx, convID, userID)
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("not a participant of this conversation")
	}
	return nil
}

func (s *ConversationService) toConversationResponse(conv *model.Conversation, userID uuid.UUID) dto.ConversationResponse {
	resp := dto.ConversationResponse{
		ID:            conv.ID,
		Type:          string(conv.Type),
		Name:          conv.Name,
		GroupID:       conv.GroupID,
		LastMessageAt: conv.LastMessageAt,
		MessageCount:  conv.MessageCount,
		CreatedAt:     conv.CreatedAt,
		UpdatedAt:     conv.UpdatedAt,
	}

	if conv.LastMessage != nil {
		msgResp := s.toMessageResponse(conv.LastMessage)
		resp.LastMessage = &msgResp
	}

	for _, p := range conv.Participants {
		pr := dto.ParticipantResponse{
			UserID:     p.UserID,
			JoinedAt:   p.JoinedAt,
			LastReadAt: p.LastReadAt,
		}
		if p.User.ID != uuid.Nil {
			pr.UserName = p.User.UserName
			pr.Email = p.User.Email
			pr.AvatarURL = p.User.AvatarURL
			pr.IsOnline = s.notifier.IsUserOnline(p.UserID)
		}
		if p.UserID == userID {
			resp.UnreadCount = p.UnreadCount
			resp.IsMuted = p.IsMuted
			resp.IsPinned = p.IsPinned
		}
		resp.Participants = append(resp.Participants, pr)
	}

	return resp
}

func (s *ConversationService) toMessageResponse(msg *model.Message) dto.MessageResponse {
	resp := dto.MessageResponse{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Type:           string(msg.Type),
		Content:        msg.Content,
		Status:         string(msg.Status),
		IsEdited:       msg.IsEdited,
		EditedAt:       msg.EditedAt,
		IsPinned:       msg.IsPinned,
		CreatedAt:      msg.CreatedAt,
	}

	if len(msg.Metadata) > 0 {
		var meta interface{}
		if json.Unmarshal(msg.Metadata, &meta) == nil {
			resp.Metadata = meta
		}
	}

	if msg.Sender != nil {
		resp.SenderName = msg.Sender.UserName
		resp.SenderAvatar = msg.Sender.AvatarURL
	}

	if msg.ReplyTo != nil {
		reply := &dto.MessageReplyResponse{
			ID:      msg.ReplyTo.ID,
			Content: msg.ReplyTo.Content,
			Type:    string(msg.ReplyTo.Type),
		}
		if msg.ReplyTo.Sender != nil {
			reply.SenderID = msg.ReplyTo.SenderID
			reply.SenderName = msg.ReplyTo.Sender.UserName
		}
		resp.ReplyTo = reply
	}

	// Group reactions by emoji
	if len(msg.Reactions) > 0 {
		reactionMap := make(map[string]*dto.MessageReactionResponse)
		for _, r := range msg.Reactions {
			if _, ok := reactionMap[r.Emoji]; !ok {
				reactionMap[r.Emoji] = &dto.MessageReactionResponse{
					Emoji: r.Emoji,
					Users: []uuid.UUID{},
				}
			}
			reactionMap[r.Emoji].Count++
			reactionMap[r.Emoji].Users = append(reactionMap[r.Emoji].Users, r.UserID)
		}
		for _, v := range reactionMap {
			resp.Reactions = append(resp.Reactions, *v)
		}
	}

	for _, a := range msg.Attachments {
		resp.Attachments = append(resp.Attachments, dto.MessageAttachmentResponse{
			ID:           a.ID,
			FileName:     a.FileName,
			FileURL:      a.FileURL,
			FileSize:     a.FileSize,
			MimeType:     a.MimeType,
			ThumbnailURL: a.ThumbnailURL,
		})
	}

	return resp
}
