package repository

import (
	"testing"

	"github.com/google/uuid"
)

// Test THUẦN — không cần Postgres. Đỏ ngay nếu ai xoá/nới lỏng guard trong
// evaluateLastSystemAdminGuard / evaluateLastActiveRoleGuard.

func TestEvaluateLastSystemAdminGuard_ChanKhoaAdminCuoiCung(t *testing.T) {
	target := uuid.New()
	admins := []uuid.UUID{target}
	status := map[uuid.UUID]bool{target: true}

	err := evaluateLastSystemAdminGuard(admins, status, target)
	if err != ErrLastSystemAdmin {
		t.Fatalf("muon ErrLastSystemAdmin (chi con 1 admin, la target), duoc: %v", err)
	}
}

func TestEvaluateLastSystemAdminGuard_ChoPhepKhiConAdminKhacDangHoatDong(t *testing.T) {
	target := uuid.New()
	other := uuid.New()
	admins := []uuid.UUID{target, other}
	status := map[uuid.UUID]bool{target: true, other: true}

	if err := evaluateLastSystemAdminGuard(admins, status, target); err != nil {
		t.Fatalf("muon cho phep (con admin khac dang hoat dong), loi: %v", err)
	}
}

func TestEvaluateLastSystemAdminGuard_ChanKhiAdminKhacDaBiKhoaSan(t *testing.T) {
	// admin B ton tai nhung is_active=false (da bi khoa tu truoc) -> khong tinh la "dang hoat
	// dong", nen khoa/go admin A (target) van la lam rong he thong.
	target := uuid.New()
	lockedOther := uuid.New()
	admins := []uuid.UUID{target, lockedOther}
	status := map[uuid.UUID]bool{target: true, lockedOther: false}

	err := evaluateLastSystemAdminGuard(admins, status, target)
	if err != ErrLastSystemAdmin {
		t.Fatalf("muon ErrLastSystemAdmin (admin con lai da bi khoa), duoc: %v", err)
	}
}

func TestEvaluateLastActiveRoleGuard_ChanGoVaiTroCuoiCung(t *testing.T) {
	if err := evaluateLastActiveRoleGuard(1); err != ErrLastActiveRoleOfUser {
		t.Fatalf("muon ErrLastActiveRoleOfUser khi chi con 1 vai tro, duoc: %v", err)
	}
}

func TestEvaluateLastActiveRoleGuard_ChoPhepKhiConTuHaiVaiTroTroLen(t *testing.T) {
	if err := evaluateLastActiveRoleGuard(2); err != nil {
		t.Fatalf("muon cho phep khi con 2 vai tro, loi: %v", err)
	}
}
