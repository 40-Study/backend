package main

import (
	"strings"
	"testing"

	"study.com/v1/internal/database/seeds"
)

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

func TestSeedDoneMessageDoesNotPrintDemoPassword(t *testing.T) {
	if strings.Contains(seedDoneMessage, seeds.DemoPassword) {
		t.Fatal("dòng log cuối của seed không được in mật khẩu demo")
	}
}

func TestDemoTimestampRepairOnlyInDevAndTest(t *testing.T) {
	cases := []struct {
		env       string
		allowProd bool
		want      bool
	}{
		{"dev", false, true}, {"test", false, true}, {" DEV ", false, true},
		{"dev", true, false}, {"prod", false, false}, {"prod", true, false}, {"staging", false, false}, {"", false, false},
	}
	for _, c := range cases {
		if got := demoTimestampRepairAllowed(c.env, c.allowProd); got != c.want {
			t.Errorf("env=%q allowProduction=%v: %v, muốn %v", c.env, c.allowProd, got, c.want)
		}
	}
}
