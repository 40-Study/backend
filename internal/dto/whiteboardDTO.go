package dto

import "github.com/google/uuid"

type WhiteboardSnapshotDTO struct {
	SessionID string `json:"session_id" validate:"required,uuid"`
	Elements  any    `json:"elements"`
	AppState  any    `json:"app_state"`
	Version   int    `json:"version"`
}

type WhiteboardSnapshotResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Elements  any      `json:"elements"`
	AppState  any      `json:"app_state"`
	Version   int      `json:"version"`
	SavedAt   string   `json:"saved_at"`
}

type WhiteboardEventDTO struct {
	Type    string `json:"type" validate:"required"`
	Action  string `json:"action" validate:"required"`
	Payload string `json:"payload"`
}
