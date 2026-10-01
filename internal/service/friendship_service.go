package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Lỗi nghiệp vụ Bạn bè; handler ánh xạ sang HTTP status + `code` theo contract-api.md §1 (mã lỗi bạn bè).
var (
	ErrFriendSelfRequest       = errors.New("friend: self request")
	ErrFriendRoleNotAllowed    = errors.New("friend: role not allowed")
	ErrFriendRequestNotAllowed = errors.New("friend: request not allowed")
	ErrFriendUserNotFound      = errors.New("friend: user not found")
	ErrFriendRequestNotFound   = errors.New("friend: request not found")
	ErrFriendNotFound          = errors.New("friend: not friends")
	ErrFriendAlreadyFriends    = errors.New("friend: already friends")
	ErrFriendRequestExists     = errors.New("friend: request exists")
	ErrFriendRequestNotPending = errors.New("friend: request not pending")
	ErrFriendRequestCooldown   = errors.New("friend: request cooldown")
	ErrFriendLimitReached      = errors.New("friend: friend limit reached")
	ErrFriendDailyLimit        = errors.New("friend: daily request limit reached")
	ErrFriendPendingLimit      = errors.New("friend: pending request limit reached")
	// Lỗi đầu vào của tìm kiếm/danh sách: handler trả 400 ERR_VALIDATION.
	ErrFriendQueryTooShort    = errors.New("friend: search query too short")
	ErrFriendInvalidDirection = errors.New("friend: invalid direction")
)

// FriendshipChecker — phần Bạn bè mà nơi khác cần (tạo DM, mời vào nhóm, hồ sơ chế độ `friends`).
// Interface hẹp để Conversation/UserStats không phụ thuộc cả FriendshipService. AreFriends CHỈ nhận
// ACCEPTED: PENDING/DECLINED/CANCELLED tuyệt đối không được coi là bạn.
type FriendshipChecker interface {
	AreFriends(ctx context.Context, a, b uuid.UUID) (bool, error)
	IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error)
}

// FriendNotifier — phần NotificationService dùng để báo lời mời/chấp nhận. Lỗi gửi chỉ log.
type FriendNotifier interface {
	SendNotification(req dto.CreateNotificationDTO) error
}

// FriendSendOutcome — kết quả POST /friends/requests. AutoAccepted=true khi đối phương đã gửi cho mình
// từ trước (HTTP 200 ACCEPTED), ngược lại là lời mời mới (HTTP 201 PENDING).
type FriendSendOutcome struct {
	Result       dto.FriendRequestResultDTO
	AutoAccepted bool
}

type FriendshipService struct {
	repo     *repository.FriendshipRepository
	blocks   *repository.UserBlockRepository
	notifier FriendNotifier
	now      func() time.Time
}

func NewFriendshipService(repo *repository.FriendshipRepository, blocks *repository.UserBlockRepository) *FriendshipService {
	return &FriendshipService{repo: repo, blocks: blocks, now: time.Now}
}

// SetNotifier nối bộ gửi thông báo (setter để không đổi chữ ký constructor, giống SetInviteGuard).
func (s *FriendshipService) SetNotifier(n FriendNotifier) { s.notifier = n }

// requireStudent — mọi API Bạn bè chỉ dành cho học viên (Q1). Trả thông tin công khai của chính người gọi
// (dùng làm tên hiển thị trong thông báo).
func (s *FriendshipService) requireStudent(ctx context.Context, userID uuid.UUID) (*repository.FriendUserRow, error) {
	me, err := s.repo.FindEligibleTarget(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	if me == nil {
		return nil, ErrFriendRoleNotAllowed
	}
	return me, nil
}

func toFriendUserDTO(r repository.FriendUserRow) dto.FriendUserDTO {
	return dto.FriendUserDTO{UserID: r.UserID, UserName: r.UserName, FullName: r.FullName, AvatarURL: r.AvatarURL}
}

func friendDisplayName(r repository.FriendUserRow) string {
	if r.FullName != nil && *r.FullName != "" {
		return *r.FullName
	}
	return r.UserName
}

// notify gửi thông báo sau khi transaction đã commit; lỗi (kể cả notifier chưa nối) chỉ log để không làm
// hỏng thao tác chính.
func (s *FriendshipService) notify(userID uuid.UUID, ntype, title, content, refType string, refID uuid.UUID) {
	if s.notifier == nil {
		return
	}
	err := s.notifier.SendNotification(dto.CreateNotificationDTO{
		Title: title, Content: content, NotificationType: ntype,
		ReferenceType: &refType, ReferenceID: &refID, UserIDs: []uuid.UUID{userID},
	})
	if err != nil {
		log.Printf("friendship: gửi thông báo %s cho %s lỗi: %v", ntype, userID, err)
	}
}

// checkFriendCaps — không ai trong cặp được vượt quá FriendMaxFriends bạn.
func checkFriendCaps(ctx context.Context, tx *repository.FriendshipRepository, ids ...uuid.UUID) error {
	for _, id := range ids {
		n, err := tx.CountFriends(ctx, id)
		if err != nil {
			return err
		}
		if n >= constants.FriendMaxFriends {
			return ErrFriendLimitReached
		}
	}
	return nil
}

// checkSendQuota — hạn mức của người GỬI: 20 lời mời/24h (mọi trạng thái) và 30 đang chờ. excludeID là dòng
// sắp bị ghi đè khi gửi lại (không tính hai lần).
func checkSendQuota(ctx context.Context, tx *repository.FriendshipRepository, me uuid.UUID, excludeID uuid.UUID, now time.Time) error {
	sent, err := tx.CountRequestedSince(ctx, me, now.Add(-constants.FriendDailyWindow), excludeID)
	if err != nil {
		return err
	}
	if sent >= constants.FriendDailyRequestLimit {
		return ErrFriendDailyLimit
	}
	pending, err := tx.CountOutgoingPending(ctx, me)
	if err != nil {
		return err
	}
	if pending >= constants.FriendPendingRequestLimit {
		return ErrFriendPendingLimit
	}
	return nil
}

// SendRequest — POST /friends/requests. Mọi kiểm tra + ghi nằm trong một transaction có advisory lock cả
// hai user: hạn mức không bị vượt khi gửi song song, và hai người gửi cho nhau cùng lúc chỉ ra MỘT dòng
// (người đến sau thấy dòng PENDING của người kia và tự chấp nhận). Unique index uq_friendships_pair là
// lớp bảo vệ cuối cùng ở tầng DB.
func (s *FriendshipService) SendRequest(ctx context.Context, me, targetID uuid.UUID) (*FriendSendOutcome, error) {
	meRow, err := s.requireStudent(ctx, me)
	if err != nil {
		return nil, err
	}
	if me == targetID {
		return nil, ErrFriendSelfRequest
	}
	target, err := s.repo.FindEligibleTarget(ctx, targetID, true)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrFriendUserNotFound
	}

	var out FriendSendOutcome
	var rowID uuid.UUID
	err = s.repo.Transaction(ctx, func(tx *repository.FriendshipRepository) error {
		if err := tx.LockUsers(ctx, me, targetID); err != nil {
			return err
		}
		txBlocks := s.blocks.WithTx(tx)
		blockedByMe, err := txBlocks.IsBlockedBy(ctx, me, targetID)
		if err != nil {
			return err
		}
		if blockedByMe {
			// Chính người gọi đã chặn: họ biết, nên báo rõ để bỏ chặn.
			return ErrFriendRequestNotAllowed
		}
		blockedByThem, err := txBlocks.IsBlockedBy(ctx, targetID, me)
		if err != nil {
			return err
		}
		if blockedByThem {
			// Bị chặn: trả y hệt người dùng không tồn tại/ẩn hồ sơ, để không đoán ra mình bị chặn (khớp
			// relationship = NONE và search giấu hai phía).
			return ErrFriendUserNotFound
		}
		existing, err := tx.FindByPair(ctx, me, targetID)
		if err != nil {
			return err
		}
		now := s.now()
		row, autoAccepted, err := s.applySend(ctx, tx, existing, me, targetID, now)
		if err != nil {
			return err
		}
		rowID = row.ID
		out.AutoAccepted = autoAccepted
		out.Result = dto.FriendRequestResultDTO{ID: row.ID, Status: row.Status, User: toFriendUserDTO(*target)}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if out.AutoAccepted {
		s.notify(targetID, model.NotificationTypeFriendAccepted, "Lời mời kết bạn được chấp nhận",
			fmt.Sprintf("%s đã trở thành bạn của bạn.", friendDisplayName(*meRow)), "user", me)
	} else {
		s.notify(targetID, model.NotificationTypeFriendRequest, "Lời mời kết bạn",
			fmt.Sprintf("%s đã gửi cho bạn lời mời kết bạn.", friendDisplayName(*meRow)), "friendship", rowID)
	}
	return &out, nil
}

// applySend quyết định ghi gì dựa trên dòng hiện có của cặp. Trả dòng sau khi ghi và cờ tự chấp nhận.
func (s *FriendshipService) applySend(ctx context.Context, tx *repository.FriendshipRepository, existing *model.Friendship, me, targetID uuid.UUID, now time.Time) (*model.Friendship, bool, error) {
	if existing == nil {
		if err := checkFriendCaps(ctx, tx, me, targetID); err != nil {
			return nil, false, err
		}
		if err := checkSendQuota(ctx, tx, me, uuid.Nil, now); err != nil {
			return nil, false, err
		}
		row := &model.Friendship{RequesterID: me, AddresseeID: targetID, Status: model.FriendshipStatusPending, RequestedAt: now}
		return row, false, tx.Create(ctx, row)
	}

	switch existing.Status {
	case model.FriendshipStatusAccepted:
		return nil, false, ErrFriendAlreadyFriends

	case model.FriendshipStatusPending:
		if existing.RequesterID == me {
			return nil, false, ErrFriendRequestExists
		}
		// Đối phương đã gửi cho mình: gửi lại = đồng ý. Không phải lời mời mới nên không tính hạn mức gửi,
		// nhưng vẫn phải giữ trần số bạn của cả hai.
		if err := checkFriendCaps(ctx, tx, me, targetID); err != nil {
			return nil, false, err
		}
		existing.Status = model.FriendshipStatusAccepted
		existing.RespondedAt = &now
		return existing, true, tx.Save(ctx, existing)

	case model.FriendshipStatusDeclined, model.FriendshipStatusCancelled:
		// Cooldown chỉ ràng buộc chính người đã gửi lời mời cũ. Đối phương (người từ chối) muốn chủ động
		// gửi thì được, không bị cooldown của người kia.
		if existing.RequesterID == me && existing.RespondedAt != nil {
			wait := constants.FriendDeclineCooldown
			if existing.Status == model.FriendshipStatusCancelled {
				wait = constants.FriendCancelCooldown
			}
			if now.Before(existing.RespondedAt.Add(wait)) {
				return nil, false, ErrFriendRequestCooldown
			}
		}
		if err := checkFriendCaps(ctx, tx, me, targetID); err != nil {
			return nil, false, err
		}
		if err := checkSendQuota(ctx, tx, me, existing.ID, now); err != nil {
			return nil, false, err
		}
		// Dùng lại dòng cũ (unique theo cặp) với chiều mới.
		existing.RequesterID, existing.AddresseeID = me, targetID
		existing.Status = model.FriendshipStatusPending
		existing.RequestedAt = now
		existing.RespondedAt = nil
		return existing, false, tx.Save(ctx, existing)
	}
	return nil, false, fmt.Errorf("friendship: trạng thái không xác định %q", existing.Status)
}

// lockRequestPair khoá (advisory, theo thứ tự id) hai người của lời mời với CÙNG khoá mà Gửi/Chấp nhận/Chặn
// dùng, rồi người gọi đọc lại dòng trong transaction. Thiếu bước này thì Huỷ/Từ chối đọc-rồi-ghi đè nhau với
// Chấp nhận (cả hai cùng báo thành công). Dòng không tồn tại: không khoá, để người gọi tự trả NotFound.
func lockRequestPair(ctx context.Context, tx *repository.FriendshipRepository, requestID uuid.UUID) error {
	row, err := tx.GetByID(ctx, requestID)
	if err != nil || row == nil {
		return err
	}
	return tx.LockUsers(ctx, row.RequesterID, row.AddresseeID)
}

// loadRequestForAddressee đọc lời mời cho accept/decline: chỉ người NHẬN thấy dòng (người khác và dòng đã
// thu hồi đều là "không tồn tại", không lộ việc có lời mời).
func (s *FriendshipService) loadRequestForAddressee(ctx context.Context, tx *repository.FriendshipRepository, me, requestID uuid.UUID) (*model.Friendship, error) {
	row, err := tx.GetByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if row == nil || row.AddresseeID != me || row.Status == model.FriendshipStatusCancelled {
		return nil, ErrFriendRequestNotFound
	}
	if row.Status != model.FriendshipStatusPending {
		return nil, ErrFriendRequestNotPending
	}
	return row, nil
}

// AcceptRequest — POST /friends/requests/:id/accept (chỉ người nhận).
func (s *FriendshipService) AcceptRequest(ctx context.Context, me, requestID uuid.UUID) (*dto.FriendRequestResultDTO, error) {
	meRow, err := s.requireStudent(ctx, me)
	if err != nil {
		return nil, err
	}
	first, err := s.repo.GetByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if first == nil || first.AddresseeID != me {
		return nil, ErrFriendRequestNotFound
	}
	requester, err := s.repo.FindEligibleTarget(ctx, first.RequesterID, false)
	if err != nil {
		return nil, err
	}

	var result dto.FriendRequestResultDTO
	err = s.repo.Transaction(ctx, func(tx *repository.FriendshipRepository) error {
		if err := tx.LockUsers(ctx, me, first.RequesterID); err != nil {
			return err
		}
		// Đọc lại trong transaction: trạng thái có thể đã đổi (thu hồi/chặn) sau lần đọc đầu.
		row, err := s.loadRequestForAddressee(ctx, tx, me, requestID)
		if err != nil {
			return err
		}
		if requester == nil {
			return ErrFriendUserNotFound
		}
		if err := checkFriendCaps(ctx, tx, me, row.RequesterID); err != nil {
			return err
		}
		now := s.now()
		row.Status = model.FriendshipStatusAccepted
		row.RespondedAt = &now
		if err := tx.Save(ctx, row); err != nil {
			return err
		}
		result = dto.FriendRequestResultDTO{ID: row.ID, Status: row.Status, User: toFriendUserDTO(*requester)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notify(first.RequesterID, model.NotificationTypeFriendAccepted, "Lời mời kết bạn được chấp nhận",
		fmt.Sprintf("%s đã chấp nhận lời mời kết bạn của bạn.", friendDisplayName(*meRow)), "user", me)
	return &result, nil
}

// DeclineRequest — POST /friends/requests/:id/decline (chỉ người nhận). Giữ dòng DECLINED để tính cooldown.
func (s *FriendshipService) DeclineRequest(ctx context.Context, me, requestID uuid.UUID) (*dto.FriendDeclineResultDTO, error) {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return nil, err
	}
	var result dto.FriendDeclineResultDTO
	err := s.repo.Transaction(ctx, func(tx *repository.FriendshipRepository) error {
		if err := lockRequestPair(ctx, tx, requestID); err != nil {
			return err
		}
		row, err := s.loadRequestForAddressee(ctx, tx, me, requestID)
		if err != nil {
			return err
		}
		now := s.now()
		row.Status = model.FriendshipStatusDeclined
		row.RespondedAt = &now
		if err := tx.Save(ctx, row); err != nil {
			return err
		}
		result = dto.FriendDeclineResultDTO{ID: row.ID, Status: row.Status}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelRequest — DELETE /friends/requests/:id (chỉ người GỬI). Thu hồi = chuyển CANCELLED (xem
// model.FriendshipStatusCancelled), người dùng không còn thấy lời mời nữa.
func (s *FriendshipService) CancelRequest(ctx context.Context, me, requestID uuid.UUID) error {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return err
	}
	return s.repo.Transaction(ctx, func(tx *repository.FriendshipRepository) error {
		if err := lockRequestPair(ctx, tx, requestID); err != nil {
			return err
		}
		row, err := tx.GetByID(ctx, requestID)
		if err != nil {
			return err
		}
		if row == nil || row.RequesterID != me || row.Status == model.FriendshipStatusCancelled {
			return ErrFriendRequestNotFound
		}
		if row.Status != model.FriendshipStatusPending {
			return ErrFriendRequestNotPending
		}
		now := s.now()
		row.Status = model.FriendshipStatusCancelled
		row.RespondedAt = &now
		return tx.Save(ctx, row)
	})
}

// Unfriend — DELETE /friends/:userId. Xoá hẳn dòng ACCEPTED (kết bạn lại được ngay).
func (s *FriendshipService) Unfriend(ctx context.Context, me, otherID uuid.UUID) error {
	if _, err := s.requireStudent(ctx, me); err != nil {
		return err
	}
	n, err := s.repo.DeleteAcceptedPair(ctx, me, otherID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrFriendNotFound
	}
	return nil
}
