package repository

import (
	"context"
	"errors"
	"time"

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
	TryReserveMemberSlot(ctx context.Context, groupID uuid.UUID) (bool, error)
}

type GroupMemberRepositoryInterface interface {
	Create(ctx context.Context, member *model.GroupMember) error
	GetByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error)
	Update(ctx context.Context, member *model.GroupMember) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByGroupID(ctx context.Context, groupID uuid.UUID, status model.GroupMemberStatus, page, pageSize int) ([]model.GroupMember, int64, error)
	GetActiveByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupMember, error)
}

type GroupJoinRequestRepositoryInterface interface {
	Create(ctx context.Context, req *model.GroupJoinRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.GroupJoinRequest, error)
	GetPendingByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupJoinRequest, error)
	GetByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupJoinRequest, error)
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

	// S6: danh sách công khai KHÔNG BAO GIỜ trả nhóm SECRET. Trước đây ?privacy=SECRET liệt kê được chúng
	// (chỉ trường hợp mặc định mới lọc), làm lộ slug của nhóm bí mật.
	query = query.Where("privacy != ?", model.GroupPrivacySecret)
	if privacy != "" {
		query = query.Where("privacy = ?", privacy)
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

// TryReserveMemberSlot tăng member_count thêm 1 CHỈ KHI nhóm còn chỗ (member_count < max_members), trong một
// câu UPDATE nguyên tử. Trả false khi nhóm đã đầy (hoặc không tồn tại). Kiểm "còn chỗ" rồi mới ghi ở hai
// bước riêng thì hai người vào cùng lúc vượt được max_members; câu lệnh này đóng khe đó.
func (r *GroupRepository) TryReserveMemberSlot(ctx context.Context, groupID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Group{}).
		Where("id = ? AND member_count < max_members", groupID).
		UpdateColumn("member_count", gorm.Expr("member_count + 1"))
	return res.RowsAffected > 0, res.Error
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

// HiddenProfileUserIDs trả tập user (trong ids) đặt profile_visibility = hidden.
func (r *GroupMemberRepository) HiddenProfileUserIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	var found []uuid.UUID
	if err := r.db.WithContext(ctx).Table("user_preferences").
		Where("user_id IN ? AND profile_visibility = ?", ids, "hidden").Pluck("user_id", &found).Error; err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(found))
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

// MoveFromActive chuyển dòng thành viên khỏi ACTIVE (sang LEFT hoặc BANNED) bằng MỘT câu UPDATE có điều kiện
// status = ACTIVE (chủ nhóm không bao giờ bị chuyển) và trả true CHỈ KHI chính câu này chuyển được. Người gọi
// chỉ giảm member_count khi nhận true: rời/gỡ/cấm đồng thời không còn trừ hai lần (cùng mẫu Reactivate).
func (r *GroupMemberRepository) MoveFromActive(ctx context.Context, id uuid.UUID, to model.GroupMemberStatus) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.GroupMember{}).
		Where("id = ? AND status = ? AND role <> ?", id, model.GroupMemberActive, model.GroupRoleOwner).
		Update("status", to)
	return res.RowsAffected == 1, res.Error
}

// Reactivate chuyển dòng thành viên cũ (LEFT/INVITED/PENDING) sang ACTIVE với vai trò MEMBER bằng MỘT câu
// UPDATE có điều kiện trạng thái và trả true CHỈ KHI chính câu này thực hiện việc chuyển (RowsAffected = 1).
// Hai request đồng thời cho cùng một dòng: đúng một bên thấy true, bên kia false. Người gọi chỉ giữ chỗ
// (member_count) khi nhận true, nên đếm không trôi. BANNED và ACTIVE không bao giờ bị ghi đè ở đây.
func (r *GroupMemberRepository) Reactivate(ctx context.Context, id uuid.UUID, invitedBy *uuid.UUID, now time.Time) (bool, error) {
	updates := map[string]any{
		"status":    model.GroupMemberActive,
		"role":      model.GroupRoleMember,
		"joined_at": now,
	}
	if invitedBy != nil {
		updates["invited_by"] = *invitedBy
	}
	res := r.db.WithContext(ctx).Model(&model.GroupMember{}).
		Where("id = ? AND status IN ?", id,
			[]model.GroupMemberStatus{model.GroupMemberLeft, model.GroupMemberInvited, model.GroupMemberPending}).
		Updates(updates)
	return res.RowsAffected == 1, res.Error
}

func (r *GroupMemberRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.GroupMember{}, "id = ?", id).Error
}

// ListByGroupID liệt kê thành viên theo trạng thái (ACTIVE cho danh sách thường, BANNED cho danh sách bị cấm).
func (r *GroupMemberRepository) ListByGroupID(ctx context.Context, groupID uuid.UUID, status model.GroupMemberStatus, page, pageSize int) ([]model.GroupMember, int64, error) {
	var members []model.GroupMember
	var total int64

	query := r.db.WithContext(ctx).Model(&model.GroupMember{}).
		Where("group_id = ? AND status = ?", groupID, status).
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

// GetByGroupAndUser trả yêu cầu của user với nhóm ở MỌI trạng thái (unique theo cặp group_id+user_id nên tối
// đa một dòng) — dùng để tái sử dụng dòng cũ khi xin vào lại sau khi bị từ chối hoặc đã rời nhóm.
func (r *GroupJoinRequestRepository) GetByGroupAndUser(ctx context.Context, groupID, userID uuid.UUID) (*model.GroupJoinRequest, error) {
	var req model.GroupJoinRequest
	err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).First(&req).Error
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
