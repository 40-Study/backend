package model

import (
	"time"

	"github.com/google/uuid"
)

type Class struct {
	BaseModel
	Name        string     `gorm:"type:varchar(255);not null" json:"name"`
	Description *string    `gorm:"type:text" json:"description,omitempty"`
	CourseID    *uuid.UUID `gorm:"type:uuid;index;column:course_id" json:"course_id,omitempty"`
	Status      string     `gorm:"type:varchar(20);default:'draft';not null" json:"status"` // draft, active, archived
	MaxStudents *int       `gorm:"column:max_students" json:"max_students,omitempty"`
	StartDate   *time.Time `gorm:"type:date;column:start_date" json:"start_date,omitempty"`
	EndDate     *time.Time `gorm:"type:date;column:end_date" json:"end_date,omitempty"`

	// Relationships
	Course   *Course        `gorm:"foreignKey:CourseID" json:"-"`
	Teachers []TeacherClass `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Students []StudentClass `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Class) TableName() string {
	return "classes"
}

type TeacherClass struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TeacherID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_teacher_class,priority:1;column:teacher_id"`
	ClassID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_teacher_class,priority:2;column:class_id"`
	Role       string    `gorm:"type:varchar(20);default:'primary';not null" json:"role"` // primary, assistant
	AssignedAt time.Time `gorm:"autoCreateTime;column:assigned_at" json:"assigned_at"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Teacher User  `gorm:"foreignKey:TeacherID;constraint:OnDelete:CASCADE" json:"-"`
	Class   Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
}

func (TeacherClass) TableName() string {
	return "teacher_classes"
}

type StudentClass struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	StudentID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_student_class,priority:1;column:student_id"`
	ClassID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_student_class,priority:2;column:class_id"`
	EnrolledAt time.Time `gorm:"autoCreateTime;column:enrolled_at" json:"enrolled_at"`
	Status     string    `gorm:"type:varchar(20);default:'active';not null" json:"status"` // active, dropped, completed
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Student User  `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
	Class   Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
}

func (StudentClass) TableName() string {
	return "student_classes"
}

type ClassSchedule struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClassID   uuid.UUID `gorm:"type:uuid;not null;index;column:class_id"`
	DayOfWeek int       `gorm:"not null;column:day_of_week" json:"day_of_week"` // 0=Sun, 1=Mon, ..., 6=Sat
	StartTime string    `gorm:"type:time;not null;column:start_time" json:"start_time"`
	EndTime   string    `gorm:"type:time;not null;column:end_time" json:"end_time"`
	Room      *string   `gorm:"type:varchar(100)" json:"room,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Class Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ClassSchedule) TableName() string {
	return "class_schedules"
}
