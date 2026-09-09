package middleware

import "testing"

func TestHasAllPermissions(t *testing.T) {
	tests := []struct {
		name     string
		granted  []string
		required []string
		want     bool
	}{
		{
			name:     "co du permission yeu cau",
			granted:  []string{"ORG_CREATE", "COURSES_CREATE"},
			required: []string{"ORG_CREATE"},
			want:     true,
		},
		{
			name:     "thieu permission yeu cau",
			granted:  []string{"COURSES_CREATE"},
			required: []string{"ORG_CREATE"},
			want:     false,
		},
		{
			name:     "wildcard vuot qua moi permission",
			granted:  []string{"*"},
			required: []string{"ROLES_MANAGE_SYSTEM", "ORG_CREATE"},
			want:     true,
		},
		{
			name:     "khong co permission nao",
			granted:  []string{},
			required: []string{"ORG_CREATE"},
			want:     false,
		},
		{
			name:     "khong yeu cau permission nao",
			granted:  []string{"COURSES_CREATE"},
			required: []string{},
			want:     true,
		},
		{
			name:     "yeu cau nhieu permission, co du tat ca",
			granted:  []string{"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE"},
			required: []string{"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE"},
			want:     true,
		},
		{
			name:     "yeu cau nhieu permission, thieu 1",
			granted:  []string{"ORG_ROLES_MANAGE"},
			required: []string{"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAllPermissions(tt.granted, tt.required)
			if got != tt.want {
				t.Errorf("hasAllPermissions(%v, %v) = %v, want %v", tt.granted, tt.required, got, tt.want)
			}
		})
	}
}

func TestHasWildcard(t *testing.T) {
	if !hasWildcard([]string{"ORG_CREATE", "*"}) {
		t.Error("expected wildcard to be detected")
	}
	if hasWildcard([]string{"ORG_CREATE"}) {
		t.Error("expected no wildcard")
	}
	if hasWildcard(nil) {
		t.Error("expected no wildcard on nil slice")
	}
}
