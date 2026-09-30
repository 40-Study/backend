package service

import (
	"context"
	"errors"
	"log"
	"strings"
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

	// status: "" hoặc ACTIVE (mặc định) | BANNED (chỉ OWNER/ADMIN).
	ListMembers(ctx context.Context, requesterID, groupID uuid.UUID, status string, page, pageSize int) (*dto.GroupMemberListResponse, error)
	InviteMembers(ctx context.Context, inviterID, groupID uuid.UUID, userIDs []uuid.UUID) (*dto.InviteMembersResult, error)
	UpdateMemberRole(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error
	BanMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error
	UnbanMember(ctx context.Context, requesterID, groupID, targetUserID uuid.UUID) error

	ListJoinRequests(ctx context.Context, requesterID, groupID uuid.UUID, page, pageSize int) (*dto.JoinRequestListResponse, error)
	ApproveRequest(ctx context.Context, requesterID, groupID, requestID uuid.UUID) error
	RejectRequest(ctx context.Context, requesterID, groupID, requestID uuid.UUID, reason *string) error
}

// GroupInviteGuard quyet dinh nguoi moi co duoc phep them vao nhom hay khong. Dung chung quy tac
// "gioi han nhan tin theo quan he" cua ConversationService (Lane G) - nhom co hoi thoai chung nen
// moi vao nhom la mot cach mo duong nhan tin, khong duoc lach guard.
type GroupInviteGuard interface {
	CanStartDirectConversation(ctx context.Context, userA, userB uuid.UUID) (bool, error)
}

// ErrGroupInviteGuardMissing - loi cau hinh: chua gan guard thi tu choi moi, khong mo cua.
var ErrGroupInviteGuardMissing = errors.New("group invite guard is not configured")

// GroupNotifier gửi thông báo cho người vừa được thêm vào nhóm (NotificationService cài đặt). Tuỳ chọn:
// nil = bỏ qua. Lỗi gửi chỉ được log, không làm hỏng việc thêm thành viên.
type GroupNotifier interface {
	SendNotification(req dto.CreateNotificationDTO) error
}

type GroupService struct {
	inviteGuard     GroupInviteGuard
	notifier        GroupNotifier
	evictor         ChannelEvictor
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

// SetInviteGuard gan guard moi thanh vien (xem GroupInviteGuard).
func (s *GroupService) SetInviteGuard(g GroupInviteGuard) { s.inviteGuard = g }

// SetNotifier nối bộ gửi thông báo group_added (xem GroupNotifier).
func (s *GroupService) SetNotifier(n GroupNotifier) { s.notifier = n }

// notifyGroupAdded báo cho những người vừa được thêm vào nhóm (một lần gọi cho cả lô). Tham chiếu là nhóm
// (reference_type "group"): thông báo không mang slug nên web điều hướng tới /groups.
func (s *GroupService) notifyGroupAdded(group *model.Group, userIDs []uuid.UUID) {
	if s.notifier == nil || len(userIDs) == 0 {
		return
	}
	refType, refID := "group", group.ID
	err := s.notifier.SendNotification(dto.CreateNotificationDTO{
		Title:            "Bạn được thêm vào nhóm",
		Content:          "Bạn đã được thêm vào nhóm \"" + group.Name + "\".",
		NotificationType: model.NotificationTypeGroupAdded,
		ReferenceType:    &refType,
		ReferenceID:      &refID,
		UserIDs:          userIDs,
	})
	if err != nil {
		log.Printf("group: gửi thông báo group_added (nhóm %s) lỗi: %v", group.ID, err)
	}
}

// ChannelEvictor ngừng phát kênh WebSocket cho một user (socket.Notifier cài đặt). Tuỳ chọn: nil = bỏ qua.
type ChannelEvictor interface {
	EvictUserFromChannel(userID uuid.UUID, channel string)
}

// SetChannelEvictor gắn evictor để người rời/bị kick/ban ngừng nhận tin qua kết nối WebSocket đang mở (S6).
func (s *GroupService) SetChannelEvictor(e ChannelEvictor) { s.evictor = e }

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
		return nil, ErrGroupNotFound
	}

	// S6: nhóm SECRET chỉ thành viên thấy; người không phải thành viên (kể cả khách chưa đăng nhập)
	// nhận 404 như nhóm không tồn tại. Nhóm PRIVATE vẫn hiện tên và mô tả để xin vào, nhưng KHÔNG kèm
	// id hội thoại (xem toGroupResponse): id đó là chìa khoá để nghe/gửi tin của nhóm.
	if group.Privacy == model.GroupPrivacySecret {
		isMember := false
		if userID != nil {
			member, err := s.memberRepo.GetActiveByGroupAndUser(ctx, group.ID, *userID)
			if err != nil {
				return nil, err
			}
			isMember = member != nil
		}
		if !isMember {
			return nil, ErrGroupNotFound
		}
	}

	resp := s.toGroupResponse(group, userID)
	// Người xem đã đăng nhập, chưa là thành viên: cho biết đã có yêu cầu xin vào đang chờ chưa (web hiện
	// "Đã gửi yêu cầu" thay vì "Xin tham gia"). Nhóm SECRET đã bị chặn ở trên nên không lộ qua đây.
	if userID != nil && resp.MyRole == nil {
		pending, err := s.joinRequestRepo.GetPendingByGroupAndUser(ctx, group.ID, *userID)
		if err != nil {
			return nil, err
		}
		if pending != nil {
			resp.MyJoinRequest = &dto.MyJoinRequestBrief{ID: pending.ID, Status: string(model.JoinRequestPending)}
		}
	}
	return resp, nil
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
		return nil, ErrGroupNotFound
	}

	// Check if already a member
	existing, err := s.memberRepo.GetByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	// S6: nhóm SECRET không tồn tại với người chưa được mời: xin vào bằng id cũng 404.
	if group.Privacy == model.GroupPrivacySecret && (existing == nil || (existing.Status != model.GroupMemberInvited && existing.Status != model.GroupMemberActive)) {
		return nil, ErrGroupNotFound
	}
	if existing != nil {
		if existing.Status == model.GroupMemberActive {
			return nil, ErrGroupAlreadyMember
		}
		if existing.Status == model.GroupMemberBanned {
			return nil, ErrGroupBanned
		}
	}

	if group.MemberCount >= group.MaxMembers {
		return nil, ErrGroupFull
	}

	// PUBLIC -> join directly
	if group.Privacy == model.GroupPrivacyPublic {
		if err := s.activateMember(ctx, groupID, userID, nil); err != nil {
			return nil, err
		}
		return map[string]string{"status": "joined"}, nil
	}

	// PRIVATE/SECRET -> create join request
	pendingReq, err := s.joinRequestRepo.GetPendingByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if pendingReq != nil {
		return nil, ErrGroupJoinRequestExists
	}

	// group_join_requests unique theo (group_id, user_id): người từng bị từ chối, hoặc từng được duyệt rồi rời
	// nhóm, còn một dòng cũ không ở trạng thái chờ. Chèn dòng mới sẽ vi phạm unique và họ không bao giờ xin
	// vào lại được; dùng lại dòng cũ và đặt về trạng thái chờ.
	joinReq, err := s.joinRequestRepo.GetByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if joinReq != nil {
		joinReq.Status = model.JoinRequestPending
		joinReq.Message = message
		joinReq.ReviewedBy, joinReq.ReviewedAt, joinReq.RejectionReason = nil, nil, nil
		joinReq.CreatedAt = time.Now() // thứ tự danh sách chờ duyệt theo lần xin gần nhất
		if err := s.joinRequestRepo.Update(ctx, joinReq); err != nil {
			return nil, err
		}
	} else {
		joinReq = &model.GroupJoinRequest{
			GroupID: groupID,
			UserID:  userID,
			Message: message,
			Status:  model.JoinRequestPending,
		}
		if err := s.joinRequestRepo.Create(ctx, joinReq); err != nil {
			return nil, err
		}
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
	return s.removeFromGroupConversation(ctx, groupID, userID)
}

// ============================================================================
// MEMBER MANAGEMENT
// ============================================================================

// ErrGroupNotFound (S5): nhóm không tồn tại HOẶC là nhóm SECRET mà người gọi không phải thành viên (404, không phân biệt).
var ErrGroupNotFound = errors.New("group not found")

// Lỗi nhóm có `code` riêng (contract-api.md §2). Thông điệp giữ nguyên câu tiếng Anh cũ vì web đang khớp
// theo câu ở những nơi chưa đọc `code` (web/src/lib/error-messages.ts).
var (
	ErrGroupAlreadyMember     = errors.New("already a member of this group")
	ErrGroupBanned            = errors.New("you are banned from this group")
	ErrGroupFull              = errors.New("group is full")
	ErrGroupJoinRequestExists = errors.New("you already have a pending join request")
	// ErrGroupForbidden: người gọi không đủ quyền xem (403), khác ErrGroupNotFound (404 không lộ tồn tại).
	ErrGroupForbidden = errors.New("insufficient permissions")
	// ErrGroupInvalidMemberStatus: ?status= của danh sách thành viên ngoài ACTIVE|BANNED (400).
	ErrGroupInvalidMemberStatus = errors.New("status must be ACTIVE or BANNED")
)

func (s *GroupService) ListMembers(ctx context.Context, requesterID, groupID uuid.UUID, status string, page, pageSize int) (*dto.GroupMemberListResponse, error) {
	wanted := model.GroupMemberActive
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "", string(model.GroupMemberActive):
	case string(model.GroupMemberBanned):
		wanted = model.GroupMemberBanned
	default:
		return nil, ErrGroupInvalidMemberStatus
	}

	// S5: nhóm SECRET bị ẩn khỏi danh sách nhóm (group_repository.go) nên danh sách thành viên cũng chỉ thành viên xem được;
	// trước đây mọi tài khoản đăng nhập liệt kê được tên và avatar thành viên của nhóm SECRET bằng id.
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrGroupNotFound
	}
	requester, err := s.memberRepo.GetActiveByGroupAndUser(ctx, groupID, requesterID)
	if err != nil {
		return nil, err
	}
	if group.Privacy == model.GroupPrivacySecret && requester == nil {
		return nil, ErrGroupNotFound
	}
	// Q10: nhóm PRIVATE cho người ngoài thấy tên và mô tả để xin vào, nhưng KHÔNG thấy tên/avatar thành viên
	// (đa số là học sinh): 403, không phải 404 vì nhóm vẫn tồn tại và tìm được.
	if group.Privacy == model.GroupPrivacyPrivate && requester == nil {
		return nil, ErrGroupForbidden
	}
	// Danh sách bị cấm chỉ OWNER/ADMIN: người bị cấm là dữ liệu quản trị, thành viên thường không cần thấy.
	if wanted == model.GroupMemberBanned &&
		(requester == nil || (requester.Role != model.GroupRoleOwner && requester.Role != model.GroupRoleAdmin)) {
		return nil, ErrGroupForbidden
	}
	members, total, err := s.memberRepo.ListByGroupID(ctx, groupID, wanted, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.GroupMemberResponse, len(members))
	for i, m := range members {
		responses[i] = dto.GroupMemberResponse{
			ID:        m.ID,
			UserID:    m.UserID,
			UserName:  m.User.UserName,
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

func (s *GroupService) InviteMembers(ctx context.Context, inviterID, groupID uuid.UUID, userIDs []uuid.UUID) (*dto.InviteMembersResult, error) {
	if err := s.requireRole(ctx, groupID, inviterID, model.GroupRoleOwner, model.GroupRoleAdmin, model.GroupRoleModerator); err != nil {
		return nil, err
	}

	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("group not found")
	}
	if s.inviteGuard == nil {
		return nil, ErrGroupInviteGuardMissing
	}

	result := &dto.InviteMembersResult{Invited: []uuid.UUID{}, Rejected: []dto.InviteRejection{}}
	seen := make(map[uuid.UUID]bool, len(userIDs))
	for _, uid := range userIDs {
		if seen[uid] { // cùng một id lặp trong request: chỉ xử lý lần đầu, không báo "đã là thành viên" giả
			continue
		}
		seen[uid] = true

		// Guard nhan tin (Lane G): nguoi moi phai co quan he hop le voi nguoi moi; admin di qua.
		// Loi ha tang -> tra loi, khong duoc coi la "cho phep". Guard chạy TRƯỚC mọi kiểm tra trạng thái
		// thành viên: người lạ không được biết một người khác đã ở trong nhóm hay đã bị cấm.
		allowed, gerr := s.inviteGuard.CanStartDirectConversation(ctx, inviterID, uid)
		if gerr != nil {
			return nil, gerr
		}
		if !allowed {
			result.Rejected = append(result.Rejected, dto.InviteRejection{UserID: uid, Code: dto.GroupInviteNotAllowedCode})
			continue
		}

		// activateMember dùng chung với join/approve: kích hoạt lại cả người đã rời (trước đây chèn dòng mới
		// vi phạm unique và bị bỏ qua im lặng), kiểm sức chứa, không nuốt lỗi hạ tầng.
		switch err := s.activateMember(ctx, groupID, uid, &inviterID); {
		case err == nil:
			result.Invited = append(result.Invited, uid)
		case errors.Is(err, ErrGroupAlreadyMember):
			result.Rejected = append(result.Rejected, dto.InviteRejection{UserID: uid, Code: dto.GroupAlreadyMemberCode})
		case errors.Is(err, ErrGroupBanned):
			result.Rejected = append(result.Rejected, dto.InviteRejection{UserID: uid, Code: dto.GroupMemberBannedCode})
		case errors.Is(err, ErrGroupFull):
			result.Rejected = append(result.Rejected, dto.InviteRejection{UserID: uid, Code: dto.GroupFullCode})
		default:
			return nil, err
		}
	}

	s.notifyGroupAdded(group, result.Invited)
	return result, nil
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
	return s.removeFromGroupConversation(ctx, groupID, targetUserID)
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
	// Bị cấm thì luôn gỡ khỏi hội thoại, kể cả khi trạng thái trước đó không phải ACTIVE.
	return s.removeFromGroupConversation(ctx, groupID, targetUserID)
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

	// Thêm thành viên TRƯỚC khi đánh dấu APPROVED: nhóm đầy hoặc người xin đã bị cấm thì yêu cầu phải còn
	// nguyên trạng thái chờ (không có yêu cầu "đã duyệt" mà người đó không vào được nhóm). Đã là thành viên
	// (duyệt lại sau lần lỗi trước) thì vẫn chốt yêu cầu, không báo lỗi.
	if err := s.activateMember(ctx, groupID, req.UserID, nil); err != nil && !errors.Is(err, ErrGroupAlreadyMember) {
		return err
	}

	now := time.Now()
	req.Status = model.JoinRequestApproved
	req.ReviewedBy = &requesterID
	req.ReviewedAt = &now
	return s.joinRequestRepo.Update(ctx, req)
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

// removeFromGroupConversation (S6) gỡ người dùng khỏi hội thoại của nhóm (đặt left_at). Lỗi được trả về:
// nuốt lỗi sẽ để người bị kick/ban đọc và gửi tin tiếp mà không ai biết.
func (s *GroupService) removeFromGroupConversation(ctx context.Context, groupID, userID uuid.UUID) error {
	conv, err := s.convRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return err
	}
	if conv == nil {
		return nil
	}
	if err := s.participantRepo.MarkLeft(ctx, conv.ID, userID); err != nil {
		return err
	}
	if s.evictor != nil {
		s.evictor.EvictUserFromChannel(userID, "conversation:"+conv.ID.String())
		s.evictor.EvictUserFromChannel(userID, "group:"+groupID.String())
	}
	return nil
}

// activateMember đưa userID vào nhóm ở trạng thái ACTIVE với vai trò MEMBER: dùng chung cho vào nhóm công
// khai (join), mời (invite) và duyệt yêu cầu (approve) để ba đường không còn lệch nhau.
//
//   - Đã ACTIVE -> ErrGroupAlreadyMember; đang BANNED -> ErrGroupBanned (dù mời hay duyệt đều không vượt được).
//   - Giữ chỗ bằng câu UPDATE nguyên tử (TryReserveMemberSlot) nên không ai vượt max_members; ghi member
//     lỗi thì trả lại chỗ. Hết chỗ -> ErrGroupFull.
//   - Dòng cũ (LEFT/INVITED/PENDING) được kích hoạt lại thay vì chèn dòng mới (unique group_id+user_id),
//     và vai trò đặt lại về MEMBER: người từng là ADMIN rồi rời nhóm không được lấy lại quyền cũ.
//   - Thêm vào hội thoại nhóm; lỗi ở bước này được trả ra, không nuốt.
func (s *GroupService) activateMember(ctx context.Context, groupID, userID uuid.UUID, invitedBy *uuid.UUID) error {
	existing, err := s.memberRepo.GetByGroupAndUser(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if existing != nil {
		switch existing.Status {
		case model.GroupMemberActive:
			return ErrGroupAlreadyMember
		case model.GroupMemberBanned:
			return ErrGroupBanned
		}
	}

	reserved, err := s.groupRepo.TryReserveMemberSlot(ctx, groupID)
	if err != nil {
		return err
	}
	if !reserved {
		return ErrGroupFull
	}

	now := time.Now()
	var writeErr error
	if existing != nil {
		existing.Status = model.GroupMemberActive
		existing.Role = model.GroupRoleMember
		existing.JoinedAt = &now
		if invitedBy != nil {
			existing.InvitedBy = invitedBy
		}
		writeErr = s.memberRepo.Update(ctx, existing)
	} else {
		writeErr = s.memberRepo.Create(ctx, &model.GroupMember{
			GroupID:   groupID,
			UserID:    userID,
			Role:      model.GroupRoleMember,
			Status:    model.GroupMemberActive,
			InvitedBy: invitedBy,
			JoinedAt:  &now,
		})
	}
	if writeErr != nil {
		_ = s.groupRepo.DecrementMemberCount(ctx, groupID) // trả lại chỗ đã giữ
		return writeErr
	}
	return s.addToGroupConversation(ctx, groupID, userID)
}

func (s *GroupService) addToGroupConversation(ctx context.Context, groupID, userID uuid.UUID) error {
	conv, err := s.convRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return err
	}
	if conv == nil {
		return nil
	}

	existing, err := s.participantRepo.GetByConvAndUser(ctx, conv.ID, userID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	// Vào lại nhóm sau khi đã rời: kích hoạt lại dòng cũ thay vì chèn dòng mới (chỉ mục duy nhất).
	rejoined, err := s.participantRepo.Rejoin(ctx, conv.ID, userID)
	if err != nil {
		return err
	}
	if rejoined {
		return nil
	}

	return s.participantRepo.Create(ctx, &model.ConversationParticipant{
		ConversationID: conv.ID,
		UserID:         userID,
		JoinedAt:       time.Now(),
	})
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

	if userID != nil {
		member, err := s.memberRepo.GetActiveByGroupAndUser(context.Background(), group.ID, *userID)
		if err == nil && member != nil {
			role := string(member.Role)
			resp.MyRole = &role
			// S6: id hội thoại chỉ trả cho thành viên còn hiệu lực. Trước đây GET /groups/:slug (công khai)
			// trả cho cả khách, đủ để đăng ký nghe tin nhắn của nhóm qua WebSocket.
			if group.Conversation != nil {
				resp.Conversation = &dto.ConversationBrief{ID: group.Conversation.ID}
			}
		}
	}

	return resp
}
