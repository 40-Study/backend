package service

// Test cho C-1 (review vòng 2, BLOCKER): HAI đường nội dung còn hở sau bản vá B-2 —
//   - GET /api/courses/:id            -> CourseService.GetCourseByID
//   - GET /api/sections/:id/lessons   -> LessonService.GetAllLessons
// Trước bản vá, cả hai trả thẳng contents[].video_url cho BẤT KỲ ai đã đăng nhập, không kiểm
// enroll, không kiểm khoá. Nhóm test dưới đây khẳng định bất biến của bản vá:
//   - bài BỊ KHOÁ  => contents rỗng (không có video_url/video_hls_url/video_upload_id)
//   - bài MỞ       => contents đầy đủ
//   - chủ khoá học / admin (CAO-4) không bao giờ bị khoá nên vẫn thấy contents
// Mỗi test đều pin được một đột biến cụ thể: bỏ nhánh `if locked` (truyền les.Contents vô điều
// kiện) làm test khoá ĐỎ; bỏ tham số bypass làm test chủ sở hữu ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeCourseDetailRepoForC1 phục vụ CourseService.GetCourseByID (GetDetailByID, đã preload
// Sections.Lessons.Contents như repository thật).
type fakeCourseDetailRepoForC1 struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseDetailRepoForC1) GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

// fakeCourseRepoForC1 phục vụ LessonService.GetAllLessons (tra course từ section.CourseID).
type fakeCourseRepoForC1 struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseRepoForC1) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

// fakeSectionRepoForC1: GetByID kèm CourseID để LessonService suy ra khoá học.
type fakeSectionRepoForC1 struct {
	repository.SectionRepositoryInterface
	section *model.Section
}

func (f *fakeSectionRepoForC1) GetByID(ctx context.Context, id uuid.UUID) (*model.Section, error) {
	return f.section, nil
}

// fakeLessonRepoForC1: lessons kèm Contents thật (trường hợp duy nhất sinh ra video_url).
type fakeLessonRepoForC1 struct {
	repository.LessonRepositoryInterface
	lessons []model.Lesson
}

func (f *fakeLessonRepoForC1) GetAllBySectionID(ctx context.Context, sectionID uuid.UUID) ([]model.Lesson, error) {
	return f.lessons, nil
}

// buildCourseWithTwoLessons: một section, hai bài (không preview), bài 2 có video_url thật (dạng
// /hls/{uuid}/... — chính chuỗi mà toLessonResponseDTO viết lại thành /api/hls/...). Trả về CẢ
// course (cho GetDetailByID) và section (cho sectionRepo.GetByID) vì hai đường C-1 đọc dữ liệu
// qua hai repository khác nhau.
func buildCourseWithTwoLessons(sequential bool, instructorID uuid.UUID) (bai1, bai2 uuid.UUID, course *model.Course, section *model.Section) {
	courseID := uuid.New()
	sectionID := uuid.New()
	bai1, bai2 = uuid.New(), uuid.New()

	videoURL := "/hls/11111111-2222-3333-4444-555555555555/master.m3u8"
	section = &model.Section{
		BaseModel: model.BaseModel{ID: sectionID},
		CourseID:  courseID,
		Title:     "Chuong 1",
		Lessons: []model.Lesson{
			{ID: bai1, SectionID: sectionID, Title: "Bai 1", DisplayOrder: 0},
			{ID: bai2, SectionID: sectionID, Title: "Bai 2", DisplayOrder: 1,
				Contents: []model.LessonContent{{Type: "video", VideoURL: &videoURL}}},
		},
	}

	course = &model.Course{
		InstructorID: instructorID,
		Sequential:   sequential,
		Sections:     []model.Section{*section},
	}
	course.ID = courseID
	return bai1, bai2, course, section
}

// TestGetCourseByID_ChuaEnroll_BaiKhoa_KhongLoContents (C-1 đường 1): người dùng CHƯA enroll gọi
// GET /courses/:id — mọi bài không phải preview bị khoá not_enrolled và contents phải RỖNG, dù
// repository đã preload Contents thật (dữ liệu thật, không phải nhánh chết).
func TestGetCourseByID_ChuaEnroll_BaiKhoa_KhongLoContents(t *testing.T) {
	bai1, bai2, course, _ := buildCourseWithTwoLessons(true, uuid.New())
	svc := NewCourseService(
		&fakeCourseDetailRepoForC1{course: course},
		nil, nil,
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: nil, // chua enroll
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	got, err := svc.GetCourseByID(context.Background(), course.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if len(got.Sections) != 1 || len(got.Sections[0].Lessons) != 2 {
		t.Fatalf("syllabus phai van co 1 section 2 bai, nhan duoc %d section", len(got.Sections))
	}

	for _, les := range got.Sections[0].Lessons {
		if !les.Locked {
			t.Errorf("bai %s: locked = false, muon true (chua enroll, bai khong preview)", les.ID)
		}
		if les.LockReason == nil || *les.LockReason != LockReasonNotEnrolled {
			t.Errorf("bai %s: lock_reason = %v, muon %q", les.ID, les.LockReason, LockReasonNotEnrolled)
		}
		if len(les.Contents) != 0 {
			t.Errorf("bai %s: contents = %d phan tu — bai dang khoa KHONG duoc lo video_url", les.ID, len(les.Contents))
		}
		// video_url/video_hls_url/video_upload_id nam tren tung phan tu contents, nen contents
		// rong la du de khong con duong nao tra chung ra — kiem tra tuong minh de test doc ro
		// rang buoc ma khong phu thuoc vao chi mot phep dem.
		for _, ct := range les.Contents {
			if ct.VideoURL != nil || ct.VideoHLSURL != nil || ct.VideoUploadID != nil {
				t.Errorf("bai %s: content %s con video_url/video_hls_url/video_upload_id tren bai dang khoa", les.ID, ct.ID)
			}
		}
	}
}

// TestGetCourseByID_DaEnroll_BaiTruocXong_BaiSauMo_CoContents: đã enroll, bài 1 completed => bài 2
// MỞ và phải có contents đầy đủ — bản vá không được "khoá nhầm" người học hợp lệ.
func TestGetCourseByID_DaEnroll_BaiTruocXong_BaiSauMo_CoContents(t *testing.T) {
	bai1, bai2, course, _ := buildCourseWithTwoLessons(true, uuid.New())
	nguoiHoc := uuid.New()
	svc := NewCourseService(
		&fakeCourseDetailRepoForC1{course: course},
		nil, nil,
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: &model.Enrollment{UserID: nguoiHoc, CourseID: course.ID},
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
			progress: map[uuid.UUID]*model.LessonProgress{
				bai1: {Status: "completed"},
			},
		},
	)

	got, err := svc.GetCourseByID(context.Background(), course.ID, nguoiHoc, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}

	// Thu tu Lessons trong response bam dung thu tu repository tra ve: [0] = bai 1, [1] = bai 2.
	bai2DTO := got.Sections[0].Lessons[1]
	if bai2DTO.ID != bai2 {
		t.Fatalf("phan tu [1] la bai %s, muon bai 2 %s", bai2DTO.ID, bai2)
	}
	if bai2DTO.Locked {
		t.Error("bai 2 bi khoa du bai 1 da completed — nguoi hoc hop le bi chan oan")
	}
	if len(bai2DTO.Contents) != 1 {
		t.Fatalf("bai 2 mo nhung contents = %d, muon 1 (nguoi hoc duoc quyen xem video_url)", len(bai2DTO.Contents))
	}
}

// TestGetCourseByID_GiangVienSoHuu_ThayContents (C-1 + CAO-4): giảng viên sở hữu khoá học gọi
// GET /courses/:id — chưa enroll nhưng KHÔNG bị khoá, và trang quản lý khoá của họ cần
// `c.video_url` (web/src/app/(teacher)/courses/[id]/page.tsx) nên contents phải còn nguyên.
// Đây là test chặn phương án "truyền nil contents cho mọi người" — nếu ai đó chọn cách đó, test
// này ĐỎ.
func TestGetCourseByID_GiangVienSoHuu_ThayContents(t *testing.T) {
	giangVien := uuid.New()
	bai1, bai2, course, _ := buildCourseWithTwoLessons(true, giangVien)
	svc := NewCourseService(
		&fakeCourseDetailRepoForC1{course: course},
		nil, nil,
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: nil,
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	got, err := svc.GetCourseByID(context.Background(), course.ID, giangVien, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	for _, les := range got.Sections[0].Lessons {
		if les.Locked {
			t.Errorf("bai %s: giang vien so huu khoa bi khoa — CAO-4 bi vi pham", les.ID)
		}
	}
	if len(got.Sections[0].Lessons[1].Contents) != 1 {
		t.Fatal("giang vien so huu khong thay contents — UI quan ly khoa se mat video_url")
	}
}

// TestGetAllLessons_ChuaEnroll_BaiKhoa_KhongLoContents (C-1 đường 2): cùng bất biến, áp cho
// GET /sections/:id/lessons. Repository đã preload Contents, nên đây là dữ liệu thật.
func TestGetAllLessons_ChuaEnroll_BaiKhoa_KhongLoContents(t *testing.T) {
	bai1, bai2, course, section := buildCourseWithTwoLessons(true, uuid.New())
	svc := NewLessonService(
		&fakeLessonRepoForC1{lessons: section.Lessons},
		&fakeSectionRepoForC1{section: section},
		&fakeCourseRepoForC1{course: course},
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: nil,
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	got, err := svc.GetAllLessons(context.Background(), section.ID, uuid.New(), false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(result) = %d, muon 2 (metadata van phai duoc tra)", len(got))
	}
	for _, les := range got {
		if !les.Locked {
			t.Errorf("bai %s: locked = false, muon true (chua enroll)", les.ID)
		}
		if len(les.Contents) != 0 {
			t.Errorf("bai %s: contents = %d phan tu — bai dang khoa KHONG duoc lo video_url", les.ID, len(les.Contents))
		}
	}
}

// TestGetAllLessons_DaEnroll_MoThiCoContents_KhoaThiRong: người học hợp lệ vẫn nhận contents của
// bài mở; bài đang chờ bài trước (previous_incomplete) vẫn bị chặn nội dung.
func TestGetAllLessons_DaEnroll_MoThiCoContents_KhoaThiRong(t *testing.T) {
	bai1, bai2, course, section := buildCourseWithTwoLessons(true, uuid.New())
	nguoiHoc := uuid.New()

	// (1) bai 1 completed => bai 2 mo, contents con nguyen.
	svc := NewLessonService(
		&fakeLessonRepoForC1{lessons: section.Lessons},
		&fakeSectionRepoForC1{section: section},
		&fakeCourseRepoForC1{course: course},
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: &model.Enrollment{UserID: nguoiHoc, CourseID: course.ID},
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
			progress: map[uuid.UUID]*model.LessonProgress{
				bai1: {Status: "completed"},
			},
		},
	)
	got, err := svc.GetAllLessons(context.Background(), section.ID, nguoiHoc, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if got[0].Locked {
		t.Error("bai dau chuoi bi khoa — khong co bai truoc de doi hoan thanh")
	}
	if got[1].Locked {
		t.Error("bai 2 bi khoa du bai 1 da completed")
	}
	if len(got[1].Contents) != 1 {
		t.Fatalf("bai 2 mo nhung contents = %d, muon 1", len(got[1].Contents))
	}

	// (2) bai 1 con dang hoc => bai 2 khoa previous_incomplete, contents rong.
	svcChuaXong := NewLessonService(
		&fakeLessonRepoForC1{lessons: section.Lessons},
		&fakeSectionRepoForC1{section: section},
		&fakeCourseRepoForC1{course: course},
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: &model.Enrollment{UserID: nguoiHoc, CourseID: course.ID},
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
			progress: map[uuid.UUID]*model.LessonProgress{
				bai1: {Status: "in_progress"},
			},
		},
	)
	got2, err := svcChuaXong.GetAllLessons(context.Background(), section.ID, nguoiHoc, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if !got2[1].Locked || got2[1].LockReason == nil || *got2[1].LockReason != LockReasonPreviousIncomplete {
		t.Errorf("bai 2: locked = %v, reason = %v — muon khoa previous_incomplete", got2[1].Locked, got2[1].LockReason)
	}
	if len(got2[1].Contents) != 0 {
		t.Errorf("bai 2 (previous_incomplete): contents = %d, muon 0", len(got2[1].Contents))
	}
}

// TestGetAllLessons_Admin_KhongBiKhoa: admin hệ thống (isAdmin=true) không bị khoá dù chưa enroll.
func TestGetAllLessons_Admin_KhongBiKhoa(t *testing.T) {
	bai1, bai2, course, section := buildCourseWithTwoLessons(true, uuid.New())
	svc := NewLessonService(
		&fakeLessonRepoForC1{lessons: section.Lessons},
		&fakeSectionRepoForC1{section: section},
		&fakeCourseRepoForC1{course: course},
		&fakeEnrollmentRepoForLock{
			courseID:   course.ID,
			enrollment: nil,
			order:      []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}},
		},
	)

	got, err := svc.GetAllLessons(context.Background(), section.ID, uuid.New(), true)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	for _, les := range got {
		if les.Locked {
			t.Errorf("bai %s: admin bi khoa — CAO-4 bi vi pham", les.ID)
		}
	}
	if len(got[1].Contents) != 1 {
		t.Fatal("admin khong thay contents — khong kiem duyet duoc noi dung")
	}
}
