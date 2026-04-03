package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type CourseRepositoryInterface interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	Create(ctx context.Context, course *model.Course) error
	GetAll(ctx context.Context, params CourseFilterDBParams) ([]model.Course, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error)
	GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Course, error)
	GetDetailBySlug(ctx context.Context, slug string) (*model.Course, error)
	Update(ctx context.Context, course *model.Course) error
	Delete(ctx context.Context, id uuid.UUID) error
	ReplaceTags(ctx context.Context, course *model.Course, tags []model.Tag) error
}

type CourseFilterDBParams struct {
	CategoryID   *uuid.UUID
	InstructorID *uuid.UUID
	Level        string
	Status       string
	Keyword      string
	IsFree       *bool
	IsFeatured   *bool
	MinPrice     *float64
	MaxPrice     *float64
	TagIDs       []uuid.UUID
	Page         int
	PageSize     int
}

type CourseRepository struct {
	db *gorm.DB
}

func NewCourseRepository(db *gorm.DB) *CourseRepository {
	return &CourseRepository{db: db}
}

func (r *CourseRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Course{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *CourseRepository) Create(ctx context.Context, course *model.Course) error {
	return r.db.WithContext(ctx).Create(course).Error
}

func (r *CourseRepository) GetAll(ctx context.Context, params CourseFilterDBParams) ([]model.Course, int64, error) {
	var courses []model.Course
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Course{})

	if params.CategoryID != nil {
		query = query.Where("category_id = ?", *params.CategoryID)
	}
	if params.InstructorID != nil {
		query = query.Where("instructor_id = ?", *params.InstructorID)
	}
	if params.Level != "" {
		query = query.Where("level = ?", params.Level)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}
	if params.IsFree != nil {
		query = query.Where("is_free = ?", *params.IsFree)
	}
	if params.IsFeatured != nil {
		query = query.Where("is_featured = ?", *params.IsFeatured)
	}
	if params.MinPrice != nil {
		query = query.Where("price >= ?", *params.MinPrice)
	}
	if params.MaxPrice != nil {
		query = query.Where("price <= ?", *params.MaxPrice)
	}
	if len(params.TagIDs) > 0 {
		query = query.Joins("JOIN course_tags ON course_tags.course_id = courses.id").
			Where("course_tags.tag_id IN ?", params.TagIDs)
	}
	query = utils.ApplyKeywordSearch(query, params.Keyword, "courses.title", "courses.short_description")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, params.Page, params.PageSize).
		Preload("Category").
		Preload("Tags").
		Order("courses.created_at DESC").
		Find(&courses).Error; err != nil {
		return nil, 0, err
	}

	return courses, total, nil
}

func (r *CourseRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	var course model.Course
	err := r.db.WithContext(ctx).
		Preload("Category").
		Preload("Tags").
		Preload("Instructor").
		Where("id = ?", id).
		First(&course).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &course, nil
}

func (r *CourseRepository) GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	var course model.Course
	err := r.db.WithContext(ctx).
		Preload("Category").
		Preload("Tags").
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Sections.Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Where("id = ?", id).
		First(&course).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &course, nil
}

// GetDetailBySlug retrieves a course with all relations by its URL slug
func (r *CourseRepository) GetDetailBySlug(ctx context.Context, slug string) (*model.Course, error) {
	var course model.Course
	err := r.db.WithContext(ctx).
		Preload("Category").
		Preload("Tags").
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Sections.Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Where("slug = ?", slug).
		First(&course).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &course, nil
}

func (r *CourseRepository) Update(ctx context.Context, course *model.Course) error {
	return r.db.WithContext(ctx).Save(course).Error
}

func (r *CourseRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Course{}, "id = ?", id).Error
}

func (r *CourseRepository) ReplaceTags(ctx context.Context, course *model.Course, tags []model.Tag) error {
	err := r.db.WithContext(ctx).Model(course).Association("Tags").Replace(tags)
	if err != nil {
		return err
	}
	return nil
}
