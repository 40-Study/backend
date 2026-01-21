package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Category struct {
	gorm.Model
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ParentID     *uuid.UUID `gorm:"type:uuid;index" json:"parent_id,omitempty"`
	Name         string     `gorm:"type:varchar(100);not null" json:"name"`
	Slug         string     `gorm:"type:varchar(100);uniqueIndex;not null" json:"slug"`
	Description  *string    `gorm:"type:text" json:"description,omitempty"`
	IconURL      *string    `gorm:"type:varchar(500);column:icon_url" json:"icon_url,omitempty"`
	DisplayOrder int        `gorm:"default:0" json:"display_order"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`

	// Relationships
	Parent   *Category  `gorm:"foreignKey:ParentID" json:"-"`
	Children []Category `gorm:"foreignKey:ParentID" json:"-"`
	Courses  []Course   `gorm:"foreignKey:CategoryID" json:"-"`
}

func (Category) TableName() string {
	return "categories"
}

type Tag struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Name      string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"name"`
	Slug      string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"slug"`

	// Relationships
	Courses []Course `gorm:"many2many:course_tags" json:"-"`
}

func (Tag) TableName() string {
	return "tags"
}

type Course struct {
	gorm.Model
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InstructorID      uuid.UUID        `gorm:"type:uuid;not null;index" json:"instructor_id"`
	CategoryID        *uuid.UUID       `gorm:"type:uuid;index" json:"category_id,omitempty"`
	Title             string           `gorm:"type:varchar(255);not null" json:"title"`
	Slug              string           `gorm:"type:varchar(255);uniqueIndex;not null" json:"slug"`
	ShortDescription  *string          `gorm:"type:varchar(500)" json:"short_description,omitempty"`
	Description       *string          `gorm:"type:text" json:"description,omitempty"`
	ThumbnailURL      *string          `gorm:"type:varchar(500);column:thumbnail_url" json:"thumbnail_url,omitempty"`
	PreviewVideoURL   *string          `gorm:"type:varchar(500);column:preview_video_url" json:"preview_video_url,omitempty"`
	Level             string           `gorm:"type:varchar(20);default:'beginner';check:level IN ('beginner', 'intermediate', 'advanced', 'all_levels')" json:"level"`
	Language          string           `gorm:"type:varchar(10);default:'vi'" json:"language"`
	Price             decimal.Decimal  `gorm:"type:decimal(12,2);default:0;index" json:"price"`
	DiscountPrice     *decimal.Decimal `gorm:"type:decimal(12,2)" json:"discount_price,omitempty"`
	DiscountExpiresAt *time.Time       `json:"discount_expires_at,omitempty"`
	TotalDurationMins int              `gorm:"default:0;column:total_duration_minutes" json:"total_duration_minutes"`
	TotalLessons      int              `gorm:"default:0" json:"total_lessons"`
	TotalStudents     int              `gorm:"default:0" json:"total_students"`
	AverageRating     decimal.Decimal  `gorm:"type:decimal(2,1);default:0;index" json:"average_rating"`
	TotalReviews      int              `gorm:"default:0" json:"total_reviews"`
	Requirements      pq.StringArray   `gorm:"type:text[]" json:"requirements"`
	Objectives        pq.StringArray   `gorm:"type:text[]" json:"objectives"`
	TargetAudience    pq.StringArray   `gorm:"type:text[]" json:"target_audience"`
	Status            string           `gorm:"type:varchar(20);default:'draft';check:status IN ('draft', 'pending_review', 'published', 'archived');index" json:"status"`
	PublishedAt       *time.Time       `json:"published_at,omitempty"`
	IsFeatured        bool             `gorm:"default:false" json:"is_featured"`
	IsFree            bool             `gorm:"default:false" json:"is_free"`

	// Relationships
	Instructor  User         `gorm:"foreignKey:InstructorID" json:"-"`
	Category    *Category    `gorm:"foreignKey:CategoryID" json:"-"`
	Tags        []Tag        `gorm:"many2many:course_tags" json:"-"`
	Sections    []Section    `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Enrollments []Enrollment `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Quizzes     []Quiz       `gorm:"foreignKey:CourseID" json:"-"`
}

func (Course) TableName() string {
	return "courses"
}

type Section struct {
	gorm.Model
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CourseID     uuid.UUID `gorm:"type:uuid;not null;index" json:"course_id"`
	Title        string    `gorm:"type:varchar(255);not null" json:"title"`
	Description  *string   `gorm:"type:text" json:"description,omitempty"`
	DisplayOrder int       `gorm:"not null" json:"display_order"`

	// Relationships
	Course  Course   `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Lessons []Lesson `gorm:"foreignKey:SectionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Section) TableName() string {
	return "sections"
}

type Lesson struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SectionID    uuid.UUID `gorm:"type:uuid;not null;index" json:"section_id"`
	Title        string    `gorm:"type:varchar(255);not null" json:"title"`
	Description  *string   `gorm:"type:text" json:"description,omitempty"`
	ContentType  string    `gorm:"type:varchar(20);not null;check:content_type IN ('video', 'article', 'quiz', 'assignment')" json:"content_type"`
	DisplayOrder int       `gorm:"not null" json:"display_order"`
	DurationMins int       `gorm:"default:0;column:duration_minutes" json:"duration_minutes"`
	IsPreview    bool      `gorm:"default:false" json:"is_preview"`
	IsMandatory  bool      `gorm:"default:true" json:"is_mandatory"`

	// Relationships
	Section        Section            `gorm:"foreignKey:SectionID;constraint:OnDelete:CASCADE" json:"-"`
	Video          *LessonVideo       `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Article        *LessonArticle     `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Attachments    []LessonAttachment `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Quiz           *Quiz              `gorm:"foreignKey:LessonID" json:"-"`
	LessonProgress []LessonProgress   `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	UserNotes      []UserNote         `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Discussions    []Discussion       `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Lesson) TableName() string {
	return "lessons"
}
