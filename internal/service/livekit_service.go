package service

import (
	"context"
	"fmt"
	"time"

	"github.com/livekit/protocol/auth"
	livekit "github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
)

type LivekitServiceInterface interface {
	CreateRoom(ctx context.Context, req dto.CreateRoomDTO) (*livekit.Room, error)
	ListRooms(ctx context.Context) ([]*livekit.Room, error)
	GetRoom(ctx context.Context, roomName string) (*livekit.Room, error)
	DeleteRoom(ctx context.Context, roomName string) error
	UpdateRoomMetadata(ctx context.Context, roomName string, req dto.UpdateRoomMetadataDTO) (*livekit.Room, error)
	CreateJoinToken(ctx context.Context, roomName string, req dto.JoinTokenDTO) (string, error)
	ListParticipants(ctx context.Context, roomName string) ([]*livekit.ParticipantInfo, error)
	GetParticipant(ctx context.Context, roomName, identity string) (*livekit.ParticipantInfo, error)
	RemoveParticipant(ctx context.Context, roomName, identity string) error
	UpdateParticipant(ctx context.Context, roomName, identity string, req dto.UpdateParticipantDTO) (*livekit.ParticipantInfo, error)
	SendData(ctx context.Context, roomName string, req dto.SendDataDTO) error
}

type LivekitService struct {
	apiKey    string
	apiSecret string
	host      string
}

func NewLivekitService(cfg *config.Config) *LivekitService {
	return &LivekitService{
		apiKey:    cfg.LivekitAPIKey,
		apiSecret: cfg.LivekitAPISecret,
		host:      fmt.Sprintf("http://%s:%s", cfg.LivekitNodeIP, cfg.LivekitNodePort),
	}
}

func (s *LivekitService) client() *lksdk.RoomServiceClient {
	return lksdk.NewRoomServiceClient(s.host, s.apiKey, s.apiSecret)
}

// CreateRoom creates a new LiveKit room.
func (s *LivekitService) CreateRoom(ctx context.Context, req dto.CreateRoomDTO) (*livekit.Room, error) {
	return s.client().CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            req.RoomName,
		EmptyTimeout:    req.EmptyTimeout,
		MaxParticipants: req.MaxParticipants,
		Metadata:        req.Metadata,
	})
}

// ListRooms returns all active rooms.
func (s *LivekitService) ListRooms(ctx context.Context) ([]*livekit.Room, error) {
	res, err := s.client().ListRooms(ctx, &livekit.ListRoomsRequest{})
	if err != nil {
		return nil, err
	}
	return res.Rooms, nil
}

// GetRoom returns info for a single room by name.
func (s *LivekitService) GetRoom(ctx context.Context, roomName string) (*livekit.Room, error) {
	res, err := s.client().ListRooms(ctx, &livekit.ListRoomsRequest{
		Names: []string{roomName},
	})
	if err != nil {
		return nil, err
	}
	if len(res.Rooms) == 0 {
		return nil, fmt.Errorf("room %q not found", roomName)
	}
	return res.Rooms[0], nil
}

// DeleteRoom ends a room and disconnects all participants.
func (s *LivekitService) DeleteRoom(ctx context.Context, roomName string) error {
	_, err := s.client().DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: roomName})
	return err
}

// UpdateRoomMetadata updates the metadata of a room.
func (s *LivekitService) UpdateRoomMetadata(ctx context.Context, roomName string, req dto.UpdateRoomMetadataDTO) (*livekit.Room, error) {
	return s.client().UpdateRoomMetadata(ctx, &livekit.UpdateRoomMetadataRequest{
		Room:     roomName,
		Metadata: req.Metadata,
	})
}

// CreateJoinToken generates a JWT token for a participant to join a room.
func (s *LivekitService) CreateJoinToken(ctx context.Context, roomName string, req dto.JoinTokenDTO) (string, error) {
	canPublish := true
	if req.CanPublish != nil {
		canPublish = *req.CanPublish
	}
	canSubscribe := true
	if req.CanSubscribe != nil {
		canSubscribe = *req.CanSubscribe
	}
	canPublishData := true
	if req.CanPublishData != nil {
		canPublishData = *req.CanPublishData
	}

	grant := &auth.VideoGrant{
		RoomJoin:       true,
		Room:           roomName,
		CanPublish:     &canPublish,
		CanSubscribe:   &canSubscribe,
		CanPublishData: &canPublishData,
	}
	if req.IsHost {
		grant.RoomAdmin = true
		grant.RoomCreate = true
	}

	at := auth.NewAccessToken(s.apiKey, s.apiSecret)
	at.SetIdentity(req.Identity)
	if req.Name != "" {
		at.SetName(req.Name)
	} else {
		at.SetName(req.Identity)
	}
	// F-5 (issue #58 review vòng 2): 24h cũ là dư thừa so với một buổi học và làm token của
	// người bị kick sống quá lâu — RemoveParticipant chỉ ngắt kết nối, không thu hồi được JWT đã
	// ký, nên hạ TTL xuống độ dài một buổi học là lớp phòng thủ duy nhất còn lại phía server.
	at.SetValidFor(4 * time.Hour)
	at.SetVideoGrant(grant)
	return at.ToJWT()
}

// ListParticipants returns all participants currently in a room.
func (s *LivekitService) ListParticipants(ctx context.Context, roomName string) ([]*livekit.ParticipantInfo, error) {
	res, err := s.client().ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: roomName})
	if err != nil {
		return nil, err
	}
	return res.Participants, nil
}

// GetParticipant returns info for a single participant.
func (s *LivekitService) GetParticipant(ctx context.Context, roomName, identity string) (*livekit.ParticipantInfo, error) {
	return s.client().GetParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room:     roomName,
		Identity: identity,
	})
}

// RemoveParticipant forcefully disconnects a participant from a room.
func (s *LivekitService) RemoveParticipant(ctx context.Context, roomName, identity string) error {
	_, err := s.client().RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room:     roomName,
		Identity: identity,
	})
	return err
}

// buildUpdateParticipantRequest (R3-1, issue #58 review vòng 4): tách phần XÂY
// *livekit.UpdateParticipantRequest ra khỏi phần gọi mạng thật (`s.client().UpdateParticipant`),
// để test đọc được ĐÚNG kết quả build mà không cần fake/mock gRPC client — cùng tinh thần
// buildRestoreAndReactivateQuery (enrollment_repository.go) và buildStudentClassExistsQuery
// (class_repository.go): xoá/sửa sai mặc định ở đây sẽ làm test đỏ ngay, thay vì chỉ đọc diff.
func buildUpdateParticipantRequest(roomName, identity string, req dto.UpdateParticipantDTO) *livekit.UpdateParticipantRequest {
	updateReq := &livekit.UpdateParticipantRequest{
		Room:     roomName,
		Identity: identity,
		Metadata: req.Metadata,
	}
	// D3/F-2 (issue #58 review vòng 2): trước đây nhánh này chỉ kích hoạt khi CanPublish HOẶC
	// CanSubscribe được truyền, và luôn HARDCODE CanPublishData=true — nên gọi UpdateParticipant
	// CHỈ để tắt CanPublishData (khoá bảng trắng) không có tác dụng gì (field không tồn tại
	// trong request, hoặc bị ghi đè về true). LiveKit REPLACE toàn bộ permission khi Permission
	// khác nil (không merge từng field) — nên khi chỉ một field được truyền, các field còn lại
	// PHẢI được set tường minh theo giá trị hiện tại mà caller biết (không suy đoán ở đây).
	//
	// R2-1 (issue #58 review vòng 3, BLOCKER): bản vá D3/F-2 ở trên chỉ mặc định CanPublishData,
	// bỏ quên CanSubscribe — nó ở zero-value (false) trừ khi caller truyền tường minh, và KHÔNG
	// call site nào trong livestream_service.go từng truyền field này (mute/duyệt-thu screenshare/
	// khoá-mở bảng). Hệ quả thật: mute một học sinh cắt luôn khả năng nghe/nhìn của họ; khoá/mở
	// bảng chạy vòng lặp UpdateParticipant cho MỌI người không phải host/GV nên CẢ LỚP mất
	// subscribe — phòng học đen hình. Mặc định CanSubscribe=true khi không truyền, đúng ý đã ghi
	// trong comment ở trên nhưng trước đây chưa làm. R3-1 (review vòng 3): mặc định này KHÔNG có
	// test bảo vệ — xem TestBuildUpdateParticipantRequest_* trong livekit_service_test.go.
	if req.CanPublish != nil || req.CanSubscribe != nil || req.CanPublishData != nil || len(req.CanPublishSources) > 0 {
		perm := &livekit.ParticipantPermission{
			CanSubscribe:   true,
			CanPublishData: true,
		}
		if req.CanPublish != nil {
			perm.CanPublish = *req.CanPublish
		}
		if req.CanSubscribe != nil {
			perm.CanSubscribe = *req.CanSubscribe
		}
		if req.CanPublishData != nil {
			perm.CanPublishData = *req.CanPublishData
		}
		if len(req.CanPublishSources) > 0 {
			perm.CanPublishSources = trackSourcesFromStrings(req.CanPublishSources)
		}
		updateReq.Permission = perm
	}
	return updateReq
}

// UpdateParticipant updates a participant's permissions or metadata.
func (s *LivekitService) UpdateParticipant(ctx context.Context, roomName, identity string, req dto.UpdateParticipantDTO) (*livekit.ParticipantInfo, error) {
	return s.client().UpdateParticipant(ctx, buildUpdateParticipantRequest(roomName, identity, req))
}

// trackSourcesFromStrings (D5, issue #58 review vòng 3) chuyển các tên nguồn publish dạng chuỗi
// (dùng ở tầng DTO để không ép internal/dto phụ thuộc kiểu vendor livekit) sang
// []livekit.TrackSource mà ParticipantPermission.CanPublishSources yêu cầu.
func trackSourcesFromStrings(sources []string) []livekit.TrackSource {
	out := make([]livekit.TrackSource, 0, len(sources))
	for _, s := range sources {
		switch s {
		case "camera":
			out = append(out, livekit.TrackSource_CAMERA)
		case "microphone":
			out = append(out, livekit.TrackSource_MICROPHONE)
		case "screen_share":
			out = append(out, livekit.TrackSource_SCREEN_SHARE)
		case "screen_share_audio":
			out = append(out, livekit.TrackSource_SCREEN_SHARE_AUDIO)
		}
	}
	return out
}

// trackSourceStrings (R2-1/R2-11, issue #58 review vòng 3) chuyển ngược []livekit.TrackSource ->
// []string — dùng khi LockWhiteboard đọc lại permission HIỆN TẠI của participant (từ
// ListParticipants) để truyền nguyên vẹn CanPublishSources thay vì làm mất nguồn đã được duyệt
// khi chỉ muốn đổi CanPublishData.
func trackSourceStrings(sources []livekit.TrackSource) []string {
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		switch s {
		case livekit.TrackSource_CAMERA:
			out = append(out, "camera")
		case livekit.TrackSource_MICROPHONE:
			out = append(out, "microphone")
		case livekit.TrackSource_SCREEN_SHARE:
			out = append(out, "screen_share")
		case livekit.TrackSource_SCREEN_SHARE_AUDIO:
			out = append(out, "screen_share_audio")
		}
	}
	return out
}

// SendData broadcasts a data message to participants in a room.
func (s *LivekitService) SendData(ctx context.Context, roomName string, req dto.SendDataDTO) error {
	sendReq := &livekit.SendDataRequest{
		Room: roomName,
		Data: []byte(req.Data),
		Kind: livekit.DataPacket_RELIABLE,
	}
	if req.Topic != "" {
		sendReq.Topic = &req.Topic
	}
	if len(req.Identities) > 0 {
		sendReq.DestinationIdentities = req.Identities
	}
	_, err := s.client().SendData(ctx, sendReq)
	return err
}
