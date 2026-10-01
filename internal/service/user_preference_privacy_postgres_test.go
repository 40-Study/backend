package service

// Lane L2: cài đặt riêng tư bảng xếp hạng. Backend là SSOT của tên field: `leaderboard_display`. Web từng
// gửi `leaderboard_visibility`, backend bỏ qua khoá lạ nên lựa chọn không bao giờ được lưu. Test này chốt
// phía backend: tên khoá trong request/response, và lưu xong tải lại (service mới, cùng DB) vẫn giữ giá trị.

import (
	"encoding/json"
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

func TestL2_PrivacySettings_LeaderboardDisplay_LuuRoiTaiLaiVanGiu(t *testing.T) {
	f := newS2Fixture(t)
	u := f.user("privacy")
	newSvc := func() *UserPreferenceService {
		return NewUserPreferenceService(repository.NewUserPreferenceRepository(f.db))
	}

	// Mặc định khi chưa từng lưu.
	if got, err := newSvc().GetPrivacySettings(u.ID); err != nil || got.LeaderboardDisplay != "name" {
		t.Fatalf("mặc định: %+v err=%v, muốn leaderboard_display=name", got, err)
	}

	for _, want := range []string{"anonymous", "username", "name"} {
		var req dto.UpdatePrivacySettingsDTO
		if err := json.Unmarshal([]byte(`{"leaderboard_display":"`+want+`"}`), &req); err != nil {
			t.Fatal(err)
		}
		if _, err := newSvc().UpdatePrivacySettings(u.ID, req); err != nil {
			t.Fatalf("lưu %s: %v", want, err)
		}
		// "Tải lại": service mới, đọc từ DB.
		got, err := newSvc().GetPrivacySettings(u.ID)
		if err != nil || got.LeaderboardDisplay != want {
			t.Errorf("sau khi lưu %q và tải lại: %+v err=%v", want, got, err)
		}
	}
}

// Khoá `leaderboard_visibility` (tên web dùng trước đây) không phải field của backend: nó không được ghi vào
// `leaderboard_display`. Test này ghim hợp đồng để ai đó "thêm alias" phải chủ động đổi nó.
func TestL2_PrivacySettings_TenFieldLaHopDong(t *testing.T) {
	var req dto.UpdatePrivacySettingsDTO
	if err := json.Unmarshal([]byte(`{"leaderboard_visibility":"anonymous"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.LeaderboardDisplay != nil {
		t.Fatalf("leaderboard_visibility không được map vào LeaderboardDisplay, thấy %q", *req.LeaderboardDisplay)
	}

	raw, err := json.Marshal(dto.PrivacySettingsResponseDTO{ProfileVisibility: "public", ActivityStatus: "everyone", LeaderboardDisplay: "name"})
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]string
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"profile_visibility", "activity_status", "leaderboard_display"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("response thiếu khoá %q: %s", k, raw)
		}
	}
	if _, ok := keys["leaderboard_visibility"]; ok {
		t.Errorf("response không được có khoá leaderboard_visibility: %s", raw)
	}
}
