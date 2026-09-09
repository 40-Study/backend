package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// TestIsSelfServiceSystemRole kiem tra allowlist tu-cap role, dung chung cho ca SelectRole
// (C-01) lan POST /auth/me/profiles (B-01, review vong 1, da dieu chinh 260909). Cho phep
// STUDENT/PARENT/TEACHER tu tao UserSystemRole (TEACHER duoc mo lai vi backend chua co luong
// duyet ho so giao vien nao khac); ORG_OWNER va SYSTEM_ADMIN phai duoc gan qua route quan tri
// (RequirePermissions ROLES_MANAGE_SYSTEM).
func TestIsSelfServiceSystemRole(t *testing.T) {
	tests := []struct {
		roleName string
		want     bool
	}{
		{"STUDENT", true},
		{"PARENT", true},
		{"TEACHER", true},
		{"ORG_OWNER", false},
		{"SYSTEM_ADMIN", false},
		{"", false},
		{"student", false}, // phan biet hoa/thuong, khop dung ten trong data/roles.json
	}

	for _, tt := range tests {
		t.Run(tt.roleName, func(t *testing.T) {
			if got := isSelfServiceSystemRole(tt.roleName); got != tt.want {
				t.Errorf("isSelfServiceSystemRole(%q) = %v, want %v", tt.roleName, got, tt.want)
			}
		})
	}
}

// fakeSystemRoleRepoForProfile la fake toi thieu cho SystemRoleRepositoryInterface, chi
// override GetSystemRoleByID (duy nhat method CreateProfile can). Embed interface nil de
// cac method con lai panic ngay neu bi goi nham (khong duoc dung trong test nay).
type fakeSystemRoleRepoForProfile struct {
	repository.SystemRoleRepositoryInterface
	roles map[uuid.UUID]*model.SystemRole
}

func (f *fakeSystemRoleRepoForProfile) GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*model.SystemRole, error) {
	return f.roles[id], nil
}

// fakeUserSystemRoleRepoForProfile la fake toi thieu cho UserSystemRoleRepositoryInterface,
// chi override FindByUserAndSystemRole (luon tra nil = chua co profile) va Create (ghi nhan
// da goi, khong that su ghi DB).
type fakeUserSystemRoleRepoForProfile struct {
	repository.UserSystemRoleRepositoryInterface
	created *model.UserSystemRole
}

func (f *fakeUserSystemRoleRepoForProfile) FindByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (*model.UserSystemRole, error) {
	return nil, nil
}

func (f *fakeUserSystemRoleRepoForProfile) Create(ctx context.Context, userSystemRole *model.UserSystemRole) error {
	userSystemRole.ID = uuid.New()
	f.created = userSystemRole
	return nil
}

// TestCreateProfile_SelfServiceAllowlist kiem tra CreateProfile (POST /auth/me/profiles)
// dung chung allowlist voi isSelfServiceSystemRole thay vi protectedRoles cu (B-01).
// Truoc fix: chi chan dung "SYSTEM_ADMIN", nen ORG_OWNER van tu-cap duoc -> leo quyen.
func TestCreateProfile_SelfServiceAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		roleName  string
		wantError bool
	}{
		{"STUDENT tu-cap duoc", "STUDENT", false},
		{"TEACHER tu-cap duoc (moi mo lai)", "TEACHER", false},
		{"PARENT tu-cap duoc", "PARENT", false},
		{"ORG_OWNER KHONG tu-cap duoc (B-01)", "ORG_OWNER", true},
		{"SYSTEM_ADMIN KHONG tu-cap duoc", "SYSTEM_ADMIN", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleID := uuid.New()
			systemRole := &model.SystemRole{Name: tt.roleName}
			systemRole.ID = roleID

			sysRoleRepo := &fakeSystemRoleRepoForProfile{
				roles: map[uuid.UUID]*model.SystemRole{roleID: systemRole},
			}
			userSysRoleRepo := &fakeUserSystemRoleRepoForProfile{}

			s := &AuthService{
				systemRoleRepo:     sysRoleRepo,
				userSystemRoleRepo: userSysRoleRepo,
			}

			profile, err := s.CreateProfile(context.Background(), uuid.New(), dto.CreateProfileRequestDto{
				SystemRoleID: roleID.String(),
			})

			if tt.wantError {
				if err == nil {
					t.Fatalf("CreateProfile(%s) = nil error, want error (role phai bi chan tu-cap)", tt.roleName)
				}
				if userSysRoleRepo.created != nil {
					t.Fatalf("CreateProfile(%s) da tao UserSystemRole du bi chan: %+v", tt.roleName, userSysRoleRepo.created)
				}
			} else {
				if err != nil {
					t.Fatalf("CreateProfile(%s) loi khong mong doi: %v", tt.roleName, err)
				}
				if profile == nil || profile.RoleName != tt.roleName {
					t.Fatalf("CreateProfile(%s) ket qua sai: %+v", tt.roleName, profile)
				}
				if userSysRoleRepo.created == nil {
					t.Fatalf("CreateProfile(%s) khong goi Create tren userSystemRoleRepo", tt.roleName)
				}
			}
		})
	}
}
