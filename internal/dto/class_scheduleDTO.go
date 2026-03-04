package dto

import "github.com/google/uuid"

type CreateClassScheduleDTO struct {
	DayOfWeek int     `json:"day_of_week" binding:"min=0,max=6"`
	StartTime string  `json:"start_time" binding:"required"`
	EndTime   string  `json:"end_time" binding:"required"`
	Room      *string `json:"room" binding:"omitempty,max=100"`
}

type UpdateClassScheduleDTO struct {
	DayOfWeek *int    `json:"day_of_week" binding:"omitempty,min=0,max=6"`
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
	Room      *string `json:"room" binding:"omitempty,max=100"`
}

type ClassScheduleResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	ClassID   uuid.UUID `json:"class_id"`
	DayOfWeek int       `json:"day_of_week"`
	StartTime string    `json:"start_time"`
	EndTime   string    `json:"end_time"`
	Room      *string   `json:"room,omitempty"`
	CreatedAt string    `json:"created_at"`
}
