package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type DiscussionServiceInterface interface {
	CreatePost(ctx context.Context, userID uuid.UUID, req dto.CreateForumPostDTO) (*dto.ForumPostResponseDTO, error)
	GetPostBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.ForumPostDetailResponseDTO, error)
	ListPosts(ctx context.Context, category string, page, pageSize int, userID *uuid.UUID) (*dto.ForumPostListResponseDTO, error)
	AddComment(ctx context.Context, postSlug string, userID uuid.UUID, req dto.CreateForumCommentDTO) (*dto.ForumCommentResponseDTO, error)
	VoteDiscussion(ctx context.Context, discussionID, userID uuid.UUID, voteType string) error
	RemoveVote(ctx context.Context, discussionID, userID uuid.UUID) error
	DeletePost(ctx context.Context, postID, userID uuid.UUID) error
}

type DiscussionService struct {
	repo repository.DiscussionRepositoryInterface
}

func NewDiscussionService(repo repository.DiscussionRepositoryInterface) *DiscussionService {
	return &DiscussionService{repo: repo}
}

func (s *DiscussionService) CreatePost(ctx context.Context, userID uuid.UUID, req dto.CreateForumPostDTO) (*dto.ForumPostResponseDTO, error) {
	slug, err := utils.GenerateUniqueSlug(req.Title, func(candidate string) (bool, error) {
		return s.repo.SlugExists(ctx, candidate)
	})
	if err != nil {
		return nil, err
	}

	post := &model.Discussion{
		UserID:   userID,
		Title:    &req.Title,
		Content:  req.Content,
		Category: &req.Category,
		Slug:     &slug,
	}

	if err := s.repo.CreatePost(ctx, post); err != nil {
		return nil, err
	}

	// Reload with user
	created, err := s.repo.GetPostByID(ctx, post.ID)
	if err != nil || created == nil {
		return nil, errors.New("failed to load created post")
	}

	return mapPostToDTO(created, nil), nil
}

func (s *DiscussionService) GetPostBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.ForumPostDetailResponseDTO, error) {
	post, err := s.repo.GetPostBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, errors.New("post not found")
	}

	comments, err := s.repo.GetCommentsByPostID(ctx, post.ID)
	if err != nil {
		return nil, err
	}

	// Gather all IDs for vote lookup
	var userVoteMap map[uuid.UUID]string
	if userID != nil {
		allIDs := []uuid.UUID{post.ID}
		for _, c := range comments {
			allIDs = append(allIDs, c.ID)
		}
		userVoteMap, err = s.getUserVoteMap(ctx, *userID, allIDs)
		if err != nil {
			return nil, err
		}
	}

	postVote := voteFromMap(userVoteMap, post.ID)
	postDTO := mapPostToDTO(post, postVote)

	commentDTOs := buildCommentTree(comments, post.ID, userVoteMap)

	return &dto.ForumPostDetailResponseDTO{
		ForumPostResponseDTO: *postDTO,
		Comments:             commentDTOs,
	}, nil
}

func (s *DiscussionService) ListPosts(ctx context.Context, category string, page, pageSize int, userID *uuid.UUID) (*dto.ForumPostListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	posts, total, err := s.repo.ListForumPosts(ctx, category, page, pageSize)
	if err != nil {
		return nil, err
	}

	var userVoteMap map[uuid.UUID]string
	if userID != nil && len(posts) > 0 {
		ids := make([]uuid.UUID, len(posts))
		for i, p := range posts {
			ids[i] = p.ID
		}
		userVoteMap, err = s.getUserVoteMap(ctx, *userID, ids)
		if err != nil {
			return nil, err
		}
	}

	postDTOs := make([]dto.ForumPostResponseDTO, len(posts))
	for i, p := range posts {
		vote := voteFromMap(userVoteMap, p.ID)
		postDTOs[i] = *mapPostToDTO(&p, vote)
	}

	return &dto.ForumPostListResponseDTO{
		Posts:    postDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *DiscussionService) AddComment(ctx context.Context, postSlug string, userID uuid.UUID, req dto.CreateForumCommentDTO) (*dto.ForumCommentResponseDTO, error) {
	post, err := s.repo.GetPostBySlug(ctx, postSlug)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, errors.New("post not found")
	}

	comment := &model.Discussion{
		UserID:   userID,
		Content:  req.Content,
		ParentID: &post.ID,
	}

	if req.ParentID != nil {
		parentID, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return nil, errors.New("invalid parent_id")
		}
		comment.ParentID = &parentID
	}

	if err := s.repo.CreatePost(ctx, comment); err != nil {
		return nil, err
	}

	// Increment reply count on root post
	if err := s.repo.IncrementReplyCount(ctx, post.ID, 1); err != nil {
		return nil, err
	}

	created, err := s.repo.GetPostByID(ctx, comment.ID)
	if err != nil || created == nil {
		return nil, errors.New("failed to load created comment")
	}

	return mapCommentToDTO(created, nil), nil
}

func (s *DiscussionService) VoteDiscussion(ctx context.Context, discussionID, userID uuid.UUID, voteType string) error {
	existing, err := s.repo.GetUserVote(ctx, userID, discussionID)
	if err != nil {
		return err
	}

	if existing != nil {
		if existing.VoteType == voteType {
			return errors.New("already voted with same type")
		}
		// Change vote: remove old, add new
		if err := s.repo.DeleteVote(ctx, userID, discussionID); err != nil {
			return err
		}
		delta := 0
		if voteType == "upvote" {
			delta = 1
		} else {
			delta = -1
		}
		if err := s.repo.IncrementUpvoteCount(ctx, discussionID, delta); err != nil {
			return err
		}
	} else {
		delta := 0
		if voteType == "upvote" {
			delta = 1
		} else {
			delta = -1
		}
		if err := s.repo.IncrementUpvoteCount(ctx, discussionID, delta); err != nil {
			return err
		}
	}

	vote := &model.DiscussionVote{
		UserID:       userID,
		DiscussionID: discussionID,
		VoteType:     voteType,
	}
	return s.repo.CreateVote(ctx, vote)
}

func (s *DiscussionService) RemoveVote(ctx context.Context, discussionID, userID uuid.UUID) error {
	existing, err := s.repo.GetUserVote(ctx, userID, discussionID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("no vote to remove")
	}

	delta := 0
	if existing.VoteType == "upvote" {
		delta = -1
	} else {
		delta = 1
	}

	if err := s.repo.IncrementUpvoteCount(ctx, discussionID, delta); err != nil {
		return err
	}
	return s.repo.DeleteVote(ctx, userID, discussionID)
}

func (s *DiscussionService) DeletePost(ctx context.Context, postID, userID uuid.UUID) error {
	post, err := s.repo.GetPostByID(ctx, postID)
	if err != nil {
		return err
	}
	if post == nil {
		return errors.New("post not found")
	}
	if post.UserID != userID {
		return errors.New("forbidden: not the owner")
	}
	return s.repo.DeletePost(ctx, postID)
}

// --- helpers ---

func (s *DiscussionService) getUserVoteMap(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	votes, err := s.repo.GetUserVotes(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]string, len(votes))
	for _, v := range votes {
		m[v.DiscussionID] = v.VoteType
	}
	return m, nil
}

func voteFromMap(m map[uuid.UUID]string, id uuid.UUID) *string {
	if m == nil {
		return nil
	}
	if v, ok := m[id]; ok {
		return &v
	}
	return nil
}

func mapPostToDTO(post *model.Discussion, userVote *string) *dto.ForumPostResponseDTO {
	slug := ""
	if post.Slug != nil {
		slug = *post.Slug
	}
	title := ""
	if post.Title != nil {
		title = *post.Title
	}
	category := ""
	if post.Category != nil {
		category = *post.Category
	}

	authorName := post.User.UserName
	if post.User.FullName != nil {
		authorName = *post.User.FullName
	}

	return &dto.ForumPostResponseDTO{
		ID:          post.ID,
		Slug:        slug,
		Title:       title,
		Content:     post.Content,
		Category:    category,
		AuthorID:    post.UserID,
		AuthorName:  authorName,
		AvatarURL:   post.User.AvatarURL,
		CreatedAt:   post.CreatedAt,
		UpdatedAt:   post.UpdatedAt,
		UpvoteCount: post.UpvoteCount,
		ReplyCount:  post.ReplyCount,
		UserVote:    userVote,
	}
}

func mapCommentToDTO(c *model.Discussion, userVote *string) *dto.ForumCommentResponseDTO {
	authorName := c.User.UserName
	if c.User.FullName != nil {
		authorName = *c.User.FullName
	}
	return &dto.ForumCommentResponseDTO{
		ID:          c.ID,
		Content:     c.Content,
		AuthorID:    c.UserID,
		AuthorName:  authorName,
		AvatarURL:   c.User.AvatarURL,
		CreatedAt:   c.CreatedAt,
		UpvoteCount: c.UpvoteCount,
		UserVote:    userVote,
		Replies:     []dto.ForumCommentResponseDTO{},
	}
}

// buildCommentTree groups flat comments into nested tree (one level deep for now).
func buildCommentTree(comments []model.Discussion, postID uuid.UUID, voteMap map[uuid.UUID]string) []dto.ForumCommentResponseDTO {
	roots := []dto.ForumCommentResponseDTO{}
	childMap := map[uuid.UUID][]dto.ForumCommentResponseDTO{}

	for _, c := range comments {
		vote := voteFromMap(voteMap, c.ID)
		dto := mapCommentToDTO(&c, vote)
		if c.ParentID != nil && *c.ParentID != postID {
			childMap[*c.ParentID] = append(childMap[*c.ParentID], *dto)
		} else {
			roots = append(roots, *dto)
		}
	}

	// Attach children
	for i := range roots {
		if children, ok := childMap[roots[i].ID]; ok {
			roots[i].Replies = children
		}
	}

	return roots
}
