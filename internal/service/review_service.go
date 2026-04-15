package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ReviewServiceInterface interface {
	CreateReview(ctx context.Context, userID, courseID uuid.UUID, req dto.CreateReviewDTO) (*dto.ReviewResponseDTO, error)
	GetReviewsByCourse(ctx context.Context, courseID uuid.UUID, userID *uuid.UUID, page, pageSize int) (*dto.ReviewListDTO, error)
	UpdateReview(ctx context.Context, reviewID, userID uuid.UUID, req dto.UpdateReviewDTO) (*dto.ReviewResponseDTO, error)
	DeleteReview(ctx context.Context, reviewID, userID uuid.UUID) error
	ReactToReview(ctx context.Context, reviewID, userID uuid.UUID, req dto.CreateReviewReactionDTO) error
	RemoveReaction(ctx context.Context, reviewID, userID uuid.UUID) error
}

type ReviewService struct {
	repo  repository.ReviewRepositoryInterface
	redis *redis.Client
}

func NewReviewService(repo repository.ReviewRepositoryInterface, redis *redis.Client) *ReviewService {
	return &ReviewService{repo: repo, redis: redis}
}

const (
	reviewRatingCachePrefix = "course_rating:"
	reviewCacheTTL          = 15 * time.Minute
)

func (s *ReviewService) invalidateRatingCache(ctx context.Context, courseID uuid.UUID) {
	if s.redis != nil {
		s.redis.Del(ctx, reviewRatingCachePrefix+courseID.String())
	}
}

func (s *ReviewService) CreateReview(ctx context.Context, userID, courseID uuid.UUID, req dto.CreateReviewDTO) (*dto.ReviewResponseDTO, error) {
	// Check if already reviewed
	existing, _ := s.repo.GetReviewByUserAndCourse(ctx, userID, courseID)
	if existing != nil {
		return nil, errors.New("you have already reviewed this course")
	}

	review := &model.Review{
		UserID:   userID,
		CourseID: courseID,
		Rating:   req.Rating,
	}
	if req.Comment != "" {
		review.Comment = &req.Comment
	}

	if err := s.repo.CreateReview(ctx, review); err != nil {
		return nil, err
	}

	s.invalidateRatingCache(ctx, courseID)

	loaded, _ := s.repo.GetReviewByID(ctx, review.ID)
	if loaded != nil {
		return s.mapReviewToDTO(loaded, 0, nil), nil
	}
	return s.mapReviewToDTO(review, 0, nil), nil
}

func (s *ReviewService) GetReviewsByCourse(ctx context.Context, courseID uuid.UUID, userID *uuid.UUID, page, pageSize int) (*dto.ReviewListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	reviews, total, err := s.repo.GetReviewsByCourseID(ctx, courseID, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Get rating stats (cached)
	avgRating, ratingCounts, err := s.getCachedRatingStats(ctx, courseID)
	if err != nil {
		return nil, err
	}

	// Get user reactions
	var userReactions map[uuid.UUID]string
	if userID != nil && len(reviews) > 0 {
		reviewIDs := make([]uuid.UUID, len(reviews))
		for i, r := range reviews {
			reviewIDs[i] = r.ID
		}
		userReactions, _ = s.repo.GetUserReactions(ctx, *userID, reviewIDs)
	}

	data := make([]dto.ReviewResponseDTO, len(reviews))
	for i, r := range reviews {
		helpfulCount, _ := s.repo.GetHelpfulCount(ctx, r.ID)
		var reaction *string
		if userReactions != nil {
			if rt, ok := userReactions[r.ID]; ok {
				reaction = &rt
			}
		}
		data[i] = *s.mapReviewToDTO(&r, helpfulCount, reaction)
	}

	return &dto.ReviewListDTO{
		Data:          data,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
		AverageRating: avgRating,
		RatingCounts:  ratingCounts,
	}, nil
}

func (s *ReviewService) getCachedRatingStats(ctx context.Context, courseID uuid.UUID) (float64, map[int]int64, error) {
	cacheKey := reviewRatingCachePrefix + courseID.String()

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var stats struct {
				Avg    float64          `json:"avg"`
				Counts map[int]int64    `json:"counts"`
			}
			if json.Unmarshal([]byte(cached), &stats) == nil {
				return stats.Avg, stats.Counts, nil
			}
		}
	}

	avg, err := s.repo.GetAverageRating(ctx, courseID)
	if err != nil {
		return 0, nil, err
	}

	counts, err := s.repo.GetRatingCounts(ctx, courseID)
	if err != nil {
		return 0, nil, err
	}

	// Cache
	if s.redis != nil {
		stats := struct {
			Avg    float64       `json:"avg"`
			Counts map[int]int64 `json:"counts"`
		}{Avg: avg, Counts: counts}
		if data, err := json.Marshal(stats); err == nil {
			s.redis.Set(ctx, cacheKey, data, reviewCacheTTL)
		}
	}

	return avg, counts, nil
}

func (s *ReviewService) UpdateReview(ctx context.Context, reviewID, userID uuid.UUID, req dto.UpdateReviewDTO) (*dto.ReviewResponseDTO, error) {
	review, err := s.repo.GetReviewByID(ctx, reviewID)
	if err != nil || review == nil {
		return nil, errors.New("review not found")
	}
	if review.UserID != userID {
		return nil, errors.New("forbidden: not the owner")
	}

	if req.Rating != nil {
		review.Rating = *req.Rating
	}
	if req.Comment != nil {
		review.Comment = req.Comment
	}

	if err := s.repo.UpdateReview(ctx, review); err != nil {
		return nil, err
	}

	s.invalidateRatingCache(ctx, review.CourseID)

	helpfulCount, _ := s.repo.GetHelpfulCount(ctx, reviewID)
	return s.mapReviewToDTO(review, helpfulCount, nil), nil
}

func (s *ReviewService) DeleteReview(ctx context.Context, reviewID, userID uuid.UUID) error {
	review, err := s.repo.GetReviewByID(ctx, reviewID)
	if err != nil || review == nil {
		return errors.New("review not found")
	}
	if review.UserID != userID {
		return fmt.Errorf("forbidden: not the owner")
	}

	courseID := review.CourseID
	if err := s.repo.DeleteReview(ctx, reviewID); err != nil {
		return err
	}

	s.invalidateRatingCache(ctx, courseID)
	return nil
}

func (s *ReviewService) ReactToReview(ctx context.Context, reviewID, userID uuid.UUID, req dto.CreateReviewReactionDTO) error {
	existing, _ := s.repo.GetReaction(ctx, reviewID, userID)
	if existing != nil {
		// Replace reaction
		s.repo.DeleteReaction(ctx, reviewID, userID)
	}

	reaction := &model.ReviewReaction{
		ReviewID:     reviewID,
		UserID:       userID,
		ReactionType: req.ReactionType,
	}
	return s.repo.CreateReaction(ctx, reaction)
}

func (s *ReviewService) RemoveReaction(ctx context.Context, reviewID, userID uuid.UUID) error {
	return s.repo.DeleteReaction(ctx, reviewID, userID)
}

// ============================================================================
// MAPPERS
// ============================================================================

func (s *ReviewService) mapReviewToDTO(r *model.Review, helpfulCount int, userReaction *string) *dto.ReviewResponseDTO {
	name := r.User.UserName
	if r.User.FullName != nil {
		name = *r.User.FullName
	}

	return &dto.ReviewResponseDTO{
		ID:           r.ID,
		UserID:       r.UserID,
		UserName:     name,
		AvatarURL:    r.User.AvatarURL,
		CourseID:     r.CourseID,
		Rating:       r.Rating,
		Comment:      r.Comment,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		HelpfulCount: helpfulCount,
		UserReaction: userReaction,
	}
}
