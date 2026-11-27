package service

// #158 L3: PARENT_INVITATION_DAILY_LIMIT=0 (đặt tường minh) -> InviteParent trả ErrParentInvitationDisabled NGAY, không chạm
// Redis/DB (service dựng với redis, repo nil: chạm vào là panic). Bỏ nhánh `== 0` -> test ĐỎ (panic / không phải lỗi này).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
)

func TestInviteParent_HanMucKhong_TinhNangTat403(t *testing.T) {
	svc := NewParentInvitationService(&config.Config{ParentInvitationDailyLimit: 0}, nil, nil, nil, nil, nil)
	resp, err := svc.InviteParent(context.Background(), uuid.New(), dto.InviteParentRequestDto{Email: "ph@40study.test"})
	if resp != nil || !errors.Is(err, ErrParentInvitationDisabled) {
		t.Fatalf("resp=%v err=%v, muốn ErrParentInvitationDisabled", resp, err)
	}
	var known *apperr.KnownError
	if !errors.As(err, &known) || known.Status != 403 || known.Code != "ERR_FEATURE_DISABLED" {
		t.Fatalf("err=%#v, muốn KnownError 403 ERR_FEATURE_DISABLED", err)
	}
}
