package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type GroupRepositoryInterface interface {
	Create(ctx context.Context, group *model.Group) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Group, error)
	GetBySlug(ctx context.Context, slug string) (*model.Group, error)
	Update(ctx context.Context, group *model.Group) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, keyword string, privacy string, page, pageSize int) ([]model.Group, int64, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Group, int64, error)
	GetOwnedByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Group, int64, error)
	IncrementMemberCount(ctx context.Context, groupID uuid.UUID) error
	DecrementMemberCount(ctx context.Context, groupID uuid.UUID) error
}

type GroupMemberRepositoryInterface interface {
	Create(ctx context.Context, member *model.GroupMember) error
	GetByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error)
	Update(ctx context.Context, member *model.GroupMember) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByGroupID(ctx context.Context, groupID uuid.UUID, page, pageSize int) ([]model.GroupMember, int64, error)
	GetActiveByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error)
}

type GroupJoinRequestRepositoryInterface interface {
	Create(ctx context.Context, req *model.GroupJoinRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.GroupJoinRequest, error)
	GetPendingByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupJoinRequest, error)
	Update(ctx context.Context, req *model.GroupJoinRequest) error
	ListByGroupID(ctx context.Context, groupID uuid.UUID, status string, page, pageSize int) ([]model.GroupJoinRequest, int64, error)
}

// ============================================================================
// GROUP REPOSITORY
// ============================================================================

type GroupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

func (r *GroupRepository) Create(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Create(group).Error
}

func (r *GroupRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).
		Preload("Conversation").
		First(&group, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (r *GroupRepository) GetBySlug(ctx context.Context, slug string) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).
		Preload("Conversation").
		First(&group, "slug = ?", slug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (r *GroupRepository) Update(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Save(group).Error
}

func (r *GroupRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Group{}, "id = ?", id).Error
}

func (r *GroupRepository) List(ctx context.Context, keyword string, privacy string, page, pageSize int) ([]model.Group, int64, error) {
	var groups []model.Group
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Group{})

	if privacy != "" {
		query = query.Where("privacy = ?", privacy)
	} else {
		// By default, don't show SECRET groups in listings
		query = query.Where("privacy != ?", model.GroupPrivacySecret)
	}

	if keyword != "" {
		query = utils.ApplyKeywordSearch(query, keyword, "name", "description")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("created_at DESC").Find(&groups).Error
	return groups, total, err
}

func (r *GroupRepository) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Group, int64, error) {
	var groups []model.Group
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Group{}).
		Joins("JOIN group_members ON group_members.group_id = groups.id").
		Where("group_members.user_id = ? AND group_members.status = ?", userID, model.GroupMemberActive).
		Where("groups.deleted_at IS NULL")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("groups.created_at DESC").Find(&groups).Error
	return groups, total, err
}

func (r *GroupRepository) GetOwnedByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Group, int64, error) {
	var groups []model.Group
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Group{}).Where("created_by = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("created_at DESC").Find(&groups).Error
	return groups, total, err
}

func (r *GroupRepository) IncrementMemberCount(ctx context.Context, groupID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Group{}).
		Where("id = ?", groupID).
		UpdateColumn("member_count", gorm.Expr("member_count + 1")).Error
}

func (r *GroupRepository) DecrementMemberCount(ctx context.Context, groupID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Group{}).
		Where("id = ? AND member_count > 0", groupID).
		UpdateColumn("member_count", gorm.Expr("member_count - 1")).Error
}

// ============================================================================
// GROUP MEMBER REPOSITORY
// ============================================================================

type GroupMemberRepository struct {
	db *gorm.DB
}

func NewGroupMemberRepository(db *gorm.DB) *GroupMemberRepository {
	return &GroupMemberRepository{db: db}
}

func (r *GroupMemberRepository) Create(ctx context.Context, member *model.GroupMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

func (r *GroupMemberRepository) GetByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error) {
	var member model.GroupMember
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

func (r *GroupMemberRepository) GetActiveByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error) {
	var member model.GroupMember
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.GroupMemberActive).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

func (r *GroupMemberRepository) Update(ctx context.Context, member *model.GroupMember) error {
	return r.db.WithContext(ctx).Save(member).Error
}

func (r *GroupMemberRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.GroupMember{}, "id = ?", id).Error
}

func (r *GroupMemberRepository) ListByGroupID(ctx context.Context, groupID uuid.UUID, page, pageSize int) ([]model.GroupMember, int64, error) {
	var members []model.GroupMember
	var total int64

	query := r.db.WithContext(ctx).Model(&model.GroupMember{}).
		Where("group_id = ? AND status = ?", groupID, model.GroupMemberActive).
		Preload("User")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("role ASC, joined_at ASC").Find(&members).Error
	return members, total, err
}

// ============================================================================
// GROUP JOIN REQUEST REPOSITORY
// ============================================================================

type GroupJoinRequestRepository struct {
	db *gorm.DB
}

func NewGroupJoinRequestRepository(db *gorm.DB) *GroupJoinRequestRepository {
	return &GroupJoinRequestRepository{db: db}
}

func (r *GroupJoinRequestRepository) Create(ctx context.Context, req *model.GroupJoinRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

func (r *GroupJoinRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.GroupJoinRequest, error) {
	var req model.GroupJoinRequest
	err := r.db.WithContext(ctx).Preload("User").First(&req, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func (r *GroupJoinRequestRepository) GetPendingByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupJoinRequest, error) {
	var req model.GroupJoinRequest
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.JoinRequestPending).
		First(&req).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func (r *GroupJoinRequestRepository) Update(ctx context.Context, req *model.GroupJoinRequest) error {
	return r.db.WithContext(ctx).Save(req).Error
}

func (r *GroupJoinRequestRepository) ListByGroupID(ctx context.Context, groupID uuid.UUID, status string, page, pageSize int) ([]model.GroupJoinRequest, int64, error) {
	var requests []model.GroupJoinRequest
	var total int64

	query := r.db.WithContext(ctx).Model(&model.GroupJoinRequest{}).
		Where("group_id = ?", groupID).
		Preload("User")

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("created_at DESC").Find(&requests).Error
	return requests, total, err
}
