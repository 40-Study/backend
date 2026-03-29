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
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	Reorder(ctx context.Context, items []ReorderItem) error

	// LessonContent
	CreateContent(ctx context.Context, content *model.LessonContent) error
	GetContentByID(ctx context.Context, id uuid.UUID) (*model.LessonContent, error)
	GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonContent, error)
	UpdateContent(ctx context.Context, content *model.LessonContent) error
	DeleteContent(ctx context.Context, id uuid.UUID) error

	// LessonSession
	CreateSession(ctx context.Context, session *model.LessonSession) error
	GetSessionByID(ctx context.Context, id uuid.UUID) (*model.LessonSession, error)
	GetSessionsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonSession, error)
	UpdateSession(ctx context.Context, session *model.LessonSession) error
	DeleteSession(ctx context.Context, id uuid.UUID) error
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

func (r *LessonRepository) DeleteContent(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.LessonContent{}, "id = ?", id).Error
}

// LessonSession methods

func (r *LessonRepository) CreateSession(ctx context.Context, session *model.LessonSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *LessonRepository) GetSessionByID(ctx context.Context, id uuid.UUID) (*model.LessonSession, error) {
	var session model.LessonSession
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *LessonRepository) GetSessionsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonSession, error) {
	var sessions []model.LessonSession
	err := r.db.WithContext(ctx).
		Where("lesson_id = ?", lessonID).
		Order("start_time ASC").
		Find(&sessions).Error
	return sessions, err
}

func (r *LessonRepository) UpdateSession(ctx context.Context, session *model.LessonSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *LessonRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.LessonSession{}, "id = ?", id).Error
}
