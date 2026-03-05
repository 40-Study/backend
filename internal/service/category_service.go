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

type CategoryServiceInterface interface {
	CreateCategory(ctx context.Context, req dto.CreateCategoryDTO) (*dto.CategoryResponseDTO, error)
	GetAllCategories(ctx context.Context, keyword string) (*dto.CategoryListResponseDTO, error)
	GetCategoryByID(ctx context.Context, id uuid.UUID) (*dto.CategoryResponseDTO, error)
	UpdateCategory(ctx context.Context, id uuid.UUID, req dto.UpdateCategoryDTO) (*dto.CategoryResponseDTO, error)
	DeleteCategory(ctx context.Context, id uuid.UUID) error
}

type CategoryService struct {
	categoryRepo repository.CategoryRepositoryInterface
}

func NewCategoryService(categoryRepo repository.CategoryRepositoryInterface) *CategoryService {
	return &CategoryService{categoryRepo: categoryRepo}
}

func (s *CategoryService) CreateCategory(ctx context.Context, req dto.CreateCategoryDTO) (*dto.CategoryResponseDTO, error) {
	if req.ParentID != nil {
		exists, err := s.categoryRepo.Exists(ctx, *req.ParentID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("parent category not found")
		}
	}

	categorySlug := utils.GenerateSlug(req.Name)

	category := &model.Category{
		Name:        req.Name,
		Slug:        categorySlug,
		ParentID:    req.ParentID,
		Description: req.Description,
		IconURL:     req.IconURL,
	}

	if req.DisplayOrder != nil {
		category.DisplayOrder = *req.DisplayOrder
	}

	if err := s.categoryRepo.Create(ctx, category); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return nil, errors.New("category with this name already exists")
		}
		return nil, err
	}

	return s.toCategoryResponseDTO(category), nil
}

func (s *CategoryService) GetAllCategories(ctx context.Context, keyword string) (*dto.CategoryListResponseDTO, error) {
	categories, total, err := s.categoryRepo.GetAll(ctx, keyword)
	if err != nil {
		return nil, err
	}

	tree := s.buildTree(categories)

	return &dto.CategoryListResponseDTO{
		Categories: tree,
		Total:      total,
	}, nil
}

func (s *CategoryService) GetCategoryByID(ctx context.Context, id uuid.UUID) (*dto.CategoryResponseDTO, error) {
	category, err := s.categoryRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if category == nil {
		return nil, errors.New("category not found")
	}
	return s.toCategoryResponseDTO(category), nil
}

func (s *CategoryService) UpdateCategory(ctx context.Context, id uuid.UUID, req dto.UpdateCategoryDTO) (*dto.CategoryResponseDTO, error) {
	category, err := s.categoryRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if category == nil {
		return nil, errors.New("category not found")
	}

	if req.ParentID != nil {
		if *req.ParentID == id {
			return nil, errors.New("category cannot be its own parent")
		}
		exists, err := s.categoryRepo.Exists(ctx, *req.ParentID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("parent category not found")
		}
		category.ParentID = req.ParentID
	}

	if req.Name != nil {
		category.Name = *req.Name
		category.Slug = utils.GenerateSlug(*req.Name)
	}
	if req.Description != nil {
		category.Description = req.Description
	}
	if req.IconURL != nil {
		category.IconURL = req.IconURL
	}
	if req.DisplayOrder != nil {
		category.DisplayOrder = *req.DisplayOrder
	}
	if req.IsActive != nil {
		category.IsActive = *req.IsActive
	}

	if err := s.categoryRepo.Update(ctx, category); err != nil {
		return nil, err
	}

	return s.toCategoryResponseDTO(category), nil
}

func (s *CategoryService) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	category, err := s.categoryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if category == nil {
		return errors.New("category not found")
	}

	children, err := s.categoryRepo.GetChildren(ctx, id)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return errors.New("cannot delete category with children")
	}

	return s.categoryRepo.Delete(ctx, id)
}

func (s *CategoryService) buildTree(categories []model.Category) []dto.CategoryResponseDTO {
	categoryMap := make(map[uuid.UUID]*dto.CategoryResponseDTO)
	for i := range categories {
		d := s.toCategoryResponseDTO(&categories[i])
		categoryMap[categories[i].ID] = d
	}

	var rootPtrs []*dto.CategoryResponseDTO
	for i := range categories {
		cat := &categories[i]
		d := categoryMap[cat.ID]
		if cat.ParentID == nil {
			rootPtrs = append(rootPtrs, d)
		} else if parent, ok := categoryMap[*cat.ParentID]; ok {
			parent.Children = append(parent.Children, *d)
		} else {
			rootPtrs = append(rootPtrs, d)
		}
	}

	roots := make([]dto.CategoryResponseDTO, len(rootPtrs))
	for i, r := range rootPtrs {
		roots[i] = *r
	}
	return roots
}

func (s *CategoryService) toCategoryResponseDTO(category *model.Category) *dto.CategoryResponseDTO {
	return &dto.CategoryResponseDTO{
		ID:           category.ID,
		ParentID:     category.ParentID,
		Name:         category.Name,
		Slug:         category.Slug,
		Description:  category.Description,
		IconURL:      category.IconURL,
		DisplayOrder: category.DisplayOrder,
		IsActive:     category.IsActive,
		CreatedAt:    category.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    category.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
