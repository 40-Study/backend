package service

// Lane W2-A (Postgres thật, schema tạm): xoá / đánh dấu đã đọc thông báo của NGƯỜI KHÁC hoặc id không có phải trả
// ErrNotificationNotFound (handler 404) và KHÔNG đổi dữ liệu; thông báo của chính mình vẫn xoá/đọc được. Bỏ kiểm
// RowsAffected ở NotificationRepository thì ca "người khác" ĐỎ (trả nil = handler 200 giả).

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestW2A_Notification_XoaCuaNguoiKhac404_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	owner, other := f.user("notif-owner"), f.user("notif-other")
	n := model.Notification{UserID: owner.ID, Title: "t", Content: "c", NotificationType: "system"}
	if err := f.db.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewNotificationService(repository.NewNotificationRepository(f.db), nil)
	stillThere := func() (exists, read bool) {
		var got model.Notification
		if err := f.db.First(&got, "id = ?", n.ID).Error; err != nil {
			return false, false
		}
		return true, got.IsRead
	}

	if err := svc.DeleteNotification(n.ID, other.ID); !errors.Is(err, ErrNotificationNotFound) {
		t.Errorf("xoá thông báo của người khác: err=%v, muốn ErrNotificationNotFound", err)
	}
	if err := svc.MarkAsRead(n.ID, other.ID); !errors.Is(err, ErrNotificationNotFound) {
		t.Errorf("đọc thông báo của người khác: err=%v, muốn ErrNotificationNotFound", err)
	}
	if err := svc.DeleteNotification(uuid.New(), owner.ID); !errors.Is(err, ErrNotificationNotFound) {
		t.Errorf("xoá id không có: err=%v, muốn ErrNotificationNotFound", err)
	}
	if exists, read := stillThere(); !exists || read {
		t.Fatalf("thao tác của người khác đã đổi dữ liệu: exists=%v read=%v", exists, read)
	}

	if err := svc.MarkAsRead(n.ID, owner.ID); err != nil {
		t.Errorf("chủ đọc thông báo của mình: %v", err)
	}
	if err := svc.DeleteNotification(n.ID, owner.ID); err != nil {
		t.Errorf("chủ xoá thông báo của mình: %v", err)
	}
	if exists, _ := stillThere(); exists {
		t.Error("chủ xoá nhưng thông báo còn")
	}
}
