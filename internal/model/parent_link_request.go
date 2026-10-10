package model

import (
	"time"

	"github.com/google/uuid"
)

// ParentLinkRequest — yêu cầu liên kết do PHỤ HUYNH gửi tới email của học sinh (QA vòng 2, lane E,
// quyết định Q4 của chủ dự án: "phụ huynh tự gửi yêu cầu liên kết con, con phải xác nhận").
//
// Vì sao là bảng riêng, không ghi thẳng một dòng `pending` vào parent_student_relations: mọi API
// xem dữ liệu con (parent_dashboard_service.verifyParentChildRelation) đọc bảng quan hệ. Chỉ khi
// con bấm xác nhận thì dòng quan hệ `active` mới được tạo/kích hoạt, nên trước lúc đó phụ huynh
// không có bất kỳ dòng quan hệ nào để lọt qua kiểm tra quyền (chống IDOR theo cấu trúc).
//
// Vì sao vẫn lưu theo EMAIL và StudentUserID vẫn nullable: từ quyết định D8 (đảo thiết kế chống dò
// của PR #81, MAJOR-1) yêu cầu chỉ được tạo khi email thuộc một tài khoản HỌC SINH, và
// StudentUserID được gán ngay lúc tạo; email không phải học sinh nhận 404 STUDENT_NOT_FOUND và
// không có dòng nào. Cột vẫn nullable (và truy vấn khớp theo email cho dòng NULL vẫn giữ) chỉ vì
// các dòng cũ tạo trước D8; runParentLinkGhostCleanup huỷ mềm những dòng cũ không còn ai trả lời.
//
// Chiều ngược lại (học sinh mời phụ huynh qua email) vẫn ở parent_invitations.
type ParentLinkRequest struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ParentUserID uuid.UUID `gorm:"type:uuid;not null;index:idx_plr_parent" json:"parent_user_id"`
	// StudentUserID: được gán ngay lúc tạo (D8). Chỉ còn nil ở các dòng cũ tạo trước D8, khi email
	// chưa thuộc tài khoản học sinh nào; những dòng đó được gán khi học sinh có email đó trả lời.
	StudentUserID *uuid.UUID `gorm:"type:uuid;index:idx_plr_student" json:"student_user_id,omitempty"`
	// StudentEmail: email phụ huynh nhập, đã chuẩn hoá (trim + chữ thường). default '' chỉ để
	// AutoMigrate thêm được cột NOT NULL trên bảng đã có dòng; RunPostMigrations điền lại.
	StudentEmail string `gorm:"type:varchar(255);not null;default:'';index:idx_plr_student_email" json:"student_email"`
	Relationship string `gorm:"type:varchar(50);not null;default:'parent'" json:"relationship"`
	// Giá trị hợp lệ: ParentLinkRequestStatuses (CHECK sinh bằng buildCheckConstraintSQL).
	Status      string     `gorm:"type:varchar(20);not null;default:'pending';index:idx_plr_status" json:"status"`
	Message     *string    `gorm:"type:text" json:"message,omitempty"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	CreatedAt   time.Time  `gorm:"index:idx_plr_created_at" json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Parent  *User `gorm:"foreignKey:ParentUserID;constraint:OnDelete:CASCADE" json:"parent,omitempty"`
	Student *User `gorm:"foreignKey:StudentUserID;constraint:OnDelete:SET NULL" json:"student,omitempty"`
}

func (ParentLinkRequest) TableName() string {
	return "parent_link_requests"
}

const (
	ParentLinkRequestStatusPending   = "pending"   // phụ huynh vừa gửi, chờ con xác nhận
	ParentLinkRequestStatusAccepted  = "accepted"  // con đã xác nhận, quan hệ active đã được tạo
	ParentLinkRequestStatusRejected  = "rejected"  // con từ chối — bắt đầu thời gian chờ gửi lại
	ParentLinkRequestStatusCancelled = "cancelled" // phụ huynh rút, hoặc bị huỷ kèm khi huỷ liên kết
)

// ParentLinkRequestStatuses là nguồn sự thật duy nhất cho CHECK constraint của
// parent_link_requests.status (RunPostMigrations sinh SQL từ slice này).
var ParentLinkRequestStatuses = []string{
	ParentLinkRequestStatusPending,
	ParentLinkRequestStatusAccepted,
	ParentLinkRequestStatusRejected,
	ParentLinkRequestStatusCancelled,
}

// ParentLinkAttempt — MỖI lần phụ huynh bấm gửi yêu cầu liên kết (thành công hay không) là một
// dòng. Hạn mức theo ngày đếm bảng này, và được kiểm TRƯỚC khi tra email (review PR #81,
// MAJOR-1): nếu chỉ đếm yêu cầu đã lưu, các lần dò email thất bại không bao giờ bị tính.
type ParentLinkAttempt struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ParentUserID uuid.UUID `gorm:"type:uuid;not null;index:idx_pla_parent_created,priority:1"`
	CreatedAt    time.Time `gorm:"not null;index:idx_pla_parent_created,priority:2"`
}

func (ParentLinkAttempt) TableName() string {
	return "parent_link_attempts"
}
