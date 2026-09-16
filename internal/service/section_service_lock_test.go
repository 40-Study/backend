package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Test cho SectionService.GetAllSections (Phase 1 §2, "fetcher server" ma trang hoc dung) —
// khang dinh curriculum tra dung locked/lock_reason/progress THEO NGUOI DANG XEM, chu khong
// phai mot ban curriculum "chung" khong phu thuoc ai goi.

type fakeSectionRepoForSections struct {
	repository.SectionRepositoryInterface
	sections []model.Section
}

func (f *fakeSectionRepoForSections) GetAllByCourseID(ctx context.Context, courseID uuid.UUID) ([]model.Section, error) {
	return f.sections, nil
}

type fakeCourseRepoForSections struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseRepoForSections) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

type fakeEnrollmentRepoForSections struct {
	repository.EnrollmentRepositoryInterface
	enrolled bool
	order    []repository.LessonOrderInfo
	progress map[uuid.UUID]*model.LessonProgress

	// onlyEnrolledUserID (T-1, review vòng 2): khi khác uuid.Nil, GetByUserAndCourse chỉ trả
	// "đã enroll" cho ĐÚNG userID này — mọi userID khác (kể cả uuid.Nil) đều "chưa enroll". Trước
	// bản vá, fake này trả CÙNG MỘT kết quả bất kể userID truyền vào là ai, nên
	// SectionService.GetAllSections lỡ tính khoá cho uuid.Nil thay vì người đang gọi (N3) vẫn
	// xanh — trường này tồn tại để ÍT NHẤT MỘT test ràng buộc kết quả vào ĐÚNG danh tính người gọi.
	onlyEnrolledUserID uuid.UUID
}

func (f *fakeEnrollmentRepoForSections) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	if f.onlyEnrolledUserID != uuid.Nil && userID != f.onlyEnrolledUserID {
		return nil, nil
	}
	if !f.enrolled {
		return nil, nil
	}
	return &model.Enrollment{UserID: userID, CourseID: courseID}, nil
}

func (f *fakeEnrollmentRepoForSections) GetLessonOrderInfoByCourseID(ctx context.Context, courseID uuid.UUID) ([]repository.LessonOrderInfo, error) {
	return f.order, nil
}

func (f *fakeEnrollmentRepoForSections) GetLessonProgressMapByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (map[uuid.UUID]*model.LessonProgress, error) {
	if f.progress == nil {
		return map[uuid.UUID]*model.LessonProgress{}, nil
	}
	return f.progress, nil
}

// buildTwoLessonSequentialCourse dung mot section co 2 bai (khong preview), khoa bat sequential
// — dung chung cho ca nhom test duoi day.
func buildTwoLessonSequentialCourse() (bai1, bai2 uuid.UUID, sections []model.Section, course *model.Course) {
	courseID := uuid.New()
	bai1, bai2 = uuid.New(), uuid.New()
	sections = []model.Section{
		{
			BaseModel: model.BaseModel{ID: uuid.New()},
			CourseID:  courseID,
			Title:     "Chuong 1",
			Lessons: []model.Lesson{
				{ID: bai1, Title: "Bai 1", DisplayOrder: 0},
				{ID: bai2, Title: "Bai 2", DisplayOrder: 1},
			},
		},
	}
	course = &model.Course{}
	course.ID = courseID
	course.Sequential = true
	return bai1, bai2, sections, course
}

func findLesson(sections []dto.SectionResponseDTO, id uuid.UUID) (found bool, locked bool, reason *string) {
	for _, sec := range sections {
		for _, les := range sec.Lessons {
			if les.ID == id {
				return true, les.Locked, les.LockReason
			}
		}
	}
	return false, false, nil
}

// TestSectionService_GetAllSections_ChuaEnroll_MoiBaiKhoa: nguoi dung chua enroll thi ca hai
// bai (khong phai preview) deu khoa, ly do not_enrolled.
func TestSectionService_GetAllSections_ChuaEnroll_MoiBaiKhoa(t *testing.T) {
	bai1, bai2, sections, course := buildTwoLessonSequentialCourse()
	svc := NewSectionService(
		&fakeSectionRepoForSections{sections: sections},
		&fakeCourseRepoForSections{course: course},
		&fakeEnrollmentRepoForSections{
			enrolled: false,
			order:    []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	result, err := svc.GetAllSections(context.Background(), course.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	ok, locked, reason := findLesson(result, bai1)
	if !ok || !locked || reason == nil || *reason != LockReasonNotEnrolled {
		t.Fatalf("bai1: found=%v locked=%v reason=%v, muon locked=true reason=not_enrolled", ok, locked, reason)
	}
	ok, locked, reason = findLesson(result, bai2)
	if !ok || !locked || reason == nil || *reason != LockReasonNotEnrolled {
		t.Fatalf("bai2: found=%v locked=%v reason=%v, muon locked=true reason=not_enrolled", ok, locked, reason)
	}
}

// TestSectionService_GetAllSections_DaEnroll_BaiTruocChuaXongThiBaiSauKhoa: khoa sequential,
// da enroll, bai 1 dang hoc do dang (chua completed) -> bai 2 phai khoa previous_incomplete;
// bai 1 (khong co bai truoc) phai mo.
func TestSectionService_GetAllSections_DaEnroll_BaiTruocChuaXongThiBaiSauKhoa(t *testing.T) {
	bai1, bai2, sections, course := buildTwoLessonSequentialCourse()
	svc := NewSectionService(
		&fakeSectionRepoForSections{sections: sections},
		&fakeCourseRepoForSections{course: course},
		&fakeEnrollmentRepoForSections{
			enrolled: true,
			order:    []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
			progress: map[uuid.UUID]*model.LessonProgress{
				bai1: {Status: "in_progress"},
			},
		},
	)

	result, err := svc.GetAllSections(context.Background(), course.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	if ok, locked, _ := findLesson(result, bai1); !ok || locked {
		t.Fatalf("bai1 phai mo (khong co bai truoc), found=%v locked=%v", ok, locked)
	}
	ok, locked, reason := findLesson(result, bai2)
	if !ok || !locked || reason == nil || *reason != LockReasonPreviousIncomplete {
		t.Fatalf("bai2: found=%v locked=%v reason=%v, muon locked=true reason=previous_incomplete", ok, locked, reason)
	}
}

// TestSectionService_GetAllSections_BaiTruocDaCompleted_BaiSauMo: bai 1 da completed thi bai 2
// phai mo, va progress tra ve dung watched_pct/status da luu cua bai 1.
func TestSectionService_GetAllSections_BaiTruocDaCompleted_BaiSauMo(t *testing.T) {
	bai1, bai2, sections, course := buildTwoLessonSequentialCourse()
	svc := NewSectionService(
		&fakeSectionRepoForSections{sections: sections},
		&fakeCourseRepoForSections{course: course},
		&fakeEnrollmentRepoForSections{
			enrolled: true,
			order:    []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
			progress: map[uuid.UUID]*model.LessonProgress{
				bai1: {Status: "completed"},
			},
		},
	)

	result, err := svc.GetAllSections(context.Background(), course.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	if ok, locked, _ := findLesson(result, bai2); !ok || locked {
		t.Fatalf("bai2 phai mo khi bai1 da completed, found=%v locked=%v", ok, locked)
	}

	// Kiem tra progress dinh kem dung la CUA bai1, khong phai gia tri mac dinh.
	var bai1Progress *string
	for _, sec := range result {
		for _, les := range sec.Lessons {
			if les.ID == bai1 {
				if les.Progress == nil {
					t.Fatal("progress cua bai1 khong duoc nil")
				}
				s := les.Progress.Status
				bai1Progress = &s
			}
		}
	}
	if bai1Progress == nil || *bai1Progress != "completed" {
		t.Fatalf("progress.status cua bai1 = %v, muon \"completed\"", bai1Progress)
	}
}

// TestSectionService_GetAllSections_KhoaKhongSequential_KhongKhoaBaiNao: khoa KHONG bat
// sequential thi ca hai bai deu mo, du bai 1 chua hoc gi.
func TestSectionService_GetAllSections_KhoaKhongSequential_KhongKhoaBaiNao(t *testing.T) {
	bai1, bai2, sections, course := buildTwoLessonSequentialCourse()
	course.Sequential = false
	svc := NewSectionService(
		&fakeSectionRepoForSections{sections: sections},
		&fakeCourseRepoForSections{course: course},
		&fakeEnrollmentRepoForSections{
			enrolled: true,
			order:    []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	result, err := svc.GetAllSections(context.Background(), course.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	if ok, locked, _ := findLesson(result, bai2); !ok || locked {
		t.Fatalf("khoa khong sequential: bai2 phai mo, found=%v locked=%v", ok, locked)
	}
}

// TestSectionService_GetAllSections_TinhKhoaTheoDungNguoiGoi_KhongPhaiUuidNil (T-1, review vòng
// 2): fake CHỈ coi "đã enroll" đối với ĐÚNG userID được truyền vào GetAllSections — nếu service
// lỡ tính khoá cho uuid.Nil (hoặc bất kỳ userID nào khác) thay vì người đang gọi, fake sẽ trả
// "chưa enroll" và bài sẽ hiện khoá sai. Đóng khoảng hở N3 mà reviewer chỉ ra: mọi test khác
// dùng cùng một fake trả cùng kết quả bất kể userID, nên hoán userID thành uuid.Nil vẫn xanh.
func TestSectionService_GetAllSections_TinhKhoaTheoDungNguoiGoi_KhongPhaiUuidNil(t *testing.T) {
	bai1, bai2, sections, course := buildTwoLessonSequentialCourse()
	course.Sequential = false // don gian hoa: chi can biet "mo hay khoa vi not_enrolled"
	nguoiGoiThat := uuid.New()
	svc := NewSectionService(
		&fakeSectionRepoForSections{sections: sections},
		&fakeCourseRepoForSections{course: course},
		&fakeEnrollmentRepoForSections{
			enrolled:           true,
			onlyEnrolledUserID: nguoiGoiThat,
			order:              []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	result, err := svc.GetAllSections(context.Background(), course.ID, nguoiGoiThat, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	// nguoiGoiThat DA enroll: ca hai bai phai MO. Neu service lo tinh khoa cho uuid.Nil (hoac
	// mot userID khac) thay vi nguoiGoiThat, fake se tra "chua enroll" va bai se bi khoa sai.
	if ok, locked, reason := findLesson(result, bai1); !ok || locked {
		t.Fatalf("bai1: found=%v locked=%v reason=%v, muon mo (dung userID cua nguoi goi da enroll)", ok, locked, reason)
	}
	if ok, locked, reason := findLesson(result, bai2); !ok || locked {
		t.Fatalf("bai2: found=%v locked=%v reason=%v, muon mo (dung userID cua nguoi goi da enroll)", ok, locked, reason)
	}
}
