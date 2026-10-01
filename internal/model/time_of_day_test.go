package model

import (
	"testing"
	"time"
)

func TestParseTimeOfDay(t *testing.T) {
	ok := map[string]TimeOfDay{"14:00": "14:00", "08:30:00": "08:30", " 7:05 ": "07:05", "23:59:59": "23:59"}
	for in, want := range ok {
		got, err := ParseTimeOfDay(in)
		if err != nil || got != want {
			t.Errorf("ParseTimeOfDay(%q) = %q, %v; muốn %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "24:00", "14h", "2026-09-02T19:00:00+07:00", "2026-01-01T08:00:00Z"} {
		if got, err := ParseTimeOfDay(in); err == nil {
			t.Errorf("ParseTimeOfDay(%q) = %q, muốn lỗi", in, got)
		}
	}
}

func TestTimeOfDayValueRejectsInvalid(t *testing.T) {
	if _, err := TimeOfDay("2026-01-01T08:00:00Z").Value(); err == nil {
		t.Fatal("Value phải từ chối timestamp đầy đủ")
	}
	v, err := TimeOfDay("9:15").Value()
	if err != nil || v != "09:15" {
		t.Fatalf("Value = %v, %v; muốn 09:15", v, err)
	}
}

func TestTimeOfDayScan(t *testing.T) {
	cases := []interface{}{"19:30:00", []byte("19:30:00"), time.Date(0, 1, 1, 19, 30, 0, 0, time.UTC)}
	for _, src := range cases {
		var got TimeOfDay
		if err := got.Scan(src); err != nil || got != "19:30" {
			t.Errorf("Scan(%#v) = %q, %v; muốn 19:30", src, got, err)
		}
	}
	var bad TimeOfDay
	if err := bad.Scan(42); err == nil {
		t.Error("Scan(int) phải lỗi")
	}
}

func TestTimeOfDayOn(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	got, err := TimeOfDay("19:00").On(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), loc)
	want := time.Date(2026, 9, 2, 19, 0, 0, 0, loc)
	if err != nil || !got.Equal(want) {
		t.Fatalf("On = %v, %v; muốn %v", got, err, want)
	}
}
