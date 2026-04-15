package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ReviewRepositoryInterface interface {
	CreateReview(ctx context.Context, review *model.Review) error
	GetReviewByID(ctx context.Context, id uuid.UUID) (*model.Review, error)
	GetReviewByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Review, error)
	GetReviewsByCourseID(ctx context.Context, courseID uuid.UUID, page, pageSize int) ([]model.Review, int64, error)
	UpdateReview(ctx context.Context, review *model.Review) error
	DeleteReview(ctx context.Context, id uuid.UUID) error
	GetAverageRating(ctx context.Context, courseID uuid.UUID) (float64, error)
	GetRatingCounts(ctx context.Context, courseID uuid.UUID) (map[int]int64, error)
	GetHelpfulCount(ctx context.Context, reviewID uuid.UUID) (int, error)

	// ReviewReaction
	CreateReaction(ctx context.Context, reaction *model.ReviewReaction) error
	GetReaction(ctx context.Context, reviewID, userID uuid.UUID) (*model.ReviewReaction, error)
	DeleteReaction(ctx context.Context, reviewID, userID uuid.UUID) error
	GetUserReactions(ctx context.Context, userID uuid.UUID, reviewIDs []uuid.UUID) (map[uuid.UUID]string, error)
}

type ReviewRepository struct {
	db *gorm.DB
}

func NewReviewRepository(db *gorm.DB) *ReviewRepository {
	return &ReviewRepository{db: db}
}

func (r *ReviewRepository) CreateReview(ctx context.Context, review *model.Review) error {
	return r.db.WithContext(ctx).Create(review).Error
}

func (r *ReviewRepository) GetReviewByID(ctx context.Context, id uuid.UUID) (*model.Review, error) {
	var review model.Review
	err := r.db.WithContext(ctx).Preload("User").First(&review, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &review, nil
}

func (r *ReviewRepository) GetReviewByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Review, error) {
	var review model.Review
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		First(&review).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &review, nil
}

func (r *ReviewRepository) GetReviewsByCourseID(ctx context.Context, courseID uuid.UUID, page, pageSize int) ([]model.Review, int64, error) {
	var reviews []model.Review
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Review{}).Where("course_id = ?", courseID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&reviews).Error
	return reviews, total, err
}

func (r *ReviewRepository) UpdateReview(ctx context.Context, review *model.Review) error {
	return r.db.WithContext(ctx).Save(review).Error
}

func (r *ReviewRepository) DeleteReview(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Review{}, "id = ?", id).Error
}

func (r *ReviewRepository) GetAverageRating(ctx context.Context, courseID uuid.UUID) (float64, error) {
	var avg float64
	err := r.db.WithContext(ctx).
		Model(&model.Review{}).
		Where("course_id = ?", courseID).
		Select("COALESCE(AVG(rating), 0)").
		Scan(&avg).Error
	return avg, err
}

func (r *ReviewRepository) GetRatingCounts(ctx context.Context, courseID uuid.UUID) (map[int]int64, error) {
	type RatingCount struct {
		Rating int   `json:"rating"`
		Count  int64 `json:"count"`
	}
	var counts []RatingCount
	err := r.db.WithContext(ctx).
		Model(&model.Review{}).
		Where("course_id = ?", courseID).
		Select("rating, COUNT(*) as count").
		Group("rating").
		Scan(&counts).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int]int64)
	for i := 1; i <= 5; i++ {
		result[i] = 0
	}
	for _, rc := range counts {
		result[rc.Rating] = rc.Count
	}
	return result, nil
}

func (r *ReviewRepository) GetHelpfulCount(ctx context.Context, reviewID uuid.UUID) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ReviewReaction{}).
		Where("review_id = ? AND reaction_type = 'helpful'", reviewID).
		Count(&count).Error
	return int(count), err
}

// ============================================================================
// REVIEW REACTION
// ============================================================================

func (r *ReviewRepository) CreateReaction(ctx context.Context, reaction *model.ReviewReaction) error {
	return r.db.WithContext(ctx).Create(reaction).Error
}

func (r *ReviewRepository) GetReaction(ctx context.Context, reviewID, userID uuid.UUID) (*model.ReviewReaction, error) {
	var reaction model.ReviewReaction
	err := r.db.WithContext(ctx).
		Where("review_id = ? AND user_id = ?", reviewID, userID).
		First(&reaction).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &reaction, nil
}

func (r *ReviewRepository) DeleteReaction(ctx context.Context, reviewID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("review_id = ? AND user_id = ?", reviewID, userID).
		Delete(&model.ReviewReaction{}).Error
}

func (r *ReviewRepository) GetUserReactions(ctx context.Context, userID uuid.UUID, reviewIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	var reactions []model.ReviewReaction
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND review_id IN ?", userID, reviewIDs).
		Find(&reactions).Error
	if err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID]string)
	for _, r := range reactions {
		result[r.ReviewID] = r.ReactionType
	}
	return result, nil
}
