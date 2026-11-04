package dto_test

import (
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

func fieldTags(errs []utils.ValidationError) map[string]string {
	m := map[string]string{}
	for _, e := range errs {
		m[e.Field] = e.Tag
	}
	return m
}

func device() dto.DeviceInfoDTO {
	return dto.DeviceInfoDTO{DeviceID: "550e8400-e29b-41d4-a716-446655440000", DeviceName: "Test", OS: "iOS"}
}

func wantTag(t *testing.T, label string, errs []utils.ValidationError, field, tag string) {
	t.Helper()
	if got := fieldTags(errs)[field]; got != tag {
		t.Errorf("%s: field %s tag = %q, muốn %q (errs=%+v)", label, field, got, tag, errs)
	}
}

// P-04: đăng ký, đặt lại và đổi mật khẩu dùng CHUNG một chính sách.
func TestPasswordPolicy_AppliesToRegisterResetAndChange(t *testing.T) {
	const weak = "abcdefgh"
	const strong = "Abcdef1!"

	reg := dto.RegisterRequestDto{Email: "a@b.com", Password: weak, ConfirmPassword: weak, UserName: "user_1"}
	wantTag(t, "register weak", utils.ValidateStruct(reg), "password", "strong_password")
	reg.Password, reg.ConfirmPassword = strong, strong
	if errs := utils.ValidateStruct(reg); len(errs) != 0 {
		t.Errorf("register strong bị từ chối: %+v", errs)
	}

	reset := dto.ResetPasswordRequestDto{Email: "a@b.com", Otp: "123456", NewPassword: weak, ConfirmPassword: weak}
	wantTag(t, "reset weak", utils.ValidateStruct(reset), "new_password", "strong_password")
	reset.NewPassword, reset.ConfirmPassword = strong, strong
	if errs := utils.ValidateStruct(reset); len(errs) != 0 {
		t.Errorf("reset strong bị từ chối: %+v", errs)
	}

	chg := dto.ChangePasswordRequestDto{OldPassword: "oldpass12", NewPassword: weak, ConfirmPassword: weak, DeviceInfo: device()}
	wantTag(t, "change weak", utils.ValidateStruct(chg), "new_password", "strong_password")
	chg.NewPassword, chg.ConfirmPassword = strong, strong
	if errs := utils.ValidateStruct(chg); len(errs) != 0 {
		t.Errorf("change strong bị từ chối: %+v", errs)
	}
}

// Mật khẩu yếu cũ vẫn đăng nhập được: login chỉ kiểm độ dài, không kiểm độ mạnh.
func TestPasswordPolicy_LoginStillAcceptsLegacyWeakPassword(t *testing.T) {
	login := dto.LoginRequestDto{Email: "a@b.com", Password: "abcdefgh", DeviceInfo: device()}
	if errs := utils.ValidateStruct(login); len(errs) != 0 {
		t.Errorf("login với mật khẩu yếu cũ bị từ chối: %+v", errs)
	}
}
