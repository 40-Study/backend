package model

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// TimeOfDayColumnType là kiểu cột Postgres cho mọi trường giờ trong ngày (lịch học lặp tuần,
// buổi học, giờ điểm danh dự kiến).
//
// Vì sao không viết `type:time`: GORM hạ chữ thường giá trị tag `type` và nếu nó trùng tên kiểu
// logic ("time" == schema.Time) thì coi đó là kiểu logic chứ không phải tên kiểu SQL
// (gorm@v1.30.0 schema/field.go:321-324); driver Postgres ánh xạ schema.Time thành `timestamptz`
// (driver/postgres@v1.6.0 postgres.go:251-255). Kết quả: cột thành TIMESTAMPTZ, chuỗi "14:00" bị
// từ chối (SQLSTATE 22007) và API trả cả ngày + múi giờ vô nghĩa. Tên đầy đủ không trùng kiểu
// logic nào nên GORM giữ nguyên.
const TimeOfDayColumnType = "time without time zone"

// TimeOfDay là giờ trong ngày không gắn ngày hay múi giờ, dạng chuẩn "HH:MM" (24 giờ, đủ 2 chữ số).
// Dạng chuẩn cố định độ dài nên so sánh chuỗi cũng là so sánh thời gian.
//
// Độ chính xác là phút: lịch học không cần giây; giây đọc từ DB bị bỏ.
type TimeOfDay string

// ParseTimeOfDay nhận "HH:MM" hoặc "HH:MM:SS" (dạng web gửi và dạng Postgres trả) và trả dạng chuẩn.
// Chuỗi khác (kể cả timestamp đầy đủ) là lỗi: giờ lặp tuần không có ngày để lấy.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"15:04", "15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return TimeOfDay(t.Format("15:04")), nil
		}
	}
	return "", fmt.Errorf("giờ %q không hợp lệ, dùng định dạng HH:MM", s)
}

// String trả dạng "HH:MM".
func (t TimeOfDay) String() string { return string(t) }

// On ghép giờ này với ngày `day` trong múi giờ `loc`.
func (t TimeOfDay) On(day time.Time, loc *time.Location) (time.Time, error) {
	clock, err := time.Parse("15:04", string(t))
	if err != nil {
		return time.Time{}, fmt.Errorf("giờ %q không hợp lệ: %w", string(t), err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, loc), nil
}

// Value ghi xuống DB dạng "HH:MM". Kiểm định dạng ở đây để giá trị sai không tới Postgres dưới dạng
// lỗi 22007 khó hiểu.
func (t TimeOfDay) Value() (driver.Value, error) {
	norm, err := ParseTimeOfDay(string(t))
	if err != nil {
		return nil, err
	}
	return string(norm), nil
}

// Scan đọc cột `time`: driver trả chuỗi "HH:MM:SS" (hoặc []byte); time.Time cũng được chấp nhận phòng
// khi driver đổi cách trả.
func (t *TimeOfDay) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return t.scanString(v)
	case []byte:
		return t.scanString(string(v))
	case time.Time:
		*t = TimeOfDay(v.Format("15:04"))
		return nil
	default:
		return fmt.Errorf("không đọc được TimeOfDay từ %T", src)
	}
}

func (t *TimeOfDay) scanString(s string) error {
	norm, err := ParseTimeOfDay(s)
	if err != nil {
		return err
	}
	*t = norm
	return nil
}
