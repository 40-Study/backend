package model

import (
	"time"

	"github.com/google/uuid"
)

// ParentStudentRelation đại diện cho quan hệ phụ huynh - học sinh
type ParentStudentRelation struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ParentUserID  uuid.UUID `gorm:"type:uuid;not null;index:idx_psr_parent_id;column:parent_user_id" json:"parent_user_id"`
	StudentUserID uuid.UUID `gorm:"type:uuid;not null;index:idx_psr_student_id;column:student_user_id" json:"student_user_id"`
	Relationship  string    `gorm:"type:varchar(50);not null;default:'parent'" json:"relationship"` // parent, guardian, grandparent
	Status        string    `gorm:"type:varchar(20);default:'pending';index:idx_psr_status" json:"status"` // pending, active, revoked

	// Permissions
	CanViewProgress    bool `gorm:"default:true;column:can_view_progress" json:"can_view_progress"`
	CanViewGrades      bool `gorm:"default:true;column:can_view_grades" json:"can_view_grades"`
	CanViewAttendance  bool `gorm:"default:true;column:can_view_attendance" json:"can_view_attendance"`
	CanContactTeachers bool `gorm:"default:true;column:can_contact_teachers" json:"can_contact_teachers"`
	CanMakePayments    bool `gorm:"default:true;column:can_make_payments" json:"can_make_payments"`
	CanManageAccount   bool `gorm:"default:false;column:can_manage_account" json:"can_manage_account"`

	// Confirmation
	ConfirmedAt *time.Time `gorm:"column:confirmed_at" json:"confirmed_at,omitempty"`
	ConfirmedBy *string    `gorm:"type:varchar(50);column:confirmed_by" json:"confirmed_by,omitempty"` // student, system, admin

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	Parent  *User `gorm:"foreignKey:ParentUserID;constraint:OnDelete:CASCADE" json:"parent,omitempty"`
	Student *User `gorm:"foreignKey:StudentUserID;constraint:OnDelete:CASCADE" json:"student,omitempty"`
}

func (ParentStudentRelation) TableName() string {
	return "parent_student_relations"
}

// Các hằng số trạng thái ParentStudentRelation
const (
	ParentStudentStatusPending = "pending"
	ParentStudentStatusActive  = "active"
	ParentStudentStatusRevoked = "revoked"
)

// Các hằng số quan hệ
const (
	RelationshipParent      = "parent"
	RelationshipGuardian    = "guardian"
	RelationshipGrandparent = "grandparent"
)
