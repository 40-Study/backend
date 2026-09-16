package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type LessonRepositoryInterface interface {
	Create(ctx context.Context, lesson *model.Lesson) error
	GetAllBySectionID(ctx context.Context, sectionID uuid.UUID) ([]model.Lesson, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error)
	Update(ctx context.Context, lesson *model.Lesson) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetMaxDisplayOrder(ctx context.Context, sectionID uuid.UUID) (int, error)
	BelongsToSection(ctx context.Context, lessonID, sectionID uuid.UUID) (bool, error)
	CountByIDsAndSection(ctx context.Context, ids []uuid.UUID, sectionID uuid.UUID) (int64, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	Reorder(ctx context.Context, items []ReorderItem) error

	// LessonContent
	CreateContent(ctx context.Context, content *model.LessonContent) error
	GetContentByID(ctx context.Context, id uuid.UUID) (*model.LessonContent, error)
	GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonContent, error)
	UpdateContent(ctx context.Context, content *model.LessonContent) error
	// UpdateContentDuration (C-5, review vòng 3): cập nhật DUY NHẤT cột duration. UpdateContent
	// ở trên là db.Save() — ghi ĐÈ MỌI cột từ struct đang giữ, nên dùng nó để chữa một cột sẽ
	// nuốt im lặng mọi thay đổi mà request song song vừa ghi (đúng anti-pattern mà
	// UpdateLessonProgressFields được tạo ra để tránh cho bảng tiến độ).
	UpdateContentDuration(ctx context.Context, id uuid.UUID, duration int) error
	DeleteContent(ctx context.Context, id uuid.UUID) error
	ReorderContents(ctx context.Context, items []ReorderItem) error
	CountContentsByIDsAndLesson(ctx context.Context, ids []uuid.UUID, lessonID uuid.UUID) (int64, error)

	// GetLegacyVideoDurationByLessonID doc duration (giay) tu bang legacy lesson_videos
	// (model.LessonVideo) — la nguon du phong THU HAI trong chuoi uu tien server-truth cua B-1
	// (lesson_contents truoc, lesson_videos sau, chi khi ca hai deu 0 moi roi ve client). Tra
	// (0, nil) khi khong co ban ghi — KHONG phai loi, chi la "khong co du lieu o day".
	GetLegacyVideoDurationByLessonID(ctx context.Context, lessonID uuid.UUID) (int, error)
}

type LessonRepository struct {
	db *gorm.DB
}

func NewLessonRepository(db *gorm.DB) *LessonRepository {
	return &LessonRepository{db: db}
}

func (r *LessonRepository) Create(ctx context.Context, lesson *model.Lesson) error {
	return r.db.WithContext(ctx).Create(lesson).Error
}

func (r *LessonRepository) GetAllBySectionID(ctx context.Context, sectionID uuid.UUID) ([]model.Lesson, error) {
	var lessons []model.Lesson
	err := r.db.WithContext(ctx).
		Preload("Contents", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Where("section_id = ?", sectionID).
		Order("display_order ASC").
		Find(&lessons).Error
	return lessons, err
}

func (r *LessonRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error) {
	var lesson model.Lesson
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&lesson).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &lesson, nil
}

func (r *LessonRepository) Update(ctx context.Context, lesson *model.Lesson) error {
	return r.db.WithContext(ctx).Save(lesson).Error
}

func (r *LessonRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.Lesson{}, "id = ?", id).Error
}

func (r *LessonRepository) GetMaxDisplayOrder(ctx context.Context, sectionID uuid.UUID) (int, error) {
	var maxOrder *int
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Where("section_id = ?", sectionID).
		Select("COALESCE(MAX(display_order), -1)").
		Scan(&maxOrder).Error
	if err != nil {
		return -1, err
	}
	if maxOrder == nil {
		return -1, nil
	}
	return *maxOrder, nil
}

func (r *LessonRepository) BelongsToSection(ctx context.Context, lessonID, sectionID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Where("id = ? AND section_id = ?", lessonID, sectionID).
		Count(&count).Error
	return count > 0, err
}

func (r *LessonRepository) CountByIDsAndSection(ctx context.Context, ids []uuid.UUID, sectionID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Where("id IN ? AND section_id = ?", ids, sectionID).
		Count(&count).Error
	return count, err
}

func (r *LessonRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Lesson{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *LessonRepository) Reorder(ctx context.Context, items []ReorderItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if err := tx.Model(&model.Lesson{}).
				Where("id = ?", item.ID).
				Update("display_order", item.DisplayOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// LessonContent methods

func (r *LessonRepository) CreateContent(ctx context.Context, content *model.LessonContent) error {
	return r.db.WithContext(ctx).Create(content).Error
}

func (r *LessonRepository) GetContentByID(ctx context.Context, id uuid.UUID) (*model.LessonContent, error) {
	var content model.LessonContent
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&content).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &content, nil
}

func (r *LessonRepository) GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonContent, error) {
	var contents []model.LessonContent
	err := r.db.WithContext(ctx).
		Where("lesson_id = ?", lessonID).
		Order("display_order ASC").
		Find(&contents).Error
	return contents, err
}

func (r *LessonRepository) UpdateContent(ctx context.Context, content *model.LessonContent) error {
	return r.db.WithContext(ctx).Save(content).Error
}

// UpdateContentDuration (C-5, review vòng 3): update MỘT cột, cùng dạng với ReorderContents.
// Không dùng UpdateContent (db.Save) vì nó ghi đè mọi cột từ bản chụp trong bộ nhớ.
func (r *LessonRepository) UpdateContentDuration(ctx context.Context, id uuid.UUID, duration int) error {
	return r.db.WithContext(ctx).Model(&model.LessonContent{}).
		Where("id = ?", id).
		Update("duration", duration).Error
}

func (r *LessonRepository) DeleteContent(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.LessonContent{}, "id = ?", id).Error
}

func (r *LessonRepository) ReorderContents(ctx context.Context, items []ReorderItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if err := tx.Model(&model.LessonContent{}).
				Where("id = ?", item.ID).
				Update("display_order", item.DisplayOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *LessonRepository) CountContentsByIDsAndLesson(ctx context.Context, ids []uuid.UUID, lessonID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.LessonContent{}).
		Where("id IN ? AND lesson_id = ?", ids, lessonID).
		Count(&count).Error
	return count, err
}

func (r *LessonRepository) GetLegacyVideoDurationByLessonID(ctx context.Context, lessonID uuid.UUID) (int, error) {
	var video model.LessonVideo
	err := r.db.WithContext(ctx).
		Where("lesson_id = ?", lessonID).
		First(&video).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return video.DurationSeconds, nil
}
