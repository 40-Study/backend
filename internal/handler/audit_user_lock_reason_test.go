package handler

// QA2 ADM-N3: dòng nhật ký user.lock/user.unlock có metadata NULL và lý do khoá bị xoá khỏi users.locked_reason khi
// mở khoá, nên lý do không còn ở đâu. Khoá -> metadata {reason}; mở khoá -> metadata {previous_reason,
// previous_locked_at} đọc TRƯỚC khi mở khoá xoá nó. Handler thật + middleware.Audit thật + service giả.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type auditLockSvc struct {
	service.UserAdminServiceInterface
	detail    *dto.AdminUserDetailDTO
	detailErr error
	statusErr error
	detailHit int
}

func (s *auditLockSvc) GetUserDetail(context.Context, uuid.UUID) (*dto.AdminUserDetailDTO, error) {
	s.detailHit++
	return s.detail, s.detailErr
}

func (s *auditLockSvc) UpdateUserStatus(context.Context, uuid.UUID, uuid.UUID, bool, *string) (*dto.AdminUserDetailDTO, error) {
	return &dto.AdminUserDetailDTO{}, s.statusErr
}

func callUserStatus(t *testing.T, svc *auditLockSvc, body string) (*auditWiringSpy, int) {
	t.Helper()
	h := NewUserAdminHandler(svc)
	id := uuid.New().String()
	spy, _, status := auditWiringCall(t, "PUT", "/users/:id/status", "/users/"+id+"/status", body,
		model.AuditActionUserLock, "user", "id", h.UpdateUserStatus)
	return spy, status
}

func TestUserAdminHandler_LockAuditsReason(t *testing.T) {
	spy, status := callUserStatus(t, &auditLockSvc{}, `{"is_active":false,"reason":"QA2 session test"}`)
	if status != 200 || len(spy.entries) != 1 {
		t.Fatalf("status=%d entries=%+v", status, spy.entries)
	}
	e := spy.entries[0]
	if e.Action != model.AuditActionUserLock || e.Metadata["reason"] != "QA2 session test" {
		t.Errorf("entry = %+v, muốn user.lock với metadata.reason = lý do đã nhập", e)
	}
}

func TestUserAdminHandler_UnlockAuditsPreviousReason(t *testing.T) {
	lockedAt := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	reason := "spam"
	svc := &auditLockSvc{detail: &dto.AdminUserDetailDTO{LockedReason: &reason, LockedAt: &lockedAt}}
	spy, status := callUserStatus(t, svc, `{"is_active":true}`)
	if status != 200 || len(spy.entries) != 1 {
		t.Fatalf("status=%d entries=%+v", status, spy.entries)
	}
	e := spy.entries[0]
	if e.Action != model.AuditActionUserUnlock || e.Metadata["previous_reason"] != "spam" || e.Metadata["previous_locked_at"] == nil {
		t.Errorf("entry = %+v, muốn user.unlock với metadata.previous_reason=spam và previous_locked_at", e)
	}
}

// Không đọc được lý do cũ (lỗi DB, hoặc tài khoản chưa từng bị khoá): mở khoá vẫn thành công và KHÔNG bịa lý do.
func TestUserAdminHandler_UnlockWithoutReadableReasonStillAudits(t *testing.T) {
	for name, svc := range map[string]*auditLockSvc{
		"đọc lỗi":        {detailErr: errors.New("db down")},
		"không có lý do": {detail: &dto.AdminUserDetailDTO{}},
	} {
		spy, status := callUserStatus(t, svc, `{"is_active":true}`)
		if status != 200 || len(spy.entries) != 1 || spy.entries[0].Action != model.AuditActionUserUnlock {
			t.Fatalf("%s: status=%d entries=%+v", name, status, spy.entries)
		}
		if _, has := spy.entries[0].Metadata["previous_reason"]; has {
			t.Errorf("%s: metadata = %v, không được có previous_reason", name, spy.entries[0].Metadata)
		}
	}
}

// Khoá thất bại (vd khoá admin cuối cùng) không ghi nhật ký -> không có metadata nào để rò.
func TestUserAdminHandler_FailedLockDoesNotAudit(t *testing.T) {
	spy, status := callUserStatus(t, &auditLockSvc{statusErr: repository.ErrLastSystemAdmin}, `{"is_active":false,"reason":"x"}`)
	if status != 400 || len(spy.entries) != 0 {
		t.Errorf("status=%d entries=%+v, muốn 400 và không có dòng nhật ký", status, spy.entries)
	}
}
