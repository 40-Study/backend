package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type DiscussionRepositoryInterface interface {
	CreatePost(ctx context.Context, post *model.Discussion) error
	GetPostBySlug(ctx context.Context, slug string) (*model.Discussion, error)
	GetPostByID(ctx context.Context, id uuid.UUID) (*model.Discussion, error)
	ListForumPosts(ctx context.Context, category string, page, pageSize int) ([]model.Discussion, int64, error)
	GetCommentsByPostID(ctx context.Context, postID uuid.UUID) ([]model.Discussion, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
	CreateVote(ctx context.Context, vote *model.DiscussionVote) error
	DeleteVote(ctx context.Context, userID, discussionID uuid.UUID) error
	GetUserVote(ctx context.Context, userID, discussionID uuid.UUID) (*model.DiscussionVote, error)
	GetUserVotes(ctx context.Context, userID uuid.UUID, discussionIDs []uuid.UUID) ([]model.DiscussionVote, error)
	IncrementUpvoteCount(ctx context.Context, id uuid.UUID, delta int) error
	IncrementReplyCount(ctx context.Context, id uuid.UUID, delta int) error
	DeletePost(ctx context.Context, id uuid.UUID) error
}

type DiscussionRepository struct {
	db *gorm.DB
}

func NewDiscussionRepository(db *gorm.DB) *DiscussionRepository {
	return &DiscussionRepository{db: db}
}

func (r *DiscussionRepository) CreatePost(ctx context.Context, post *model.Discussion) error {
	return r.db.WithContext(ctx).Create(post).Error
}

func (r *DiscussionRepository) GetPostBySlug(ctx context.Context, slug string) (*model.Discussion, error) {
	var post model.Discussion
	err := r.db.WithContext(ctx).
		Preload("User").
		First(&post, "slug = ? AND lesson_id IS NULL AND parent_id IS NULL", slug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &post, nil
}

func (r *DiscussionRepository) GetPostByID(ctx context.Context, id uuid.UUID) (*model.Discussion, error) {
	var post model.Discussion
	err := r.db.WithContext(ctx).
		Preload("User").
		First(&post, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &post, nil
}

func (r *DiscussionRepository) ListForumPosts(ctx context.Context, category string, page, pageSize int) ([]model.Discussion, int64, error) {
	var posts []model.Discussion
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Discussion{}).
		Where("lesson_id IS NULL AND parent_id IS NULL AND is_hidden = false")

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&posts).Error

	return posts, total, err
}

func (r *DiscussionRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Discussion{}).
		Where("slug = ?", slug).Count(&count).Error
	return count > 0, err
}

func (r *DiscussionRepository) GetCommentsByPostID(ctx context.Context, postID uuid.UUID) ([]model.Discussion, error) {
	var comments []model.Discussion
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("parent_id = ? AND lesson_id IS NULL", postID).
		Order("created_at ASC").
		Find(&comments).Error
	return comments, err
}

func (r *DiscussionRepository) CreateVote(ctx context.Context, vote *model.DiscussionVote) error {
	return r.db.WithContext(ctx).Create(vote).Error
}

func (r *DiscussionRepository) DeleteVote(ctx context.Context, userID, discussionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND discussion_id = ?", userID, discussionID).
		Delete(&model.DiscussionVote{}).Error
}

func (r *DiscussionRepository) GetUserVote(ctx context.Context, userID, discussionID uuid.UUID) (*model.DiscussionVote, error) {
	var vote model.DiscussionVote
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND discussion_id = ?", userID, discussionID).
		First(&vote).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &vote, nil
}

func (r *DiscussionRepository) GetUserVotes(ctx context.Context, userID uuid.UUID, discussionIDs []uuid.UUID) ([]model.DiscussionVote, error) {
	var votes []model.DiscussionVote
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND discussion_id IN ?", userID, discussionIDs).
		Find(&votes).Error
	return votes, err
}

func (r *DiscussionRepository) IncrementUpvoteCount(ctx context.Context, id uuid.UUID, delta int) error {
	return r.db.WithContext(ctx).
		Model(&model.Discussion{}).
		Where("id = ?", id).
		Update("upvote_count", gorm.Expr("upvote_count + ?", delta)).Error
}

func (r *DiscussionRepository) IncrementReplyCount(ctx context.Context, id uuid.UUID, delta int) error {
	return r.db.WithContext(ctx).
		Model(&model.Discussion{}).
		Where("id = ?", id).
		Update("reply_count", gorm.Expr("reply_count + ?", delta)).Error
}

func (r *DiscussionRepository) DeletePost(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Discussion{}, "id = ?", id).Error
}
