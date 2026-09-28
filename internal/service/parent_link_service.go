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
// Chống spam gồm 3 lớp, đều kiểm trong 1 transaction đã khoá theo phụ huynh:
//  1. Mỗi cặp phụ huynh-học sinh chỉ 1 yêu cầu đang chờ (thêm unique index một phần ở DB).
//  2. Tối đa ParentLinkDailyLimit yêu cầu / phụ huynh / 24 giờ trượt.
//  3. Học sinh đã từ chối thì phụ huynh phải chờ ParentLinkRejectCooldown mới gửi lại được cho
//     đúng học sinh đó — không có lớp này, từ chối chẳng có tác dụng gì với người cố tình làm phiền.
const (
	ParentLinkDailyLimit     = 5
	ParentLinkRejectCooldown = 7 * 24 * time.Hour
	parentLinkListLimit      = 50
	roleParent               = "PARENT"
	roleStudent              = "STUDENT"
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
	// Cùng một thông điệp cho "không có email" và "có nhưng không phải học sinh": không để form
	// này thành công cụ dò xem một email có tài khoản loại nào.
	errLinkStudentNotFound = linkErr(http.StatusNotFound, "STUDENT_NOT_FOUND", "Không tìm thấy tài khoản học sinh với email này.")
	errLinkSelf            = linkErr(http.StatusBadRequest, "LINK_SELF", "Bạn không thể gửi yêu cầu liên kết cho chính mình.")
	errLinkAlreadyActive   = linkErr(http.StatusConflict, "LINK_ALREADY_ACTIVE", "Bạn đã liên kết với học sinh này.")
	errLinkPendingExists   = linkErr(http.StatusConflict, "LINK_REQUEST_PENDING", "Bạn đã gửi yêu cầu cho học sinh này, vui lòng chờ con xác nhận.")
	errLinkDailyLimit      = linkErr(http.StatusTooManyRequests, "LINK_REQUEST_DAILY_LIMIT",
		fmt.Sprintf("Bạn đã gửi tối đa %d yêu cầu liên kết trong 24 giờ. Vui lòng thử lại sau.", ParentLinkDailyLimit))
	errLinkRequestNotFound = linkErr(http.StatusNotFound, "LINK_REQUEST_NOT_FOUND", "Không tìm thấy yêu cầu liên kết.")
	errLinkNotPending      = linkErr(http.StatusConflict, "LINK_REQUEST_NOT_PENDING", "Yêu cầu này đã được xử lý trước đó.")
	errLinkNotFound        = linkErr(http.StatusNotFound, "LINK_NOT_FOUND", "Không tìm thấy liên kết đang hoạt động.")
	errLinkInvalidAction   = linkErr(http.StatusBadRequest, "INVALID_ACTION", "Hành động không hợp lệ, chỉ nhận 'accept' hoặc 'reject'.")
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

func (s *ParentLinkService) CreateRequest(ctx context.Context, parentID uuid.UUID, in dto.CreateParentLinkRequestDto) (*dto.ParentLinkRequestDto, error) {
	email := strings.TrimSpace(in.StudentEmail)
	var created *model.ParentLinkRequest
	var student *model.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := s.repo(tx)
		if err := r.LockParent(ctx, parentID); err != nil {
			return err
		}
		isParent, err := r.HasActiveSystemRole(ctx, parentID, roleParent)
		if err != nil {
			return err
		}
		if !isParent {
			return errLinkParentRoleRequired
		}
		if student, err = r.FindUserByEmailCI(ctx, email); err != nil {
			return err
		}
		if student == nil {
			return errLinkStudentNotFound
		}
		if student.ID == parentID {
			return errLinkSelf
		}
		isStudent, err := r.HasActiveSystemRole(ctx, student.ID, roleStudent)
		if err != nil {
			return err
		}
		if !isStudent {
			return errLinkStudentNotFound
		}
		rel, err := r.FindRelationForUpdate(ctx, parentID, student.ID)
		if err != nil {
			return err
		}
		if rel != nil && rel.Status == model.ParentStudentStatusActive {
			return errLinkAlreadyActive
		}
		if pending, err := r.FindPending(ctx, parentID, student.ID); err != nil {
			return err
		} else if pending != nil {
			return errLinkPendingExists
		}
		now := s.now()
		if rejectedAt, err := r.LatestRejectedAt(ctx, parentID, student.ID); err != nil {
			return err
		} else if rejectedAt != nil && now.Before(rejectedAt.Add(ParentLinkRejectCooldown)) {
			// Giờ đọc từ DB mang múi giờ DSN (Asia/Ho_Chi_Minh) nên in trực tiếp là giờ Việt Nam.
			retryAt := rejectedAt.Add(ParentLinkRejectCooldown).Format("02/01/2006 15:04")
			// 409 (xung đột với trạng thái "vừa bị từ chối"), không phải 429: đây là luật nghiệp vụ
			// theo cặp, và web hiện thay mọi 429 bằng thông điệp chung, sẽ mất ngày gửi lại được.
			return linkErr(http.StatusConflict, "LINK_REQUEST_COOLDOWN",
				"Học sinh đã từ chối yêu cầu trước của bạn. Bạn có thể gửi lại sau "+retryAt+".")
		}
		sent, err := r.CountCreatedSince(ctx, parentID, now.Add(-24*time.Hour))
		if err != nil {
			return err
		}
		if sent >= ParentLinkDailyLimit {
			return errLinkDailyLimit
		}
		created = &model.ParentLinkRequest{
			ParentUserID:  parentID,
			StudentUserID: student.ID,
			Relationship:  in.Relationship,
			Status:        model.ParentLinkRequestStatusPending,
			Message:       in.Message,
		}
		return r.Create(ctx, created)
	})
	if err != nil {
		return nil, mapUniqueViolation(err)
	}
	created.Student = student
	out := toParentLinkRequestDto(created)
	return &out, nil
}

// mapUniqueViolation: khoá tư vấn theo phụ huynh đã tuần tự hoá các lần gửi, nên vi phạm
// uq_parent_link_requests_pending chỉ còn xảy ra nếu có đường ghi khác bỏ qua khoá. Khi đó vẫn trả
// 409 nghiệp vụ thay vì 500. Bắt cả 2 dạng: gorm.ErrDuplicatedKey (API bật TranslateError trong
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
	rows, err := s.repo(s.db).ListPendingForStudent(ctx, studentID)
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
