package main

import "testing"

func TestValidateSeedTargetRejectsProductionByDefault(t *testing.T) {
	if err := validateSeedTarget("prod", "full", false); err == nil {
		t.Fatal("production seed phai bi chan neu khong co xac nhan")
	}
}

func TestValidateSeedTargetAllowsExplicitProductionOverride(t *testing.T) {
	if err := validateSeedTarget("prod", "full", true); err != nil {
		t.Fatalf("override production hop le bi tu choi: %v", err)
	}
}

func TestValidateSeedTargetRejectsUnknownMode(t *testing.T) {
	if err := validateSeedTarget("dev", "typo", false); err == nil {
		t.Fatal("mode khong hop le khong duoc roi vao full seed")
	}
}

func TestValidateSeedTargetAllowsDevelopmentModes(t *testing.T) {
	for _, mode := range []string{"base", "full"} {
		if err := validateSeedTarget("dev", mode, false); err != nil {
			t.Fatalf("mode %s o dev bi tu choi: %v", mode, err)
		}
	}
}
