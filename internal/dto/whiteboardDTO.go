package dto

import "github.com/google/uuid"

type WhiteboardSnapshotDTO struct {
	SessionID    string `json:"session_id" validate:"required,uuid"`
	SnapshotData string `json:"snapshot_data" validate:"required"`
	Version      int    `json:"version"`
}

type WhiteboardSnapshotResponseDTO struct {
	ID           uuid.UUID `json:"id"`
	SessionID    uuid.UUID `json:"session_id"`
	SnapshotData string    `json:"snapshot_data"`
	Version      int       `json:"version"`
	SavedAt      string    `json:"saved_at"`
}

type WhiteboardEventDTO struct {
	Type    string `json:"type" validate:"required"`
	Action  string `json:"action" validate:"required,oneof=draw erase move undo redo"`
	Payload string `json:"payload"`
}
