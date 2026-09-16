package repository

import "testing"

// TestOrderBySort_MoiGiaTri (Phase 1 §3): "oldest" dich sang ASC, moi gia tri khac — ke ca
// "newest" va CHUOI RONG (client khong gui ?sort) — deu phai dich sang DESC, dung y contract
// "mac dinh newest". Day la logic THAT SU quyet dinh cau ORDER BY, khong phai tham so truyen
// qua service (da kiem o note_service_test.go).
func TestOrderBySort_MoiGiaTri(t *testing.T) {
	cases := []struct {
		sort string
		want string
	}{
		{"oldest", "created_at ASC"},
		{"newest", "created_at DESC"},
		{"", "created_at DESC"},
		{"gia-tri-rac", "created_at DESC"},
	}

	for _, c := range cases {
		if got := orderBySort(c.sort); got != c.want {
			t.Errorf("orderBySort(%q) = %q, muon %q", c.sort, got, c.want)
		}
	}
}
