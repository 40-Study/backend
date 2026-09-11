package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
	SlugExists(ctx context.Context, slug string) (bool, error)
	// UpdateRatingStats (H6): ghi lại average_rating/total_reviews đã tính thật từ
	// bảng reviews — method mới, KHÔNG sửa method cũ. Dùng bởi ReviewService sau
	// khi tạo/xoá review để tránh derived-field drift (courses.average_rating từng
	// chỉ được seeder ghi, không service nào cập nhật).
	UpdateRatingStats(ctx context.Context, courseID uuid.UUID, avgRating decimal.Decimal, totalReviews int64) error
	// IncrementTotalStudents (C-06 audit 260909): cộng/trừ courses.total_students bằng
	// gorm.Expr (không đọc-sửa-ghi) khi enroll/unenroll — method mới, KHÔNG sửa method cũ.
	IncrementTotalStudents(ctx context.Context, courseID uuid.UUID, delta int) error
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
		Preload("Instructor").
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
		Preload("Instructor").
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Sections.Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Sections.Lessons.Contents", func(db *gorm.DB) *gorm.DB {
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
		Preload("Instructor").
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Sections.Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Preload("Sections.Lessons.Contents", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Where("slug = ? AND status = ?", slug, "published").
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

// SlugExists checks if a course with the given slug already exists
func (r *CourseRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Course{}).Where("slug = ?", slug).Count(&count).Error
	return count > 0, err
}

// UpdateRatingStats (H6) ghi average_rating/total_reviews thật từ bảng reviews
// vào courses, thay vì để 2 cột này đứng yên từ lúc seed.
func (r *CourseRepository) UpdateRatingStats(ctx context.Context, courseID uuid.UUID, avgRating decimal.Decimal, totalReviews int64) error {
	return r.db.WithContext(ctx).Model(&model.Course{}).
		Where("id = ?", courseID).
		Updates(map[string]interface{}{
			"average_rating": avgRating,
			"total_reviews":  totalReviews,
		}).Error
}

// IncrementTotalStudents cộng/trừ total_students bằng gorm.Expr (SQL "total_students + ?")
// thay vì đọc-sửa-ghi, tránh lost-update khi nhiều request enroll/unenroll đồng thời.
func (r *CourseRepository) IncrementTotalStudents(ctx context.Context, courseID uuid.UUID, delta int) error {
	return r.db.WithContext(ctx).Model(&model.Course{}).
		Where("id = ?", courseID).
		Update("total_students", gorm.Expr("total_students + ?", delta)).Error
}

func (r *CourseRepository) ReplaceTags(ctx context.Context, course *model.Course, tags []model.Tag) error {
	err := r.db.WithContext(ctx).Model(course).Association("Tags").Replace(tags)
	if err != nil {
		return err
	}
	return nil
}
