package service

// Test cho S1 (QA 260929), câu hỏi #2 của chủ dự án: khi tạo/sửa nội dung bài học có video_url trỏ
// vào một upload nội bộ (/hls/{upload_id}), upload đó phải do CHÍNH người thao tác tải lên, hoặc
// người thao tác là admin. Trước đây API cấp URL ký theo upload_id trong video_url mà không hỏi ai là
// chủ upload — giảng viên B gắn upload_id của A vào bài của mình là xem/tải được video của A.
//
// Dùng VideoUploadService THẬT (chỉ thay repo) nên bỏ requireVideoUploadUsable hoặc bỏ so sánh chủ
// upload ở RequireUploadUsableBy đều làm các test này ĐỎ.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// uploadOwnRepo: upload theo id; id lạ trả đúng sentinel mà repo thật trả (ErrVideoUploadNotFound).
type uploadOwnRepo struct {
	repository.VideoUploadRepositoryInterface
	uploads map[uuid.UUID]*model.VideoUpload
	objects map[string]*model.VideoUpload // object_key -> upload (file phu de .vtt)
}

func (r *uploadOwnRepo) GetUploadByObjectKey(_ context.Context, key string) (*model.VideoUpload, error) {
	if u, ok := r.objects[key]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("%w: object_key=%s", repository.ErrVideoUploadNotFound, key)
}

func (r *uploadOwnRepo) GetUploadByID(_ context.Context, id uuid.UUID) (*model.VideoUpload, error) {
	if u, ok := r.uploads[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("%w: %s", repository.ErrVideoUploadNotFound, id)
}

// contentWriteRepo ghi lại content được tạo/sửa để khẳng định KHÔNG ghi gì khi bị chặn.
type contentWriteRepo struct {
	repository.LessonRepositoryInterface
	lesson  *model.Lesson
	content *model.LessonContent
	created *model.LessonContent
	updated *model.LessonContent
	// UpdateLesson: bai hoc co bi ghi hay khong
	lessonUpdated bool
}

func (r *contentWriteRepo) GetContentsByLessonID(context.Context, uuid.UUID) ([]model.LessonContent, error) {
	if r.content == nil {
		return nil, nil
	}
	return []model.LessonContent{*r.content}, nil
}
func (r *contentWriteRepo) Update(context.Context, *model.Lesson) error {
	r.lessonUpdated = true
	return nil
}

func (r *contentWriteRepo) GetByID(context.Context, uuid.UUID) (*model.Lesson, error) {
	return r.lesson, nil
}
func (r *contentWriteRepo) GetContentByID(context.Context, uuid.UUID) (*model.LessonContent, error) {
	return r.content, nil
}
func (r *contentWriteRepo) CreateContent(_ context.Context, c *model.LessonContent) error {
	r.created = c
	return nil
}
func (r *contentWriteRepo) UpdateContent(_ context.Context, c *model.LessonContent) error {
	r.updated = c
	return nil
}

type ownerFixture struct {
	svc      *LessonContentService
	repo     *contentWriteRepo
	lessonID uuid.UUID
	teacherA uuid.UUID // chủ upload uploadA
	teacherB uuid.UUID // giảng viên khác
	admin    uuid.UUID
	uploadA  uuid.UUID
	uploadB  uuid.UUID
	subKeyA  string // file phu de .vtt do teacherA tai len
	subKeyB  string // file phu de .vtt do teacherB tai len
	uploads  *VideoUploadService
	section  *model.Section
	course   *model.Course
}

func hlsURLFor(id uuid.UUID) *string {
	s := "/api/hls/" + id.String() + "/master.m3u8"
	return &s
}

// buildOwnerFixture: khoá do `courseOwner` sở hữu; uploadA thuộc teacherA, uploadB thuộc teacherB.
func buildOwnerFixture(courseOwner func(a, b uuid.UUID) uuid.UUID, existingVideo *string) *ownerFixture {
	f := &ownerFixture{
		lessonID: uuid.New(), teacherA: uuid.New(), teacherB: uuid.New(), admin: uuid.New(),
		uploadA: uuid.New(), uploadB: uuid.New(),
	}
	courseID, sectionID := uuid.New(), uuid.New()
	course := &model.Course{InstructorID: courseOwner(f.teacherA, f.teacherB), Status: model.CourseStatusDraft}
	course.ID = courseID
	section := &model.Section{CourseID: courseID}
	section.ID = sectionID
	lesson := &model.Lesson{ID: f.lessonID, SectionID: sectionID}

	content := &model.LessonContent{ID: uuid.New(), LessonID: f.lessonID, Type: "video", VideoURL: existingVideo}
	f.repo = &contentWriteRepo{lesson: lesson, content: content}

	f.subKeyA = "videos/lesson_content/" + uuid.NewString() + "/1_a.vtt"
	f.subKeyB = "videos/lesson_content/" + uuid.NewString() + "/1_b.vtt"
	uploads := &uploadOwnRepo{
		uploads: map[uuid.UUID]*model.VideoUpload{
			f.uploadA: {ID: f.uploadA, UserID: f.teacherA},
			f.uploadB: {ID: f.uploadB, UserID: f.teacherB},
		},
		objects: map[string]*model.VideoUpload{
			f.subKeyA: {ID: uuid.New(), UserID: f.teacherA, ObjectKey: f.subKeyA},
			f.subKeyB: {ID: uuid.New(), UserID: f.teacherB, ObjectKey: f.subKeyB},
		},
	}
	f.uploads = NewVideoUploadService(uploads, nil, nil, nil, nil)
	f.section, f.course = section, course
	f.svc = NewLessonContentService(f.repo, &previewSectionRepoStub{section: section}, &previewCourseRepoStub2{course: course}, nil, f.uploads)
	return f
}

// previewCourseRepoStub2: requireLessonCourseOwnerOrAdmin đọc course qua GetByID (stub preview chỉ có GetDetailBySlug).
type previewCourseRepoStub2 struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (s *previewCourseRepoStub2) GetByID(context.Context, uuid.UUID) (*model.Course, error) {
	return s.course, nil
}

func createReq(video *string) dto.CreateLessonContentDTO {
	return dto.CreateLessonContentDTO{Type: "video", VideoURL: video}
}

func ownedByB(_, b uuid.UUID) uuid.UUID { return b }
func ownedByA(a, _ uuid.UUID) uuid.UUID { return a }

// Giảng viên B (chủ khoá của B) gắn upload của A -> bị chặn, KHÔNG tạo content.
func TestCreateContent_GiangVienBGanUploadCuaA_BiChan(t *testing.T) {
	f := buildOwnerFixture(ownedByB, nil)
	_, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherB, false, createReq(hlsURLFor(f.uploadA)))
	if !errors.Is(err, ErrUploadNotOwned) {
		t.Fatalf("err = %v, muon ErrUploadNotOwned", err)
	}
	if f.repo.created != nil {
		t.Error("content van duoc tao du upload cua nguoi khac")
	}
}

// A gắn upload của chính A -> được.
func TestCreateContent_GiangVienAGanUploadCuaChinhA_ThanhCong(t *testing.T) {
	f := buildOwnerFixture(ownedByA, nil)
	got, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherA, false, createReq(hlsURLFor(f.uploadA)))
	if err != nil {
		t.Fatalf("chu upload gan upload cua minh phai duoc: %v", err)
	}
	if got == nil || f.repo.created == nil {
		t.Fatal("content phai duoc tao")
	}
}

// Admin gắn upload của bất kỳ ai -> được (admin qua kiểm chủ khoá bằng isAdmin).
func TestCreateContent_AdminGanUploadCuaNguoiKhac_ThanhCong(t *testing.T) {
	f := buildOwnerFixture(ownedByA, nil)
	if _, err := f.svc.CreateContent(context.Background(), f.lessonID, f.admin, true, createReq(hlsURLFor(f.uploadB))); err != nil {
		t.Fatalf("admin gan upload cua ai cung duoc: %v", err)
	}
	if f.repo.created == nil {
		t.Fatal("content phai duoc tao")
	}
}

// Upload không tồn tại -> ErrUploadNotFound (handler: 404), không tạo content.
func TestCreateContent_UploadKhongTonTai_NotFound(t *testing.T) {
	f := buildOwnerFixture(ownedByA, nil)
	_, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherA, false, createReq(hlsURLFor(uuid.New())))
	if !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("err = %v, muon ErrUploadNotFound", err)
	}
	if f.repo.created != nil {
		t.Error("content van duoc tao du upload khong ton tai")
	}
}

// video_url ngoài hệ thống (không có /hls/{uuid}, vd video mẫu) không phải tài nguyên của ta: không kiểm.
func TestCreateContent_VideoNgoaiHeThong_KhongKiemUpload(t *testing.T) {
	f := buildOwnerFixture(ownedByA, nil)
	ext := "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"
	if _, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherA, false, createReq(&ext)); err != nil {
		t.Fatalf("video ngoai he thong phai duoc: %v", err)
	}
}

// Sửa content: đổi video_url sang upload của người khác -> bị chặn, KHÔNG ghi.
func TestUpdateContent_DoiSangUploadCuaNguoiKhac_BiChan(t *testing.T) {
	f := buildOwnerFixture(ownedByB, nil)
	req := dto.UpdateLessonContentDTO{VideoURL: hlsURLFor(f.uploadA)}
	_, err := f.svc.UpdateContent(context.Background(), f.repo.content.ID, f.teacherB, false, req)
	if !errors.Is(err, ErrUploadNotOwned) {
		t.Fatalf("err = %v, muon ErrUploadNotOwned", err)
	}
	if f.repo.updated != nil {
		t.Error("content van bi sua du upload cua nguoi khac")
	}
}

// Sửa content: đổi sang upload của chính mình, và admin đổi sang upload của ai cũng được.
func TestUpdateContent_UploadCuaMinhVaAdmin_ThanhCong(t *testing.T) {
	f := buildOwnerFixture(ownedByA, nil)
	if _, err := f.svc.UpdateContent(context.Background(), f.repo.content.ID, f.teacherA, false, dto.UpdateLessonContentDTO{VideoURL: hlsURLFor(f.uploadA)}); err != nil {
		t.Fatalf("chu upload sua sang upload cua minh phai duoc: %v", err)
	}
	if f.repo.updated == nil {
		t.Fatal("content phai duoc sua")
	}
	f2 := buildOwnerFixture(ownedByA, nil)
	if _, err := f2.svc.UpdateContent(context.Background(), f2.repo.content.ID, f2.admin, true, dto.UpdateLessonContentDTO{VideoURL: hlsURLFor(f2.uploadB)}); err != nil {
		t.Fatalf("admin phai duoc: %v", err)
	}
}

// Form sửa thường gửi lại nguyên video_url đang có: giữ upload_id cũ thì không kiểm lại, để dữ liệu có
// sẵn (kể cả seed / upload đã bị xoá) không bị hỏng chỉ vì sửa tiêu đề. Đổi sang upload KHÁC thì vẫn kiểm.
func TestUpdateContent_GiuNguyenUploadCu_KhongKiemLai(t *testing.T) {
	legacy := hlsURLFor(uuid.New()) // upload_id không còn trong DB
	f := buildOwnerFixture(ownedByB, legacy)
	title := "doi tieu de"
	req := dto.UpdateLessonContentDTO{Title: &title, VideoURL: legacy}
	if _, err := f.svc.UpdateContent(context.Background(), f.repo.content.ID, f.teacherB, false, req); err != nil {
		t.Fatalf("giu nguyen video_url cu phai duoc: %v", err)
	}
}

// Không có dịch vụ upload để kiểm chủ mà video_url là upload nội bộ: từ chối, không cho qua âm thầm.
func TestCreateContent_KhongCoDichVuUpload_TuChoiUploadNoiBo(t *testing.T) {
	f := buildOwnerFixture(ownedByA, nil)
	f.svc.videoUploadService = nil
	if _, err := f.svc.CreateContent(context.Background(), f.lessonID, f.teacherA, false, createReq(hlsURLFor(f.uploadA))); err == nil {
		t.Fatal("khong kiem duoc chu upload thi khong duoc tao content")
	}
}
