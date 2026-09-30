// Package data nhúng (go:embed) các file SSOT trong data/ để code chạy đọc CÙNG nguồn với seeder,
// không lặp lại danh sách quyền bằng tay ở nơi khác.
package data

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed permissions/org_owner_permissions.json
var orgOwnerPermissionsJSON []byte

// orgPermissionNames giữ đúng thứ tự trong file JSON; orgPermissionSet để tra O(1).
var (
	orgPermissionNames []string
	orgPermissionSet   map[string]struct{}
)

func init() {
	var entries []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(orgOwnerPermissionsJSON, &entries); err != nil {
		panic(fmt.Sprintf("data: org_owner_permissions.json không hợp lệ: %v", err))
	}
	if len(entries) == 0 {
		// Danh sách rỗng khiến IsOrgPermission luôn false: mọi org role mất hết quyền một cách
		// im lặng. Thà dừng lúc khởi động còn hơn để tổ chức tê liệt.
		panic("data: org_owner_permissions.json rỗng")
	}
	orgPermissionSet = make(map[string]struct{}, len(entries))
	for _, e := range entries {
		orgPermissionNames = append(orgPermissionNames, e.Name)
		orgPermissionSet[e.Name] = struct{}{}
	}
}

// OrgPermissionNames trả bản sao danh sách quyền THUỘC PHẠM VI TỔ CHỨC (SSOT:
// data/permissions/org_owner_permissions.json). Chỉ những quyền này được gán cho một org role và
// chỉ chúng được tính khi gộp quyền của org role vào quyền của người dùng.
func OrgPermissionNames() []string {
	out := make([]string, len(orgPermissionNames))
	copy(out, orgPermissionNames)
	return out
}

// IsOrgPermission cho biết name có thuộc phạm vi tổ chức không. Quyền hệ thống (SYSTEM_*,
// PAYMENTS_MANAGE, USERS_BAN, ROLES_MANAGE_SYSTEM...) và quyền giảng viên (COURSES_CREATE...) trả false.
func IsOrgPermission(name string) bool {
	_, ok := orgPermissionSet[name]
	return ok
}
