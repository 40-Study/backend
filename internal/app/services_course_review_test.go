package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/testutil/pgtest"
)

// QA2 ADM-N1: wireCourseReviewNotifications PHẢI nối CourseReviewService với NotificationService. Bỏ lời gọi (hoặc
// bỏ nội dung hàm) thì admin từ chối khoá mà giảng viên không nhận dòng notifications nào. Kiểm bằng HÀNH VI thật vì
// notifier là field private.
func TestWireCourseReviewNotifications_RejectWritesTeacherNotification(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	repos := InitRepositories(db)
	s := &Services{
		CourseReview: service.NewCourseReviewService(repos.CourseReview),
		Notification: service.NewNotificationService(repos.Notification, socket.NewNotifier(socket.NewHub())),
	}
	wireCourseReviewNotifications(s)

	teacher := model.User{Email: "qa-wire-n1-teacher@wire.test", UserName: "wire-n1-teacher", PasswordHash: "x"}
	admin := model.User{Email: "qa-wire-n1-admin@wire.test", UserName: "wire-n1-admin", PasswordHash: "x"}
	for _, u := range []*model.User{&teacher, &admin} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	course := model.Course{InstructorID: teacher.ID, Title: "QA wire N1", Slug: "qa-wire-n1-" + uuid.NewString()[:8], Status: model.CourseStatusPendingReview}
	if err := db.Create(&course).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := s.CourseReview.Reject(context.Background(), course.ID, admin.ID, "Thiếu mô tả khoá"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	var n int64
	db.Model(&model.Notification{}).Where("user_id = ? AND content LIKE ?", teacher.ID, "%Thiếu mô tả khoá%").Count(&n)
	if n != 1 {
		t.Fatalf("giảng viên nhận %d thông báo có lý do từ chối, muốn 1", n)
	}
}
