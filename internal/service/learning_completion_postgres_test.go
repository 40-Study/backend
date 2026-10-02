package service

// QA vòng 2, lane A — test tích hợp Postgres THẬT cho luồng học trọn một khoá:
// bài video (A1) -> bài tập không có video (A2) -> tiến độ 100% -> chứng chỉ tự cấp đúng 1 lần (A4).
// Dữ liệu COMMIT trong schema tạm (pgtest.IsolatedSchema) bị DROP khi test xong.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestLearningCompletion_Postgres_VideoRoiBaiTapRoiChungChiDung1Lan(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()
	s := uuid.NewString()[:8]

	teacher := model.User{Email: "qa-r2a-teacher-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-r2a-t-" + s}
	student := model.User{Email: "qa-r2a-student-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-r2a-s-" + s}
	for _, u := range []*model.User{&teacher, &student} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("tạo user: %v", err)
		}
	}

	newCourse := func(slug string) (model.Course, model.Lesson, model.Lesson) {
		c := model.Course{InstructorID: teacher.ID, Title: "QA-r2a " + slug, Slug: "qa-r2a-" + slug + "-" + s, Price: decimal.Zero, Status: "published"}
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("tạo course: %v", err)
		}
		sec := model.Section{CourseID: c.ID, Title: "Chương 1", DisplayOrder: 1}
		if err := db.Create(&sec).Error; err != nil {
			t.Fatalf("tạo section: %v", err)
		}
		video := model.Lesson{SectionID: sec.ID, Title: "Bài video", DisplayOrder: 1, IsMandatory: true}
		exercise := model.Lesson{SectionID: sec.ID, Title: "Bài tập", DisplayOrder: 2, IsMandatory: true}
		for _, l := range []*model.Lesson{&video, &exercise} {
			if err := db.Create(l).Error; err != nil {
				t.Fatalf("tạo lesson: %v", err)
			}
		}
		// Video ngoài hệ thống (không có video_uploads) khai đúng độ dài thật 5s — đúng dạng seed demo.
		url := "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"
		contents := []model.LessonContent{
			{LessonID: video.ID, Type: "video", VideoURL: &url, Duration: 5, IsMandatory: true, DisplayOrder: 1},
			{LessonID: exercise.ID, Type: "exercise", Duration: 1800, IsMandatory: true, DisplayOrder: 1},
		}
		for i := range contents {
			if err := db.Create(&contents[i]).Error; err != nil {
				t.Fatalf("tạo content: %v", err)
			}
		}
		return c, video, exercise
	}
	course, videoLesson, exerciseLesson := newCourse("done")
	otherCourse, _, _ := newCourse("todo")

	enrollRepo := repository.NewEnrollmentRepository(db)
	enrollSvc := NewEnrollmentService(enrollRepo, repository.NewCourseRepository(db),
		repository.NewLessonRepository(db), repository.NewVideoUploadRepository(db))
	certSvc := NewCertificateService(repository.NewCertificateRepository(db), enrollRepo, nil, nil)

	for _, c := range []model.Course{course, otherCourse} {
		if _, err := enrollSvc.Enroll(ctx, student.ID, c.ID); err != nil {
			t.Fatalf("enroll: %v", err)
		}
	}

	// A1: xem trọn video 5s -> hoàn thành.
	res, err := enrollSvc.UpdateLessonProgress(ctx, student.ID, videoLesson.ID,
		dto.UpdateLessonProgressDTO{PlayedRanges: dto.PlayedRangesDTO{{Start: 0, End: 5}}}, false)
	if err != nil {
		t.Fatalf("progress video: %v", err)
	}
	if res.Status != "completed" || res.CourseCompleted {
		t.Fatalf("sau bài video: status=%q course_completed=%v, muốn completed/false", res.Status, res.CourseCompleted)
	}

	// A2: bài tập không có video — client gửi completed, server chấp nhận.
	done := "completed"
	res, err = enrollSvc.UpdateLessonProgress(ctx, student.ID, exerciseLesson.ID,
		dto.UpdateLessonProgressDTO{Status: &done}, false)
	if err != nil {
		t.Fatalf("progress bài tập: %v", err)
	}
	if res.Status != "completed" || !res.CourseCompleted {
		t.Fatalf("sau bài tập: status=%q course_completed=%v, muốn completed/true", res.Status, res.CourseCompleted)
	}

	// A4: chứng chỉ tự cấp cho khoá đã xong, đúng 1 lần dù đọc nhiều lần; khoá chưa xong thì không.
	for i := 0; i < 2; i++ {
		list, err := certSvc.GetMyCertificates(ctx, student.ID, 1, 10)
		if err != nil {
			t.Fatalf("GetMyCertificates lần %d: %v", i+1, err)
		}
		if list.Total != 1 || len(list.Data) != 1 || list.Data[0].CourseID != course.ID {
			t.Fatalf("lần %d: total=%d data=%+v, muốn đúng 1 chứng chỉ của khoá %s", i+1, list.Total, list.Data, course.ID)
		}
	}
	var n int64
	if err := db.Model(&model.Certificate{}).Where("user_id = ?", student.ID).Count(&n).Error; err != nil {
		t.Fatalf("đếm certificates: %v", err)
	}
	if n != 1 {
		t.Fatalf("certificates trong DB = %d, muốn 1 (idempotent)", n)
	}
}

// TestIssueMissingCertificates_Postgres_ChiCapChoGhiDanhDaHoanThanh: cấp bù cho người đã hoàn thành
// TRƯỚC bản vá (completed_at có sẵn), không cấp cho ghi danh chưa xong.
func TestIssueMissingCertificates_Postgres_ChiCapChoGhiDanhDaHoanThanh(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()
	s := uuid.NewString()[:8]

	teacher := model.User{Email: "qa-r2a-t2-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-r2a-t2-" + s}
	student := model.User{Email: "qa-r2a-s2-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-r2a-s2-" + s}
	for _, u := range []*model.User{&teacher, &student} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("tạo user: %v", err)
		}
	}
	completedAt := time.Now().Add(-48 * time.Hour)
	var courseIDs []uuid.UUID
	for i, done := range []bool{true, false} {
		c := model.Course{InstructorID: teacher.ID, Title: "QA-r2a backfill", Slug: "qa-r2a-bf-" + s + "-" + string(rune('a'+i)), Price: decimal.Zero}
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("tạo course: %v", err)
		}
		e := model.Enrollment{UserID: student.ID, CourseID: c.ID, EnrolledAt: completedAt}
		if done {
			e.CompletedAt = &completedAt
			e.ProgressPercent = decimal.NewFromInt(100)
		}
		if err := db.Create(&e).Error; err != nil {
			t.Fatalf("tạo enrollment: %v", err)
		}
		courseIDs = append(courseIDs, c.ID)
	}

	certSvc := NewCertificateService(repository.NewCertificateRepository(db), repository.NewEnrollmentRepository(db), nil, nil)
	list, err := certSvc.GetMyCertificates(ctx, student.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetMyCertificates: %v", err)
	}
	if list.Total != 1 || list.Data[0].CourseID != courseIDs[0] {
		t.Fatalf("total=%d data=%+v, muốn đúng 1 chứng chỉ cho khoá đã hoàn thành %s", list.Total, list.Data, courseIDs[0])
	}
}
