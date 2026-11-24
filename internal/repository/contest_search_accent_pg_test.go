package repository

// QA4-A05: tìm cuộc thi ở /admin/contests bằng "vong loai" (không dấu) không ra "… vòng loại" vì
// ContestRepository.List dùng ILIKE trần. Chạy SQL THẬT trên Postgres (schema tạm); token ngẫu nhiên
// giữ kết quả độc lập với dữ liệu sẵn có.
// Mutation đã kiểm: về lại "contests.title ILIKE ?" làm test ĐỎ.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestContestList_Keyword_BoQuaDauTiengViet(t *testing.T) {
	tx := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()
	owner := apvUser(t, tx, "contest-accent")
	token := "cst" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	now := time.Now()

	titles := map[string]string{
		"vong-loai": token + " Python cho người mới: vòng loại",
		"lap-trinh": token + " Lập trình thi đấu",
	}
	ids := map[uuid.UUID]string{}
	for key, title := range titles {
		c := model.Contest{Title: title, Slug: "acc-" + uuid.NewString(), Type: "CODING", Status: "PUBLISHED",
			StartTime: now.Add(time.Hour), EndTime: now.Add(2 * time.Hour), CreatedBy: owner.ID}
		if err := tx.Create(&c).Error; err != nil {
			t.Fatalf("tạo cuộc thi %s: %v", key, err)
		}
		ids[c.ID] = key
	}

	repo := NewContestRepository(tx)
	cases := []struct {
		q    string
		want string
	}{
		{token + " vong loai", "vong-loai"},
		{token + " VÒNG LOẠI", "vong-loai"},
		{token + " lap trinh", "lap-trinh"},
		{token + " vong lai", ""},
	}
	for _, tc := range cases {
		got, total, err := repo.List(ctx, ContestListFilter{Q: tc.q, Now: now, Page: 1, Limit: 50})
		if err != nil {
			t.Fatalf("List(%q): %v", tc.q, err)
		}
		if tc.want == "" {
			if total != 0 || len(got) != 0 {
				t.Errorf("List(%q) = %d kết quả, muốn 0", tc.q, len(got))
			}
			continue
		}
		if total != 1 || len(got) != 1 || ids[got[0].ID] != tc.want {
			t.Errorf("List(%q) total=%d got=%v, muốn đúng %s", tc.q, total, got, tc.want)
		}
	}
}
