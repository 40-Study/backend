package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// Review web #1: web probe 404 (middleware) tin vào hai endpoint này. 404 CHỈ cho "không có"; lỗi hạ tầng (DB chậm/sập)
// phải là 500, nếu không crawler thấy cả catalog 404 và deindex.

type probeCourseStub struct {
	service.CourseServiceInterface
	err error
}

func (s probeCourseStub) GetCourseBySlug(context.Context, string) (*dto.CourseDetailDTO, error) {
	return nil, s.err
}

type probeQuizStub struct {
	service.QuizServiceInterface
	err error
}

func (s probeQuizStub) GetQuizByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.QuizDetailDTO, error) {
	return nil, s.err
}

func TestGetCourseBySlug_404ChiKhiKhongCo_500KhiLoiHaTang(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"khong co hoac chua xuat ban", service.ErrCourseHidden, 404},
		{"boc trong loi khong co", errors.Join(errors.New("ctx"), service.ErrCourseHidden), 404},
		{"DB timeout", errors.New("pq: connection refused"), 500},
		{"context deadline", context.DeadlineExceeded, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewCourseHandler(probeCourseStub{err: tc.err}, nil)
			app := mountWithCaller("GET", "/courses/slug/:slug", uuid.New(), h.GetCourseBySlug)
			if code := doJSON(t, app, "GET", "/courses/slug/abc", ""); code != tc.want {
				t.Fatalf("status %d, muon %d", code, tc.want)
			}
		})
	}
}

func TestGetQuizByID_404ChiKhiKhongCo_500KhiLoiHaTang(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"khong co", service.ErrQuizNotFound, 404},
		{"khoa an", service.ErrCourseHidden, 404},
		{"DB timeout", errors.New("pq: connection refused"), 500},
		{"context deadline", context.DeadlineExceeded, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewQuizHandler(probeQuizStub{err: tc.err}, nil)
			app := mountWithCaller("GET", "/quizzes/:id", uuid.New(), h.GetQuizByID)
			if code := doJSON(t, app, "GET", "/quizzes/"+uuid.NewString(), ""); code != tc.want {
				t.Fatalf("status %d, muon %d", code, tc.want)
			}
		})
	}
}
