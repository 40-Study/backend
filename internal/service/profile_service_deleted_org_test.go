package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type fakeUserOrgRoleRepoForProfile struct {
	repository.UserOrganizationRoleRepositoryInterface
	rows []model.UserOrganizationRole
}

func (f *fakeUserOrgRoleRepoForProfile) FindByUserIDWithDetails(context.Context, uuid.UUID, string) ([]model.UserOrganizationRole, error) {
	return f.rows, nil
}

// QA3-02: dòng gán trỏ vào tổ chức đã xoá mềm có Organization = nil; GetOrganizations từng deref nil và panic (500).
// Tuyến phòng thủ thứ hai sau bộ lọc ở repository: dữ liệu mồ côi lọt tới đây thì bị bỏ qua, không được panic.
func TestProfileService_GetOrganizations_SkipsRowsWithoutOrganization(t *testing.T) {
	orgID := uuid.New()
	svc := NewProfileService(nil, &fakeUserOrgRoleRepoForProfile{rows: []model.UserOrganizationRole{
		{OrganizationID: uuid.New(), Role: &model.Role{Name: "ORG_OWNER"}, Organization: nil},
		{OrganizationID: orgID, Role: &model.Role{Name: "ORG_OWNER"}, Organization: &model.Organization{BaseModel: model.BaseModel{ID: orgID}, Name: "Còn sống", Status: "active"}},
		{OrganizationID: orgID, Role: nil, Organization: &model.Organization{BaseModel: model.BaseModel{ID: uuid.New()}, Name: "Role mất"}},
	}})

	got, err := svc.GetOrganizations(context.Background(), uuid.New(), 1, 20)
	if err != nil {
		t.Fatalf("GetOrganizations: %v", err)
	}
	if got.Total != 1 || len(got.Organizations) != 1 || got.Organizations[0].Name != "Còn sống" {
		t.Errorf("muốn đúng 1 tổ chức 'Còn sống', nhận %+v", got)
	}
}
