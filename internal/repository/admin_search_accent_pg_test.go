package repository

// QA3-08: ô tìm kiếm admin dùng ILIKE nên "nguyen" không ra "Nguyễn" (0 kết quả, trong khi "NGUYỄN" ra 16). Người Việt
// thường gõ không dấu. Chạy SQL THẬT (schema tạm). Token ngẫu nhiên giữ kết quả độc lập với dữ liệu sẵn có; mỗi bảng có
// một dòng "mồi" khác chữ cái cơ sở để chứng minh bỏ dấu không biến thành khớp bừa.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func accentToken() string {
	return "acc" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

func TestAdminListUsers_Keyword_BoQuaDauTiengViet(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	token := accentToken()
	mk := func(key, fullName string) uuid.UUID {
		u := model.User{Email: key + "-" + token + "@40study.test", PasswordHash: "x", UserName: key + token, FullName: &fullName}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", key, err)
		}
		return u.ID
	}
	nguyen := mk("nguyen", "Nguyễn Văn "+token)
	other := mk("tran", "Trần Thị "+token)

	repo := NewUserRepository(db)
	for _, tc := range []struct {
		keyword string
		want    []uuid.UUID
	}{
		{"nguyen van " + token, []uuid.UUID{nguyen}}, // ca báo cáo: không dấu
		{"NGUYỄN VĂN " + token, []uuid.UUID{nguyen}}, // có dấu, hoa: vẫn ra
		{"Nguyên văn " + token, []uuid.UUID{nguyen}}, // dấu khác, cùng chữ cái cơ sở: khớp
		{"tran thi " + token, []uuid.UUID{other}},
		{"nguyan " + token, nil}, // sai chữ cái cơ sở: không khớp
		{token + "%", nil},       // ký tự đặc biệt vẫn được escape
	} {
		users, total, err := repo.AdminListUsers(context.Background(), AdminUserListFilter{Keyword: tc.keyword, Page: 1, Limit: 100})
		if err != nil {
			t.Fatalf("keyword %q: %v", tc.keyword, err)
		}
		var got []uuid.UUID
		for _, u := range users {
			got = append(got, u.ID)
		}
		if len(got) != len(tc.want) || int(total) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
			t.Errorf("keyword %q -> %v (total=%d), muốn %v", tc.keyword, got, total, tc.want)
		}
	}
}

// GET /admin/courses (ListForReview) đi qua utils.ApplyKeywordSearch: "nhom" phải ra "Làm việc nhóm…".
func TestListForReview_Keyword_BoQuaDauTiengViet(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	teacher := apvUser(t, db, "accent-review")
	token := accentToken()
	titles := map[string]string{"nhom": token + " Làm việc nhóm hiệu quả", "khac": token + " Lập trình web"}
	byID := map[uuid.UUID]string{}
	for key, title := range titles {
		c := model.Course{InstructorID: teacher.ID, Title: title, Slug: "acc-" + uuid.NewString(), Status: model.CourseStatusPendingReview}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		byID[c.ID] = key
	}

	repo := NewCourseReviewRepository(db)
	for _, tc := range []struct {
		keyword string
		want    []string
	}{
		{token + " lam viec nhom", []string{"nhom"}},
		{"NHÓM HIỆU QUẢ", []string{"nhom"}},
		{token + " lap trinh", []string{"khac"}},
		{"nhon hieu qua", nil},
	} {
		list, total, err := repo.ListForReview(context.Background(), AdminCourseReviewFilter{Status: model.CourseStatusPendingReview, Keyword: tc.keyword, Page: 1, PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, c := range list {
			if key, ok := byID[c.ID]; ok {
				got = append(got, key)
			}
		}
		if sortedJoin(got) != sortedJoin(append([]string{}, tc.want...)) || int(total) != len(tc.want) {
			t.Errorf("keyword %q -> %v (total=%d), muốn %v", tc.keyword, got, total, tc.want)
		}
	}
}

func TestAdminSearch_OrganizationsAndVouchers_BoQuaDauTiengViet(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()
	token := accentToken()

	org := model.Organization{Name: "Trường Đại học " + token}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	orgs, total, err := NewOrganizationRepository(db).GetAllOrganizations(ctx, 1, 20, "truong dai hoc "+token, "")
	if err != nil || total != 1 || len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Errorf("tổ chức: 'truong dai hoc' phải ra đúng 1 (total=%d, err=%v)", total, err)
	}

	amount := decimal.NewFromInt(1000)
	v := model.Voucher{Code: "ACC" + strings.ToUpper(token[3:]), Name: "Ưu đãi khai giảng " + token, DiscountUnit: model.DiscountUnitMoney,
		DiscountMethod: model.DiscountMethodFixed, DiscountAmountMoney: &amount, AcceptAllPaymentMethods: true, IsActive: true}
	if err := db.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	vs, vtotal, err := NewVoucherRepository(db).GetAllVouchers(ctx, 20, 0, "uu dai khai giang "+token, nil, nil, DeletedExclude)
	if err != nil || vtotal != 1 || len(vs) != 1 || vs[0].ID != v.ID {
		t.Errorf("voucher: 'uu dai khai giang' phải ra đúng 1 (total=%d, err=%v)", vtotal, err)
	}
}
