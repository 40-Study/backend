package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// Quyết định D8 đảo thiết kế chống dò của PR #81 (MAJOR-1): email học sinh thật tạo yêu cầu, còn
// email không tồn tại và email không phải học sinh nhận CÙNG một 404 và KHÔNG tạo dòng. Phần còn
// giữ nguyên: mọi lần gửi (kể cả lần 404 / lỗi) đều bị tính vào hạn mức ngày, và khi đã hết hạn mức
// mọi loại email nhận cùng một 429 (hạn mức được kiểm TRƯỚC khi tra email).
//
// Đây là test đã ĐẢO (không re-pin): nếu một email lạ lại tạo được dòng pending, test này đỏ.
func TestParentLink_Postgres_KhongDoDuocEmailHocSinhOMoiTrangThaiHanMuc(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	student := f.user("child", "STUDENT")
	ghostEmails := func() []string {
		return []string{
			"khong-ton-tai-" + uuid.NewString()[:8] + "@40study.test",
			f.user("teacher", "TEACHER").Email,
		}
	}
	ghosts := ghostEmails()

	// 1. Trong hạn mức: học sinh thật tạo yêu cầu pending; email lạ và email giáo viên cùng một 404.
	out, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(student.Email))
	if err != nil {
		t.Fatalf("gửi tới học sinh thật: %v", err)
	}
	if out.Status != model.ParentLinkRequestStatusPending || out.Student != nil || out.StudentEmail != strings.ToLower(student.Email) {
		t.Fatalf("yêu cầu tới học sinh thật sai: %+v", out)
	}
	sameError(t, "trong hạn mức", "STUDENT_NOT_FOUND", func(email string) error {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
		return err
	}, ghosts)
	if n := countLinkRows(t, f, parent.ID); n != 1 {
		t.Fatalf("email không phải học sinh vẫn tạo dòng: %d dòng, muốn 1", n)
	}

	// 2. Gửi lại: học sinh đang chờ là 409, email không phải học sinh vẫn là 404 (không có dòng nào).
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq(student.Email))
	wantLinkCode(t, err, "LINK_REQUEST_PENDING")
	sameError(t, "gửi lại", "STUDENT_NOT_FOUND", func(email string) error {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
		return err
	}, ghosts)

	// 3. Lần gửi lỗi cũng bị tính: đã dùng 2*(1+len(ghosts)) lượt, phần còn lại LINK_SELF đưa tới đúng hạn mức.
	used := 2 * (1 + len(ghosts))
	for i := 0; i < ParentLinkDailyLimit-used; i++ {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(parent.Email))
		wantLinkCode(t, err, "LINK_SELF")
	}

	// 4. Hết hạn mức: học sinh mới, email lạ, email giáo viên, và cả học sinh đang chờ, nhận cùng một 429.
	sameError(t, "hết hạn mức", "LINK_REQUEST_DAILY_LIMIT", func(email string) error {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
		return err
	}, append(append(ghostEmails(), f.user("child", "STUDENT").Email), student.Email))

	if n := countLinkRows(t, f, parent.ID); n != 1 {
		t.Fatalf("hết hạn mức mà vẫn lưu yêu cầu: %d dòng, muốn 1", n)
	}
}

// sameError khẳng định mọi email trả CÙNG một ParentLinkError (status, mã, thông điệp).
func sameError(t *testing.T, stage, code string, send func(email string) error, emails []string) {
	t.Helper()
	var ref *ParentLinkError
	for _, email := range emails {
		err := send(email)
		var le *ParentLinkError
		if !errors.As(err, &le) || le.Code != code {
			t.Fatalf("%s, %s: muốn %s, nhận %v", stage, email, code, err)
		}
		if ref == nil {
			ref = le
		} else if *le != *ref {
			t.Fatalf("%s: phản hồi khác nhau theo loại email: %+v vs %+v", stage, *le, *ref)
		}
	}
}
