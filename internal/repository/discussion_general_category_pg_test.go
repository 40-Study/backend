package repository

// QA6 M3 (Q6-PG-13): chip "Chung" của web đếm `general` + bài chưa phân loại, nhưng `?category=general` chỉ lọc
// `category = 'general'` nên bài NULL/rỗng không bao giờ ra trong danh sách. Chạy SQL THẬT (schema tạm).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestListForumPosts_General_GomCaBaiChuaPhanLoai(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	author := model.User{Email: "m3-" + uuid.NewString() + "@40study.test", PasswordHash: "x", UserName: "m3" + uuid.NewString()[:8]}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("tạo user: %v", err)
	}
	str := func(s string) *string { return &s }
	mk := func(key string, category *string, hidden bool) uuid.UUID {
		d := model.Discussion{UserID: author.ID, Title: str("Bài " + key), Slug: str(key + "-" + uuid.NewString()[:8]),
			Content: "nội dung", Category: category, IsHidden: hidden}
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("tạo bài %s: %v", key, err)
		}
		return d.ID
	}
	general := mk("general", str("general"), false)
	null := mk("null", nil, false)
	empty := mk("empty", str(""), false)
	hiddenNull := mk("hidden-null", nil, true)
	prog := mk("prog", str("programming"), false)

	repo := NewDiscussionRepository(db)
	ids := func(category string) map[uuid.UUID]bool {
		posts, total, err := repo.ListForumPosts(context.Background(), category, 1, 50)
		if err != nil {
			t.Fatalf("ListForumPosts(%q): %v", category, err)
		}
		if int(total) != len(posts) {
			t.Fatalf("category %q: total %d lệch số dòng %d", category, total, len(posts))
		}
		out := map[uuid.UUID]bool{}
		for _, p := range posts {
			out[p.ID] = true
		}
		return out
	}

	g := ids("general")
	for name, id := range map[string]uuid.UUID{"general": general, "NULL": null, "rỗng": empty} {
		if !g[id] {
			t.Errorf("category=general phải có bài %s", name)
		}
	}
	if g[prog] || g[hiddenNull] || len(g) != 3 {
		t.Errorf("category=general không được lẫn bài có tên khác hay bài đã ẩn: %v", g)
	}

	if p := ids("programming"); !p[prog] || len(p) != 1 {
		t.Errorf("category=programming chỉ gồm bài programming: %v", p)
	}
	if all := ids(""); len(all) != 4 { // không lọc: 4 bài hiển thị (bài ẩn bị loại)
		t.Errorf("không lọc phải có 4 bài hiển thị, được %d", len(all))
	}
}
