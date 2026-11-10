package service

import (
	"testing"
	"time"

	"study.com/v1/internal/model"
)

// QA-B-12: lớp "Flutter Mobile K5" end_date 04/11 vẫn hiện "Đang hoạt động" ngày 10/11. is_ended tính lúc đọc.
func TestIsClassEnded_TheoNgayVN(t *testing.T) {
	end := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC) // giá trị cột DATE như driver trả về
	at := func(utc string) time.Time {
		v, err := time.Parse(time.RFC3339, utc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name   string
		status string
		end    *time.Time
		now    time.Time
		want   bool
	}{
		{"đã qua end_date nhiều ngày", "active", &end, at("2026-11-10T03:00:00Z"), true},
		{"đúng ngày cuối (giờ VN) vẫn đang học", "active", &end, at("2026-11-04T16:59:00Z"), false},
		{"00:01 giờ VN ngày sau end_date là đã kết thúc", "active", &end, at("2026-11-04T17:01:00Z"), true},
		{"không có end_date", "active", nil, at("2030-01-01T00:00:00Z"), false},
		{"lớp nháp không gắn cờ", "draft", &end, at("2026-11-10T03:00:00Z"), false},
		{"lớp lưu trữ không gắn cờ", "archived", &end, at("2026-11-10T03:00:00Z"), false},
	}
	for _, c := range cases {
		if got := isClassEnded(&model.Class{Status: c.status, EndDate: c.end}, c.now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
