package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type LessonContentServiceInterface interface {
	// Video
	CreateVideo(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonVideoDTO) (*dto.LessonVideoResponseDTO, error)
	GetVideo(ctx context.Context, lessonID uuid.UUID) (*dto.LessonVideoResponseDTO, error)
	UpdateVideo(ctx context.Context, lessonID uuid.UUID, req dto.UpdateLessonVideoDTO) (*dto.LessonVideoResponseDTO, error)
	DeleteVideo(ctx context.Context, lessonID uuid.UUID) error

	// Article
	CreateArticle(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonArticleDTO) (*dto.LessonArticleResponseDTO, error)
	GetArticle(ctx context.Context, lessonID uuid.UUID) (*dto.LessonArticleResponseDTO, error)
	UpdateArticle(ctx context.Context, lessonID uuid.UUID, req dto.UpdateLessonArticleDTO) (*dto.LessonArticleResponseDTO, error)
	DeleteArticle(ctx context.Context, lessonID uuid.UUID) error

	// Attachment
	CreateAttachment(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonAttachmentDTO) (*dto.LessonAttachmentResponseDTO, error)
	GetAttachments(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonAttachmentResponseDTO, error)
	DeleteAttachment(ctx context.Context, lessonID, attachmentID uuid.UUID) error
}

type LessonContentService struct {
	contentRepo repository.LessonContentRepositoryInterface
	lessonRepo  repository.LessonRepositoryInterface
}

func NewLessonContentService(
	contentRepo repository.LessonContentRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
) *LessonContentService {
	return &LessonContentService{
		contentRepo: contentRepo,
		lessonRepo:  lessonRepo,
	}
}

func (s *LessonContentService) validateLesson(ctx context.Context, lessonID uuid.UUID) error {
	exists, err := s.lessonRepo.Exists(ctx, lessonID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("lesson not found")
	}
	return nil
}

// Video

func (s *LessonContentService) CreateVideo(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonVideoDTO) (*dto.LessonVideoResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	existing, err := s.contentRepo.GetVideoByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("video already exists for this lesson")
	}

	video := &model.LessonVideo{
		ID:              uuid.New(),
		LessonID:        lessonID,
		VideoURL:        req.VideoURL,
		VideoHlsURL:     req.VideoHlsURL,
		ThumbnailURL:    req.ThumbnailURL,
		DurationSeconds: req.DurationSeconds,
		Resolution:      req.Resolution,
		FileSizeBytes:   req.FileSizeBytes,
	}

	if err := s.contentRepo.CreateVideo(ctx, video); err != nil {
		return nil, err
	}

	return s.toVideoResponseDTO(video), nil
}

func (s *LessonContentService) GetVideo(ctx context.Context, lessonID uuid.UUID) (*dto.LessonVideoResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	video, err := s.contentRepo.GetVideoByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if video == nil {
		return nil, errors.New("video not found")
	}

	return s.toVideoResponseDTO(video), nil
}

func (s *LessonContentService) UpdateVideo(ctx context.Context, lessonID uuid.UUID, req dto.UpdateLessonVideoDTO) (*dto.LessonVideoResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	video, err := s.contentRepo.GetVideoByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if video == nil {
		return nil, errors.New("video not found")
	}

	if req.VideoURL != nil {
		video.VideoURL = *req.VideoURL
	}
	if req.VideoHlsURL != nil {
		video.VideoHlsURL = req.VideoHlsURL
	}
	if req.ThumbnailURL != nil {
		video.ThumbnailURL = req.ThumbnailURL
	}
	if req.DurationSeconds != nil {
		video.DurationSeconds = *req.DurationSeconds
	}
	if req.Resolution != nil {
		video.Resolution = req.Resolution
	}
	if req.FileSizeBytes != nil {
		video.FileSizeBytes = req.FileSizeBytes
	}

	if err := s.contentRepo.UpdateVideo(ctx, video); err != nil {
		return nil, err
	}

	return s.toVideoResponseDTO(video), nil
}

func (s *LessonContentService) DeleteVideo(ctx context.Context, lessonID uuid.UUID) error {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return err
	}
	return s.contentRepo.DeleteVideo(ctx, lessonID)
}

// Article

func (s *LessonContentService) CreateArticle(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonArticleDTO) (*dto.LessonArticleResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	existing, err := s.contentRepo.GetArticleByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("article already exists for this lesson")
	}

	readingTime := 5
	if req.ReadingTimeMins != nil {
		readingTime = *req.ReadingTimeMins
	}

	article := &model.LessonArticle{
		ID:              uuid.New(),
		LessonID:        lessonID,
		Content:         req.Content,
		ReadingTimeMins: readingTime,
	}

	if err := s.contentRepo.CreateArticle(ctx, article); err != nil {
		return nil, err
	}

	return s.toArticleResponseDTO(article), nil
}

func (s *LessonContentService) GetArticle(ctx context.Context, lessonID uuid.UUID) (*dto.LessonArticleResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	article, err := s.contentRepo.GetArticleByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, errors.New("article not found")
	}

	return s.toArticleResponseDTO(article), nil
}

func (s *LessonContentService) UpdateArticle(ctx context.Context, lessonID uuid.UUID, req dto.UpdateLessonArticleDTO) (*dto.LessonArticleResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	article, err := s.contentRepo.GetArticleByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, errors.New("article not found")
	}

	if req.Content != nil {
		article.Content = *req.Content
	}
	if req.ReadingTimeMins != nil {
		article.ReadingTimeMins = *req.ReadingTimeMins
	}

	if err := s.contentRepo.UpdateArticle(ctx, article); err != nil {
		return nil, err
	}

	return s.toArticleResponseDTO(article), nil
}

func (s *LessonContentService) DeleteArticle(ctx context.Context, lessonID uuid.UUID) error {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return err
	}
	return s.contentRepo.DeleteArticle(ctx, lessonID)
}

// Attachment

func (s *LessonContentService) CreateAttachment(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonAttachmentDTO) (*dto.LessonAttachmentResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	attachment := &model.LessonAttachment{
		ID:            uuid.New(),
		LessonID:      lessonID,
		FileName:      req.FileName,
		FileURL:       req.FileURL,
		FileType:      req.FileType,
		FileSizeBytes: req.FileSizeBytes,
	}

	if err := s.contentRepo.CreateAttachment(ctx, attachment); err != nil {
		return nil, err
	}

	return s.toAttachmentResponseDTO(attachment), nil
}

func (s *LessonContentService) GetAttachments(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonAttachmentResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	attachments, err := s.contentRepo.GetAttachmentsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonAttachmentResponseDTO, len(attachments))
	for i, a := range attachments {
		result[i] = *s.toAttachmentResponseDTO(&a)
	}

	return result, nil
}

func (s *LessonContentService) DeleteAttachment(ctx context.Context, lessonID, attachmentID uuid.UUID) error {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return err
	}

	attachment, err := s.contentRepo.GetAttachmentByID(ctx, attachmentID)
	if err != nil {
		return err
	}
	if attachment == nil {
		return errors.New("attachment not found")
	}
	if attachment.LessonID != lessonID {
		return errors.New("attachment does not belong to this lesson")
	}

	return s.contentRepo.DeleteAttachment(ctx, attachmentID)
}

func (s *LessonContentService) toVideoResponseDTO(video *model.LessonVideo) *dto.LessonVideoResponseDTO {
	return &dto.LessonVideoResponseDTO{
		ID:                  video.ID,
		LessonID:            video.LessonID,
		VideoURL:            video.VideoURL,
		VideoHlsURL:         video.VideoHlsURL,
		ThumbnailURL:        video.ThumbnailURL,
		DurationSeconds:     video.DurationSeconds,
		Resolution:          video.Resolution,
		FileSizeBytes:       video.FileSizeBytes,
		TranscriptionStatus: video.TranscriptionStatus,
		CreatedAt:           video.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           video.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *LessonContentService) toArticleResponseDTO(article *model.LessonArticle) *dto.LessonArticleResponseDTO {
	return &dto.LessonArticleResponseDTO{
		ID:              article.ID,
		LessonID:        article.LessonID,
		Content:         article.Content,
		ReadingTimeMins: article.ReadingTimeMins,
		CreatedAt:       article.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       article.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *LessonContentService) toAttachmentResponseDTO(a *model.LessonAttachment) *dto.LessonAttachmentResponseDTO {
	return &dto.LessonAttachmentResponseDTO{
		ID:            a.ID,
		LessonID:      a.LessonID,
		FileName:      a.FileName,
		FileURL:       a.FileURL,
		FileType:      a.FileType,
		FileSizeBytes: a.FileSizeBytes,
		DownloadCount: a.DownloadCount,
		CreatedAt:     a.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
