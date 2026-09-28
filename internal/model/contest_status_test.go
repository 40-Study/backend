package model

import (
	"testing"
	"time"
)

// Tag `check:` của Contest.Status (dùng khi AutoMigrate tạo bảng mới) phải khớp ContestStatuses
// (SSOT, dùng cho CHECK trên DB cũ) ở CẢ HAI CHIỀU — contract §1.1, test (l).
func TestContestStatusTagMatchesSSOT(t *testing.T) {
	assertSameSet(t, "Contest.Status", checkTagValues(t, Contest{}, "Status"), ContestStatuses)
}

func TestContestPhaseAt(t *testing.T) {
	start := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	fin := end.Add(time.Hour)
	cases := []struct {
		name, status string
		now          time.Time
		finalized    *time.Time
		want         string
	}{
		{"draft giu nguyen status", ContestStatusDraft, start.Add(time.Minute), nil, ContestPhaseDraft},
		{"cho duyet", ContestStatusPendingReview, start.Add(-time.Hour), nil, ContestPhasePendingReview},
		{"bi tu choi", ContestStatusRejected, end.Add(time.Hour), nil, ContestPhaseRejected},
		{"da huy trong gio thi", ContestStatusCancelled, start.Add(time.Minute), nil, ContestPhaseCancelled},
		{"truoc gio bat dau", ContestStatusPublished, start.Add(-time.Second), nil, ContestPhaseUpcoming},
		{"dung gio bat dau", ContestStatusPublished, start, nil, ContestPhaseActive},
		{"truoc gio ket thuc", ContestStatusPublished, end.Add(-time.Second), nil, ContestPhaseActive},
		{"dung gio ket thuc", ContestStatusPublished, end, nil, ContestPhaseEnded},
		{"da chot", ContestStatusPublished, end.Add(2 * time.Hour), &fin, ContestPhaseFinalized},
	}
	for _, tc := range cases {
		got := ContestPhaseAt(tc.status, start, end, tc.finalized, tc.now)
		if got != tc.want {
			t.Errorf("%s: phase=%s, muon %s", tc.name, got, tc.want)
		}
	}
}

// Mọi phase tính ra phải thuộc ContestPhases (web dùng union type sinh từ slice này).
func TestContestPhasesCoverStatuses(t *testing.T) {
	set := map[string]bool{}
	for _, p := range ContestPhases {
		set[p] = true
	}
	for _, s := range ContestStatuses {
		if s != ContestStatusPublished && !set[s] {
			t.Errorf("status %s khong co trong ContestPhases", s)
		}
	}
}
