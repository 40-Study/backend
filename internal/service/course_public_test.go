package service

import (
	"context"
	"errors"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type publicCourseRepoStub struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (s *publicCourseRepoStub) GetDetailBySlug(context.Context, string) (*model.Course, error) {
	return s.course, nil
}

func TestGetCourseBySlugRejectsUnpublishedCourse(t *testing.T) {
	// Tham so thu 4 (C-1, review vòng 2): enrollmentRepo — nil duoc vi GetCourseBySlug (public,
	// khong co nguoi dang nhap) khong di qua duong khoa nao.
	svc := NewCourseService(&publicCourseRepoStub{course: &model.Course{Status: "draft"}}, nil, nil, nil)

	if _, err := svc.GetCourseBySlug(context.Background(), "draft-course"); err == nil {
		t.Fatal("muon public slug tu choi course chua published")
	}
}

func TestGetCourseBySlugDoesNotExposeLessonContents(t *testing.T) {
	svc := NewCourseService(&publicCourseRepoStub{course: &model.Course{
		Status: "published",
		Sections: []model.Section{{
			Lessons: []model.Lesson{{Contents: []model.LessonContent{{Type: "video"}}}},
		}},
	}}, nil, nil, nil)

	got, err := svc.GetCourseBySlug(context.Background(), "published-course")
	if err != nil {
		t.Fatalf("course published bi tu choi: %v", err)
	}
	if len(got.Sections) != 1 || len(got.Sections[0].Lessons) != 1 {
		t.Fatal("public syllabus phai van co section va lesson")
	}
	if len(got.Sections[0].Lessons[0].Contents) != 0 {
		t.Fatal("public course detail khong duoc lo lesson contents")
	}
}

// Review web #1: không có / chưa xuất bản phải là sentinel ErrCourseHidden để handler trả 404 và coi mọi lỗi khác là 500.
func TestGetCourseBySlug_KhongCoVaChuaXuatBan_TraErrCourseHidden(t *testing.T) {
	for name, course := range map[string]*model.Course{"khong co": nil, "draft": {Status: "draft"}, "archived": {Status: "archived"}} {
		svc := NewCourseService(&publicCourseRepoStub{course: course}, nil, nil, nil)
		if _, err := svc.GetCourseBySlug(context.Background(), "x"); !errors.Is(err, ErrCourseHidden) {
			t.Errorf("%s: err = %v, muon ErrCourseHidden", name, err)
		}
	}
}
