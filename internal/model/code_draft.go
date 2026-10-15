package model

import (
	"time"

	"github.com/google/uuid"
)

// CodeDraft: bản nháp code phía server của một người dùng (contract sec. 4 và 6). Scope là "exercise:<uuid>" (trình
// soạn của bài tập trong bài học) hoặc "sandbox:<path>" (một tệp trong cây tệp sandbox); thư mục là tiền tố đường
// dẫn, không có dòng riêng. Mỗi (user_id, scope) có đúng một dòng: lưu là upsert.
type CodeDraft struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_code_drafts_user_scope,priority:1" json:"user_id"`
	Scope    string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_code_drafts_user_scope,priority:2" json:"scope"`
	Language string    `gorm:"type:varchar(20);not null;default:''" json:"language"` // "" hoặc khoá chạy được (contract sec. 3)
	Code     string    `gorm:"type:text;not null;default:''" json:"code"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CodeDraft) TableName() string {
	return "code_drafts"
}
