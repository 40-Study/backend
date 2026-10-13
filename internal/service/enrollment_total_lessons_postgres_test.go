package service

// Enrollment vừa tạo phải mang đúng tổng số bài bắt buộc ngay lúc ghi danh (không đợi lần tính lại
// tiến độ đầu tiên) trên MỌI đường tạo ghi danh: tự ghi danh khoá miễn phí (mới + ghi danh lại sau
// Unenroll) và hoàn tất đơn hàng (mới + mua lại). Postgres thật, schema tạm (orderFixture).
// Mutation đã kiểm: bỏ việc đếm trong EnrollmentRepository.Create hoặc RestoreAndReactivate làm test tương ứng đỏ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// courseWithLessons dựng khoá đã xuất bản có `mandatory` bài bắt buộc + 1 bài không bắt buộc (không được đếm).
func (f *orderFixture) courseWithLessons(price decimal.Decimal, mandatory int) uuid.UUID {
	f.t.Helper()
	teacher := f.user()
	s := uuid.NewString()
	c := model.Course{InstructorID: teacher, Title: "QA totals " + s[:8], Slug: "qa-totals-" + s, Price: price, Status: model.CourseStatusPublished}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo course: %v", err)
	}
	sec := model.Section{CourseID: c.ID, Title: "S1", DisplayOrder: 1}
	if err := f.db.Create(&sec).Error; err != nil {
		f.t.Fatalf("tạo section: %v", err)
	}
	f.addLessons(sec.ID, mandatory, true)
	f.addLessons(sec.ID, 1, false)
	return c.ID
}

func (f *orderFixture) addLessons(sectionID uuid.UUID, n int, mandatory bool) {
	f.t.Helper()
	for i := 0; i < n; i++ {
		l := model.Lesson{SectionID: sectionID, Title: "L-" + uuid.NewString()[:6], DisplayOrder: i + 1}
		if err := f.db.Create(&l).Error; err != nil {
			f.t.Fatalf("tạo lesson: %v", err)
		}
		if !mandatory { // default:true của GORM nuốt false lúc Create → ghi lại bằng SQL
			f.exec("UPDATE lessons SET is_mandatory = false WHERE id = ?", l.ID)
		}
	}
}

func (f *orderFixture) enrollmentService() *EnrollmentService {
	return NewEnrollmentService(
		repository.NewEnrollmentRepository(f.db),
		repository.NewCourseRepository(f.db),
		repository.NewLessonRepository(f.db),
		repository.NewVideoUploadRepository(f.db),
	)
}

// storedTotal đọc total_lessons từ DB (kể cả dòng soft-delete) — nguồn sự thật, không phải DTO.
func (f *orderFixture) storedTotal(userID, courseID uuid.UUID) int {
	f.t.Helper()
	var e model.Enrollment
	if err := f.db.Unscoped().Where("user_id = ? AND course_id = ?", userID, courseID).First(&e).Error; err != nil {
		f.t.Fatalf("đọc enrollment: %v", err)
	}
	return e.TotalLessons
}

func (f *orderFixture) fulfill(userID, courseID uuid.UUID) {
	f.t.Helper()
	order := &model.Order{UserID: userID}
	items := []model.OrderItem{{CourseID: courseID}}
	if err := completeOrderFulfillment(context.Background(), f.db, repository.NewEnrollmentRepository(f.db), repository.NewCourseRepository(f.db), nil, items, order); err != nil {
		f.t.Fatalf("completeOrderFulfillment: %v", err)
	}
}

func TestEnroll_FreeCourse_TotalLessonsSetAtEnrollTime(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.courseWithLessons(decimal.Zero, 4)

	resp, err := f.enrollmentService().Enroll(context.Background(), student, course)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if resp.TotalLessons != 4 || resp.CompletedLessons != 0 {
		t.Errorf("DTO lúc ghi danh: %d/%d bài, muốn 0/4", resp.CompletedLessons, resp.TotalLessons)
	}
	if got := f.storedTotal(student, course); got != 4 {
		t.Errorf("DB total_lessons = %d, muốn 4 (không đếm bài không bắt buộc)", got)
	}
}

func TestEnroll_ReEnrollAfterUnenroll_TotalLessonsRefreshed(t *testing.T) {
	f := newOrderFixture(t)
	svc := f.enrollmentService()
	student := f.user()
	course := f.courseWithLessons(decimal.Zero, 2)
	ctx := context.Background()

	if _, err := svc.Enroll(ctx, student, course); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if err := svc.Unenroll(ctx, student, course); err != nil {
		t.Fatalf("Unenroll: %v", err)
	}
	// Giảng viên thêm 3 bài trong lúc học viên đã rời khoá.
	var sec model.Section
	f.db.Where("course_id = ?", course).First(&sec)
	f.addLessons(sec.ID, 3, true)

	resp, err := svc.Enroll(ctx, student, course)
	if err != nil {
		t.Fatalf("re-Enroll: %v", err)
	}
	if resp.TotalLessons != 5 {
		t.Errorf("DTO re-enroll total = %d, muốn 5", resp.TotalLessons)
	}
	if got := f.storedTotal(student, course); got != 5 {
		t.Errorf("DB total_lessons sau re-enroll = %d, muốn 5", got)
	}
}

func TestOrderFulfillment_NewEnrollment_TotalLessonsSetAtEnrollTime(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.courseWithLessons(decimal.NewFromInt(499000), 4)

	f.fulfill(student, course)

	if got := f.storedTotal(student, course); got != 4 {
		t.Errorf("DB total_lessons sau hoàn tất đơn = %d, muốn 4", got)
	}
}

func TestOrderFulfillment_RepurchaseAfterUnenroll_TotalLessonsRefreshed(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.courseWithLessons(decimal.NewFromInt(499000), 2)
	f.fulfill(student, course)
	if err := f.enrollmentService().Unenroll(context.Background(), student, course); err != nil {
		t.Fatalf("Unenroll: %v", err)
	}
	var sec model.Section
	f.db.Where("course_id = ?", course).First(&sec)
	f.addLessons(sec.ID, 3, true)

	f.fulfill(student, course)

	if got := f.storedTotal(student, course); got != 5 {
		t.Errorf("DB total_lessons sau mua lại = %d, muốn 5", got)
	}
}
