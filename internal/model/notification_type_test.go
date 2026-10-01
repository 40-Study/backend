package model

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestNotificationTypeTagMatchesSSOT — tag `check:` của Notification.NotificationType phải liệt kê ĐÚNG các
// giá trị trong NotificationTypes. Tag dùng cho DB mới (AutoMigrate), slice dùng cho DB cũ (RunPostMigrations
// đồng bộ constraint): lệch nhau thì DB mới và DB cũ chấp nhận hai tập loại thông báo khác nhau.
func TestNotificationTypeTagMatchesSSOT(t *testing.T) {
	field, ok := reflect.TypeOf(Notification{}).FieldByName("NotificationType")
	if !ok {
		t.Fatal("Notification không còn field NotificationType")
	}
	tag := field.Tag.Get("gorm")
	m := regexp.MustCompile(`check:notification_type IN \(([^)]*)\)`).FindStringSubmatch(tag)
	if m == nil {
		t.Fatalf("không tìm thấy check:notification_type IN (...) trong tag %q", tag)
	}
	var fromTag []string
	for _, part := range strings.Split(m[1], ",") {
		fromTag = append(fromTag, strings.Trim(strings.TrimSpace(part), "'"))
	}

	want := append([]string(nil), NotificationTypes...)
	sort.Strings(fromTag)
	sort.Strings(want)
	if !reflect.DeepEqual(fromTag, want) {
		t.Fatalf("tag check = %v, NotificationTypes = %v — hai nơi phải khớp nhau", fromTag, want)
	}
}

// TestFriendshipStatusesCoverEveryConstant — mọi hằng trạng thái phải nằm trong SSOT dùng sinh CHECK.
func TestFriendshipStatusesCoverEveryConstant(t *testing.T) {
	got := map[string]bool{}
	for _, s := range FriendshipStatuses {
		got[s] = true
	}
	for _, s := range []string{FriendshipStatusPending, FriendshipStatusAccepted, FriendshipStatusDeclined, FriendshipStatusCancelled} {
		if !got[s] {
			t.Errorf("trạng thái %q thiếu trong FriendshipStatuses: ghi dòng với trạng thái này sẽ vi phạm CHECK", s)
		}
	}
}
