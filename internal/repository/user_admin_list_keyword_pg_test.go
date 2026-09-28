package repository

// Review vòng 2 (plans/reports/review-260928-users-round2-pr72-pr28.md mục 4): escape ILIKE trong
// AdminListUsers chỉ từng được kiểm bằng gọi API sống, chưa có test tự động. Test này chạy trên
// Postgres THẬT (ILIKE là hành vi của Postgres, fake repo không pin được), trong 1 transaction rồi
// ROLLBACK. Mỗi cặp user gồm 1 user chứa ký tự đặc biệt literal và 1 "mồi" chỉ khớp khi ký tự đó
// bị hiểu là wildcard/escape: `%` khớp mọi chuỗi, `_` khớp 1 ký tự, `\d` là 'd' đã escape.

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func TestAdminListUsers_KeywordKyTuDacBiet_KhopLiteral(t *testing.T) {
	db := openTestPostgres(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	defer sqlDB.Close()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("db.Begin(): %v", tx.Error)
	}
	defer tx.Rollback()

	// Token duy nhất (chỉ chữ/số) để keyword không khớp dữ liệu có sẵn trong DB dev.
	token := "ilk" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	userNames := map[string]string{
		"percent":      token + "p%q",
		"percentDecoy": token + "pzzq",
		"under":        token + "a_b",
		"underDecoy":   token + "axb",
		"backslash":    token + `c\d`,
		"bsDecoy":      token + "cd",
	}
	ids := map[string]uuid.UUID{}
	for key, name := range userNames {
		u := model.User{
			Email:        "ilike-" + key + "-" + token + "@40study.test",
			PasswordHash: "x",
			UserName:     name,
		}
		if err := tx.Create(&u).Error; err != nil {
			t.Fatalf("tao user %s: %v", key, err)
		}
		ids[key] = u.ID
	}

	repo := NewUserRepository(tx)
	cases := []struct {
		keyword string
		want    []string
	}{
		{token + "p%q", []string{"percent"}},
		{token + "a_b", []string{"under"}},
		{token + `c\d`, []string{"backslash"}},
		{token, []string{"backslash", "bsDecoy", "percent", "percentDecoy", "under", "underDecoy"}},
	}
	for _, tc := range cases {
		users, total, err := repo.AdminListUsers(context.Background(), AdminUserListFilter{Keyword: tc.keyword, Page: 1, Limit: 100})
		if err != nil {
			t.Fatalf("keyword %q: loi %v", tc.keyword, err)
		}
		got := make([]string, 0, len(users))
		for _, u := range users {
			for key, id := range ids {
				if u.ID == id {
					got = append(got, key)
				}
			}
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") || int(total) != len(tc.want) {
			t.Fatalf("keyword %q: khop %v (total=%d), muon %v -- ky tu dac biet phai khop LITERAL, khong phai wildcard", tc.keyword, got, total, tc.want)
		}
	}
}
