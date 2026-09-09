package service

import (
	"testing"

	"github.com/google/uuid"
)

// TestRequireOrgMatch (H-01 residual, review vòng 1) — pin lại đúng hợp đồng của
// requireOrgMatch: dùng để kiểm POST/DELETE /users/:user_id/org-roles vì organization_id nằm
// trong body/bản ghi đang thao tác, không phải trên URL, nên router không dùng
// RequireOrgPermission được — kiểm phải làm ở tầng service này.
func TestRequireOrgMatch(t *testing.T) {
	orgA := uuid.New()
	orgB := uuid.New()

	tests := []struct {
		name        string
		targetOrgID uuid.UUID
		activeOrgID *uuid.UUID
		isAdmin     bool
		wantErr     bool
	}{
		{"khop dung active_org_id -> cho phep", orgA, &orgA, false, false},
		{"khac active_org_id -> chan (org A dung quyen sua org B)", orgB, &orgA, false, true},
		{"active_org_id nil (chua chon org) -> chan", orgA, nil, false, true},
		{"isAdmin bo qua kiem tra du khac org", orgB, &orgA, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireOrgMatch(tt.targetOrgID, tt.activeOrgID, tt.isAdmin)
			if tt.wantErr && err == nil {
				t.Fatalf("requireOrgMatch(%v, %v, %v) = nil, want error", tt.targetOrgID, tt.activeOrgID, tt.isAdmin)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("requireOrgMatch(%v, %v, %v) = %v, want nil", tt.targetOrgID, tt.activeOrgID, tt.isAdmin, err)
			}
			if tt.wantErr && err != ErrOrgRoleForbidden {
				t.Fatalf("expected ErrOrgRoleForbidden, got %v", err)
			}
		})
	}
}
