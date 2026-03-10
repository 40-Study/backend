package dto

// CreateRoomDTO - payload to create a new LiveKit room
type CreateRoomDTO struct {
	RoomName        string `json:"room_name" validate:"required"`
	EmptyTimeout    uint32 `json:"empty_timeout"`    // seconds before auto-delete when empty (default 0 = server default)
	MaxParticipants uint32 `json:"max_participants"` // 0 = unlimited
	Metadata        string `json:"metadata"`
}

// JoinTokenDTO - payload to generate a participant join token
type JoinTokenDTO struct {
	Identity       string `json:"identity" validate:"required"` // unique participant ID
	Name           string `json:"name"`                         // display name
	CanPublish     *bool  `json:"can_publish"`
	CanSubscribe   *bool  `json:"can_subscribe"`
	CanPublishData *bool  `json:"can_publish_data"`
	IsHost         bool   `json:"is_host"` // grants room admin permissions
}

// UpdateParticipantDTO - payload to update participant permissions/metadata
type UpdateParticipantDTO struct {
	Metadata     string `json:"metadata"`
	CanPublish   *bool  `json:"can_publish"`
	CanSubscribe *bool  `json:"can_subscribe"`
}

// SendDataDTO - payload to send a data message in a room
type SendDataDTO struct {
	Data       string   `json:"data" validate:"required"`
	Topic      string   `json:"topic"`
	Identities []string `json:"identities"` // empty = broadcast to all
}

// UpdateRoomMetadataDTO - payload to update room metadata
type UpdateRoomMetadataDTO struct {
	Metadata string `json:"metadata" validate:"required"`
}
