package service

// QA6-A10: dòng nhật ký withdrawal.* cần tên đối tượng (trước đây "Yêu cầu rút tiền: <uuid>" vì bộ tra tên không có loại
// "withdrawal"). Bỏ dòng "withdrawal" khỏi auditTargetNameSQL thì test ĐỎ (tên rỗng).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestQA6_AuditTargetName_Withdrawal(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	svc := NewAuditLogService(repository.NewAuditLogRepository(f.db))
	teacher := f.user("a10-teacher")
	qa5Must(t, f.db.Model(&model.User{}).Where("id = ?", teacher.ID).Update("full_name", "Lê Thị Giảng").Error)
	payout := model.InstructorPayout{InstructorID: teacher.ID, Amount: decimal.NewFromInt(500000)}
	qa5Must(t, f.db.Create(&payout).Error)

	if got, err := svc.AuditTargetName(ctx, "withdrawal", payout.ID.String()); err != nil || got != "Yêu cầu rút tiền của Lê Thị Giảng" {
		t.Errorf("withdrawal: got %q err=%v", got, err)
	}
	if got, err := svc.AuditTargetName(ctx, "withdrawal", uuid.NewString()); err != nil || got != "" {
		t.Errorf("withdrawal không tồn tại: got %q err=%v, muốn rỗng", got, err)
	}
}
