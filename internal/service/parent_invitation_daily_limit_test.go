package service

// Q10 (#159): hết hạn mức lời mời phụ huynh trong ngày -> lỗi đã biết 429 ERR_INVITATION_DAILY_LIMIT (câu tiếng Việt giữ
// nguyên) thay vì phản hồi {Status:"error"} mà handler đổi thành 400 chỉ có câu chữ. Redis giả (miniredis), repo nil: nhánh
// hạn mức phải trả TRƯỚC khi chạm DB. Quay lại trả resp "error" -> test ĐỎ.

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
)

func TestInviteParent_HetHanMucTrongNgay_429ERR_INVITATION_DAILY_LIMIT(t *testing.T) {
	const limit = 3
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	studentID := uuid.New()
	if err := mr.Set(constants.KeyParentInviteRateLimit(studentID), strconv.Itoa(limit)); err != nil {
		t.Fatal(err)
	}

	svc := NewParentInvitationService(&config.Config{ParentInvitationDailyLimit: limit}, rdb, nil, nil, nil, nil)
	resp, err := svc.InviteParent(context.Background(), studentID, dto.InviteParentRequestDto{Email: "ph@40study.test"})
	if resp != nil || !errors.Is(err, ErrParentInvitationDailyLimit) {
		t.Fatalf("resp=%v err=%v, muốn ErrParentInvitationDailyLimit", resp, err)
	}
	var known *apperr.KnownError
	if !errors.As(err, &known) || known.Status != 429 || known.Code != "ERR_INVITATION_DAILY_LIMIT" ||
		known.Message != "Bạn đã đạt giới hạn gửi lời mời phụ huynh trong ngày. Vui lòng thử lại sau 24 giờ." {
		t.Fatalf("err=%#v, muốn KnownError 429 ERR_INVITATION_DAILY_LIMIT với câu tiếng Việt cũ", err)
	}
}
