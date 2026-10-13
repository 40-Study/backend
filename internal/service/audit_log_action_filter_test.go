package service

// QA2 ADM-L7: GET /admin/audit-logs?action=bogus trả 200 với danh sách rỗng trong khi from/to/actor_id sai trả 400
// INVALID_FILTER. Mã hành động hợp lệ là SSOT model.AuditActions (cũng là nguồn của /audit-logs/actions cho ô lọc).

import (
	"errors"
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestAuditService_List_UnknownActionIsInvalidFilter(t *testing.T) {
	repo := &auditFakeRepo{}
	svc := NewAuditLogService(repo)

	for _, action := range []string{"bogus", "user.lock.extra", "USER.LOCK", "user.", "%"} {
		_, err := svc.List(t.Context(), dto.AuditLogFilterDTO{Action: action})
		var bad *InvalidAuditFilterError
		if !errors.As(err, &bad) || bad.Field != "action" {
			t.Errorf("action %q: err = %v, muốn InvalidAuditFilterError{action}", action, err)
		}
	}
}

func TestAuditService_List_KnownOrEmptyActionPasses(t *testing.T) {
	for _, action := range append([]string{"", "  "}, model.AuditActions...) {
		repo := &auditFakeRepo{}
		if _, err := NewAuditLogService(repo).List(t.Context(), dto.AuditLogFilterDTO{Action: action}); err != nil {
			t.Errorf("action %q: lỗi %v, muốn hợp lệ", action, err)
		}
	}
	// Khoảng trắng hai đầu được cắt trước khi so với SSOT, như TargetType/TargetID.
	repo := &auditFakeRepo{}
	if _, err := NewAuditLogService(repo).List(t.Context(), dto.AuditLogFilterDTO{Action: " user.lock "}); err != nil || repo.filter.Action != "user.lock" {
		t.Errorf("action có khoảng trắng: err=%v filter.Action=%q, muốn user.lock", err, repo.filter.Action)
	}
}
