package model

import (
	"reflect"
	"regexp"
	"sort"
	"testing"
)

// checkTagValues rút danh sách giá trị trong `check:<col> IN ('a', 'b', ...)` của tag gorm trên
// field fieldName — test đọc CHÍNH tag mà GORM AutoMigrate dùng để tạo constraint trên DB mới.
func checkTagValues(t *testing.T, v interface{}, fieldName string) []string {
	t.Helper()
	field, ok := reflect.TypeOf(v).FieldByName(fieldName)
	if !ok {
		t.Fatalf("khong tim thay field %s", fieldName)
	}
	tag := field.Tag.Get("gorm")
	inClause := regexp.MustCompile(`check:[a-z_]+ IN \(([^)]*)\)`).FindStringSubmatch(tag)
	if inClause == nil {
		t.Fatalf("tag gorm cua %s khong co check IN (...): %q", fieldName, tag)
	}
	var values []string
	for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(inClause[1], -1) {
		values = append(values, m[1])
	}
	return values
}

// assertSameSet so 2 CHIỀU: thiếu ở tag (DB mới thiếu giá trị code dùng) HOẶC thừa ở tag (tag có
// giá trị SSOT không biết) đều đỏ.
func assertSameSet(t *testing.T, name string, fromTag, ssot []string) {
	t.Helper()
	a := append([]string(nil), fromTag...)
	b := append([]string(nil), ssot...)
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%s: tag gorm %v lech SSOT %v — sua CA tag lan slice SSOT (course_status.go)", name, a, b)
	}
}

func TestCourseStatusTagMatchesSSOT(t *testing.T) {
	assertSameSet(t, "Course.Status", checkTagValues(t, Course{}, "Status"), CourseStatuses)
}

func TestTeacherApprovalStatusTagMatchesSSOT(t *testing.T) {
	assertSameSet(t, "TeacherProfile.ApprovalStatus",
		checkTagValues(t, TeacherProfile{}, "ApprovalStatus"), TeacherApprovalStatuses)
}

// TestCourseStatusesContainRejected pin riêng giá trị mới của phase 3: nếu ai đó xoá 'rejected'
// khỏi CẢ tag lẫn slice cùng lúc thì 2 test đối chiếu ở trên vẫn xanh, nhưng luồng từ chối khoá
// học sẽ vỡ ở DB (vi phạm CHECK) — test này bắt đúng trường hợp đó.
func TestCourseStatusesContainRejected(t *testing.T) {
	for _, s := range CourseStatuses {
		if s == CourseStatusRejected {
			return
		}
	}
	t.Fatal("CourseStatuses phai co 'rejected' (luong admin tu choi khoa hoc)")
}
