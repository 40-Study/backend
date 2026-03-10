package model

import (
	"time"

	"github.com/google/uuid"
)

type WhiteboardSnapshot struct {
	BaseModel
	SessionID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"session_id"`
	SnapshotData string    `gorm:"type:jsonb;not null" json:"snapshot_data"`
	Version      int       `gorm:"default:1" json:"version"`
	SavedAt      time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"saved_at"`

	Session *LivestreamSession `gorm:"foreignKey:SessionID" json:"-"`
}

func (WhiteboardSnapshot) TableName() string {
	return "whiteboard_snapshots"
}
