package service

// QA3-02 (follow-up): xoá tổ chức gỡ vai trò của thành viên, nên token của họ (đang chọn tổ chức đó) phải hết hiệu lực
// như khi thu hồi vai trò: user_version tăng cho đúng các thành viên đang active, tổ chức khác không bị ảnh hưởng.

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type fakeOrgRepoForDelete struct {
	repository.OrganizationRepositoryInterface
	members   []uuid.UUID
	deleteErr error
	deleted   bool
}

func (f *fakeOrgRepoForDelete) GetOrganizationByID(_ context.Context, id uuid.UUID) (*model.Organization, error) {
	return &model.Organization{BaseModel: model.BaseModel{ID: id}}, nil
}
func (f *fakeOrgRepoForDelete) ActiveMemberUserIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return f.members, nil
}
func (f *fakeOrgRepoForDelete) DeleteOrganization(context.Context, uuid.UUID, bool) error {
	f.deleted = f.deleteErr == nil
	return f.deleteErr
}

func TestDeleteOrganization_BumpsUserVersionOfMembers(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	m1, m2, bystander := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{m1, m2, bystander} {
		rdb.Set(ctx, constants.KeyUserVersion(id.String()), int64(1), 0)
	}
	version := func(id uuid.UUID) int64 {
		v, _ := rdb.Get(ctx, constants.KeyUserVersion(id.String())).Int64()
		return v
	}

	// Xoá lỗi: không ai bị đăng xuất.
	failing := &fakeOrgRepoForDelete{members: []uuid.UUID{m1}, deleteErr: errors.New("db down")}
	if err := NewOrganizationService(failing, rdb).DeleteOrganization(ctx, uuid.New(), false); err == nil {
		t.Fatal("lỗi xoá phải được trả về")
	}
	if version(m1) != 1 {
		t.Errorf("xoá thất bại mà user_version của m1 đã đổi thành %d", version(m1))
	}

	repo := &fakeOrgRepoForDelete{members: []uuid.UUID{m1, m2}}
	if err := NewOrganizationService(repo, rdb).DeleteOrganization(ctx, uuid.New(), false); err != nil {
		t.Fatal(err)
	}
	if !repo.deleted || version(m1) != 2 || version(m2) != 2 {
		t.Errorf("deleted=%v m1=%d m2=%d, muốn đã xoá và user_version của cả hai = 2", repo.deleted, version(m1), version(m2))
	}
	if version(bystander) != 1 {
		t.Errorf("người ngoài tổ chức bị đổi user_version: %d", version(bystander))
	}
}
