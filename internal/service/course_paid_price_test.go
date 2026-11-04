package service

// Q3-09 (QA3): khoá có phí (is_free = false) phải có giá > 0. Muốn giá 0 thì is_free = true. Tạo và sửa đều chặn;
// sửa trường không liên quan KHÔNG bị chặn bởi dữ liệu cũ (có phí mà giá 0).

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestCreateCourse_PaidCourseNeedsPositivePrice(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name    string
		price   string
		isFree  *bool
		invalid bool
	}{
		{"có phí, giá 0", "0", &no, true},
		{"is_free bỏ trống (mặc định có phí), giá 0", "0", nil, true},
		{"có phí, giá âm", "-1", &no, true},
		{"có phí, giá làm tròn về 0", "0.004", &no, true},
		{"có phí, giá hợp lệ", "499000", &no, false},
		{"miễn phí, giá 0", "0", &yes, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &discountCreateRepo{}
			req := dto.CreateCourseDTO{Title: "Khoa hoc Q3-09", Price: decimal.RequireFromString(c.price), IsFree: c.isFree}
			_, err := NewCourseService(repo, nil, nil, nil).CreateCourse(context.Background(), req)
			if c.invalid {
				if !errors.Is(err, ErrPaidCoursePriceInvalid) || repo.created != nil {
					t.Fatalf("err=%v created=%v, muốn ErrPaidCoursePriceInvalid và không tạo khoá", err, repo.created)
				}
				return
			}
			if err != nil || repo.created == nil {
				t.Fatalf("err=%v created=%v, muốn tạo được", err, repo.created)
			}
		})
	}
}

func updateCourseBody(t *testing.T, price int64, isFree bool, body string) (*editLockFixture, error) {
	t.Helper()
	f := newEditLockFixture(model.CourseStatusDraft)
	f.course.course.Price = decimal.NewFromInt(price)
	f.course.course.IsFree = isFree
	var req dto.UpdateCourseDTO
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	_, err := NewCourseService(f.course, nil, nil, nil).UpdateCourse(context.Background(), f.courseID(), f.owner, false, req)
	return f, err
}

func TestUpdateCourse_PaidCourseNeedsPositivePrice(t *testing.T) {
	cases := []struct {
		name    string
		price   int64
		isFree  bool
		body    string
		invalid bool
	}{
		{"đổi giá khoá có phí về 0", 500000, false, `{"price":0}`, true},
		{"chuyển khoá miễn phí giá 0 sang có phí", 0, true, `{"is_free":false}`, true},
		{"chuyển sang có phí kèm giá hợp lệ", 0, true, `{"is_free":false,"price":300000}`, false},
		{"chuyển sang miễn phí kèm giá 0", 500000, false, `{"is_free":true,"price":0}`, false},
		{"đổi giá khoá có phí sang giá khác", 500000, false, `{"price":300000}`, false},
		// Dữ liệu cũ có phí mà giá 0: sửa mô tả không bị chặn (cổng nộp duyệt mới là nơi chặn).
		{"sửa trường khác trên khoá cũ có phí giá 0", 0, false, `{"description":"mo ta moi"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := updateCourseBody(t, c.price, c.isFree, c.body)
			if c.invalid {
				if !errors.Is(err, ErrPaidCoursePriceInvalid) {
					t.Fatalf("err = %v, muốn ErrPaidCoursePriceInvalid", err)
				}
				if f.course.updated {
					t.Fatal("lỗi đầu vào mà vẫn ghi DB")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateCourse: %v", err)
			}
		})
	}
}
