package service

// Test cho QA A1 (261008): DELETE /system-roles/:id xoá được cả STUDENT/TEACHER/SYSTEM_ADMIN.
//
// Mutation muốn bắt:
//   - bỏ kiểm IsBuiltInSystemRole trong DeleteSystemRole phải làm TestDeleteSystemRole_VaiTroDungSan_* ĐỎ;
//   - thêm role vào data/roles.json mà quên thêm vào model.BuiltInSystemRoleNames phải làm
//     TestBuiltInSystemRoleNames_KhopRolesJSON ĐỎ (chống lệch).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type repoSystemRoleDelete struct {
	repository.SystemRoleRepositoryInterface
	role    *model.SystemRole
	deleted []bool // mỗi lần gọi DeleteSystemRole ghi lại cờ hardDelete
}

func (r *repoSystemRoleDelete) GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*model.SystemRole, error) {
	return r.role, nil
}
func (r *repoSystemRoleDelete) DeleteSystemRole(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	r.deleted = append(r.deleted, hardDelete)
	return nil
}

func TestDeleteSystemRole_VaiTroDungSan_BiTuChoiCaSoftLanHard(t *testing.T) {
	for _, name := range model.BuiltInSystemRoleNames {
		for _, hard := range []bool{false, true} {
			repo := &repoSystemRoleDelete{role: &model.SystemRole{Name: name}}
			s := NewSystemRoleService(repo, nil)
			err := s.DeleteSystemRole(context.Background(), uuid.New(), hard)
			if !errors.Is(err, ErrSystemRoleProtected) {
				t.Errorf("xoá %s (hard=%v): muốn ErrSystemRoleProtected, nhận %v", name, hard, err)
			}
			if len(repo.deleted) != 0 {
				t.Errorf("xoá %s (hard=%v): không được chạm repo, nhưng đã gọi DeleteSystemRole", name, hard)
			}
		}
	}
}

func TestDeleteSystemRole_VaiTroTuTao_VanXoaDuoc(t *testing.T) {
	repo := &repoSystemRoleDelete{role: &model.SystemRole{Name: "QA_CUSTOM_ROLE"}}
	s := NewSystemRoleService(repo, nil)
	if err := s.DeleteSystemRole(context.Background(), uuid.New(), false); err != nil {
		t.Fatalf("role tự tạo phải xoá được: %v", err)
	}
	if len(repo.deleted) != 1 {
		t.Errorf("DeleteSystemRole gọi %d lần, muốn 1", len(repo.deleted))
	}
}

func TestBuiltInSystemRoleNames_KhopRolesJSON(t *testing.T) {
	raw, err := os.ReadFile("../../data/roles.json")
	if err != nil {
		t.Fatalf("đọc data/roles.json: %v", err)
	}
	var seeded []struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &seeded); err != nil {
		t.Fatal(err)
	}
	inFile := map[string]bool{}
	for _, r := range seeded {
		inFile[r.Role] = true
		if !model.IsBuiltInSystemRole(r.Role) {
			t.Errorf("role %q có trong data/roles.json nhưng chưa nằm trong model.BuiltInSystemRoleNames", r.Role)
		}
	}
	for _, n := range model.BuiltInSystemRoleNames {
		if !inFile[n] {
			t.Errorf("model.BuiltInSystemRoleNames có %q nhưng data/roles.json không seed role này", n)
		}
	}
}
