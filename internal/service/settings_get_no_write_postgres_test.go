package service

// QA vòng 2 F-22: mở /settings làm GET ghi sẵn dòng notification_settings và user_preferences mặc định
// (ghi khi đọc). Test chạy Postgres thật: GET trả mặc định mà KHÔNG tạo dòng, PUT vẫn ghi đúng 1 dòng.
// Đưa lại Upsert vào nhánh "chưa có dòng" của GetSettings/GetPrivacySettings -> các test ĐỎ.

import (
	"testing"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func countRowsFor(t *testing.T, f *s2Fixture, table, userCol string, userID interface{}) int64 {
	t.Helper()
	var n int64
	if err := f.db.Table(table).Where(userCol+" = ?", userID).Count(&n).Error; err != nil {
		t.Fatalf("đếm %s: %v", table, err)
	}
	return n
}

func TestF22_GetNotificationSettings_KhongGhiDong_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	u := f.user("f22-notif")
	svc := NewNotificationService(repository.NewNotificationRepository(f.db), nil)

	got, err := svc.GetSettings(u.ID)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !got.EmailCourseUpdates || got.PushPromotions || !got.PushAchievements {
		t.Errorf("GET phải trả mặc định (email_course_updates=true, push_promotions=false, push_achievements=true), nhận %+v", got)
	}
	if n := countRowsFor(t, f, "notification_settings", "user_id", u.ID); n != 0 {
		t.Fatalf("GET đã ghi %d dòng notification_settings, muốn 0 (đọc không được có tác dụng phụ)", n)
	}

	on := true
	if _, err := svc.UpdateSettings(u.ID, dto.UpdateNotificationSettingsDTO{PushPromotions: &on}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if n := countRowsFor(t, f, "notification_settings", "user_id", u.ID); n != 1 {
		t.Fatalf("sau PUT có %d dòng, muốn 1", n)
	}
	again, err := svc.GetSettings(u.ID)
	if err != nil || !again.PushPromotions {
		t.Errorf("GET sau PUT phải thấy push_promotions=true: %+v err=%v", again, err)
	}
}

func TestF22_GetPrivacySettings_KhongGhiDong_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	u := f.user("f22-priv")
	svc := NewUserPreferenceService(repository.NewUserPreferenceRepository(f.db))

	got, err := svc.GetPrivacySettings(u.ID)
	if err != nil {
		t.Fatalf("GetPrivacySettings: %v", err)
	}
	if got.ProfileVisibility != "public" || got.ActivityStatus != "everyone" || got.LeaderboardDisplay != "name" {
		t.Errorf("GET phải trả mặc định public/everyone/name, nhận %+v", got)
	}
	if n := countRowsFor(t, f, "user_preferences", "user_id", u.ID); n != 0 {
		t.Fatalf("GET đã ghi %d dòng user_preferences, muốn 0", n)
	}

	hidden := "hidden"
	if _, err := svc.UpdatePrivacySettings(u.ID, dto.UpdatePrivacySettingsDTO{ProfileVisibility: &hidden}); err != nil {
		t.Fatalf("UpdatePrivacySettings: %v", err)
	}
	var pref model.UserPreference
	if err := f.db.Where("user_id = ?", u.ID).First(&pref).Error; err != nil || pref.ProfileVisibility != "hidden" {
		t.Errorf("sau PUT phải có dòng profile_visibility=hidden: %+v err=%v", pref, err)
	}
}
