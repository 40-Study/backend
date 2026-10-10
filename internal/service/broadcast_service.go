package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

const (
	// broadcastChunkSize: số người nhận mỗi lần gọi SendNotification (một INSERT batch + một lượt đẩy WS).
	// Gửi một lượt cho cả hệ thống sẽ giữ một câu INSERT khổng lồ và khoá request quá lâu.
	broadcastChunkSize = 500
	// broadcastMaxRoles chặn danh sách vai trò phình to (mỗi tên là một tham số của câu IN).
	broadcastMaxRoles        = 50
	broadcastTitleMaxChars   = 255
	broadcastContentMaxChars = 2000
	broadcastDefaultType     = "system"
	broadcastPromotionType   = "promotion"
)

// BroadcastError là lỗi nghiệp vụ mang sẵn HTTP status + mã máy đọc + thông điệp tiếng Việt (contract C3).
type BroadcastError struct {
	Status  int
	Code    string
	Message string
}

func (e *BroadcastError) Error() string { return e.Message }

func broadcastErr(status int, code, msg string) *BroadcastError {
	return &BroadcastError{Status: status, Code: code, Message: msg}
}

var (
	errBroadcastNoRecipients = broadcastErr(http.StatusUnprocessableEntity, "NO_RECIPIENTS", "Không có người nhận nào phù hợp với đối tượng đã chọn.")
	errBroadcastNoRoles      = broadcastErr(http.StatusBadRequest, "VALIDATION_FAILED", "Vui lòng chọn ít nhất một vai trò.")
	errBroadcastTooManyRoles = broadcastErr(http.StatusBadRequest, "VALIDATION_FAILED", "Số vai trò được chọn vượt quá giới hạn cho phép.")
	errBroadcastTitle        = broadcastErr(http.StatusBadRequest, "VALIDATION_FAILED",
		fmt.Sprintf("Tiêu đề không được để trống và tối đa %d ký tự.", broadcastTitleMaxChars))
	errBroadcastContent = broadcastErr(http.StatusBadRequest, "VALIDATION_FAILED",
		fmt.Sprintf("Nội dung không được để trống và tối đa %d ký tự.", broadcastContentMaxChars))
	errBroadcastAudience = broadcastErr(http.StatusBadRequest, "VALIDATION_FAILED", "Đối tượng nhận không hợp lệ.")
	errBroadcastType     = broadcastErr(http.StatusBadRequest, "VALIDATION_FAILED", "Loại thông báo không hợp lệ.")
)

// BroadcastPartialError: đã giao một phần rồi mới lỗi. Thông báo đã gửi không thu hồi được, nên số người đã
// nhận phải đi theo lỗi để quản trị viên biết đừng gửi lại toàn bộ (người đã nhận sẽ nhận lần hai).
type BroadcastPartialError struct {
	Delivered int64
	Cause     error
}

func (e *BroadcastPartialError) Error() string {
	return fmt.Sprintf("broadcast failed after delivering to %d recipients: %v", e.Delivered, e.Cause)
}

func (e *BroadcastPartialError) Unwrap() error { return e.Cause }

// broadcastSender là phần của NotificationService mà broadcast dùng (tách ra để test bằng bản giả).
type broadcastSender interface {
	SendNotification(req dto.CreateNotificationDTO) error
}

type BroadcastServiceInterface interface {
	Preview(ctx context.Context, req dto.BroadcastPreviewRequestDTO) (*dto.BroadcastPreviewDTO, error)
	Send(ctx context.Context, req dto.BroadcastRequestDTO) (*dto.BroadcastResultDTO, error)
}

type BroadcastService struct {
	audience repository.BroadcastAudienceRepositoryInterface
	sender   broadcastSender
}

func NewBroadcastService(audience repository.BroadcastAudienceRepositoryInterface, sender broadcastSender) *BroadcastService {
	return &BroadcastService{audience: audience, sender: sender}
}

// resolveAudience kiểm audience/vai trò và trả danh sách vai trò đã khử trùng (luôn non-nil; rỗng với audience=all).
func (s *BroadcastService) resolveAudience(ctx context.Context, audience string, roles []string) ([]string, error) {
	switch audience {
	case repository.BroadcastAudienceAll:
		return []string{}, nil
	case repository.BroadcastAudienceRoles:
	default:
		return nil, errBroadcastAudience
	}
	names := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		r = strings.TrimSpace(r)
		if _, dup := seen[r]; r != "" && !dup {
			seen[r] = struct{}{}
			names = append(names, r)
		}
	}
	if len(names) == 0 {
		return nil, errBroadcastNoRoles
	}
	if len(names) > broadcastMaxRoles {
		return nil, errBroadcastTooManyRoles
	}
	found, err := s.audience.ExistingRoleNames(ctx, names)
	if err != nil {
		return nil, err
	}
	exists := make(map[string]struct{}, len(found))
	for _, n := range found {
		exists[n] = struct{}{}
	}
	var unknown []string
	for _, n := range names {
		if _, ok := exists[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return nil, broadcastErr(http.StatusBadRequest, "UNKNOWN_ROLE", "Vai trò không tồn tại: "+strings.Join(unknown, ", "))
	}
	return names, nil
}

// resolveBroadcastType chuẩn hoá loại thông báo (bỏ trống = system) và kiểm giá trị hợp lệ.
func resolveBroadcastType(nType string) (string, error) {
	if nType == "" {
		return broadcastDefaultType, nil
	}
	if nType != broadcastDefaultType && nType != broadcastPromotionType {
		return "", errBroadcastType
	}
	return nType, nil
}

// Preview đếm số người sẽ nhận (không gửi gì), áp CÙNG bộ lọc như Send theo loại thông báo.
func (s *BroadcastService) Preview(ctx context.Context, req dto.BroadcastPreviewRequestDTO) (*dto.BroadcastPreviewDTO, error) {
	nType, err := resolveBroadcastType(req.NotificationType)
	if err != nil {
		return nil, err
	}
	roles, err := s.resolveAudience(ctx, req.Audience, req.Roles)
	if err != nil {
		return nil, err
	}
	n, err := s.audience.CountRecipients(ctx, req.Audience, roles, nType == broadcastPromotionType)
	if err != nil {
		return nil, err
	}
	return &dto.BroadcastPreviewDTO{RecipientCount: n}, nil
}

// Send gửi theo từng lô broadcastChunkSize qua NotificationService.SendNotification (lưu DB + đẩy WS). Loại
// "promotion" chỉ tới người đã bật push_promotions (M3); "system" tới mọi tài khoản đang hoạt động.
// Duyệt người nhận bằng keyset nên mỗi người nhận đúng một lần. Lỗi giữa chừng trả *BroadcastPartialError.
func (s *BroadcastService) Send(ctx context.Context, req dto.BroadcastRequestDTO) (*dto.BroadcastResultDTO, error) {
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" || utf8.RuneCountInString(title) > broadcastTitleMaxChars {
		return nil, errBroadcastTitle
	}
	if content == "" || utf8.RuneCountInString(content) > broadcastContentMaxChars {
		return nil, errBroadcastContent
	}
	nType, err := resolveBroadcastType(req.NotificationType)
	if err != nil {
		return nil, err
	}
	roles, err := s.resolveAudience(ctx, req.Audience, req.Roles)
	if err != nil {
		return nil, err
	}

	promotionOnly := nType == broadcastPromotionType
	var delivered int64
	after := uuid.Nil
	for {
		ids, err := s.audience.StreamRecipientIDs(ctx, req.Audience, roles, promotionOnly, after, broadcastChunkSize)
		if err != nil {
			return nil, partialOrPlain(delivered, err)
		}
		if len(ids) == 0 {
			break
		}
		if err := s.sender.SendNotification(dto.CreateNotificationDTO{
			Title: title, Content: content, NotificationType: nType, UserIDs: ids,
		}); err != nil {
			return nil, partialOrPlain(delivered, err)
		}
		delivered += int64(len(ids))
		after = ids[len(ids)-1]
		if len(ids) < broadcastChunkSize {
			break
		}
	}
	if delivered == 0 {
		return nil, errBroadcastNoRecipients
	}
	return &dto.BroadcastResultDTO{RecipientCount: delivered, Audience: req.Audience, Roles: roles, NotificationType: nType}, nil
}

func partialOrPlain(delivered int64, err error) error {
	if delivered == 0 {
		return err
	}
	return &BroadcastPartialError{Delivered: delivered, Cause: err}
}
