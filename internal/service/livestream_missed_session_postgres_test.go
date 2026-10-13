package service

// QA2 F-06: buổi livestream đã QUA giờ kết thúc dự kiến mà chưa từng bắt đầu vẫn nằm ở trạng thái "scheduled",
// nên học viên thấy "SẮP DIỄN RA" cho buổi của 2 ngày trước. Trạng thái được TÍNH khi đọc (cùng cách contest.Phase)
// thay vì thêm job nền: không có độ trễ, không có dữ liệu cũ nếu job chết. Postgres thật (schema tạm).
// Mutation đã kiểm: bỏ EffectiveStatus ở toResponseDTO hoặc bỏ nhánh `missed` trong bộ lọc status làm test ĐỎ.

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type missedSessionFixture struct {
	t   *testing.T
	f   *orderFixture
	svc *LivestreamService
	ids map[string]uuid.UUID
}

func newMissedSessionFixture(t *testing.T) *missedSessionFixture {
	t.Helper()
	f := newOrderFixture(t)
	host := f.user()
	class := model.Class{Name: "Lớp live F-06"}
	if err := f.db.Create(&class).Error; err != nil {
		t.Fatalf("tạo lớp: %v", err)
	}
	now := time.Now()
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	specs := map[string]model.LivestreamSession{
		// Đã qua giờ kết thúc dự kiến, chưa từng bắt đầu -> "đã qua" (ca báo cáo: 2 ngày trước).
		"missed-with-end": {ScheduledAt: at(-50 * time.Hour), ScheduledEndAt: at(-49 * time.Hour)},
		// Không có giờ kết thúc: lịch vẽ mặc định 1 giờ, qua 1 giờ là đã qua.
		"missed-no-end": {ScheduledAt: at(-3 * time.Hour)},
		// Đang trong khung giờ (chưa tới giờ kết thúc) nhưng chưa bấm bắt đầu: vẫn "scheduled" để GV còn vào được.
		"in-window": {ScheduledAt: at(-10 * time.Minute), ScheduledEndAt: at(50 * time.Minute)},
		// Chưa tới giờ.
		"upcoming": {ScheduledAt: at(24 * time.Hour), ScheduledEndAt: at(25 * time.Hour)},
		// Không có lịch: không bao giờ bị coi là đã qua.
		"no-schedule": {},
	}
	ids := map[string]uuid.UUID{}
	for key, s := range specs {
		s.Title, s.HostID, s.ClassID, s.RoomName = key, host, class.ID, uuid.NewString()
		s.Status, s.MaxViewers = model.LivestreamStatusScheduled, 100
		if err := f.db.Create(&s).Error; err != nil {
			t.Fatalf("tạo phiên %s: %v", key, err)
		}
		ids[key] = s.ID
	}
	// Phiên đã kết thúc thật và phiên đang live: không bị ảnh hưởng dù lịch cũ.
	for _, st := range []model.LivestreamSessionStatus{model.LivestreamStatusEnded, model.LivestreamStatusLive, model.LivestreamStatusCancelled} {
		s := model.LivestreamSession{Title: string(st), HostID: host, ClassID: class.ID, RoomName: uuid.NewString(),
			Status: st, MaxViewers: 100, ScheduledAt: at(-72 * time.Hour), ScheduledEndAt: at(-71 * time.Hour)}
		if err := f.db.Create(&s).Error; err != nil {
			t.Fatalf("tạo phiên %s: %v", st, err)
		}
		ids["real-"+string(st)] = s.ID
	}
	svc := NewLivestreamService(repository.NewLivestreamRepository(f.db), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return &missedSessionFixture{t: t, f: f, svc: svc, ids: ids}
}

// statusByKey đọc danh sách như API: key -> trạng thái trả ra, chỉ gồm phiên của fixture.
func (m *missedSessionFixture) statusByKey(filter string) map[string]string {
	m.t.Helper()
	out, err := m.svc.GetAll(context.Background(), uuid.New(), true, 1, 100, filter, nil, nil)
	if err != nil {
		m.t.Fatalf("GetAll(%q): %v", filter, err)
	}
	byID := map[uuid.UUID]string{}
	for k, id := range m.ids {
		byID[id] = k
	}
	got := map[string]string{}
	for _, d := range out.Data {
		got[byID[d.ID]] = d.Status
	}
	return got
}

func TestLivestreamGetAll_PastScheduledSessionNoLongerShownScheduled(t *testing.T) {
	m := newMissedSessionFixture(t)
	got := m.statusByKey("")
	want := map[string]string{
		"missed-with-end": "ended", "missed-no-end": "ended",
		"in-window": "scheduled", "upcoming": "scheduled", "no-schedule": "scheduled",
		"real-ended": "ended", "real-live": "live", "real-cancelled": "cancelled",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: status = %q, muốn %q", k, got[k], w)
		}
	}
}

// Bộ lọc ?status=scheduled (danh sách "Sắp diễn ra") phải khớp với trạng thái trả ra, nếu không buổi đã qua
// vẫn lọt vào nhóm "Sắp diễn ra" của học viên và phân trang/tổng sai.
func TestLivestreamGetAll_StatusFilterAgreesWithDerivedStatus(t *testing.T) {
	m := newMissedSessionFixture(t)
	keysOf := func(filter string) string {
		var keys []string
		for k := range m.statusByKey(filter) {
			keys = append(keys, k)
		}
		return missedSortedKeys(keys)
	}
	if got, want := keysOf("scheduled"), missedSortedKeys([]string{"in-window", "upcoming", "no-schedule"}); got != want {
		t.Errorf("status=scheduled -> %s, muốn %s", got, want)
	}
	if got, want := keysOf("ended"), missedSortedKeys([]string{"missed-with-end", "missed-no-end", "real-ended"}); got != want {
		t.Errorf("status=ended -> %s, muốn %s", got, want)
	}
	if got, want := keysOf("live,scheduled"), missedSortedKeys([]string{"real-live", "in-window", "upcoming", "no-schedule"}); got != want {
		t.Errorf("status=live,scheduled -> %s, muốn %s", got, want)
	}
	if got, want := keysOf("cancelled"), missedSortedKeys([]string{"real-cancelled"}); got != want {
		t.Errorf("status=cancelled -> %s, muốn %s", got, want)
	}
}

// Quy tắc "đã qua" ở Go (trạng thái trả ra) và ở SQL (bộ lọc) là cùng một định nghĩa: kiểm ở mép giờ kết thúc.
func TestLivestreamEffectiveStatus_Boundary(t *testing.T) {
	now := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	ptr := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	cases := []struct {
		name string
		s    model.LivestreamSession
		want model.LivestreamSessionStatus
	}{
		{"kết thúc đúng lúc này", model.LivestreamSession{Status: model.LivestreamStatusScheduled, ScheduledAt: ptr(-time.Hour), ScheduledEndAt: ptr(0)}, model.LivestreamStatusScheduled},
		{"qua 1 giây", model.LivestreamSession{Status: model.LivestreamStatusScheduled, ScheduledAt: ptr(-time.Hour - time.Second), ScheduledEndAt: ptr(-time.Second)}, model.LivestreamStatusEnded},
		{"không có giờ kết thúc, đúng 1 giờ", model.LivestreamSession{Status: model.LivestreamStatusScheduled, ScheduledAt: ptr(-time.Hour)}, model.LivestreamStatusScheduled},
		{"không có giờ kết thúc, qua 1 giờ", model.LivestreamSession{Status: model.LivestreamStatusScheduled, ScheduledAt: ptr(-time.Hour - time.Second)}, model.LivestreamStatusEnded},
		{"không có lịch", model.LivestreamSession{Status: model.LivestreamStatusScheduled}, model.LivestreamStatusScheduled},
		{"live giữ nguyên", model.LivestreamSession{Status: model.LivestreamStatusLive, ScheduledAt: ptr(-72 * time.Hour)}, model.LivestreamStatusLive},
		{"cancelled giữ nguyên", model.LivestreamSession{Status: model.LivestreamStatusCancelled, ScheduledAt: ptr(-72 * time.Hour)}, model.LivestreamStatusCancelled},
	}
	for _, tc := range cases {
		if got := tc.s.EffectiveStatus(now); got != tc.want {
			t.Errorf("%s: EffectiveStatus = %q, muốn %q", tc.name, got, tc.want)
		}
	}
}

func missedSortedKeys(keys []string) string {
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
