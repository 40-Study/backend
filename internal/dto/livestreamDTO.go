package dto

import "github.com/google/uuid"

type CreateLivestreamDTO struct {
	Title       string `json:"title" validate:"required,min=3,max=255"`
	Description string `json:"description"`
	HostID      string `json:"host_id" validate:"required,uuid"`
	MaxViewers  int64  `json:"max_viewers"`
	IsRecorded  bool   `json:"is_recorded"`
}

type UpdateLivestreamDTO struct {
	Title       *string `json:"title" validate:"omitempty,min=3,max=255"`
	Description *string `json:"description"`
	MaxViewers  *int64  `json:"max_viewers"`
}

type StartLivestreamDTO struct {
	RoomName string `json:"room_name" validate:"required"`
}

type JoinLivestreamDTO struct {
	UserID string `json:"user_id" validate:"required,uuid"`
	Role   string `json:"role" validate:"omitempty,oneof=teacher assistant student viewer"`
}

type LeaveLivestreamDTO struct {
	UserID uuid.UUID `json:"user_id" validate:"required,uuid"`
}

type LivestreamResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	HostID      uuid.UUID `json:"host_id"`
	RoomName    string    `json:"room_name"`
	Status      string    `json:"status"`
	StartedAt   *string   `json:"started_at,omitempty"`
	EndedAt     *string   `json:"ended_at,omitempty"`
	MaxViewers  int64     `json:"max_viewers"`
	IsRecorded  bool      `json:"is_recorded"`
	Settings    string    `json:"settings"`
	CreatedAt   string    `json:"created_at"`
}

type LivestreamListDTO struct {
	Data     []LivestreamResponseDTO `json:"data"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type LivekitParticipantDTO struct {
	Identity     string `json:"identity"`
	Name         string `json:"name"`
	State        string `json:"state"`
	JoinedAt     int64  `json:"joined_at"`
	NumTracks    int    `json:"num_tracks"`
	IsPublishing bool   `json:"is_publishing"`
}

type LivestreamDetailDTO struct {
	LivestreamResponseDTO
	// LiveKit real-time data (only present when status=live)
	ActiveParticipants int                     `json:"active_participants"`
	NumPublishers      int                     `json:"num_publishers"`
	ActiveRecording    bool                    `json:"active_recording"`
	Participants       []LivekitParticipantDTO `json:"participants,omitempty"`
}

type ParticipantResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	JoinedAt  string    `json:"joined_at"`
	LeftAt    *string   `json:"left_at,omitempty"`
	IsActive  bool      `json:"is_active"`
	Token     string    `json:"token,omitempty"`
}

type ScreenShareDTO struct {
	UserID string `json:"user_id" validate:"required,uuid"`
	Action string `json:"action" validate:"required,oneof=start stop"`
}

type ModerationActionDTO struct {
	UserID   string `json:"user_id" validate:"required,uuid"`
	Action   string `json:"action" validate:"required,oneof=mute kick"`
	Reason   string `json:"reason"`
	Duration int    `json:"duration"`
}

type WhiteboardLockDTO struct {
	Locked bool `json:"locked"`
}
