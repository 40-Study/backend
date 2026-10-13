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
	ListPostsByLessonID(ctx context.Context, lessonID uuid.UUID, page, pageSize int) ([]model.Discussion, int64, error)
	GetCommentsByPostID(ctx context.Context, postID uuid.UUID) ([]model.Discussion, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
	CreateVote(ctx context.Context, vote *model.DiscussionVote) error
	DeleteVote(ctx context.Context, userID, discussionID uuid.UUID) error
	GetUserVote(ctx context.Context, userID, discussionID uuid.UUID) (*model.DiscussionVote, error)
	GetUserVotes(ctx context.Context, userID uuid.UUID, discussionIDs []uuid.UUID) ([]model.DiscussionVote, error)
	IncrementUpvoteCount(ctx context.Context, id uuid.UUID, delta int) error
	// CreateReply chèn một trả lời và đếm lại reply_count của bài gốc trong CÙNG giao dịch (khoá dòng bài gốc).
	CreateReply(ctx context.Context, reply *model.Discussion) error
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

// ListPostsByLessonID (Phase 1 §5): hoi dap gan voi MOT bai hoc — nguoc voi ListForumPosts
// (dien dan chung, loc lesson_id IS NULL), ham nay loc DUNG lesson_id do.
func (r *DiscussionRepository) ListPostsByLessonID(ctx context.Context, lessonID uuid.UUID, page, pageSize int) ([]model.Discussion, int64, error) {
	var posts []model.Discussion
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Discussion{}).
		Where("lesson_id = ? AND parent_id IS NULL AND is_hidden = false", lessonID)

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

// threadRootSQL: bài gốc của dòng `?` (đi lên theo parent_id, tính cả dòng đã xoá mềm). UNION nên vòng parent_id (dữ
// liệu hỏng) vẫn dừng.
const threadRootSQL = `WITH RECURSIVE up AS (
		SELECT id, parent_id FROM discussions WHERE id = ?
		UNION
		SELECT p.id, p.parent_id FROM discussions p JOIN up ON p.id = up.parent_id
	) SELECT id FROM up WHERE parent_id IS NULL LIMIT 1`

// recountRepliesSQL: ghi lại reply_count của bài gốc `?` theo số trả lời còn sống thật.
const recountRepliesSQL = `UPDATE discussions d SET reply_count = ` + model.DiscussionLiveReplyCountSQL + `
	WHERE d.deleted_at IS NULL AND d.id = ?`

// lockThreadRoot tìm bài gốc của dòng fromID và khoá dòng đó (FOR UPDATE) cho tới hết giao dịch. Mọi thay đổi đếm
// (thêm/xoá trả lời) khoá bài gốc TRƯỚC khi đổi dòng và đếm lại, nên hai thao tác đồng thời trên cùng một bài xếp hàng và
// lần đếm sau luôn thấy kết quả đã commit của lần trước (không đếm trên ảnh chụp cũ rồi ghi đè).
func lockThreadRoot(tx *gorm.DB, fromID uuid.UUID) (uuid.UUID, error) {
	var roots []string
	if err := tx.Raw(threadRootSQL, fromID).Scan(&roots).Error; err != nil {
		return uuid.Nil, err
	}
	if len(roots) == 0 {
		return uuid.Nil, gorm.ErrRecordNotFound
	}
	rootID, err := uuid.Parse(roots[0])
	if err != nil {
		return uuid.Nil, err
	}
	if err := tx.Exec("SELECT 1 FROM discussions WHERE id = ? FOR UPDATE", rootID).Error; err != nil {
		return uuid.Nil, err
	}
	return rootID, nil
}

// DeletePost xoá mềm một bài/trả lời. Xoá một TRẢ LỜI thì đếm lại reply_count của bài gốc trong CÙNG giao dịch
// (QA2 F-05: trước đây chỉ có +1 ở AddComment, không bao giờ -1). Đếm lại theo số thật thay vì trừ 1 nên số lệch
// từ dữ liệu cũ tự được sửa, không thể thành số âm. Xoá bài gốc: không có bộ đếm của ai bị ảnh hưởng.
func (r *DiscussionRepository) DeletePost(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rootID, err := lockThreadRoot(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Delete(&model.Discussion{}, "id = ?", id).Error; err != nil {
			return err
		}
		return tx.Exec(recountRepliesSQL, rootID).Error
	})
}

// CreateReply (review L2): chèn trả lời + đếm lại bài gốc của CHUỖI cha của nó trong một giao dịch. Bài gốc lấy theo
// parent_id thật của trả lời nên một parent_id trỏ sang bài khác cộng đúng vào bài chứa nó, không lệch bài đang mở.
func (r *DiscussionRepository) CreateReply(ctx context.Context, reply *model.Discussion) error {
	if reply.ParentID == nil {
		return errors.New("reply requires parent_id")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rootID, err := lockThreadRoot(tx, *reply.ParentID)
		if err != nil {
			return err
		}
		if err := tx.Create(reply).Error; err != nil {
			return err
		}
		return tx.Exec(recountRepliesSQL, rootID).Error
	})
}
