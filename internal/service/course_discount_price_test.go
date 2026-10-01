package service

// L1: discount_price phải > 0 và < price. Muốn xoá khuyến mãi thì gửi null (giữ hành vi
// UnmarshalJSON hiện có), 0 KHÔNG phải cách xoá. Đi qua JSON thật rồi service thật.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// updateWithPrice dựng khoá giá 500.000đ, khuyến mãi hiện tại `existingDiscount` (nil = không có),
// rồi áp body JSON; trả khoá sau khi cập nhật (hoặc lỗi).
func updateWithPrice(t *testing.T, existingDiscount *int64, body string) (*editLockFixture, error) {
	t.Helper()
	f := newEditLockFixture(model.CourseStatusDraft)
	f.course.course.Price = decimal.NewFromInt(500000)
	if existingDiscount != nil {
		d := decimal.NewFromInt(*existingDiscount)
		f.course.course.DiscountPrice = &d
	}
	var req dto.UpdateCourseDTO
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	_, err := NewCourseService(f.course, nil, nil, nil).UpdateCourse(context.Background(), f.courseID(), f.owner, false, req)
	return f, err
}

func TestUpdateCourse_DiscountPriceMustBePositiveAndBelowPrice(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		invalid bool
	}{
		{"bằng giá gốc", `{"discount_price":500000}`, true},
		{"lớn hơn giá gốc", `{"discount_price":600000}`, true},
		{"bằng 0 (xoá thì phải gửi null)", `{"discount_price":0}`, true},
		{"âm", `{"discount_price":-1}`, true},
		{"hợp lệ", `{"discount_price":499999}`, false},
		{"hợp lệ nhỏ", `{"discount_price":1}`, false},
		{"đổi giá gốc xuống kèm khuyến mãi hợp lệ", `{"price":300000,"discount_price":200000}`, false},
		{"đổi giá gốc xuống dưới khuyến mãi gửi kèm", `{"price":300000,"discount_price":300000}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := updateWithPrice(t, nil, c.body)
			if c.invalid {
				if !errors.Is(err, ErrDiscountPriceInvalid) {
					t.Fatalf("err = %v, muốn ErrDiscountPriceInvalid", err)
				}
				if f.course.updated {
					t.Fatalf("lỗi đầu vào mà vẫn ghi DB")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateCourse: %v", err)
			}
		})
	}
}

// Gửi null vẫn xoá được khuyến mãi — kể cả khi kèm đổi giá gốc xuống dưới khuyến mãi cũ.
func TestUpdateCourse_NullStillClearsDiscount(t *testing.T) {
	existing := int64(400000)
	f, err := updateWithPrice(t, &existing, `{"price":300000,"discount_price":null}`)
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if f.course.course.DiscountPrice != nil {
		t.Fatalf("null phải xoá khuyến mãi, còn %v", f.course.course.DiscountPrice)
	}
}

// Hạ giá gốc xuống dưới khuyến mãi đang có (không xoá cùng request) bị từ chối: nếu không, khuyến mãi
// cũ >= giá mới sẽ nằm lại trong DB rồi bị EffectivePrice âm thầm bỏ qua.
func TestUpdateCourse_LoweringPriceBelowExistingDiscountIsRejected(t *testing.T) {
	existing := int64(400000)
	if _, err := updateWithPrice(t, &existing, `{"price":300000}`); !errors.Is(err, ErrDiscountPriceInvalid) {
		t.Fatalf("hạ giá xuống dưới khuyến mãi cũ: err = %v, muốn ErrDiscountPriceInvalid", err)
	}
	if _, err := updateWithPrice(t, &existing, `{"price":450000}`); err != nil {
		t.Fatalf("hạ giá vẫn trên khuyến mãi cũ phải được: %v", err)
	}
}

// Sửa trường không liên quan KHÔNG bị chặn bởi dữ liệu cũ đã lệch (khuyến mãi >= giá từ trước).
func TestUpdateCourse_UnrelatedEditIgnoresLegacyBadDiscount(t *testing.T) {
	legacy := int64(600000) // lớn hơn giá gốc 500000, dữ liệu cũ
	if _, err := updateWithPrice(t, &legacy, `{"description":"mo ta moi"}`); err != nil {
		t.Fatalf("sửa mô tả không được vướng khuyến mãi cũ: %v", err)
	}
}

type discountCreateRepo struct {
	repository.CourseRepositoryInterface
	created *model.Course
}

func (r *discountCreateRepo) SlugExists(context.Context, string) (bool, error) { return false, nil }
func (r *discountCreateRepo) Create(_ context.Context, c *model.Course) error {
	r.created = c
	return nil
}

func TestCreateCourse_DiscountPriceValidated(t *testing.T) {
	price := decimal.NewFromInt(500000)
	mk := func(d *decimal.Decimal) dto.CreateCourseDTO {
		return dto.CreateCourseDTO{Title: "Khoa hoc L1", Price: price, DiscountPrice: d}
	}
	dec := func(n int64) *decimal.Decimal { d := decimal.NewFromInt(n); return &d }

	for name, d := range map[string]*decimal.Decimal{"bằng giá": dec(500000), "lớn hơn giá": dec(900000), "bằng 0": dec(0), "âm": dec(-5)} {
		repo := &discountCreateRepo{}
		_, err := NewCourseService(repo, nil, nil, nil).CreateCourse(context.Background(), mk(d))
		if !errors.Is(err, ErrDiscountPriceInvalid) || repo.created != nil {
			t.Errorf("%s: err=%v created=%v, muốn ErrDiscountPriceInvalid và không tạo khoá", name, err, repo.created)
		}
	}
	for name, d := range map[string]*decimal.Decimal{"không có khuyến mãi (null)": nil, "hợp lệ": dec(499000)} {
		repo := &discountCreateRepo{}
		if _, err := NewCourseService(repo, nil, nil, nil).CreateCourse(context.Background(), mk(d)); err != nil || repo.created == nil {
			t.Errorf("%s: err=%v created=%v, muốn tạo được", name, err, repo.created)
		}
	}
}
