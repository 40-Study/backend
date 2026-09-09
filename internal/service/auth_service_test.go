package service

import "testing"

// TestIsSelfServiceSystemRole kiem tra allowlist tu-cap role trong SelectRole (C-01).
// Chi STUDENT/PARENT duoc tu tao UserSystemRole; moi role khac (ke ca TEACHER, va dac biet
// la SYSTEM_ADMIN) phai duoc gan qua route quan tri (RequirePermissions ROLES_MANAGE_SYSTEM).
func TestIsSelfServiceSystemRole(t *testing.T) {
	tests := []struct {
		roleName string
		want     bool
	}{
		{"STUDENT", true},
		{"PARENT", true},
		{"TEACHER", false},
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
