package dto

import "github.com/google/uuid"

type AnalyticsResponseDTO struct {
	SessionID        uuid.UUID `json:"session_id"`
	PeakViewers      int       `json:"peak_viewers"`
	TotalViewers     int       `json:"total_viewers"`
	TotalMessages    int       `json:"total_messages"`
	AvgWatchTimeSecs int       `json:"avg_watch_time_secs"`
}

type AssignmentAnalyticsDTO struct {
	AssignmentID       uuid.UUID          `json:"assignment_id"`
	TotalSubmissions   int                `json:"total_submissions"`
	AcceptedCount      int                `json:"accepted_count"`
	AcceptanceRate     float64            `json:"acceptance_rate"`
	AvgExecutionTime   float64            `json:"avg_execution_time"`
	AvgMemoryUsed      float64            `json:"avg_memory_used"`
	Difficulty         string             `json:"difficulty"`
	SuccessRateByLevel map[string]float64 `json:"success_rate_by_level"`
}

type ParticipantAnalyticsDTO struct {
	SessionID   uuid.UUID      `json:"session_id"`
	ActiveCount int            `json:"active_count"`
	TotalJoined int            `json:"total_joined"`
	ByRole      map[string]int `json:"by_role"`
}

type ViewerTrendDTO struct {
	Timestamp string `json:"timestamp"`
	Viewers   int    `json:"viewers"`
}
