package service

import "testing"

// Q9-PG-08: tiêu đề buổi đã là "Buổi N: ..." thì câu nhắc không được thành "có buổi Buổi 21".
func TestChildReminderContent_KhongLapChuBuoi(t *testing.T) {
	cases := []struct {
		title string
		mins  int
		want  string
	}{
		{"Buổi 21: Pandas: đọc CSV", 0, "Lê Văn C có Buổi 21: Pandas: đọc CSV bắt đầu lúc 19:30 · 26/11."},
		{"Buổi 21: Pandas: đọc CSV", 15, "Lê Văn C có Buổi 21: Pandas: đọc CSV lúc 19:30 · 26/11."},
		{"  buổi 3 ôn tập", 0, "Lê Văn C có buổi 3 ôn tập bắt đầu lúc 19:30 · 26/11."},
		// Tiêu đề không mở đầu bằng "Buổi" vẫn đọc tự nhiên.
		{"Python", 0, "Lê Văn C có buổi Python bắt đầu lúc 19:30 · 26/11."},
		{"Python", 15, "Lê Văn C có buổi Python lúc 19:30 · 26/11."},
	}
	for _, tc := range cases {
		if got := childReminderContent("Lê Văn C", tc.title, "19:30 · 26/11", tc.mins); got != tc.want {
			t.Errorf("title %q mins %d:\n got  %q\n want %q", tc.title, tc.mins, got, tc.want)
		}
	}
}
