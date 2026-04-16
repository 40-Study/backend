package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type GroupServiceInterface interface {
	CreateGroup(ctx context.Context, userID uuid.UUID, req dto.CreateGroupRequest) (*dto.GroupResponse, error)
	UpdateGroup(ctx context.Context, userID, groupID uuid.UUID, req dto.UpdateGroupRequest) (*dto.GroupResponse, error)
	DeleteGroup(ctx context.Context, userID, groupID uuid.UUID) error
	GetGroupBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.GroupResponse, error)
	ListGroups(ctx context.Context, keyword, privacy string, page, pageSize int) (*dto.GroupListResponse, error)
	GetMyGroups(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.GroupListResponse, error)
	GetMyOwnedGroups(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.GroupListResponse, error)

	JoinGroup(ctx context.Context, userID, groupID uuid.UUID, message *string) (interface{}, error)
	LeaveGroup(ctx context.Context, userID, groupID uuid.UUID) error

	ListMembers(ctx context.Context, groupID uuid.UUID, page, pageSize int) (*dto.GroupMemberListResponse, error)
	InviteMembers(ctx context.Context, inviterID, groupID uuid.UUID, userIDs []uuid.UUID) error
	UpdateMemberRole(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error
	BanMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error
	UnbanMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error

	ListJoinRequests(ctx context.Context, requesterID, groupID uuid.UUID, page, pageSize int) (*dto.JoinRequestListResponse, error)
	ApproveRequest(ctx context.Context, requesterID, groupID, requestID uuid.UUID) error
	RejectRequest(ctx context.Context, requesterID, groupID, requestID uuid.UUID, reason *string) error
}

type GroupService struct {
	groupRepo       *repository.GroupRepository
	memberRepo      *repository.GroupMemberRepository
	joinRequestRepo *repository.GroupJoinRequestRepository
	convRepo        *repository.ConversationRepository
	participantRepo *repository.ConversationParticipantRepository
}

func NewGroupService(
	groupRepo *repository.GroupRepository,
	memberRepo *repository.GroupMemberRepository,
	joinRequestRepo *repository.GroupJoinRequestRepository,
	convRepo *repository.ConversationRepository,
	participantRepo *repository.ConversationParticipantRepository,
) *GroupService {
	return &GroupService{
		groupRepo:       groupRepo,
		memberRepo:      memberRepo,
		joinRequestRepo: joinRequestRepo,
		convRepo:        convRepo,
		participantRepo: participantRepo,
	}
}

func (s *GroupService) CreateGroup(ctx context.Context, userID uuid.UUID, req dto.CreateGroupRequest) (*dto.GroupResponse, error) {
	slug, err := utils.GenerateUniqueSlug(req.Name, func(slug string) (bool, error) {
		g, err := s.groupRepo.GetBySlug(ctx, slug)
		if err != nil {
			return false, err
		}
		return g != nil, nil
	})
	if err != nil {
		return nil, err
	}

	groupType := model.GroupTypeCustom
	if req.Type != "" {
		groupType = model.GroupType(req.Type)
	}
	privacy := model.GroupPrivacyPrivate
	if req.Privacy != "" {
		privacy = model.GroupPrivacy(req.Privacy)
	}
	maxMembers := 100
	if req.MaxMembers > 0 {
		maxMembers = req.MaxMembers
	}

	var orgID *uuid.UUID
	if req.OrganizationID != nil {
		parsed, err := uuid.Parse(*req.OrganizationID)
		if err == nil {
			orgID = &parsed
		}
	}

	group := &model.Group{
		Name:           req.Name,
		Slug:           slug,
		Description:    req.Description,
		Type:           groupType,
		Privacy:        privacy,
		MaxMembers:     maxMembers,
		MemberCount:    1,
		CreatedBy:      userID,
		OrganizationID: orgID,
	}

	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, err
	}

	// Add creator as OWNER
	now := time.Now()
	member := &model.GroupMember{
		GroupID:  group.ID,
		UserID:   userID,
		Role:     model.GroupRoleOwner,
		Status:   model.GroupMemberActive,
		JoinedAt: &now,
	}
	if err := s.memberRepo.Create(ctx, member); err != nil {
		return nil, err
	}

	// Create group conversation
	conv := &model.Conversation{
		Type:    model.ConversationTypeGroup,
		GroupID: &group.ID,
		Name:    &group.Name,
	}
	if err := s.convRepo.Create(ctx, conv); err != nil {
		return nil, err
	}

	// Add creator as participant
	participant := &model.ConversationParticipant{
		ConversationID: conv.ID,
		UserID:         userID,
		JoinedAt:       now,
	}
	if err := s.participantRepo.Create(ctx, participant); err != nil {
		return nil, err
	}

	ownerRole := string(model.GroupRoleOwner)
	return &dto.GroupResponse{
		ID:             group.ID,
		Name:           group.Name,
		Slug:           group.Slug,
		Description:    group.Description,
		Type:           string(group.Type),
		Privacy:        string(group.Privacy),
		MaxMembers:     group.MaxMembers,
		MemberCount:    group.MemberCount,
		CreatedBy:      group.CreatedBy,
		OrganizationID: group.OrganizationID,
		MyRole:         &ownerRole,
		Conversation:   &dto.ConversationBrief{ID: conv.ID},
		CreatedAt:      group.CreatedAt,
		UpdatedAt:      group.UpdatedAt,
	}, nil
}

func (s *GroupService) UpdateGroup(ctx context.Context, userID, groupID uuid.UUID, req dto.UpdateGroupRequest) (*dto.GroupResponse, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("group not found")
	}

	if err := s.requireRole(ctx, groupID, userID, model.GroupRoleOwner, model.GroupRoleAdmin); err != nil {
		return nil, err
	}

	if req.Name != nil {
		group.Name = *req.Name
	}
	if req.Description != nil {
		group.Description = req.Description
	}
	if req.Privacy != nil {
		group.Privacy = model.GroupPrivacy(*req.Privacy)
	}
	if req.MaxMembers != nil {
		group.MaxMembers = *req.MaxMembers
	}

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, err
	}

	return s.toGroupResponse(group, &userID), nil
}

func (s *GroupService) DeleteGroup(ctx context.Context, userID, groupID uuid.UUID) error {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("group not found")
	}

	if err := s.requireRole(ctx, groupID, userID, model.GroupRoleOwner); err != nil {
		return err
	}

	return s.groupRepo.Delete(ctx, groupID)
}

func (s *GroupService) GetGroupBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.GroupResponse, error) {
	group, err := s.groupRepo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("group not found")
	}

	return s.toGroupResponse(group, userID), nil
}

func (s *GroupService) ListGroups(ctx context.Context, keyword, privacy string, page, pageSize int) (*dto.GroupListResponse, error) {
	groups, total, err := s.groupRepo.List(ctx, keyword, privacy, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.GroupResponse, len(groups))
	for i, g := range groups {
		responses[i] = *s.toGroupResponse(&g, nil)
	}

	return &dto.GroupListResponse{
		Groups:     responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *GroupService) GetMyGroups(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.GroupListResponse, error) {
	groups, total, err := s.groupRepo.GetByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.GroupResponse, len(groups))
	for i, g := range groups {
		responses[i] = *s.toGroupResponse(&g, &userID)
	}

	return &dto.GroupListResponse{
		Groups:     responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *GroupService) GetMyOwnedGroups(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.GroupListResponse, error) {
	groups, total, err := s.groupRepo.GetOwnedByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.GroupResponse, len(groups))
	for i, g := range groups {
		ownerRole := string(model.GroupRoleOwner)
		resp := s.toGroupResponse(&g, nil)
		resp.MyRole = &ownerRole
		responses[i] = *resp
	}

	return &dto.GroupListResponse{
		Groups:     responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

// ============================================================================
// MEMBERSHIP
// ============================================================================

func (s *GroupService) JoinGroup(ctx context.Context, userID, groupID uuid.UUID, message *string) (interface{}, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("group not found")
	}

	// Check if already a member
	existing, err := s.memberRepo.GetByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Status == model.GroupMemberActive {
			return nil, errors.New("already a member of this group")
		}
		if existing.Status == model.GroupMemberBanned {
			return nil, errors.New("you are banned from this group")
		}
	}

	if group.MemberCount >= group.MaxMembers {
		return nil, errors.New("group is full")
	}

	// PUBLIC -> join directly
	if group.Privacy == model.GroupPrivacyPublic {
		now := time.Now()
		member := &model.GroupMember{
			GroupID:  groupID,
			UserID:   userID,
			Role:     model.GroupRoleMember,
			Status:   model.GroupMemberActive,
			JoinedAt: &now,
		}

		if existing != nil {
			existing.Status = model.GroupMemberActive
			existing.Role = model.GroupRoleMember
			existing.JoinedAt = &now
			if err := s.memberRepo.Update(ctx, existing); err != nil {
				return nil, err
			}
		} else {
			if err := s.memberRepo.Create(ctx, member); err != nil {
				return nil, err
			}
		}

		_ = s.groupRepo.IncrementMemberCount(ctx, groupID)

		// Add to group conversation
		s.addToGroupConversation(ctx, groupID, userID)

		return map[string]string{"status": "joined"}, nil
	}

	// PRIVATE/SECRET -> create join request
	pendingReq, err := s.joinRequestRepo.GetPendingByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if pendingReq != nil {
		return nil, errors.New("you already have a pending join request")
	}

	joinReq := &model.GroupJoinRequest{
		GroupID: groupID,
		UserID:  userID,
		Message: message,
		Status:  model.JoinRequestPending,
	}
	if err := s.joinRequestRepo.Create(ctx, joinReq); err != nil {
		return nil, err
	}

	return map[string]string{"status": "pending", "request_id": joinReq.ID.String()}, nil
}

func (s *GroupService) LeaveGroup(ctx context.Context, userID, groupID uuid.UUID) error {
	member, err := s.memberRepo.GetActiveByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("not a member of this group")
	}

	if member.Role == model.GroupRoleOwner {
		return errors.New("owner cannot leave the group, transfer ownership first")
	}

	member.Status = model.GroupMemberLeft
	if err := s.memberRepo.Update(ctx, member); err != nil {
		return err
	}

	_ = s.groupRepo.DecrementMemberCount(ctx, groupID)
	return nil
}

// ============================================================================
// MEMBER MANAGEMENT
// ============================================================================

func (s *GroupService) ListMembers(ctx context.Context, groupID uuid.UUID, page, pageSize int) (*dto.GroupMemberListResponse, error) {
	members, total, err := s.memberRepo.ListByGroupID(ctx, groupID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.GroupMemberResponse, len(members))
	for i, m := range members {
		responses[i] = dto.GroupMemberResponse{
			ID:        m.ID,
			UserID:    m.UserID,
			UserName:  m.User.UserName,
			Email:     m.User.Email,
			AvatarURL: m.User.AvatarURL,
			Role:      string(m.Role),
			Status:    string(m.Status),
			Nickname:  m.Nickname,
			JoinedAt:  m.JoinedAt,
		}
	}

	return &dto.GroupMemberListResponse{
		Members:    responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *GroupService) InviteMembers(ctx context.Context, inviterID, groupID uuid.UUID, userIDs []uuid.UUID) error {
	if err := s.requireRole(ctx, groupID, inviterID, model.GroupRoleOwner, model.GroupRoleAdmin, model.GroupRoleModerator); err != nil {
		return err
	}

	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("group not found")
	}

	for _, uid := range userIDs {
		existing, err := s.memberRepo.GetByGroupAndUser(ctx, groupID, uid)
		if err != nil {
			continue
		}
		if existing != nil && (existing.Status == model.GroupMemberActive || existing.Status == model.GroupMemberInvited) {
			continue
		}

		now := time.Now()
		member := &model.GroupMember{
			GroupID:   groupID,
			UserID:    uid,
			Role:      model.GroupRoleMember,
			Status:    model.GroupMemberActive,
			InvitedBy: &inviterID,
			JoinedAt:  &now,
		}
		if err := s.memberRepo.Create(ctx, member); err != nil {
			continue
		}
		_ = s.groupRepo.IncrementMemberCount(ctx, groupID)
		s.addToGroupConversation(ctx, groupID, uid)
	}

	return nil
}

func (s *GroupService) UpdateMemberRole(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID, role string) error {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin); err != nil {
		return err
	}

	member, err := s.memberRepo.GetActiveByGroupAndUser(ctx, groupID, targetUserID)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("member not found")
	}

	if member.Role == model.GroupRoleOwner {
		return errors.New("cannot change owner role")
	}

	member.Role = model.GroupMemberRole(role)
	return s.memberRepo.Update(ctx, member)
}

func (s *GroupService) RemoveMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin); err != nil {
		return err
	}

	member, err := s.memberRepo.GetActiveByGroupAndUser(ctx, groupID, targetUserID)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("member not found")
	}

	if member.Role == model.GroupRoleOwner {
		return errors.New("cannot remove the owner")
	}

	member.Status = model.GroupMemberLeft
	if err := s.memberRepo.Update(ctx, member); err != nil {
		return err
	}

	_ = s.groupRepo.DecrementMemberCount(ctx, groupID)
	return nil
}

func (s *GroupService) BanMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin); err != nil {
		return err
	}

	member, err := s.memberRepo.GetByGroupAndUser(ctx, groupID, targetUserID)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("member not found")
	}
	if member.Role == model.GroupRoleOwner {
		return errors.New("cannot ban the owner")
	}

	wasActive := member.Status == model.GroupMemberActive
	member.Status = model.GroupMemberBanned
	if err := s.memberRepo.Update(ctx, member); err != nil {
		return err
	}

	if wasActive {
		_ = s.groupRepo.DecrementMemberCount(ctx, groupID)
	}
	return nil
}

func (s *GroupService) UnbanMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin); err != nil {
		return err
	}

	member, err := s.memberRepo.GetByGroupAndUser(ctx, groupID, targetUserID)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("member not found")
	}
	if member.Status != model.GroupMemberBanned {
		return errors.New("member is not banned")
	}

	member.Status = model.GroupMemberLeft
	return s.memberRepo.Update(ctx, member)
}

// ============================================================================
// JOIN REQUESTS
// ============================================================================

func (s *GroupService) ListJoinRequests(ctx context.Context, requesterID, groupID uuid.UUID, page, pageSize int) (*dto.JoinRequestListResponse, error) {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin, model.GroupRoleModerator); err != nil {
		return nil, err
	}

	requests, total, err := s.joinRequestRepo.ListByGroupID(ctx, groupID, string(model.JoinRequestPending), page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.JoinRequestResponse, len(requests))
	for i, r := range requests {
		responses[i] = dto.JoinRequestResponse{
			ID:              r.ID,
			GroupID:         r.GroupID,
			UserID:          r.UserID,
			UserName:        r.User.UserName,
			Email:           r.User.Email,
			AvatarURL:       r.User.AvatarURL,
			Message:         r.Message,
			Status:          string(r.Status),
			ReviewedBy:      r.ReviewedBy,
			ReviewedAt:      r.ReviewedAt,
			RejectionReason: r.RejectionReason,
			CreatedAt:       r.CreatedAt,
		}
	}

	return &dto.JoinRequestListResponse{
		Requests:   responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *GroupService) ApproveRequest(ctx context.Context, requesterID, groupID, requestID uuid.UUID) error {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin, model.GroupRoleModerator); err != nil {
		return err
	}

	req, err := s.joinRequestRepo.GetByID(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return errors.New("request not found")
	}
	if req.GroupID != groupID {
		return errors.New("request does not belong to this group")
	}
	if req.Status != model.JoinRequestPending {
		return errors.New("request is not pending")
	}

	now := time.Now()
	req.Status = model.JoinRequestApproved
	req.ReviewedBy = &requesterID
	req.ReviewedAt = &now
	if err := s.joinRequestRepo.Update(ctx, req); err != nil {
		return err
	}

	// Add as member
	member := &model.GroupMember{
		GroupID:  groupID,
		UserID:   req.UserID,
		Role:     model.GroupRoleMember,
		Status:   model.GroupMemberActive,
		JoinedAt: &now,
	}
	if err := s.memberRepo.Create(ctx, member); err != nil {
		return err
	}

	_ = s.groupRepo.IncrementMemberCount(ctx, groupID)
	s.addToGroupConversation(ctx, groupID, req.UserID)
	return nil
}

func (s *GroupService) RejectRequest(ctx context.Context, requesterID, groupID, requestID uuid.UUID, reason *string) error {
	if err := s.requireRole(ctx, groupID, requesterID, model.GroupRoleOwner, model.GroupRoleAdmin, model.GroupRoleModerator); err != nil {
		return err
	}

	req, err := s.joinRequestRepo.GetByID(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return errors.New("request not found")
	}
	if req.GroupID != groupID {
		return errors.New("request does not belong to this group")
	}
	if req.Status != model.JoinRequestPending {
		return errors.New("request is not pending")
	}

	now := time.Now()
	req.Status = model.JoinRequestRejected
	req.ReviewedBy = &requesterID
	req.ReviewedAt = &now
	req.RejectionReason = reason
	return s.joinRequestRepo.Update(ctx, req)
}

// ============================================================================
// HELPERS
// ============================================================================

func (s *GroupService) requireRole(ctx context.Context, groupID, userID uuid.UUID, roles ...model.GroupMemberRole) error {
	member, err := s.memberRepo.GetActiveByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("not a member of this group")
	}

	for _, role := range roles {
		if member.Role == role {
			return nil
		}
	}
	return errors.New("insufficient permissions")
}

func (s *GroupService) addToGroupConversation(ctx context.Context, groupID, userID uuid.UUID) {
	conv, err := s.convRepo.GetByGroupID(ctx, groupID)
	if err != nil || conv == nil {
		return
	}

	existing, err := s.participantRepo.GetByConvAndUser(ctx, conv.ID, userID)
	if err != nil || existing != nil {
		return
	}

	participant := &model.ConversationParticipant{
		ConversationID: conv.ID,
		UserID:         userID,
		JoinedAt:       time.Now(),
	}
	_ = s.participantRepo.Create(ctx, participant)
}

func (s *GroupService) toGroupResponse(group *model.Group, userID *uuid.UUID) *dto.GroupResponse {
	resp := &dto.GroupResponse{
		ID:             group.ID,
		Name:           group.Name,
		Slug:           group.Slug,
		Description:    group.Description,
		AvatarURL:      group.AvatarURL,
		CoverURL:       group.CoverURL,
		Type:           string(group.Type),
		Privacy:        string(group.Privacy),
		MaxMembers:     group.MaxMembers,
		MemberCount:    group.MemberCount,
		CreatedBy:      group.CreatedBy,
		OrganizationID: group.OrganizationID,
		CreatedAt:      group.CreatedAt,
		UpdatedAt:      group.UpdatedAt,
	}

	if group.Conversation != nil {
		resp.Conversation = &dto.ConversationBrief{ID: group.Conversation.ID}
	}

	if userID != nil {
		member, err := s.memberRepo.GetActiveByGroupAndUser(context.Background(), group.ID, *userID)
		if err == nil && member != nil {
			role := string(member.Role)
			resp.MyRole = &role
		}
	}

	return resp
}
