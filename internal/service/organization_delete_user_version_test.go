package service

// QA-C-20 (261110), thay cho QA3-02 follow-up: xoá tổ chức KHÔNG được tăng user_version của thành viên. user_version là
// khoá "đăng xuất mọi nơi" (AuthMiddleware và RefreshToken đều từ chối khi lệch), nên tăng nó làm admin vừa xoá tổ chức
// mình là chủ mất mọi phiên. Quyền tổ chức tính từ DB mỗi request nên không cần thu hồi phiên (xem DeleteOrganization).
// Bảo đảm thay thế (token cũ không còn quyền tổ chức): TestQAC20_XoaToChucThiTokenCuMatQuyenToChuc_Postgres.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
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

func TestDeleteOrganization_KhongThuHoiPhienCuaThanhVien(t *testing.T) {
	ctx := context.Background()
	failing := &fakeOrgRepoForDelete{deleteErr: errors.New("db down")}
	if err := NewOrganizationService(failing).DeleteOrganization(ctx, uuid.New(), false); err == nil {
		t.Fatal("lỗi xoá phải được trả về")
	}
	for _, hard := range []bool{false, true} {
		// members giả lập chủ tổ chức (chính admin đang thao tác) và một thành viên khác.
		repo := &fakeOrgRepoForDelete{members: []uuid.UUID{uuid.New(), uuid.New()}}
		if err := NewOrganizationService(repo).DeleteOrganization(ctx, uuid.New(), hard); err != nil || !repo.deleted {
			t.Fatalf("hard=%v: deleted=%v err=%v", hard, repo.deleted, err)
		}
	}
}
