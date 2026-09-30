package service

// Lane U (UX-9): xoá số điện thoại (hồ sơ) và giá khuyến mãi (khoá học). Đi qua JSON THẬT
// (json.Unmarshal vào DTO) rồi service THẬT, để test đỏ nếu UnmarshalJSON hoặc nhánh Cleared
// trong service bị bỏ.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func updateCourseFromJSON(t *testing.T, body string) (*editLockFixture, error) {
	t.Helper()
	f := newEditLockFixture(model.CourseStatusDraft)
	price := decimal.NewFromInt(199000)
	expires := time.Now().Add(48 * time.Hour)
	f.course.course.Price = decimal.NewFromInt(500000)
	f.course.course.DiscountPrice = &price
	f.course.course.DiscountExpiresAt = &expires

	var req dto.UpdateCourseDTO
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	_, err := NewCourseService(f.course, nil, nil, nil).UpdateCourse(context.Background(), f.courseID(), f.owner, false, req)
	return f, err
}

func TestUpdateCourse_DiscountNull_ClearsPriceAndExpiry(t *testing.T) {
	f, err := updateCourseFromJSON(t, `{"discount_price":null}`)
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if c := f.course.course; c.DiscountPrice != nil || c.DiscountExpiresAt != nil {
		t.Fatalf("khuyến mãi phải bị xoá, còn price=%v expires=%v", c.DiscountPrice, c.DiscountExpiresAt)
	}
}

func TestUpdateCourse_DiscountEmptyString_Clears(t *testing.T) {
	f, err := updateCourseFromJSON(t, `{"discount_price":""}`)
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if f.course.course.DiscountPrice != nil {
		t.Fatalf("chuỗi rỗng phải xoá khuyến mãi, còn %v", f.course.course.DiscountPrice)
	}
}

func TestUpdateCourse_DiscountAbsent_KeepsExisting(t *testing.T) {
	f, err := updateCourseFromJSON(t, `{"description":"mo ta moi"}`)
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	c := f.course.course
	if c.DiscountPrice == nil || !c.DiscountPrice.Equal(decimal.NewFromInt(199000)) || c.DiscountExpiresAt == nil {
		t.Fatalf("không gửi discount_price thì phải giữ nguyên, còn price=%v expires=%v", c.DiscountPrice, c.DiscountExpiresAt)
	}
}

func TestUpdateCourse_DiscountValue_ReplacesPrice(t *testing.T) {
	f, err := updateCourseFromJSON(t, `{"discount_price":150000}`)
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if p := f.course.course.DiscountPrice; p == nil || !p.Equal(decimal.NewFromInt(150000)) {
		t.Fatalf("giá khuyến mãi mới = %v, muốn 150000", p)
	}
}

// ---- UpdateMe: số điện thoại ----

type clearPhoneFakeUserRepo struct {
	repository.UserRepositoryInterface
	updates []map[string]interface{}
}

func (f *clearPhoneFakeUserRepo) UpdateUserProfile(_ context.Context, _ uuid.UUID, updates map[string]interface{}) error {
	f.updates = append(f.updates, updates)
	return nil
}

// updateMeFromJSON trả map ghi xuống DB. UpdateMe gọi GetMe sau khi ghi (cần nhiều repo khác) nên
// lỗi/panic ở bước đọc lại được bỏ qua: thứ cần kiểm là map updates đã ghi.
func updateMeFromJSON(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var req dto.UpdateMeRequestDto
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	repo := &clearPhoneFakeUserRepo{}
	svc := NewAuthService(nil, repo, nil, nil, nil, nil, nil, nil)
	func() {
		defer func() { _ = recover() }()
		_, _ = svc.UpdateMe(context.Background(), uuid.New(), req)
	}()
	if len(repo.updates) != 1 {
		t.Fatalf("UpdateUserProfile gọi %d lần, muốn 1", len(repo.updates))
	}
	return repo.updates[0]
}

func TestUpdateMe_PhoneNull_WritesNULL(t *testing.T) {
	updates := updateMeFromJSON(t, `{"phone":null,"bio":"x"}`)
	v, present := updates["phone"]
	if !present || v != nil {
		t.Fatalf("phone phải được ghi NULL (present=%v, value=%v)", present, v)
	}
}

func TestUpdateMe_PhoneEmptyString_WritesNULL(t *testing.T) {
	updates := updateMeFromJSON(t, `{"phone":""}`)
	if v, present := updates["phone"]; !present || v != nil {
		t.Fatalf("phone \"\" phải xoá thành NULL (present=%v, value=%v)", present, v)
	}
}

func TestUpdateMe_PhoneAbsent_LeavesPhoneUntouched(t *testing.T) {
	updates := updateMeFromJSON(t, `{"bio":"x"}`)
	if _, present := updates["phone"]; present {
		t.Fatalf("không gửi phone thì không được đụng tới cột phone: %v", updates)
	}
}
