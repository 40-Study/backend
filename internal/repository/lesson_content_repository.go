package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type LessonContentRepositoryInterface interface {
	// Video
	CreateVideo(ctx context.Context, video *model.LessonVideo) error
	GetVideoByLessonID(ctx context.Context, lessonID uuid.UUID) (*model.LessonVideo, error)
	UpdateVideo(ctx context.Context, video *model.LessonVideo) error
	DeleteVideo(ctx context.Context, lessonID uuid.UUID) error

	// Article
	CreateArticle(ctx context.Context, article *model.LessonArticle) error
	GetArticleByLessonID(ctx context.Context, lessonID uuid.UUID) (*model.LessonArticle, error)
	UpdateArticle(ctx context.Context, article *model.LessonArticle) error
	DeleteArticle(ctx context.Context, lessonID uuid.UUID) error

	// Attachment
	CreateAttachment(ctx context.Context, attachment *model.LessonAttachment) error
	GetAttachmentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonAttachment, error)
	DeleteAttachment(ctx context.Context, id uuid.UUID) error
	GetAttachmentByID(ctx context.Context, id uuid.UUID) (*model.LessonAttachment, error)
}

type LessonContentRepository struct {
	db *gorm.DB
}

func NewLessonContentRepository(db *gorm.DB) *LessonContentRepository {
	return &LessonContentRepository{db: db}
}

// Video

func (r *LessonContentRepository) CreateVideo(ctx context.Context, video *model.LessonVideo) error {
	return r.db.WithContext(ctx).Create(video).Error
}

func (r *LessonContentRepository) GetVideoByLessonID(ctx context.Context, lessonID uuid.UUID) (*model.LessonVideo, error) {
	var video model.LessonVideo
	err := r.db.WithContext(ctx).Where("lesson_id = ?", lessonID).First(&video).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &video, nil
}

func (r *LessonContentRepository) UpdateVideo(ctx context.Context, video *model.LessonVideo) error {
	return r.db.WithContext(ctx).Save(video).Error
}

func (r *LessonContentRepository) DeleteVideo(ctx context.Context, lessonID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("lesson_id = ?", lessonID).Delete(&model.LessonVideo{}).Error
}

// Article

func (r *LessonContentRepository) CreateArticle(ctx context.Context, article *model.LessonArticle) error {
	return r.db.WithContext(ctx).Create(article).Error
}

func (r *LessonContentRepository) GetArticleByLessonID(ctx context.Context, lessonID uuid.UUID) (*model.LessonArticle, error) {
	var article model.LessonArticle
	err := r.db.WithContext(ctx).Where("lesson_id = ?", lessonID).First(&article).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &article, nil
}

func (r *LessonContentRepository) UpdateArticle(ctx context.Context, article *model.LessonArticle) error {
	return r.db.WithContext(ctx).Save(article).Error
}

func (r *LessonContentRepository) DeleteArticle(ctx context.Context, lessonID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("lesson_id = ?", lessonID).Delete(&model.LessonArticle{}).Error
}

// Attachment

func (r *LessonContentRepository) CreateAttachment(ctx context.Context, attachment *model.LessonAttachment) error {
	return r.db.WithContext(ctx).Create(attachment).Error
}

func (r *LessonContentRepository) GetAttachmentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonAttachment, error) {
	var attachments []model.LessonAttachment
	err := r.db.WithContext(ctx).
		Where("lesson_id = ?", lessonID).
		Order("created_at DESC").
		Find(&attachments).Error
	return attachments, err
}

func (r *LessonContentRepository) DeleteAttachment(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.LessonAttachment{}, "id = ?", id).Error
}

func (r *LessonContentRepository) GetAttachmentByID(ctx context.Context, id uuid.UUID) (*model.LessonAttachment, error) {
	var attachment model.LessonAttachment
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&attachment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &attachment, nil
}
