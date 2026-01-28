package model

import (
	"time"

	"gorm.io/gorm"
)

// Role code constants
const (
	RoleCodeStudentIndependent = "student_independent" // Học sinh tự chủ tài chính
	RoleCodeStudentDependent   = "student_dependent"   // Học sinh chưa tự chủ tài chính
	RoleCodeParent             = "parent"              // Phụ huynh
)

type UserRole struct {
	ID          uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Code        string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Name        string         `gorm:"type:varchar(100);not null" json:"name"`
	Description *string        `gorm:"type:varchar(500)" json:"description,omitempty"`
	IsActive    bool           `gorm:"default:true;index" json:"is_active"`

	// Relationships
	Users []User `gorm:"foreignKey:RoleID" json:"-"`
}

func (UserRole) TableName() string {
	return "user_roles"
}

// DefaultUserRoles trả về danh sách role mặc định để seed vào database
func DefaultUserRoles() []UserRole {
	studentIndependentDesc := "Học sinh có khả năng tự quản lý tài chính cá nhân"
	studentDependentDesc := "Học sinh cần sự giám sát tài chính từ phụ huynh"
	parentDesc := "Phụ huynh giám sát tài chính của học sinh"

	return []UserRole{
		{
			ID:          1,
			Code:        RoleCodeStudentIndependent,
			Name:        "Học sinh tự chủ tài chính",
			Description: &studentIndependentDesc,
			IsActive:    true,
		},
		{
			ID:          2,
			Code:        RoleCodeStudentDependent,
			Name:        "Học sinh chưa tự chủ tài chính",
			Description: &studentDependentDesc,
			IsActive:    true,
		},
		{
			ID:          3,
			Code:        RoleCodeParent,
			Name:        "Phụ huynh",
			Description: &parentDesc,
			IsActive:    true,
		},
	}
}
