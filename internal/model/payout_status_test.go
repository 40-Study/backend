package model

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestPayoutStatusTagMatchesSSOT — tag `check:` của InstructorPayout.Status phải liệt kê ĐÚNG các
// giá trị trong PayoutStatuses, không thiếu, không thừa. Sửa 1 bên mà quên bên kia thì đỏ ngay
// (tránh lặp lại lỗi B3-01 của orders: tag Go đúng nhưng constraint DB sai).
func TestPayoutStatusTagMatchesSSOT(t *testing.T) {
	field, ok := reflect.TypeOf(InstructorPayout{}).FieldByName("Status")
	if !ok {
		t.Fatal("InstructorPayout không còn field Status")
	}
	tag := field.Tag.Get("gorm")
	m := regexp.MustCompile(`check:status IN \(([^)]*)\)`).FindStringSubmatch(tag)
	if m == nil {
		t.Fatalf("không tìm thấy check:status IN (...) trong tag %q", tag)
	}
	var fromTag []string
	for _, part := range strings.Split(m[1], ",") {
		fromTag = append(fromTag, strings.Trim(strings.TrimSpace(part), "'"))
	}

	want := append([]string(nil), PayoutStatuses...)
	sort.Strings(fromTag)
	sort.Strings(want)
	if !reflect.DeepEqual(fromTag, want) {
		t.Fatalf("tag check = %v, PayoutStatuses = %v — hai nơi phải khớp nhau", fromTag, want)
	}
}

// TestPayoutStatusSubsets — tập "đang xử lý" và tập "giữ chỗ số dư" phải là tập con của SSOT, và
// "rejected" KHÔNG được nằm trong tập giữ chỗ (nếu nằm trong, từ chối không giải phóng số dư).
func TestPayoutStatusSubsets(t *testing.T) {
	for _, s := range append(append([]string{}, PayoutOpenStatuses...), PayoutReservedStatuses...) {
		if !IsValidPayoutStatus(s) {
			t.Fatalf("%q không thuộc PayoutStatuses", s)
		}
	}
	for _, s := range append(append([]string{}, PayoutOpenStatuses...), PayoutReservedStatuses...) {
		if s == PayoutStatusRejected || s == PayoutStatusCancelled {
			t.Fatalf("%s không được giữ chỗ số dư hay tính là đang xử lý", s)
		}
	}
	if IsValidPayoutStatus("processing") || IsValidPayoutStatus("") {
		t.Fatal("IsValidPayoutStatus chấp nhận giá trị ngoài enum")
	}
}
