package seeds

import (
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

func TestIsDemoOwnedUserRejectsCollidingRealAccount(t *testing.T) {
	hash, err := utils.HashPassword("real-account-password")
	if err != nil {
		t.Fatal(err)
	}
	spec := demoUserSpec{Email: "admin@demo.com", UserName: "admin"}
	user := model.User{Email: spec.Email, UserName: spec.UserName, PasswordHash: hash}

	if isDemoOwnedUser(user, spec) {
		t.Fatal("email collision must not be treated as a seeder-owned account")
	}
}

func TestIsDemoOwnedUserAllowsExistingDemoAccount(t *testing.T) {
	hash, err := utils.HashPassword(DemoPassword)
	if err != nil {
		t.Fatal(err)
	}
	spec := demoUserSpec{Email: "admin@demo.com", UserName: "admin"}
	user := model.User{Email: spec.Email, UserName: spec.UserName, PasswordHash: hash}

	if !isDemoOwnedUser(user, spec) {
		t.Fatal("existing demo account should remain idempotent")
	}
}
