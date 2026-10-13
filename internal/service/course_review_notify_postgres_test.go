package service

// QA2 ADM-N1 — admin duyệt / từ chối khoá học thì giảng viên chủ khoá phải nhận thông báo (kèm lý do khi từ
// chối). Chạy trên Postgres thật với NotificationService thật để chứng minh dòng `notifications` được ghi (triệu
// chứng gốc: 0 dòng sau khi từ chối).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/socket"
)

type courseReviewNotifyFixture struct {
	f       *contestFixture
	svc     *CourseReviewService
	teacher uuid.UUID
	admin   uuid.UUID
}

func newCourseReviewNotifyFixture(t *testing.T) *courseReviewNotifyFixture {
	f := newContestFixture(t)
	svc := NewCourseReviewService(repository.NewCourseReviewRepository(f.db))
	svc.SetNotifier(NewNotificationService(repository.NewNotificationRepository(f.db), socket.NewNotifier(socket.NewHub())))
	return &courseReviewNotifyFixture{f: f, svc: svc, teacher: f.user("teacher"), admin: f.user("admin")}
}

func (n *courseReviewNotifyFixture) pendingCourse(title string) uuid.UUID {
	n.f.t.Helper()
	c := model.Course{InstructorID: n.teacher, Title: title, Slug: "adm-n1-" + uuid.NewString(), Status: model.CourseStatusPendingReview}
	if err := n.f.db.Create(&c).Error; err != nil {
		n.f.t.Fatalf("tạo khoá: %v", err)
	}
	return c.ID
}

func (n *courseReviewNotifyFixture) notices(userID uuid.UUID) []model.Notification {
	n.f.t.Helper()
	var out []model.Notification
	if err := n.f.db.Where("user_id = ?", userID).Order("created_at").Find(&out).Error; err != nil {
		n.f.t.Fatalf("đọc thông báo: %v", err)
	}
	return out
}

func TestCourseReview_ApproveNotifiesTeacher(t *testing.T) {
	n := newCourseReviewNotifyFixture(t)
	courseID := n.pendingCourse("Khoá Python cơ bản")
	if _, err := n.svc.Approve(context.Background(), courseID, n.admin); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	got := n.notices(n.teacher)
	if len(got) != 1 {
		t.Fatalf("giảng viên nhận %d thông báo sau khi duyệt, muốn 1", len(got))
	}
	notice := got[0]
	if notice.NotificationType != "course_update" || notice.ReferenceType == nil || *notice.ReferenceType != CourseReviewNoticeRefType ||
		notice.ReferenceID == nil || *notice.ReferenceID != courseID {
		t.Errorf("thông báo duyệt: type=%q ref=%v/%v, muốn course_update + %s/%s", notice.NotificationType, notice.ReferenceType, notice.ReferenceID, CourseReviewNoticeRefType, courseID)
	}
	if !strings.Contains(notice.Title, "duyệt") || !strings.Contains(notice.Content, "Khoá Python cơ bản") {
		t.Errorf("thông báo duyệt thiếu tên khoá / chữ 'duyệt': %q / %q", notice.Title, notice.Content)
	}
	if len(n.notices(n.admin)) != 0 {
		t.Error("admin không được nhận thông báo của chính mình")
	}
}

func TestCourseReview_RejectNotifiesTeacherWithReason(t *testing.T) {
	n := newCourseReviewNotifyFixture(t)
	courseID := n.pendingCourse("Khoá Go nâng cao")
	if _, err := n.svc.Reject(context.Background(), courseID, n.admin, "  Video bài 2 mất tiếng  "); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	got := n.notices(n.teacher)
	if len(got) != 1 {
		t.Fatalf("giảng viên nhận %d thông báo sau khi từ chối, muốn 1", len(got))
	}
	if !strings.Contains(got[0].Title, "từ chối") || !strings.Contains(got[0].Content, "Video bài 2 mất tiếng") ||
		!strings.Contains(got[0].Content, "Khoá Go nâng cao") {
		t.Errorf("thông báo từ chối thiếu lý do / tên khoá: %q / %q", got[0].Title, got[0].Content)
	}
}

// Duyệt/từ chối bị chặn (sai trạng thái, thiếu lý do) thì KHÔNG gửi thông báo nào.
func TestCourseReview_NoNoticeWhenActionRejected(t *testing.T) {
	n := newCourseReviewNotifyFixture(t)
	draft := model.Course{InstructorID: n.teacher, Title: "Nháp", Slug: "adm-n1-" + uuid.NewString(), Status: model.CourseStatusDraft}
	if err := n.f.db.Create(&draft).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := n.svc.Approve(context.Background(), draft.ID, n.admin); err == nil {
		t.Fatal("duyệt khoá nháp phải lỗi")
	}
	pending := n.pendingCourse("Chờ duyệt")
	if _, err := n.svc.Reject(context.Background(), pending, n.admin, "   "); !errors.Is(err, ErrReviewReasonRequired) {
		t.Fatalf("từ chối không lý do: err=%v", err)
	}
	if got := n.notices(n.teacher); len(got) != 0 {
		t.Fatalf("hành động bị chặn vẫn gửi %d thông báo", len(got))
	}
}

type failingNotifier struct{ calls int }

func (f *failingNotifier) SendNotification(dto.CreateNotificationDTO) error {
	f.calls++
	return errors.New("kênh thông báo hỏng")
}

// Gửi thông báo lỗi chỉ được log: trạng thái khoá đã commit nên Approve vẫn phải thành công.
func TestCourseReview_NotifierFailureDoesNotUndoDecision(t *testing.T) {
	n := newCourseReviewNotifyFixture(t)
	bad := &failingNotifier{}
	n.svc.SetNotifier(bad)
	courseID := n.pendingCourse("Khoá lỗi thông báo")
	res, err := n.svc.Approve(context.Background(), courseID, n.admin)
	if err != nil || res.Status != model.CourseStatusPublished {
		t.Fatalf("Approve khi notifier hỏng: res=%v err=%v, muốn vẫn published", res, err)
	}
	if bad.calls != 1 {
		t.Fatalf("notifier được gọi %d lần, muốn 1", bad.calls)
	}
}
