package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// Review PR #81, MAJOR-1 (dò tài khoản): email học sinh thật, email không tồn tại và email không
// phải học sinh phải nhận phản hồi GIỐNG HỆT nhau ở mọi trạng thái hạn mức — trong hạn mức, khi
// gửi trùng, và khi đã hết hạn mức. Mọi lần gửi (kể cả lần lỗi) đều bị tính.
func TestParentLink_Postgres_KhongDoDuocEmailHocSinhOMoiTrangThaiHanMuc(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	targets := func() []string {
		return []string{
			f.user("child", "STUDENT").Email,
			"khong-ton-tai-" + uuid.NewString()[:8] + "@40study.test",
			f.user("teacher", "TEACHER").Email,
		}
	}
	first := targets()

	// 1. Trong hạn mức: cả 3 đều tạo yêu cầu pending cùng một hình dạng.
	var shapes []dto.ParentLinkRequestDto
	for _, email := range first {
		out, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
		if err != nil {
			t.Fatalf("gửi tới %s: %v", email, err)
		}
		if out.StudentEmail != strings.ToLower(email) {
			t.Fatalf("student_email = %q, muốn %q", out.StudentEmail, strings.ToLower(email))
		}
		s := *out
		s.ID, s.CreatedAt, s.StudentEmail = "", "", ""
		shapes = append(shapes, s)
	}
	for i := range shapes {
		if shapes[i].Status != model.ParentLinkRequestStatusPending || shapes[i].Student != nil || shapes[i] != shapes[0] {
			t.Fatalf("phản hồi khác nhau theo loại email: %+v", shapes)
		}
	}

	// 2. Gửi trùng: cùng một lỗi cho cả 3.
	sameError(t, "gửi trùng", "LINK_REQUEST_PENDING", func(email string) error {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
		return err
	}, first)

	// 3. Lần gửi lỗi cũng bị tính: đã dùng 6 lượt, 4 lượt LINK_SELF đưa tới đúng hạn mức.
	for i := 0; i < ParentLinkDailyLimit-2*len(first); i++ {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(parent.Email))
		wantLinkCode(t, err, "LINK_SELF")
	}

	// 4. Hết hạn mức: email mới thuộc cả 3 loại, và cả email đang chờ, nhận cùng một 429.
	sameError(t, "hết hạn mức", "LINK_REQUEST_DAILY_LIMIT", func(email string) error {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
		return err
	}, append(targets(), first[0]))

	var stored int64
	f.db.Model(&model.ParentLinkRequest{}).Where("parent_user_id = ?", parent.ID).Count(&stored)
	if stored != int64(len(first)) {
		t.Fatalf("hết hạn mức mà vẫn lưu yêu cầu: %d dòng, muốn %d", stored, len(first))
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
