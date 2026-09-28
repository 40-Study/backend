package repository

// QA vòng 2 (G5 — N-11): utils.ApplyKeywordSearch và 5 chỗ "name ILIKE ?" tự nối "%"+keyword+"%"
// không escape, nên keyword "%" khớp MỌI dòng và "a_b" khớp cả "axb". Escaper chỉ có ở
// AdminListUsers (PR #72). Test chạy SQL THẬT trên Postgres (qua pgtest, ROLLBACK cuối test) ở
// 2 trang duyệt (khoá học, hồ sơ giảng viên) và 1 repo từng dùng "name ILIKE ?" trần (tổ chức).
// Mỗi case có 1 dòng chứa ký tự đặc biệt LITERAL và 1 dòng "mồi" chỉ khớp khi ký tự đó bị hiểu là
// wildcard. Token ngẫu nhiên giữ kết quả độc lập với dữ liệu sẵn có trong DB dev.

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func escapeTestToken() string {
	return "esc" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

func sortedJoin(items []string) string {
	sort.Strings(items)
	return strings.Join(items, ",")
}

func TestListForReview_KeywordPhanTram_KhopLiteral(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	teacher := apvUser(t, tx, "esc-course-teacher")
	token := escapeTestToken()

	titles := map[string]string{"literal": token + "p%q", "decoy": token + "pzzq"}
	byID := map[uuid.UUID]string{}
	for key, title := range titles {
		c := model.Course{InstructorID: teacher.ID, Title: title, Slug: "esc-" + uuid.NewString(), Status: model.CourseStatusPendingReview}
		if err := tx.Create(&c).Error; err != nil {
			t.Fatalf("tạo khoá %s: %v", key, err)
		}
		byID[c.ID] = key
	}

	repo := NewCourseReviewRepository(tx)
	cases := []struct {
		keyword string
		want    []string
	}{
		{token + "%", nil},                           // "%" literal: không khoá nào có "<token>%"
		{token + "p%q", []string{"literal"}},         // chỉ khớp dòng có "%" thật
		{token, []string{"decoy", "literal"}},        // keyword thường vẫn khớp cả hai
	}
	for _, tc := range cases {
		list, total, err := repo.ListForReview(ctx, AdminCourseReviewFilter{Status: model.CourseStatusPendingReview, Keyword: tc.keyword, Page: 1, PageSize: 100})
		if err != nil {
			t.Fatalf("keyword %q: %v", tc.keyword, err)
		}
		got := []string{}
		for _, c := range list {
			if key, ok := byID[c.ID]; ok {
				got = append(got, key)
			}
		}
		if sortedJoin(got) != sortedJoin(append([]string{}, tc.want...)) || int(total) != len(tc.want) {
			t.Fatalf("duyệt khoá học, keyword %q: khớp %v (total=%d), muốn %v — %%/_ phải khớp literal", tc.keyword, got, total, tc.want)
		}
	}
}

func TestTeacherApplicationList_KeywordGachDuoi_KhopLiteral(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	token := escapeTestToken()

	specs := map[string]string{"literal": token + "a_b", "decoy": token + "axb"}
	byID := map[uuid.UUID]string{}
	for key, spec := range specs {
		u := apvUser(t, tx, "esc-applicant")
		s := spec
		p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalPending, Specialization: &s}
		if err := tx.Create(&p).Error; err != nil {
			t.Fatalf("tạo hồ sơ %s: %v", key, err)
		}
		byID[p.ID] = key
	}

	repo := NewTeacherApplicationRepository(tx)
	cases := []struct {
		keyword string
		want    []string
	}{
		{token + "%", nil},
		{token + "a_b", []string{"literal"}},
		{token, []string{"decoy", "literal"}},
	}
	for _, tc := range cases {
		rows, total, err := repo.List(ctx, TeacherApplicationFilter{Status: model.TeacherApprovalPending, Keyword: tc.keyword, Page: 1, Limit: 100})
		if err != nil {
			t.Fatalf("keyword %q: %v", tc.keyword, err)
		}
		got := []string{}
		for _, r := range rows {
			if key, ok := byID[r.ID]; ok {
				got = append(got, key)
			}
		}
		if sortedJoin(got) != sortedJoin(append([]string{}, tc.want...)) || int(total) != len(tc.want) {
			t.Fatalf("duyệt giảng viên, keyword %q: khớp %v (total=%d), muốn %v — %%/_ phải khớp literal", tc.keyword, got, total, tc.want)
		}
	}
}

func TestGetAllOrganizations_KeywordKyTuDacBiet_KhopLiteral(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	token := escapeTestToken()

	names := map[string]string{"literal": token + "a_b", "decoy": token + "axb"}
	byID := map[uuid.UUID]string{}
	for key, name := range names {
		o := model.Organization{Name: name}
		if err := tx.Create(&o).Error; err != nil {
			t.Fatalf("tạo tổ chức %s: %v", key, err)
		}
		byID[o.ID] = key
	}

	repo := NewOrganizationRepository(tx)
	cases := []struct {
		keyword string
		want    []string
	}{
		{token + "%", nil},
		{token + "a_b", []string{"literal"}},
	}
	for _, tc := range cases {
		orgs, _, err := repo.GetAllOrganizations(ctx, 1, 100, tc.keyword, "")
		if err != nil {
			t.Fatalf("keyword %q: %v", tc.keyword, err)
		}
		got := []string{}
		for _, o := range orgs {
			if key, ok := byID[o.ID]; ok {
				got = append(got, key)
			}
		}
		if sortedJoin(got) != sortedJoin(append([]string{}, tc.want...)) {
			t.Fatalf("tổ chức, keyword %q: khớp %v, muốn %v", tc.keyword, got, tc.want)
		}
	}
}
