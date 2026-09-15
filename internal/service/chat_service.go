package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type ChatServiceInterface interface {
	// SendMessage/GetMessages: userID la nguoi goi THAT SU (access token) — ca hai deu phai la
	// THANH VIEN cua phien (EnsureSessionMember) truoc khi doc/ghi chat cua phien do. Truoc day
	// khong kiem gi: bat ky user dang nhap nao cung doc duoc chat cua phien bat ky, va SendMessage
	// nhan `user_id` tu body nen gui duoc tin nhan mao danh nguoi khac (V3-6, issue #58).
	SendMessage(ctx context.Context, userID uuid.UUID, req dto.SendChatMessageDTO) (*model.ChatMessage, error)
	GetMessages(ctx context.Context, userID, sessionID uuid.UUID, page, pageSize int) (*dto.ChatMessageListDTO, error)
	// DeleteMessage/PinMessage/UnPinMessage: actorID la nguoi goi THAT SU. Xoa cho phep TAC GIA
	// tin nhan HOAC nguoi quan tri phien (EnsureSessionManage); ghim/bo ghim la thao tac kiem
	// duyet nen chi nguoi quan tri phien. Truoc day DeleteMessage nhan ca `message_id` lan
	// `deleted_by` tu body (deleted_by tuy y client khai), Pin/UnPin khong kiem gi ca.
	DeleteMessage(ctx context.Context, actorID, messageID uuid.UUID) error
	PinMessage(ctx context.Context, actorID, messageID uuid.UUID) error
	UnPinMessage(ctx context.Context, actorID, messageID uuid.UUID) error
}

type ChatService struct {
	repo           repository.ChatMessageRepositoryInterface
	analyticsRepo  repository.AnalyticsRepositoryInterface
	livestreamRepo repository.LivestreamRepositoryInterface
	livekitSvc     LivekitServiceInterface
	// livestreamSvc (V3-6, issue #58): nguon su that duy nhat cho "nguoi nay co quan he gi voi
	// phien khong" (EnsureSessionMember/EnsureSessionManage) — khong lam lai phep kiem
	// host/GV lop/instructor/hoc sinh da co san trong LivestreamService.
	livestreamSvc LivestreamServiceInterface
}

func NewChatService(
	repo repository.ChatMessageRepositoryInterface,
	analyticsRepo repository.AnalyticsRepositoryInterface,
	livestreamRepo repository.LivestreamRepositoryInterface,
	livekitSvc LivekitServiceInterface,
	livestreamSvc LivestreamServiceInterface,
) *ChatService {
	return &ChatService{
		repo:           repo,
		analyticsRepo:  analyticsRepo,
		livestreamRepo: livestreamRepo,
		livekitSvc:     livekitSvc,
		livestreamSvc:  livestreamSvc,
	}
}

func (s *ChatService) SendMessage(ctx context.Context, userID uuid.UUID, req dto.SendChatMessageDTO) (*model.ChatMessage, error) {
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return nil, errors.New("invalid session_id")
	}

	// V3-6 (issue #58): chi thanh vien phien (host/GV lop/instructor khoa/hoc sinh da enroll) moi
	// duoc gui tin nhan vao phien do.
	if err := s.livestreamSvc.EnsureSessionMember(ctx, sessionID, userID); err != nil {
		return nil, err
	}

	message := &model.ChatMessage{
		SessionID: sessionID,
		UserID:    userID,
		Message:   req.Message,
	}

	if err := s.repo.Create(ctx, message); err != nil {
		return nil, err
	}

	// M-05 (audit 260909 vòng 2): bọc SafeGo — panic trong goroutine gửi tin nhắn (vd
	// analyticsRepo/livekitSvc lỗi runtime) trước đây sập cả server, ảnh hưởng mọi session.
	utils.SafeGo(func() { s.analyticsRepo.IncrementTotalMessages(ctx, sessionID) })

	saved := message
	if err != nil {
		return nil, err
	}

	utils.SafeGo(func() {
		bg, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		session, err := s.livestreamRepo.GetByID(bg, sessionID)
		if err != nil || session == nil {
			return
		}
		lkPayload := map[string]interface{}{
			"id":        saved.ID,
			"timestamp": saved.CreatedAt.UnixMilli(),
			"message":   saved.Message,
		}
		payload, err := json.Marshal(lkPayload)
		if err != nil {
			return
		}
		// Broadcast via LiveKit data channel
		if err := s.livekitSvc.SendData(bg, session.RoomName, dto.SendDataDTO{
			Data:  string(payload),
			Topic: "lk-chat-message",
		}); err != nil {
			// Log but don't fail - chat is still saved to DB
			fmt.Printf("livekit broadcast failed: %v\n", err)
		}
	})

	return saved, nil
}

func (s *ChatService) GetMessages(ctx context.Context, userID, sessionID uuid.UUID, page, pageSize int) (*dto.ChatMessageListDTO, error) {
	// V3-6 (issue #58): chi thanh vien phien moi doc duoc lich su chat cua phien do — truoc day
	// bat ky user dang nhap nao cung doc duoc chat cua bat ky lop nao.
	if err := s.livestreamSvc.EnsureSessionMember(ctx, sessionID, userID); err != nil {
		return nil, err
	}

	messages, total, err := s.repo.GetBySession(ctx, sessionID, page, pageSize)
	if err != nil {
		return nil, err
	}

	var data []dto.ChatMessageResponseDTO
	for _, msg := range messages {
		data = append(data, s.toResponseDTO(msg))
	}

	return &dto.ChatMessageListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// DeleteMessage (V3-6, issue #58): actorID la nguoi goi that su. Cho phep XOA neu la TAC GIA tin
// nhan, hoac nguoi QUAN TRI duoc phien (host/GV lop/instructor khoa). Truoc day `deleted_by` lay
// tu body — client tu khai bat ky UUID nao cung duoc ghi nhan la nguoi xoa, khong kiem quyen gi.
func (s *ChatService) DeleteMessage(ctx context.Context, actorID, messageID uuid.UUID) error {
	message, err := s.repo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if message == nil {
		return errors.New("message not found")
	}

	if message.UserID != actorID {
		if err := s.livestreamSvc.EnsureSessionManage(ctx, message.SessionID, actorID); err != nil {
			return err
		}
	}

	return s.repo.SoftDelete(ctx, messageID, actorID)
}

// PinMessage/UnPinMessage (V3-6, issue #58): thao tac KIEM DUYET — chi nguoi quan tri duoc phien
// (host/GV lop/instructor khoa) moi ghim/bo ghim tin nhan. Truoc day khong kiem gi ca: bat ky user
// dang nhap nao cung ghim duoc tin nhan cua bat ky phien nao.
func (s *ChatService) PinMessage(ctx context.Context, actorID, messageID uuid.UUID) error {
	message, err := s.repo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if message == nil {
		return errors.New("message not found")
	}
	if err := s.livestreamSvc.EnsureSessionManage(ctx, message.SessionID, actorID); err != nil {
		return err
	}
	return s.repo.Pin(ctx, messageID)
}

func (s *ChatService) UnPinMessage(ctx context.Context, actorID, messageID uuid.UUID) error {
	message, err := s.repo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if message == nil {
		return errors.New("message not found")
	}
	if err := s.livestreamSvc.EnsureSessionManage(ctx, message.SessionID, actorID); err != nil {
		return err
	}
	return s.repo.UnPin(ctx, messageID)
}

func (s *ChatService) toResponseDTO(msg model.ChatMessage) dto.ChatMessageResponseDTO {
	var userName string
	if msg.User != nil {
		userName = msg.User.UserName
		if msg.User.FullName != nil {
			userName = *msg.User.FullName
		}
	}

	return dto.ChatMessageResponseDTO{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		UserID:    msg.UserID,
		UserName:  userName,
		Message:   msg.Message,
		IsPinned:  msg.IsPinned,
		ParentID:  msg.ParentID,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339),
	}
}
