package service

// Test cho QA A4 (261008): GET /system-roles/:id/users chỉ trả user_id nên admin không nhận ra ai
// giữ vai trò. Mỗi phần tử phải kèm `user` {id, user_name, full_name, email, avatar_url}.
//
// Mutation muốn bắt: bỏ bước gắn User trong GetUsersBySystemRole phải làm test ĐỎ.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type repoRoleExists struct {
	repository.SystemRoleRepositoryInterface
}

func (repoRoleExists) GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*model.SystemRole, error) {
	return &model.SystemRole{Name: "TEACHER"}, nil
}

type repoRoleMembers struct {
	repository.UserSystemRoleRepositoryInterface
	rows []model.UserSystemRole
}

func (r repoRoleMembers) FindBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) ([]model.UserSystemRole, int64, error) {
	return r.rows, int64(len(r.rows)), nil
}

func TestGetUsersBySystemRole_KemDinhDanhNguoiGiu(t *testing.T) {
	full, avatar := "Nguyen Van A", "https://cdn.test/a.png"
	withProfile := model.User{UserName: "vana", FullName: &full, Email: "a@demo.com", AvatarURL: &avatar}
	withProfile.ID = uuid.New()
	bare := model.User{UserName: "bare", Email: "b@demo.com"}
	bare.ID = uuid.New()
	rows := []model.UserSystemRole{
		{ID: uuid.New(), UserID: withProfile.ID, Status: "active", User: &withProfile},
		{ID: uuid.New(), UserID: bare.ID, Status: "active", User: &bare},
		{ID: uuid.New(), UserID: uuid.New(), Status: "active"}, // User chưa preload / đã xoá: không được panic
	}
	s := NewUserSystemRoleService(repoRoleMembers{rows: rows}, nil, repoRoleExists{})

	res, err := s.GetUsersBySystemRole(context.Background(), uuid.New(), 1, 20, "active")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res)
	var got struct {
		Items []map[string]any `json:"user_system_roles"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("muốn 3 phần tử, nhận %d", len(got.Items))
	}
	u, _ := got.Items[0]["user"].(map[string]any)
	if u["id"] != withProfile.ID.String() || u["full_name"] != full || u["email"] != "a@demo.com" ||
		u["avatar_url"] != avatar || u["user_name"] != "vana" {
		t.Errorf("phần tử 0: user=%v", u)
	}
	if got.Items[0]["user_id"] != withProfile.ID.String() {
		t.Errorf("user_id cũ phải còn nguyên (additive): %v", got.Items[0]["user_id"])
	}
	u1, _ := got.Items[1]["user"].(map[string]any)
	if _, has := u1["full_name"]; has || u1["email"] != "b@demo.com" {
		t.Errorf("phần tử 1 (không full_name): user=%v", u1)
	}
	if _, has := got.Items[2]["user"]; has {
		t.Errorf("phần tử 2 không có User thì không được có khoá user: %v", got.Items[2]["user"])
	}
}
