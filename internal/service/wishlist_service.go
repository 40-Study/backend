package service

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// F-10 wishlist - contract: plans/261013-qa2-role-findings/contract-be3.md §4.
type WishlistServiceInterface interface {
	Add(ctx context.Context, userID, courseID uuid.UUID) (item *dto.WishlistItemResponseDTO, created bool, err error)
	List(ctx context.Context, userID uuid.UUID) (*dto.WishlistListResponseDTO, error)
	CourseIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	Remove(ctx context.Context, userID, courseID uuid.UUID) error
	Check(ctx context.Context, userID, courseID uuid.UUID) (bool, error)
}

type WishlistService struct {
	repo       repository.WishlistRepositoryInterface
	courseRepo repository.CourseRepositoryInterface
}

func NewWishlistService(repo repository.WishlistRepositoryInterface, courseRepo repository.CourseRepositoryInterface) *WishlistService {
	return &WishlistService{repo: repo, courseRepo: courseRepo}
}

// Add thêm khoá vào wishlist; thêm lần hai là no-op trả lại item cũ (created=false) để nút tim bật/tắt an toàn.
// Khoá không tồn tại hoặc chưa xuất bản coi như không có (cùng luật giỏ hàng, D4 PR #79).
func (s *WishlistService) Add(ctx context.Context, userID, courseID uuid.UUID) (*dto.WishlistItemResponseDTO, bool, error) {
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return nil, false, err
	}
	if course == nil || course.Status != model.CourseStatusPublished {
		return nil, false, apperr.NotFound("course not found")
	}
	item, created, err := s.repo.Add(ctx, userID, courseID)
	if err != nil {
		return nil, false, err
	}
	return toWishlistItemDTO(item, course), created, nil
}

func (s *WishlistService) List(ctx context.Context, userID uuid.UUID) (*dto.WishlistListResponseDTO, error) {
	rows, err := s.repo.ListPublished(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]dto.WishlistItemResponseDTO, 0, len(rows))
	for i := range rows {
		items = append(items, *toWishlistItemDTO(&rows[i], &rows[i].Course))
	}
	return &dto.WishlistListResponseDTO{Items: items, TotalItem: len(items)}, nil
}

func (s *WishlistService) CourseIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.repo.ListPublished(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.CourseID)
	}
	return ids, nil
}

// Remove idempotent: bỏ khoá không có trong wishlist vẫn là thành công.
func (s *WishlistService) Remove(ctx context.Context, userID, courseID uuid.UUID) error {
	return s.repo.Delete(ctx, userID, courseID)
}

func (s *WishlistService) Check(ctx context.Context, userID, courseID uuid.UUID) (bool, error) {
	return s.repo.Exists(ctx, userID, courseID)
}

func toWishlistItemDTO(item *model.Wishlist, course *model.Course) *dto.WishlistItemResponseDTO {
	return &dto.WishlistItemResponseDTO{
		ID:        item.ID,
		CourseID:  item.CourseID,
		UserID:    item.UserID,
		CreatedAt: utils.FormatTimestamp(item.CreatedAt),
		Course:    toCartCourseInfoDTO(course),
	}
}
