package service

// Vòng 2 NEW-6: hồ sơ công khai của người ĐÃ chặn mình phải 404 y hệt hồ sơ `hidden` (nếu không, so với search và
// gửi lời mời đều giấu, người bị chặn suy ra được mình bị chặn). Người chặn vẫn xem được hồ sơ người bị chặn.

import (
	"context"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestS6_PublicProfile_NguoiDaChanMinh_Tra404NhuHoSoAn(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	owner, blocked, stranger := f.user("owner"), f.user("blocked"), f.user("stranger")
	svc := NewUserStatsService(repository.NewUserStatsRepository(f.db), repository.NewUserPreferenceRepository(f.db))
	svc.SetFriendshipChecker(NewFriendshipService(repository.NewFriendshipRepository(f.db), repository.NewUserBlockRepository(f.db)))
	if err := f.db.Create(&model.UserBlock{BlockerID: owner.ID, BlockedID: blocked.ID}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetPublicProfile(ctx, owner.ID, &blocked.ID, false); err != ErrPublicProfileNotFound {
		t.Errorf("người bị chặn xem hồ sơ công khai của người chặn: muốn ErrPublicProfileNotFound, nhận %v", err)
	}
	if got, err := svc.GetPublicProfile(ctx, owner.ID, &stranger.ID, false); err != nil || got == nil {
		t.Errorf("người không liên quan vẫn xem được hồ sơ công khai: %v", err)
	}
	if got, err := svc.GetPublicProfile(ctx, blocked.ID, &owner.ID, false); err != nil || got == nil {
		t.Errorf("người chặn vẫn xem được hồ sơ của người bị chặn: %v", err)
	}
}