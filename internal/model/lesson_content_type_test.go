package model

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestLessonContentTypeTagMatchesSSOT — tag `check:` của LessonContent.Type phải liệt kê ĐÚNG các giá trị trong
// LessonContentTypes. Tag dùng cho DB mới (AutoMigrate), slice dùng cho DB cũ (RunPostMigrations đồng bộ
// constraint): lệch nhau thì DB mới và DB cũ chấp nhận hai tập loại nội dung khác nhau (vd. DB cũ từ chối
// 'article'/'quiz' dù API cho phép).
func TestLessonContentTypeTagMatchesSSOT(t *testing.T) {
	field, ok := reflect.TypeOf(LessonContent{}).FieldByName("Type")
	if !ok {
		t.Fatal("LessonContent không còn field Type")
	}
	tag := field.Tag.Get("gorm")
	m := regexp.MustCompile(`check:type IN \(([^)]*)\)`).FindStringSubmatch(tag)
	if m == nil {
		t.Fatalf("không tìm thấy check:type IN (...) trong tag %q", tag)
	}
	var fromTag []string
	for _, part := range strings.Split(m[1], ",") {
		fromTag = append(fromTag, strings.Trim(strings.TrimSpace(part), "'"))
	}

	want := append([]string(nil), LessonContentTypes...)
	sort.Strings(fromTag)
	sort.Strings(want)
	if !reflect.DeepEqual(fromTag, want) {
		t.Fatalf("tag check = %v, LessonContentTypes = %v — hai nơi phải khớp nhau", fromTag, want)
	}
}

// TestLessonContentTypesCoverEveryConstant — mọi hằng loại nội dung phải nằm trong SSOT dùng sinh CHECK, và
// SSOT không được chứa giá trị trùng.
func TestLessonContentTypesCoverEveryConstant(t *testing.T) {
	got := map[string]int{}
	for _, v := range LessonContentTypes {
		got[v]++
	}
	for _, v := range []string{LessonContentTypeVideo, LessonContentTypeLivestream, LessonContentTypeExercise,
		LessonContentTypeArticle, LessonContentTypeQuiz} {
		if got[v] != 1 {
			t.Errorf("loại %q xuất hiện %d lần trong LessonContentTypes, muốn đúng 1", v, got[v])
		}
	}
	if len(LessonContentTypes) != 5 {
		t.Errorf("LessonContentTypes có %d phần tử, muốn 5 (thêm loại mới thì cập nhật hằng + test này)", len(LessonContentTypes))
	}
}
