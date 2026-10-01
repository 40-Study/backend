package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// normalizeFriendPage đưa page/limit về khoảng hợp lệ (mặc định 1/20, tối đa 100 — contract §0).
func normalizeFriendPage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = constants.FriendPageDefaultLimit
	}
	if limit > constants.FriendPageMaxLimit {
		limit = constants.FriendPageMaxLimit
	}
	return page, limit
}

// ListFriends — GET /friends.
func (s *FriendshipService) ListFriends(ctx context.Context, me uuid.UUID, keyword string, page, limit int) (*dto.FriendListResponse, error) {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return nil, err
	}
	page, limit = normalizeFriendPage(page, limit)
	rows, total, err := s.repo.ListFriends(ctx, me, strings.TrimSpace(keyword), page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.FriendItemDTO, len(rows))
	for i, r := range rows {
		items[i] = dto.FriendItemDTO{FriendshipID: r.FriendshipID, User: toFriendUserDTO(r.FriendUserRow), Since: r.Since}
	}
	return &dto.FriendListResponse{Friends: items, TotalCount: total, Page: page, Limit: limit}, nil
}

// Summary — GET /friends/summary.
func (s *FriendshipService) Summary(ctx context.Context, me uuid.UUID) (*dto.FriendSummaryResponse, error) {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return nil, err
	}
	friends, err := s.repo.CountFriends(ctx, me)
	if err != nil {
		return nil, err
	}
	in, err := s.repo.CountIncomingPending(ctx, me)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.CountOutgoingPending(ctx, me)
	if err != nil {
		return nil, err
	}
	return &dto.FriendSummaryResponse{FriendsCount: friends, IncomingRequests: in, OutgoingRequests: out}, nil
}

// ListRequests — GET /friends/requests. direction rỗng = incoming.
func (s *FriendshipService) ListRequests(ctx context.Context, me uuid.UUID, direction string, page, limit int) (*dto.FriendRequestListResponse, error) {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return nil, err
	}
	var outgoing bool
	switch direction {
	case "", "incoming":
	case "outgoing":
		outgoing = true
	default:
		return nil, ErrFriendInvalidDirection
	}
	page, limit = normalizeFriendPage(page, limit)
	rows, total, err := s.repo.ListRequests(ctx, me, outgoing, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.FriendRequestItemDTO, len(rows))
	for i, r := range rows {
		items[i] = dto.FriendRequestItemDTO{ID: r.ID, Direction: r.Direction, User: toFriendUserDTO(r.FriendUserRow), CreatedAt: r.CreatedAt}
	}
	return &dto.FriendRequestListResponse{Requests: items, TotalCount: total, Page: page, Limit: limit}, nil
}

// relationFromRow suy ra RelationStatus của viewer với đối phương từ dòng friendships. Dòng DECLINED và
// CANCELLED coi như NONE: contract không có trạng thái "bị từ chối" (không cho người gửi biết mình bị từ chối).
func relationFromRow(row *model.Friendship, viewer uuid.UUID) (string, *uuid.UUID) {
	if row == nil {
		return dto.RelationNone, nil
	}
	switch row.Status {
	case model.FriendshipStatusAccepted:
		return dto.RelationFriends, nil
	case model.FriendshipStatusPending:
		id := row.ID
		if row.RequesterID == viewer {
			return dto.RelationPendingOut, &id
		}
		return dto.RelationPendingIn, &id
	}
	return dto.RelationNone, nil
}

// Search — GET /friends/search. Trần ≥3 ký tự và tối đa 20 kết quả (Q2) để không dùng làm công cụ liệt kê
// danh sách học sinh; không trả email/điện thoại.
func (s *FriendshipService) Search(ctx context.Context, me uuid.UUID, q string, limit int) (*dto.FriendSearchResponse, error) {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < constants.FriendSearchMinChars {
		return nil, ErrFriendQueryTooShort
	}
	if limit < 1 || limit > constants.FriendSearchMaxResults {
		limit = constants.FriendSearchMaxResults
	}
	rows, err := s.repo.SearchStudents(ctx, me, q, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	rels, err := s.repo.RelationsWith(ctx, me, ids)
	if err != nil {
		return nil, err
	}
	byOther := make(map[uuid.UUID]*model.Friendship, len(rels))
	for i := range rels {
		other := rels[i].RequesterID
		if other == me {
			other = rels[i].AddresseeID
		}
		byOther[other] = &rels[i]
	}
	users := make([]dto.FriendSearchUserDTO, len(rows))
	for i, r := range rows {
		status, reqID := relationFromRow(byOther[r.UserID], me)
		users[i] = dto.FriendSearchUserDTO{FriendUserDTO: toFriendUserDTO(r), Relationship: status, RequestID: reqID}
	}
	return &dto.FriendSearchResponse{Users: users}, nil
}

// Relationship — GET /friends/relationship/:userId. Đích không phải học viên hợp lệ → ErrFriendUserNotFound
// (web ẩn nút kết bạn). Bị đối phương chặn → NONE, KHÔNG lộ việc bị chặn.
func (s *FriendshipService) Relationship(ctx context.Context, me, otherID uuid.UUID) (*dto.FriendRelationshipResponse, error) {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return nil, err
	}
	if me == otherID {
		return &dto.FriendRelationshipResponse{Status: dto.RelationSelf}, nil
	}
	target, err := s.repo.FindEligibleTarget(ctx, otherID, false)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrFriendUserNotFound
	}
	byMe, err := s.blocks.IsBlockedBy(ctx, me, otherID)
	if err != nil {
		return nil, err
	}
	if byMe {
		return &dto.FriendRelationshipResponse{Status: dto.RelationBlockedByMe}, nil
	}
	byThem, err := s.blocks.IsBlockedBy(ctx, otherID, me)
	if err != nil {
		return nil, err
	}
	if byThem {
		return &dto.FriendRelationshipResponse{Status: dto.RelationNone}, nil
	}
	row, err := s.repo.FindByPair(ctx, me, otherID)
	if err != nil {
		return nil, err
	}
	status, reqID := relationFromRow(row, me)
	return &dto.FriendRelationshipResponse{Status: status, RequestID: reqID}, nil
}

// ListBlocks — GET /friends/blocks.
func (s *FriendshipService) ListBlocks(ctx context.Context, me uuid.UUID, page, limit int) (*dto.FriendBlockListResponse, error) {
	if _, err := s.requireActive(ctx, me); err != nil {
		return nil, err
	}
	page, limit = normalizeFriendPage(page, limit)
	rows, total, err := s.blocks.List(ctx, me, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.FriendBlockItemDTO, len(rows))
	for i, r := range rows {
		items[i] = dto.FriendBlockItemDTO{User: toFriendUserDTO(r.FriendUserRow), CreatedAt: r.CreatedAt}
	}
	return &dto.FriendBlockListResponse{Blocks: items, TotalCount: total, Page: page, Limit: limit}, nil
}

// Block — POST /friends/blocks: chặn + xoá bạn + huỷ lời mời đang chờ hai chiều trong CÙNG transaction, để
// không có khoảnh khắc đã chặn mà vẫn là bạn. CHỈ quan hệ đang hiệu lực bị đụng tới (xem EndActiveRelation):
// lịch sử DECLINED/CANCELLED được giữ để block -> unblock không xoá cooldown và hạn mức. Chặn lại người đã
// chặn là thao tác idempotent.
func (s *FriendshipService) Block(ctx context.Context, me, targetID uuid.UUID) error {
	if _, err := s.requireActive(ctx, me); err != nil {
		return err
	}
	if me == targetID {
		return ErrFriendSelfRequest
	}
	// Chặn áp dụng cho MỌI tài khoản còn hoạt động, không phụ thuộc allowlist vai trò: người dùng tự thêm được
	// vai phụ (PARENT, TEACHER_APPLICANT...) cho mình, nếu đòi "học viên thuần" thì né được lệnh chặn và khoá DM.
	target, err := s.repo.FindActiveUser(ctx, targetID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrFriendUserNotFound
	}
	return s.repo.Transaction(ctx, func(tx *repository.FriendshipRepository) error {
		if err := tx.LockUsers(ctx, me, targetID); err != nil {
			return err
		}
		if err := tx.EndActiveRelation(ctx, me, targetID, s.now()); err != nil {
			return err
		}
		return s.blocks.WithTx(tx).Create(ctx, me, targetID)
	})
}

// Unblock — DELETE /friends/blocks/:userId (idempotent). Bỏ chặn KHÔNG khôi phục tình bạn.
func (s *FriendshipService) Unblock(ctx context.Context, me, targetID uuid.UUID) error {
	if _, err := s.requireActive(ctx, me); err != nil {
		return err
	}
	return s.blocks.Delete(ctx, me, targetID)
}

// AreFriends — chỉ true khi có dòng ACCEPTED (FriendshipChecker).
func (s *FriendshipService) AreFriends(ctx context.Context, a, b uuid.UUID) (bool, error) {
	row, err := s.repo.FindByPair(ctx, a, b)
	if err != nil || row == nil {
		return false, err
	}
	return row.Status == model.FriendshipStatusAccepted, nil
}

// IsBlockedBy — blocker có đang chặn blocked không (một chiều; FriendshipChecker).
func (s *FriendshipService) IsBlockedBy(ctx context.Context, blocker, blocked uuid.UUID) (bool, error) {
	return s.blocks.IsBlockedBy(ctx, blocker, blocked)
}

// IsBlockedEitherWay — có block ở bất kỳ chiều nào giữa a và b (FriendshipChecker).
func (s *FriendshipService) IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error) {
	return s.blocks.IsBlockedEitherWay(ctx, a, b)
}

var _ FriendshipChecker = (*FriendshipService)(nil)
