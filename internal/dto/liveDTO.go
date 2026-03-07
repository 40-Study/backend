package dto

type CreateLiveTokenDTO struct {
	RoomName string `json:"room_name" validate:"required"`
	Identity string `json:"identity" validate:"required"`
}