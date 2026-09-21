package handler

// Test cho R7 (code-reviewer-260919-1557): CreatePost truoc ban va nay chi BodyParser, KHONG
// bao gio goi utils.ValidateStruct — moi tag validate tren CreateForumPostDTO (required/min/
// oneof/uuid) la tag CHET. mountWithCaller/doJSON dung lai tu livestream_authz_handler_test.go
// (cung package).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// stubDiscussionServiceForValidation: CreatePost ghi lai co bi goi hay khong — neu validator
// hoat dong dung, mot request voi category khong hop le PHAI bi chan TRUOC KHI cham toi service.
type stubDiscussionServiceForValidation struct {
	service.DiscussionServiceInterface
	createCalled bool
}

func (s *stubDiscussionServiceForValidation) CreatePost(ctx context.Context, userID uuid.UUID, req dto.CreateForumPostDTO) (*dto.ForumPostResponseDTO, error) {
	s.createCalled = true
	return &dto.ForumPostResponseDTO{}, nil
}

func TestCreatePost_CategoryKhongHopLe_Bi400VaKhongGoiService(t *testing.T) {
	svc := &stubDiscussionServiceForValidation{}
	app := mountWithCaller("POST", "/discussions", uuid.New(), NewDiscussionHandler(svc).CreatePost)

	code := doJSON(t, app, "POST", "/discussions",
		`{"title":"tieu de hop le","content":"noi dung","category":"khong-ton-tai"}`)

	if code != 400 {
		t.Fatalf("category khong hop le phai bi 400 (validate oneof), nhan %d", code)
	}
	if svc.createCalled {
		t.Fatal("service.CreatePost bi goi du category khong hop le — validator khong chan request")
	}
}

// TestCreatePost_CategoryQna_HopLe (R7): "qna" la category ma web dang gui thuc te cho hoi dap
// theo bai (lesson-qna.tsx) — phai duoc validator chap nhan, khong duoc coi la "chua ton tai".
func TestCreatePost_CategoryQna_HopLe(t *testing.T) {
	svc := &stubDiscussionServiceForValidation{}
	app := mountWithCaller("POST", "/discussions", uuid.New(), NewDiscussionHandler(svc).CreatePost)

	code := doJSON(t, app, "POST", "/discussions",
		`{"title":"hoi bai nay","content":"noi dung","category":"qna"}`)

	if code != 201 {
		t.Fatalf("category=qna phai duoc chap nhan (201), nhan %d", code)
	}
	if !svc.createCalled {
		t.Fatal("service.CreatePost khong duoc goi du category hop le")
	}
}
