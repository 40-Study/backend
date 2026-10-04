package service

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// Màn chấm bài hiện họ tên học viên: bài nộp phải mang full_name của người nộp (không chỉ tên đăng nhập),
// nếu không panel chấm hiện username tới khi chấm xong (QA W2-C).
func TestSubmissionToResponseDTO_FullName(t *testing.T) {
	s := &SubmissionService{}
	userID := uuid.New()
	fullName := "Lê Văn C"

	withName := s.toResponseDTO(model.Submission{
		UserID: userID,
		User:   &model.User{BaseModel: model.BaseModel{ID: userID}, UserName: "student1", FullName: &fullName},
	})
	if withName.User == nil || withName.User.FullName != fullName {
		t.Fatalf("user.full_name = %+v, muốn %q", withName.User, fullName)
	}
	if withName.User.Username != "student1" {
		t.Errorf("username = %q, muốn giữ nguyên student1", withName.User.Username)
	}

	// Chưa nhập họ tên: để trống (omitempty), web tự rơi về username.
	noName := s.toResponseDTO(model.Submission{
		UserID: userID,
		User:   &model.User{BaseModel: model.BaseModel{ID: userID}, UserName: "student2"},
	})
	if noName.User == nil || noName.User.FullName != "" {
		t.Fatalf("user không có họ tên phải trả full_name rỗng, được %+v", noName.User)
	}
}
