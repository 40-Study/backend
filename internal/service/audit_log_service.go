package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

const (
	auditDefaultPageSize = 20
	auditMaxPageSize     = 100
	// Giới hạn độ dài cột (xem model.AuditLog): cắt thay vì để INSERT lỗi và mất cả dòng nhật ký.
	auditMaxTargetTypeLen = 40
	auditMaxTargetIDLen   = 64
	auditMaxIPLen         = 45
)

// auditSensitiveKey: khoá metadata bị loại (không phân biệt hoa thường) — nhật ký không bao giờ
// được chứa mật khẩu/token/bí mật dù handler lỡ đưa vào SetAuditMeta.
var auditSensitiveKey = regexp.MustCompile(`(?i)password|token|secret|authorization`)

// vnZone: ngày thuần YYYY-MM-DD của bộ lọc hiểu theo giờ Việt Nam (UTC+7, không có DST) — cùng
// múi giờ DSN của app (TimeZone=Asia/Ho_Chi_Minh), không phụ thuộc tzdata của máy chạy.
var vnZone = time.FixedZone("ICT", 7*60*60)

// InvalidAuditFilterError: bộ lọc GET /admin/audit-logs sai định dạng -> handler trả 400
// INVALID_FILTER. Field là tên tham số query (tập cố định, an toàn để đưa vào message).
type InvalidAuditFilterError struct{ Field string }

func (e *InvalidAuditFilterError) Error() string { return "invalid audit filter: " + e.Field }

// ErrAuditEntryInvalid: Record nhận entry thiếu actor/action (lỗi lập trình ở nơi gắn middleware).
var ErrAuditEntryInvalid = errors.New("audit entry requires actor and action")

type AuditLogServiceInterface interface {
	Record(ctx context.Context, entry model.AuditEntry) error
	List(ctx context.Context, f dto.AuditLogFilterDTO) (dto.AuditLogListDTO, error)
	Actions() []string
}

type AuditLogService struct {
	repo repository.AuditLogRepositoryInterface
}

func NewAuditLogService(repo repository.AuditLogRepositoryInterface) *AuditLogService {
	return &AuditLogService{repo: repo}
}

// Actions trả bản sao SSOT model.AuditActions (caller sửa không ảnh hưởng SSOT).
func (s *AuditLogService) Actions() []string {
	return append([]string{}, model.AuditActions...)
}

func (s *AuditLogService) Record(ctx context.Context, e model.AuditEntry) error {
	if e.ActorID == uuid.Nil || strings.TrimSpace(e.Action) == "" {
		return ErrAuditEntryInvalid
	}
	row := &model.AuditLog{
		ActorID:    e.ActorID,
		Action:     e.Action,
		TargetType: truncateRunes(e.TargetType, auditMaxTargetTypeLen),
		StatusCode: e.StatusCode,
	}
	if e.TargetID != "" {
		row.TargetID = strPtr(truncateRunes(e.TargetID, auditMaxTargetIDLen))
	}
	if e.IP != "" {
		row.IP = strPtr(truncateRunes(e.IP, auditMaxIPLen))
	}
	if clean := sanitizeAuditMetadata(e.Metadata); len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return fmt.Errorf("audit metadata: %w", err)
		}
		row.Metadata = raw
	}
	return s.repo.Create(ctx, row)
}

func (s *AuditLogService) List(ctx context.Context, q dto.AuditLogFilterDTO) (dto.AuditLogListDTO, error) {
	f, err := parseAuditFilter(q)
	if err != nil {
		return dto.AuditLogListDTO{}, err
	}
	rows, total, err := s.repo.List(ctx, f)
	if err != nil {
		return dto.AuditLogListDTO{}, err
	}
	items := make([]dto.AuditLogItemDTO, 0, len(rows))
	for _, r := range rows {
		item, err := toAuditItemDTO(r)
		if err != nil {
			return dto.AuditLogListDTO{}, err
		}
		items = append(items, item)
	}
	return dto.NewAuditLogListDTO(items, total, f.Page, f.PageSize), nil
}

func parseAuditFilter(q dto.AuditLogFilterDTO) (repository.AuditLogFilter, error) {
	f := repository.AuditLogFilter{
		Page:       q.Page,
		PageSize:   q.PageSize,
		Action:     strings.TrimSpace(q.Action),
		TargetType: strings.TrimSpace(q.TargetType),
		TargetID:   strings.TrimSpace(q.TargetID),
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = auditDefaultPageSize
	}
	if f.PageSize > auditMaxPageSize {
		f.PageSize = auditMaxPageSize
	}
	if raw := strings.TrimSpace(q.ActorID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, &InvalidAuditFilterError{Field: "actor_id"}
		}
		f.ActorID = &id
	}
	var err error
	if f.From, err = parseAuditTime(q.From, false); err != nil {
		return f, &InvalidAuditFilterError{Field: "from"}
	}
	if f.To, err = parseAuditTime(q.To, true); err != nil {
		return f, &InvalidAuditFilterError{Field: "to"}
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return f, &InvalidAuditFilterError{Field: "from"}
	}
	return f, nil
}

// parseAuditTime: RFC3339 hoặc YYYY-MM-DD. Rỗng -> nil. endOfDay chỉ áp cho ngày thuần: "to" =
// hết ngày đó (23:59:59.999999, độ chính xác microsecond của Postgres) để khoảng [from, to] đóng.
func parseAuditTime(raw string, endOfDay bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	d, err := time.ParseInLocation("2006-01-02", raw, vnZone)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		d = d.AddDate(0, 0, 1).Add(-time.Microsecond)
	}
	return &d, nil
}

func toAuditItemDTO(r repository.AuditLogRow) (dto.AuditLogItemDTO, error) {
	item := dto.AuditLogItemDTO{
		ID:         r.ID,
		CreatedAt:  r.CreatedAt,
		Action:     r.Action,
		TargetType: r.TargetType,
		TargetID:   r.TargetID,
		StatusCode: r.StatusCode,
		IP:         r.IP,
	}
	// Actor null khi user không còn trong bảng users (LEFT JOIN không khớp).
	if r.ActorEmail != nil {
		name := ""
		if r.ActorName != nil {
			name = *r.ActorName
		}
		item.Actor = &dto.AuditActorDTO{ID: r.ActorID, Name: name, Email: *r.ActorEmail}
	}
	if len(r.Metadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(r.Metadata, &meta); err != nil {
			return item, fmt.Errorf("audit metadata của %s: %w", r.ID, err)
		}
		item.Metadata = meta
	}
	return item, nil
}

// sanitizeAuditMetadata trả bản sao đã bỏ mọi khoá nhạy cảm, kể cả trong map lồng nhau và trong phần tử của slice.
func sanitizeAuditMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if auditSensitiveKey.MatchString(k) {
			continue
		}
		out[k] = sanitizeAuditValue(v)
	}
	return out
}

// sanitizeAuditValue làm sạch một giá trị metadata: map/slice được duyệt (và sao chép) đệ quy, giá trị vô hướng và
// slice thuần vô hướng ([]string, []int...) giữ nguyên.
func sanitizeAuditValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return sanitizeAuditMetadata(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = sanitizeAuditValue(e)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(t))
		for i, e := range t {
			out[i] = sanitizeAuditMetadata(e)
		}
		return out
	default:
		return v
	}
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
