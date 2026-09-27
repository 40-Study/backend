package service

// Test cho P1 QA 260927 teacher: "Quản lý học viên" luôn trống dù giáo viên có khoá đang bán
// hàng nghìn học viên — root cause là truy vấn cũ chỉ đếm học viên đã được xếp vào một LỚP
// (student_classes), trong khi hầu hết giáo viên chưa từng tạo lớp. GetMyStudents nay đọc từ
// enrollments (EnrollmentRepository.GetByInstructor). Test này CHỈ kiểm tầng ánh xạ Go (đã có
// dữ liệu enrollment thật từ repo) — không đụng SQL thật (nằm ở internal/repository, không chạy
// được trên máy này do Smart App Control chặn binary test, xem báo cáo cuối).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type stubEnrollmentRepoForTeacherStudents struct {
	repository.EnrollmentRepositoryInterface
	rows  []repository.TeacherStudentRow
	total int64
}

func (s *stubEnrollmentRepoForTeacherStudents) GetByInstructor(_ context.Context, _ uuid.UUID, _, _ int) ([]repository.TeacherStudentRow, int64, error) {
	return s.rows, s.total, nil
}

type stubParentStudentRepoEmpty struct {
	repository.ParentStudentRepositoryInterface
}

func (s *stubParentStudentRepoEmpty) GetPrimaryParentByStudentID(_ context.Context, _ uuid.UUID) (*model.ParentStudentRelation, error) {
	return nil, nil
}

// TestGetMyStudents_TraVeHocVienTuEnrollment_KhongCanLop (P1 QA 260927 teacher): 1 học viên
// enroll thẳng vào khoá (KHÔNG thuộc lớp nào — ClassID/ClassName nil) vẫn phải xuất hiện trong
// danh sách. Trước bản vá này, một học viên như vậy KHÔNG BAO GIỜ lọt vào kết quả vì nguồn dữ
// liệu cũ chỉ đọc từ student_classes.
func TestGetMyStudents_TraVeHocVienTuEnrollment_KhongCanLop(t *testing.T) {
	teacherID := uuid.New()
	studentID := uuid.New()
	courseID := uuid.New()
	fullName := "Nguyen Van Test"

	enrollRepo := &stubEnrollmentRepoForTeacherStudents{
		rows: []repository.TeacherStudentRow{
			{
				StudentID:       studentID,
				FullName:        &fullName,
				UserName:        "nguyenvantest",
				Email:           "test@demo.com",
				EnrolledAt:      time.Now(),
				ProgressPercent: decimal.NewFromInt(0),
				CourseID:        courseID,
				CourseTitle:     "QA-Khoa test GV",
				// KHÔNG có lớp — đúng kịch bản QA (mọi khoá của teacher1 đều "Chưa có lớp nào").
				ClassID:   nil,
				ClassName: nil,
			},
		},
		total: 1,
	}

	svc := NewTeacherService(nil, enrollRepo, &stubParentStudentRepoEmpty{})

	got, err := svc.GetMyStudents(context.Background(), teacherID, 1, 20)
	if err != nil {
		t.Fatalf("GetMyStudents loi: %v", err)
	}
	if got.Total != 1 || len(got.Students) != 1 {
		t.Fatalf("muon 1 hoc vien, duoc Total=%d len=%d — day chinh la bug goc: hoc vien enroll thang vao khoa (khong qua lop) bi bo sot",
			got.Total, len(got.Students))
	}
	row := got.Students[0]
	if row.ID != studentID {
		t.Errorf("ID = %s, muon %s", row.ID, studentID)
	}
	if row.ClassID != nil {
		t.Error("ClassID phai nil khi hoc vien khong thuoc lop nao (khong duoc bia du lieu lop)")
	}
	if row.CourseID == nil || *row.CourseID != courseID {
		t.Error("CourseID phai duoc dien dung tu enrollment")
	}
}
