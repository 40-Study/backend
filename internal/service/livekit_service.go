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
	}
	if req.Name == "" {
		at.SetName(req.Identity)
	}
	at.SetValidFor(24 * time.Hour)
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

// UpdateParticipant updates a participant's permissions or metadata.
func (s *LivekitService) UpdateParticipant(ctx context.Context, roomName, identity string, req dto.UpdateParticipantDTO) (*livekit.ParticipantInfo, error) {
	updateReq := &livekit.UpdateParticipantRequest{
		Room:     roomName,
		Identity: identity,
		Metadata: req.Metadata,
	}
	if req.CanPublish != nil || req.CanSubscribe != nil {
		perm := &livekit.ParticipantPermission{
			CanPublishData: true, // keep data publishing on by default
		}
		if req.CanPublish != nil {
			perm.CanPublish = *req.CanPublish
		}
		if req.CanSubscribe != nil {
			perm.CanSubscribe = *req.CanSubscribe
		}
		updateReq.Permission = perm
	}
	return s.client().UpdateParticipant(ctx, updateReq)
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
