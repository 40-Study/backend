package handler

// Test cho P1 QA 260927 teacher (2 phát hiện, cùng 1 handler course_handler.go):
//   - GetAllCourses (route công khai, KHÔNG auth): status client gửi lên PHẢI bị bỏ qua, luôn
//     ép "published" — trước bản vá này client tuỳ ý truyền status (hoặc để trống) và không nơi
//     nào lọc, nên khoá "draft" của MỌI giảng viên lộ ra trang /courses công khai.
//   - GetMyCourses (route "/courses/mine", có auth): PHẢI lọc theo instructor_id = user đang
//     đăng nhập — trước bản vá này web gọi "?mine=true" nhưng handler không đọc "mine" hay
//     "instructor_id" nên trả TOÀN BỘ khoá của MỌI giảng viên ("Khóa học của tôi" trộn lẫn khoá
//     người khác).

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubCourseServiceForList struct {
	service.CourseServiceInterface
	gotParams dto.CourseFilterParams
}

func (s *stubCourseServiceForList) GetAllCourses(_ context.Context, params dto.CourseFilterParams) (*dto.CourseListResponseDTO, error) {
	s.gotParams = params
	return &dto.CourseListResponseDTO{Courses: []dto.CourseResponseDTO{}, Total: 0}, nil
}

func TestGetAllCourses_LuonEpStatusPublished(t *testing.T) {
	svc := &stubCourseServiceForList{}
	h := NewCourseHandler(svc, nil)

	app := fiber.New()
	app.Get("/courses", h.GetAllCourses)

	req := httptest.NewRequest("GET", "/courses?status=draft", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", resp.StatusCode)
	}
	if svc.gotParams.Status != "published" {
		t.Fatalf("status truyen xuong service = %q, muon luon la \"published\" (bi ghi de boi query status=draft cua client) — day chinh la lo ro draft P1 QA 260927 teacher",
			svc.gotParams.Status)
	}
}

func TestGetMyCourses_LocTheoInstructorDangDangNhap(t *testing.T) {
	svc := &stubCourseServiceForList{}
	h := NewCourseHandler(svc, nil)

	caller := uuid.New()
	app := mountWithCaller("GET", "/courses/mine", caller, h.GetMyCourses)

	req := httptest.NewRequest("GET", "/courses/mine", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", resp.StatusCode)
	}
	if svc.gotParams.InstructorID == nil {
		t.Fatal("InstructorID = nil, muon duoc dat bang user_id dang dang nhap (khong duoc tra toan bo khoa cua moi giang vien)")
	}
	if *svc.gotParams.InstructorID != caller {
		t.Fatalf("InstructorID = %s, muon = %s (nguoi dang dang nhap)", svc.gotParams.InstructorID, caller)
	}
}
