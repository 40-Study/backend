package dto

type CollaborationEventDTO struct {
	Type      string `json:"type" validate:"required"`
	SessionID string `json:"session_id" validate:"required"`
	UserID    string `json:"user_id" validate:"required"`
	Payload   string `json:"payload"`
}

type EventTypes struct {
	WhiteboardEvent     string `json:"whiteboard_event"`
	AssignmentPublished string `json:"assignment_published"`
	HandRaised          string `json:"hand_raised"`
	ChatMessage         string `json:"chat_message"`
	PollCreated         string `json:"poll_created"`
	PollResponse        string `json:"poll_response"`
}

const (
	EventTypeWhiteboard    = "whiteboard_event"
	EventTypeAssignmentPub = "assignment_published"
	EventTypeHandRaised    = "hand_raised"
	EventTypeChat          = "chat_message"
	EventTypePollCreated   = "poll_created"
	EventTypePollResponse  = "poll_response"
	EventTypeUserJoined    = "user_joined"
	EventTypeUserLeft      = "user_left"
	EventTypeScreenshare   = "screenshare"
	EventTypeMute          = "mute"
	EventTypeKick          = "kick"
)

type HandRaiseDTO struct {
	SessionID string `json:"session_id" validate:"required,uuid"`
	UserID    string `json:"user_id" validate:"required,uuid"`
	IsRaised  bool   `json:"is_raised"`
}

type PollCreateDTO struct {
	SessionID string          `json:"session_id" validate:"required,uuid"`
	Question  string          `json:"question" validate:"required"`
	Options   []PollOptionDTO `json:"options" validate:"required,min=2,max=6"`
}

type PollOptionDTO struct {
	Text  string `json:"text" validate:"required"`
	Index int    `json:"index"`
}

type PollResponseDTO struct {
	PollID      string `json:"poll_id" validate:"required,uuid"`
	UserID      string `json:"user_id" validate:"required,uuid"`
	OptionIndex int    `json:"option_index" validate:"gte=0"`
}
