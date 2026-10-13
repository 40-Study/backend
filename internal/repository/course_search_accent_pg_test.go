package repository

// QA2 F-07: tìm khoá học "dai so" (không dấu) không ra "Đại số tuyến tính" vì ILIKE chỉ bỏ phân biệt hoa/thường.
// Chạy SQL THẬT trên Postgres (schema tạm). Token ngẫu nhiên giữ kết quả độc lập với dữ liệu sẵn có.
// Mutation đã kiểm: về lại ApplyKeywordSearch ở CourseRepository.GetAll làm test ĐỎ.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestCourseGetAll_Keyword_BoQuaDauTiengViet(t *testing.T) {
	// Schema tạm đã migrate (không skip khi DB dùng chung chưa migrate: test không được xanh vì bị bỏ qua).
	tx := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()
	teacher := apvUser(t, tx, "accent-teacher")
	token := "acc" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

	short := "Giới thiệu Đường đi ngắn nhất"
	courses := map[string]model.Course{
		"dai-so":   {Title: token + " Đại số tuyến tính ứng dụng"},
		"toan":     {Title: token + " Toán rời rạc"},
		"mo-ta":    {Title: token + " Cấu trúc dữ liệu", ShortDescription: &short},
		"khong-co": {Title: token + " Lập trình web"},
	}
	ids := map[uuid.UUID]string{}
	for key, c := range courses {
		c.InstructorID, c.Slug, c.Status = teacher.ID, "acc-"+uuid.NewString(), model.CourseStatusPublished
		if err := tx.Create(&c).Error; err != nil {
			t.Fatalf("tạo khoá %s: %v", key, err)
		}
		ids[c.ID] = key
	}

	repo := NewCourseRepository(tx)
	cases := []struct {
		keyword string
		want    []string
	}{
		{token + " dai so", []string{"dai-so"}},      // ca báo cáo: không dấu
		{token + " ĐẠI SỐ", []string{"dai-so"}},      // có dấu, viết hoa: vẫn ra
		{token + " DAI SO", []string{"dai-so"}},      // viết hoa, không dấu
		{token + " đại số", []string{"dai-so"}},      // có dấu, viết thường
		{token + " toan roi", []string{"toan"}},      // "Toán" <- "toan", "rạc" <- "rac"
		{token + " cau truc", []string{"mo-ta"}},     // không dấu giữa tiêu đề
		{token + " dai sa", nil},                     // sai chữ cái cơ sở thì không khớp
		{token + " dai sổ", []string{"dai-so"}},      // dấu khác nhưng chữ cơ sở đúng: bỏ dấu cả hai phía nên vẫn khớp
		{token + " lap trinh", []string{"khong-co"}}, // không dấu, khoá khác
		{token + "%", nil},                           // ký tự đặc biệt vẫn được escape
	}

	for _, tc := range cases {
		got, _, err := repo.GetAll(ctx, CourseFilterDBParams{Keyword: tc.keyword, Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("GetAll(%q): %v", tc.keyword, err)
		}
		var keys []string
		for _, c := range got {
			if key, mine := ids[c.ID]; mine {
				keys = append(keys, key)
			}
		}
		if sortedJoin(keys) != sortedJoin(append([]string{}, tc.want...)) {
			t.Errorf("keyword %q -> %v, muốn %v", tc.keyword, keys, tc.want)
		}
	}

	// Khớp mô tả ngắn không dấu: "duong di ngan" ra khoá có short_description "Đường đi ngắn nhất".
	got, _, err := repo.GetAll(ctx, CourseFilterDBParams{Keyword: "duong di ngan nhat", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range got {
		if ids[c.ID] == "mo-ta" {
			found = true
		}
	}
	if !found {
		t.Error("tìm 'duong di ngan nhat' phải ra khoá có short_description 'Đường đi ngắn nhất'")
	}
}
