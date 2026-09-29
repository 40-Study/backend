package service

// Test cho review S1 M1: subtitle_url do giảng viên nhập tự do và backend ký mọi .vtt trong bucket
// video, nên GV B trỏ subtitle_url sang .vtt của GV A là nhận URL ký đọc được transcript của A.
// Nay file phụ đề phải do chính người ghi tải lên (hoặc admin). Bỏ requireSubtitleUsable hoặc bỏ so
// sánh chủ ở RequireObjectUsableBy làm các test này ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

func subURL(key string) *string {
	s := "http://localhost:9000/videos/" + key
	return &s
}

func TestCreateContent_PhuDeCuaNguoiKhac_BiChan(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	req := dto.CreateLessonContentDTO{Type: "video", SubtitleURL: subURL(f.subKeyA)}
	_, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherB, false, req)
	if !errors.Is(err, ErrUploadNotOwned) {
		t.Fatalf("err = %v, muon ErrUploadNotOwned", err)
	}
	if f.repo.created != nil {
		t.Error("content van duoc tao du phu de cua nguoi khac")
	}
}

func TestCreateContent_PhuDeCuaChinhMinhVaAdmin_ThanhCong(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	if _, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherB, false, dto.CreateLessonContentDTO{Type: "video", SubtitleURL: subURL(f.subKeyB)}); err != nil {
		t.Fatalf("phu de cua chinh minh phai duoc: %v", err)
	}
	f2 := buildOwnerFixture(ownedByB, nil)
	if _, err := f2.svc.CreateContent(context.Background(), f2.lessonID, f2.admin, true, dto.CreateLessonContentDTO{Type: "video", SubtitleURL: subURL(f2.subKeyA)}); err != nil {
		t.Fatalf("admin phai duoc: %v", err)
	}
}

// File .vtt không có bản ghi upload nào -> 404, không cho gắn URL tuỳ ý trong bucket video.
func TestCreateContent_PhuDeKhongCoBanGhiUpload_NotFound(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	req := dto.CreateLessonContentDTO{Type: "video", SubtitleURL: subURL("videos/lesson_content/" + uuid.NewString() + "/x.vtt")}
	if _, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherB, false, req); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("err = %v, muon ErrUploadNotFound", err)
	}
}

func TestUpdateContent_DoiPhuDeSangCuaNguoiKhac_BiChan(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	_, err := f.svc.UpdateContent(context.Background(), f.repo.content.ID, f.teacherB, false, dto.UpdateLessonContentDTO{SubtitleURL: subURL(f.subKeyA)})
	if !errors.Is(err, ErrUploadNotOwned) {
		t.Fatalf("err = %v, muon ErrUploadNotOwned", err)
	}
	if f.repo.updated != nil {
		t.Error("content van bi sua du phu de cua nguoi khac")
	}
}

// Gửi lại nguyên phụ đề đang có (dữ liệu trước bản vá / form sửa) thì không kiểm lại.
func TestUpdateContent_GiuNguyenPhuDeCu_KhongKiemLai(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	f.repo.content.SubtitleURL = subURL(f.subKeyA) // GV khac gan tu truoc khi co ban va
	title := "doi tieu de"
	if _, err := f.svc.UpdateContent(context.Background(), f.repo.content.ID, f.teacherB, false, dto.UpdateLessonContentDTO{Title: &title, SubtitleURL: subURL(f.subKeyA)}); err != nil {
		t.Fatalf("giu nguyen phu de cu phai duoc: %v", err)
	}
}

// URL ngoài bucket video không ký được (sẽ bị bỏ khi đọc) nên không thuộc phạm vi kiểm chủ.
func TestUpdateContent_PhuDeURLNgoai_KhongKiemChu(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	ext := "https://example.com/sub.vtt"
	if _, err := f.svc.UpdateContent(context.Background(), f.repo.content.ID, f.teacherB, false, dto.UpdateLessonContentDTO{SubtitleURL: &ext}); err != nil {
		t.Fatalf("URL ngoai khong bi kiem chu: %v", err)
	}
}

// Đường thực tế của web: PUT /lessons/:id {subtitle_url}. Bị chặn TRƯỚC khi ghi bất cứ gì.
func TestUpdateLesson_PhuDeCuaNguoiKhac_BiChanTruocKhiGhi(t *testing.T) {
	withObjectBucket(t)
	f := buildOwnerFixture(ownedByB, nil)
	svc := NewLessonService(f.repo, &previewSectionRepoStub{section: f.section}, &previewCourseRepoStub2{course: f.course}, nil).WithUploadOwnership(f.uploads)

	_, err := svc.UpdateLesson(context.Background(), f.lessonID, f.teacherB, false, dto.UpdateLessonDTO{SubtitleURL: subURL(f.subKeyA)})
	if !errors.Is(err, ErrUploadNotOwned) {
		t.Fatalf("err = %v, muon ErrUploadNotOwned", err)
	}
	if f.repo.lessonUpdated || f.repo.updated != nil {
		t.Errorf("khong duoc ghi gi khi bi chan: lessonUpdated=%v contentUpdated=%v", f.repo.lessonUpdated, f.repo.updated != nil)
	}
}
