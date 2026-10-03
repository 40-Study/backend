package utils

import "testing"

// B-21: tên trường lỗi của danh sách UUID viết tắt phải là permission_ids, không phải permission_i_ds.
func TestToSnakeCase_AcronymPlural(t *testing.T) {
	cases := map[string]string{
		"PermissionIDs":    "permission_ids",
		"PermissionIDs[1]": "permission_ids[1]",
		"InstructorID":     "instructor_id",
		"HTTPServer":       "http_server",
		"UserName":         "user_name",
		"Email":            "email",
	}
	for in, want := range cases {
		if got := toSnakeCase(in); got != want {
			t.Errorf("toSnakeCase(%q) = %q, muốn %q", in, got, want)
		}
	}
}

type minMaxProbe struct {
	Name  string   `validate:"min=3"`
	Items []string `validate:"min=1"`
	Count int      `validate:"min=2"`
	Cap   float64  `validate:"max=10"`
}

func TestGetErrorMessage_UnitFollowsKind(t *testing.T) {
	errs := ValidateStruct(minMaxProbe{Name: "a", Items: nil, Count: 1, Cap: 11})
	got := map[string]string{}
	for _, e := range errs {
		got[e.Field] = e.Message
	}
	want := map[string]string{
		"name":  "name must be at least 3 characters",
		"items": "items must contain at least 1 items",
		"count": "count must be at least 2",
		"cap":   "cap must be at most 10",
	}
	for field, msg := range want {
		if got[field] != msg {
			t.Errorf("%s: %q, muốn %q", field, got[field], msg)
		}
	}
}
