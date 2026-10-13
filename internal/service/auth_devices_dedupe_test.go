package service

// QA vòng 2 PG-L1: /settings/devices hiện 8+ dòng "Chrome Browser" y hệt nhau. Phiên lưu theo device_id do
// CLIENT sinh (auth:session:{user} là HASH device_id -> JSON); mỗi trình duyệt/profile mới (localStorage trống)
// là một device_id mới, không có TTL theo từng dòng nên dồn lại mãi. Test chạy GetAllDevices THẬT (miniredis):
// phiên trùng dấu vân tay (device_name + user_agent) gộp thành 1 dòng — dòng mới nhất, hoặc chính thiết bị
// hiện tại nếu nằm trong nhóm —, thiết bị khác UA giữ riêng, thứ tự ổn định (hiện tại trước, rồi mới nhất trước).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
)

type devSeed struct {
	id        uuid.UUID
	name, ua  string
	loggedAgo time.Duration
}

func seedDeviceSessions(t *testing.T, rdb *redis.Client, userID uuid.UUID, seeds []devSeed) {
	t.Helper()
	for _, d := range seeds {
		payload, _ := json.Marshal(map[string]interface{}{
			"device_id":    d.id,
			"device_name":  d.name,
			"user_agent":   d.ua,
			"logged_in_at": time.Now().Add(-d.loggedAgo).Format(time.RFC3339),
		})
		if err := rdb.HSet(context.Background(), constants.KeySession(userID.String()), d.id.String(), payload).Err(); err != nil {
			t.Fatalf("seed phiên: %v", err)
		}
	}
}

func newDevicesTestService(t *testing.T) (*AuthService, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewAuthService(nil, nil, nil, nil, nil, nil, nil, rdb), rdb
}

const chromeMac = "Mozilla/5.0 (Macintosh) Chrome/131"

func TestGetAllDevices_GopPhienTrungDauVanTay(t *testing.T) {
	svc, rdb := newDevicesTestService(t)
	user := uuid.New()
	newest, mid, oldest, phone := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	seedDeviceSessions(t, rdb, user, []devSeed{
		{oldest, "Chrome Browser", chromeMac, 72 * time.Hour},
		{mid, "Chrome Browser", chromeMac, 24 * time.Hour},
		{newest, "Chrome Browser", chromeMac, time.Hour},
		{phone, "Mobile Browser", "Mozilla/5.0 (iPhone) Safari", 48 * time.Hour},
	})

	got, err := svc.GetAllDevices(context.Background(), user, uuid.New())
	if err != nil {
		t.Fatalf("GetAllDevices: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("có %d dòng, muốn 2 (3 phiên Chrome trùng gộp thành 1 + 1 điện thoại): %+v", len(got), got)
	}
	if got[0].DeviceID != newest.String() || got[1].DeviceID != phone.String() {
		t.Errorf("thứ tự/đại diện sai: [%s, %s], muốn [mới nhất %s, điện thoại %s]", got[0].DeviceID, got[1].DeviceID, newest, phone)
	}
}

func TestGetAllDevices_GopGiuThietBiHienTai(t *testing.T) {
	svc, rdb := newDevicesTestService(t)
	user := uuid.New()
	current, newer := uuid.New(), uuid.New()
	seedDeviceSessions(t, rdb, user, []devSeed{
		{current, "Chrome Browser", chromeMac, 24 * time.Hour},
		{newer, "Chrome Browser", chromeMac, time.Hour},
	})

	got, err := svc.GetAllDevices(context.Background(), user, current)
	if err != nil {
		t.Fatalf("GetAllDevices: %v", err)
	}
	if len(got) != 1 || got[0].DeviceID != current.String() || !got[0].IsCurrent {
		t.Fatalf("nhóm chứa thiết bị hiện tại phải đại diện bằng chính nó và is_current=true: %+v", got)
	}
	if at, err := time.Parse(time.RFC3339, got[0].LoggedInAt); err != nil || time.Since(at) > 2*time.Hour {
		t.Errorf("giờ hiển thị phải là phiên MỚI NHẤT của nhóm (~1h trước), nhận %q", got[0].LoggedInAt)
	}
}

func TestGetAllDevices_KhongGopKhiThieuUserAgent(t *testing.T) {
	svc, rdb := newDevicesTestService(t)
	user := uuid.New()
	// App di động không gửi user_agent: hai máy cùng kiểu máy là hai thiết bị thật, không được gộp.
	seedDeviceSessions(t, rdb, user, []devSeed{
		{uuid.New(), "iPhone 14 Pro", "", time.Hour},
		{uuid.New(), "iPhone 14 Pro", "", 2 * time.Hour},
	})

	got, err := svc.GetAllDevices(context.Background(), user, uuid.New())
	if err != nil {
		t.Fatalf("GetAllDevices: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("có %d dòng, muốn 2 (không UA thì không gộp): %+v", len(got), got)
	}
}
