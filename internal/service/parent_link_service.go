package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Luồng "phụ huynh gửi yêu cầu liên kết, con xác nhận" (QA vòng 2 lane E, quyết định Q4).
//
// Email không có tài khoản học sinh (quyết định D8, đảo thiết kế chống dò của PR #81 MAJOR-1): email
// không tồn tại VÀ email của tài khoản không giữ vai STUDENT nhận CÙNG một 404 STUDENT_NOT_FOUND và
// KHÔNG tạo dòng yêu cầu nào — phụ huynh biết ngay mình gõ sai email thay vì chờ mãi một yêu cầu
// không ai trả lời. Đổi lại việc dò email bị chặn bằng hạn mức, không bằng phản hồi giống hệt:
//   - Hạn mức đếm MỌI lần bấm gửi (bảng parent_link_attempts), kể cả lần 404, và được kiểm TRƯỚC
//     khi tra email.
//   - Rate-limit theo IP ở router.
//   - Email không phải học sinh và email không tồn tại trả cùng một phản hồi, nên 404 không cho biết
//     email đó có phải tài khoản (giáo viên, quản trị...) hay không.
//
// Chỉ các lỗi dựa trên điều phụ huynh ĐÃ biết mới khác nhau: đã liên kết với con (thấy trong danh
// sách con), con vừa từ chối/huỷ liên kết (phụ huynh đã thấy "Con đã từ chối" / mất quyền xem).
//
// Chống spam: tối đa ParentLinkDailyLimit lần gửi / phụ huynh / 24 giờ trượt; mỗi email chỉ 1 yêu
// cầu đang chờ (unique index một phần); con từ chối hoặc con huỷ liên kết thì phụ huynh phải chờ
// ParentLinkCooldown mới gửi lại được cho đúng con đó. Thêm rate-limit theo IP ở router.
const (
	ParentLinkDailyLimit = 10
	ParentLinkCooldown   = 7 * 24 * time.Hour
	parentLinkListLimit  = 50
	roleParent           = "PARENT"
	roleStudent          = "STUDENT"
)

// ParentLinkError là lỗi nghiệp vụ mang sẵn HTTP status + mã máy đọc + thông điệp tiếng Việt, để
// handler trả đúng envelope {"message","code"} mà không phải đoán theo chuỗi lỗi.
type ParentLinkError struct {
	Status  int
	Code    string
	Message string
}

func (e *ParentLinkError) Error() string { return e.Message }

func linkErr(status int, code, msg string) *ParentLinkError {
	return &ParentLinkError{Status: status, Code: code, Message: msg}
}

var (
	errLinkParentRoleRequired = linkErr(http.StatusForbidden, "PARENT_ROLE_REQUIRED", "Chỉ tài khoản phụ huynh mới gửi được yêu cầu liên kết.")
	errLinkSelf               = linkErr(http.StatusBadRequest, "LINK_SELF", "Bạn không thể gửi yêu cầu liên kết cho chính mình.")
	errLinkAlreadyActive      = linkErr(http.StatusConflict, "LINK_ALREADY_ACTIVE", "Bạn đã liên kết với học sinh này.")
	errLinkCircular           = linkErr(http.StatusConflict, "LINK_CIRCULAR", "Tài khoản này đang là phụ huynh của bạn, không thể liên kết ngược lại.")
	errLinkStudentNotFound    = linkErr(http.StatusNotFound, "STUDENT_NOT_FOUND",
		"Không tìm thấy tài khoản học sinh với email này. Hãy kiểm tra lại email hoặc nhờ con đăng ký trước.")
	errLinkPendingExists = linkErr(http.StatusConflict, "LINK_REQUEST_PENDING", "Bạn đã gửi yêu cầu tới email này, vui lòng chờ con xác nhận.")
	errLinkDailyLimit    = linkErr(http.StatusTooManyRequests, "LINK_REQUEST_DAILY_LIMIT",
		fmt.Sprintf("Bạn đã gửi tối đa %d yêu cầu liên kết trong 24 giờ. Vui lòng thử lại sau.", ParentLinkDailyLimit))
	errLinkRequestNotFound = linkErr(http.StatusNotFound, "LINK_REQUEST_NOT_FOUND", "Không tìm thấy yêu cầu liên kết.")
	errLinkNotPending      = linkErr(http.StatusConflict, "LINK_REQUEST_NOT_PENDING", "Yêu cầu này đã được xử lý trước đó.")
	errLinkNotFound        = linkErr(http.StatusNotFound, "LINK_NOT_FOUND", "Không tìm thấy liên kết đang hoạt động.")
	errLinkInvalidAction   = linkErr(http.StatusBadRequest, "INVALID_ACTION", "Hành động không hợp lệ, chỉ nhận 'accept' hoặc 'reject'.")
	errLinkParentNoLonger  = linkErr(http.StatusConflict, "PARENT_ROLE_REVOKED", "Tài khoản gửi yêu cầu không còn là tài khoản phụ huynh, không thể xác nhận.")
)

type ParentLinkServiceInterface interface {
	CreateRequest(ctx context.Context, parentID uuid.UUID, req dto.CreateParentLinkRequestDto) (*dto.ParentLinkRequestDto, error)
	ListSent(ctx context.Context, parentID uuid.UUID) ([]dto.ParentLinkRequestDto, error)
	Cancel(ctx context.Context, parentID, requestID uuid.UUID) error
	ListIncoming(ctx context.Context, studentID uuid.UUID) ([]dto.ParentLinkRequestDto, error)
	Respond(ctx context.Context, studentID, requestID uuid.UUID, action string) error
	ListLinkedParents(ctx context.Context, studentID uuid.UUID) ([]dto.LinkedParentDto, error)
	UnlinkByParent(ctx context.Context, parentID, childID uuid.UUID) error
	UnlinkByStudent(ctx context.Context, studentID, parentID uuid.UUID) error
}

type ParentLinkService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewParentLinkService(db *gorm.DB) *ParentLinkService {
	return &ParentLinkService{db: db, now: time.Now}
}

func (s *ParentLinkService) repo(tx *gorm.DB) *repository.ParentLinkRequestRepository {
	return repository.NewParentLinkRequestRepository(tx)
}

func normalizeLinkEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *ParentLinkService) CreateRequest(ctx context.Context, parentID uuid.UUID, in dto.CreateParentLinkRequestDto) (*dto.ParentLinkRequestDto, error) {
	email := normalizeLinkEmail(in.StudentEmail)
	var created *model.ParentLinkRequest
	// bizErr: lỗi nghiệp vụ được trả SAU KHI transaction commit, để lượt gửi (attempt) vẫn được
	// lưu — nếu trả lỗi từ trong Transaction thì attempt bị rollback và lần gửi thất bại lại không
	// bị tính vào hạn mức (đúng lỗ MAJOR-1).
	var bizErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := s.repo(tx)
		if err := r.LockKey(ctx, "parent-link:"+parentID.String()); err != nil {
			return err
		}
		isParent, err := r.HasActiveSystemRole(ctx, parentID, roleParent)
		if err != nil {
			return err
		}
		if !isParent {
			bizErr = errLinkParentRoleRequired
			return nil
		}
		now := s.now()
		attempts, err := r.CountAttemptsSince(ctx, parentID, now.Add(-24*time.Hour))
		if err != nil {
			return err
		}
		if attempts >= ParentLinkDailyLimit {
			bizErr = errLinkDailyLimit
			return nil
		}
		if err := r.CreateAttempt(ctx, parentID, now); err != nil {
			return err
		}

		parent, err := r.FindUserByID(ctx, parentID)
		if err != nil {
			return err
		}
		if parent != nil && normalizeLinkEmail(parent.Email) == email {
			bizErr = errLinkSelf
			return nil
		}
		// Tra tài khoản học sinh TRƯỚC khi xét yêu cầu đang chờ: email không có học sinh luôn là 404,
		// kể cả khi còn một dòng pending rác từ thiết kế cũ (không được trả 409 "chờ con xác nhận").
		studentID, err := r.FindStudentIDByEmailCI(ctx, email)
		if err != nil {
			return err
		}
		if studentID == nil {
			bizErr = errLinkStudentNotFound
			return nil
		}
		if pending, err := r.FindPending(ctx, parentID, email); err != nil {
			return err
		} else if pending != nil {
			bizErr = errLinkPendingExists
			return nil
		}
		if bizErr, err = s.checkKnownRelation(ctx, r, parentID, *studentID, now); err != nil || bizErr != nil {
			return err
		}
		created = &model.ParentLinkRequest{
			ParentUserID:  parentID,
			StudentUserID: studentID,
			StudentEmail:  email,
			Relationship:  in.Relationship,
			Status:        model.ParentLinkRequestStatusPending,
			Message:       in.Message,
		}
		return r.Create(ctx, created)
	})
	if err != nil {
		return nil, mapUniqueViolation(err)
	}
	if bizErr != nil {
		return nil, bizErr
	}
	out := toParentLinkRequestDto(created)
	return &out, nil
}

// checkKnownRelation — các lỗi chỉ xảy ra với email ĐÚNG là học sinh, nhưng đều dựa trên điều
// phụ huynh đã biết (đã liên kết, là con của học sinh này, con vừa từ chối/huỷ liên kết).
func (s *ParentLinkService) checkKnownRelation(ctx context.Context, r *repository.ParentLinkRequestRepository, parentID, studentID uuid.UUID, now time.Time) (error, error) {
	rel, err := r.FindRelationForUpdate(ctx, parentID, studentID)
	if err != nil {
		return nil, err
	}
	if rel != nil && rel.Status == model.ParentStudentStatusActive {
		return errLinkAlreadyActive, nil
	}
	// Review PR #81 MINOR-7: A và B đều giữ cả 2 vai thì không được thành "phụ huynh" của nhau.
	if reverse, err := r.HasActiveRelation(ctx, studentID, parentID); err != nil {
		return nil, err
	} else if reverse {
		return errLinkCircular, nil
	}
	var blockedFrom *time.Time
	if rejectedAt, err := r.LatestRejectedAt(ctx, parentID, studentID); err != nil {
		return nil, err
	} else if rejectedAt != nil {
		blockedFrom = rejectedAt
	}
	// Review PR #81 MINOR-5: con chủ động huỷ liên kết cũng tính thời gian chờ như khi từ chối.
	if rel != nil && rel.Status == model.ParentStudentStatusRevoked && rel.RevokedAt != nil &&
		rel.RevokedBy != nil && *rel.RevokedBy == model.RelationRevokedByStudent &&
		(blockedFrom == nil || rel.RevokedAt.After(*blockedFrom)) {
		blockedFrom = rel.RevokedAt
	}
	if blockedFrom != nil && now.Before(blockedFrom.Add(ParentLinkCooldown)) {
		// 409 (xung đột với trạng thái "con vừa từ chối/huỷ"), không phải 429: web hiện thay mọi
		// 429 bằng thông điệp chung và sẽ mất ngày được gửi lại. Giờ đọc từ DB mang múi giờ DSN
		// (Asia/Ho_Chi_Minh) nên in trực tiếp là giờ Việt Nam.
		retryAt := blockedFrom.Add(ParentLinkCooldown).Format("02/01/2006 15:04")
		return linkErr(http.StatusConflict, "LINK_REQUEST_COOLDOWN",
			"Con đã từ chối hoặc huỷ liên kết gần đây. Bạn có thể gửi lại yêu cầu sau "+retryAt+"."), nil
	}
	return nil, nil
}

// mapUniqueViolation: khoá tư vấn theo phụ huynh đã tuần tự hoá các lần gửi, nên vi phạm
// uq_parent_link_requests_pending_email chỉ còn xảy ra nếu có đường ghi khác bỏ qua khoá. Khi đó
// vẫn trả 409 nghiệp vụ thay vì 500. Bắt cả gorm.ErrDuplicatedKey (API bật TranslateError trong
// postgres.go) và *pgconn.PgError 23505 (kết nối không bật dịch lỗi, vd test).
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &pgErr) && pgErr.Code == "23505") {
		return errLinkPendingExists
	}
	return err
}

func (s *ParentLinkService) ListSent(ctx context.Context, parentID uuid.UUID) ([]dto.ParentLinkRequestDto, error) {
	rows, err := s.repo(s.db).ListByParent(ctx, parentID, parentLinkListLimit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ParentLinkRequestDto, len(rows))
	for i := range rows {
		out[i] = toParentLinkRequestDto(&rows[i])
	}
	return out, nil
}

func (s *ParentLinkService) ListIncoming(ctx context.Context, studentID uuid.UUID) ([]dto.ParentLinkRequestDto, error) {
	r := s.repo(s.db)
	me, err := r.FindUserByID(ctx, studentID)
	if err != nil || me == nil {
		return []dto.ParentLinkRequestDto{}, err
	}
	// Yêu cầu gửi theo email chỉ dành cho tài khoản HỌC SINH có email đó (cùng điều kiện với
	// isAddressee) — giáo viên/phụ huynh trùng email không được thấy.
	if isStudent, err := r.HasActiveSystemRole(ctx, studentID, roleStudent); err != nil || !isStudent {
		return []dto.ParentLinkRequestDto{}, err
	}
	rows, err := r.ListPendingForStudent(ctx, studentID, me.Email)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ParentLinkRequestDto, len(rows))
	for i := range rows {
		out[i] = toParentLinkRequestDto(&rows[i])
	}
	return out, nil
}

// Cancel — phụ huynh rút yêu cầu còn đang chờ. Yêu cầu của người khác trả 404 (không tiết lộ tồn tại).
func (s *ParentLinkService) Cancel(ctx context.Context, parentID, requestID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := s.repo(tx)
		req, err := r.FindByIDForUpdate(ctx, requestID)
		if err != nil {
			return err
		}
		if req == nil || req.ParentUserID != parentID {
			return errLinkRequestNotFound
		}
		if req.Status != model.ParentLinkRequestStatusPending {
			return errLinkNotPending
		}
		return r.UpdateStatus(ctx, req.ID, model.ParentLinkRequestStatusCancelled, s.now())
	})
}
