package service

import (
	"context"
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
	svc := NewCourseService(&publicCourseRepoStub{course: &model.Course{Status: "draft"}}, nil, nil)

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
	}}, nil, nil)

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
