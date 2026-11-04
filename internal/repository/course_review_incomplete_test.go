package repository

// Q3-09 (QA3): nộp duyệt phải đòi ảnh bìa, mô tả và giá (khoá có phí), ngoài nội dung bài học.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// reviewReadyField trả con trỏ chuỗi cho các fixture khoá "đủ điều kiện nộp duyệt".
func reviewReadyField(s string) *string { return &s }

func TestCourseMissingForReview(t *testing.T) {
	blank, cover, desc := "   ", "https://cdn.test/c.png", "Mô tả"
	cases := []struct {
		name   string
		course model.Course
		want   []string
	}{
		{"đủ, có phí", model.Course{ThumbnailURL: &cover, Description: &desc, Price: decimal.NewFromInt(499000)}, nil},
		{"đủ, miễn phí giá 0", model.Course{ThumbnailURL: &cover, Description: &desc, IsFree: true}, nil},
		{"thiếu tất cả, có phí giá 0", model.Course{}, []string{CourseMissingThumbnail, CourseMissingDescription, CourseMissingPrice}},
		{"chỉ khoảng trắng coi như thiếu", model.Course{ThumbnailURL: &blank, Description: &blank, IsFree: true}, []string{CourseMissingThumbnail, CourseMissingDescription}},
		{"giá làm tròn về 0", model.Course{ThumbnailURL: &cover, Description: &desc, Price: decimal.RequireFromString("0.004")}, []string{CourseMissingPrice}},
		{"miễn phí không cần giá", model.Course{ThumbnailURL: &cover, Description: &desc, IsFree: true, Price: decimal.Zero}, nil},
	}
	for _, c := range cases {
		if got := CourseMissingForReview(&c.course); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, muốn %v", c.name, got, c.want)
		}
	}
}

func TestCourseIncompleteError_MessageNamesEachMissingItemInVietnamese(t *testing.T) {
	err := &CourseIncompleteError{Missing: []string{CourseMissingThumbnail, CourseMissingDescription, CourseMissingPrice}}
	msg := err.MessageVI()
	for _, want := range []string{"ảnh bìa", "mô tả", "giá"} {
		if !strings.Contains(msg, want) {
			t.Errorf("thông báo %q thiếu %q", msg, want)
		}
	}
	if !errors.Is(err, ErrCourseIncomplete) {
		t.Error("errors.Is(err, ErrCourseIncomplete) phải đúng")
	}
}

func TestCourseReviewGate_RejectsCourseMissingCoverDescriptionOrPrice(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()
	teacher := model.User{Email: "qa-incomplete-" + uuid.NewString() + "@40study.test", PasswordHash: "x", UserName: "QA-inc-" + uuid.NewString()[:8]}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	// Khoá có phí (is_free mặc định false), giá 0, không ảnh bìa/mô tả, nhưng đã có bài có nội dung.
	course := model.Course{InstructorID: teacher.ID, Title: "QA-incomplete", Slug: "qa-incomplete-" + uuid.NewString(), Status: model.CourseStatusDraft}
	if err := db.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	lesson := apvEmptyLesson(t, db, apvSection(t, db, course.ID).ID, "Bài 1", 1)
	apvVideoContent(t, db, lesson.ID)

	repo := NewCourseReviewRepository(db)
	_, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &teacher.ID, nil, nil)
	var incomplete *CourseIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("muốn CourseIncompleteError, nhận %v", err)
	}
	if want := []string{CourseMissingThumbnail, CourseMissingDescription, CourseMissingPrice}; !reflect.DeepEqual(incomplete.Missing, want) {
		t.Fatalf("missing = %v, muốn %v", incomplete.Missing, want)
	}
	var status string
	db.Model(&model.Course{}).Select("status").Where("id = ?", course.ID).Scan(&status)
	if status != model.CourseStatusDraft {
		t.Fatalf("khoá thiếu thông tin không được đổi trạng thái, nhận %q", status)
	}

	// Bổ sung đủ thì nộp được.
	if err := db.Model(&model.Course{}).Where("id = ?", course.ID).Updates(map[string]any{
		"thumbnail_url": "https://cdn.test/cover.png", "description": "Mô tả", "price": 499000,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &teacher.ID, nil, nil); err != nil {
		t.Fatalf("khoá đủ thông tin phải nộp được: %v", err)
	}
}
