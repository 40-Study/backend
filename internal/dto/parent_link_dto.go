package dto

// Luồng phụ huynh gửi yêu cầu liên kết con (QA vòng 2 lane E, quyết định Q4).
// Giờ trả về dạng chuỗi RFC3339 có offset thật (utils.FormatTimestamp).

// CreateParentLinkRequestDto — body POST /family/link-requests (phụ huynh gửi).
type CreateParentLinkRequestDto struct {
	StudentEmail string  `json:"student_email" validate:"required,email,max=255"`
	Relationship string  `json:"relationship" validate:"required,oneof=parent guardian grandparent"`
	Message      *string `json:"message,omitempty" validate:"omitempty,max=500"`
}

// RespondParentLinkRequestDto — body POST /family/link-requests/:id/respond (học sinh trả lời).
type RespondParentLinkRequestDto struct {
	Action string `json:"action" validate:"required,oneof=accept reject"`
}

// LinkUserDto — thông tin rút gọn của bên kia trong yêu cầu/quan hệ.
type LinkUserDto struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	FullName  *string `json:"full_name,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	Email     string  `json:"email"`
}

// ParentLinkRequestDto — một yêu cầu liên kết. Danh sách "đã gửi" của phụ huynh có Student,
// danh sách "đến" của học sinh có Parent.
type ParentLinkRequestDto struct {
	ID           string       `json:"id"`
	Status       string       `json:"status"`
	Relationship string       `json:"relationship"`
	Message      *string      `json:"message,omitempty"`
	CreatedAt    string       `json:"created_at"`
	RespondedAt  *string      `json:"responded_at,omitempty"`
	Parent       *LinkUserDto `json:"parent,omitempty"`
	Student      *LinkUserDto `json:"student,omitempty"`
}

// LinkedParentDto — phụ huynh đang liên kết với học sinh (để học sinh xem và huỷ liên kết).
type LinkedParentDto struct {
	LinkUserDto
	Relationship string  `json:"relationship"`
	LinkedAt     *string `json:"linked_at,omitempty"`
}
