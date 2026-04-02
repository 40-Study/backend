package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type TagServiceInterface interface {
	CreateTag(ctx context.Context, req dto.CreateTagDTO) (*dto.TagResponseDTO, error)
	GetAllTags(ctx context.Context, page, pageSize int, keyword string) (*dto.TagListResponseDTO, error)
	GetTagByID(ctx context.Context, id uuid.UUID) (*dto.TagResponseDTO, error)
	UpdateTag(ctx context.Context, id uuid.UUID, req dto.UpdateTagDTO) (*dto.TagResponseDTO, error)
	DeleteTag(ctx context.Context, id uuid.UUID) error
}

type TagService struct {
	tagRepo repository.TagRepositoryInterface
}

func NewTagService(tagRepo repository.TagRepositoryInterface) *TagService {
	return &TagService{tagRepo: tagRepo}
}

func (s *TagService) CreateTag(ctx context.Context, req dto.CreateTagDTO) (*dto.TagResponseDTO, error) {
	tag := &model.Tag{
		ID:   uuid.New(),
		Name: req.Name,
		Slug: utils.GenerateSlug(req.Name),
	}

	if err := s.tagRepo.Create(ctx, tag); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return nil, errors.New("tag with this name already exists")
		}
		return nil, err
	}

	return s.toTagResponseDTO(tag), nil
}

func (s *TagService) GetAllTags(ctx context.Context, page, pageSize int, keyword string) (*dto.TagListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tags, total, err := s.tagRepo.GetAll(ctx, page, pageSize, keyword)
	if err != nil {
		return nil, err
	}

	tagDTOs := make([]dto.TagResponseDTO, len(tags))
	for i, t := range tags {
		tagDTOs[i] = *s.toTagResponseDTO(&t)
	}

	return &dto.TagListResponseDTO{
		Tags:     tagDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *TagService) GetTagByID(ctx context.Context, id uuid.UUID) (*dto.TagResponseDTO, error) {
	tag, err := s.tagRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, errors.New("tag not found")
	}
	return s.toTagResponseDTO(tag), nil
}

func (s *TagService) UpdateTag(ctx context.Context, id uuid.UUID, req dto.UpdateTagDTO) (*dto.TagResponseDTO, error) {
	tag, err := s.tagRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, errors.New("tag not found")
	}

	if req.Name != nil {
		tag.Name = *req.Name
		tag.Slug = utils.GenerateSlug(*req.Name)
	}

	if err := s.tagRepo.Update(ctx, tag); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return nil, errors.New("tag with this name already exists")
		}
		return nil, err
	}

	return s.toTagResponseDTO(tag), nil
}

func (s *TagService) DeleteTag(ctx context.Context, id uuid.UUID) error {
	tag, err := s.tagRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if tag == nil {
		return errors.New("tag not found")
	}
	return s.tagRepo.Delete(ctx, id)
}

func (s *TagService) toTagResponseDTO(tag *model.Tag) *dto.TagResponseDTO {
	return &dto.TagResponseDTO{
		ID:        tag.ID,
		Name:      tag.Name,
		Slug:      tag.Slug,
		CreatedAt: tag.CreatedAt,
	}
}
