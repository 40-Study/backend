package service

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/hlsauth"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type LessonContentServiceInterface interface {
	CreateContent(ctx context.Context, lessonID uuid.UUID, actorUserID uuid.UUID, isAdmin bool, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error)
	GetContentByID(ctx context.Context, contentID uuid.UUID) (*dto.LessonContentResponseDTO, error)
	UpdateContent(ctx context.Context, contentID, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateLessonContentDTO) (*dto.LessonContentResponseDTO, error)
	DeleteContent(ctx context.Context, contentID, actorUserID uuid.UUID, isAdmin bool) error
	// GetContentsByLessonID (Phase 1 §2): userID dung de chan noi dung cua bai dang bi khoa —
	// tra ve ErrLessonLocked (handler anh xa sang 403 {message:"LESSON_LOCKED"}) khi bai chua
	// mo doi voi CHINH nguoi dang goi. Contract yeu cau chan ca tang API, khong chi an o UI.
	// isAdmin (CAO-4, review vòng 2): giảng viên sở hữu khóa học chứa bài này, hoặc admin hệ
	// thống, KHÔNG BAO GIỜ bị khoá (xem BypassLock tại lesson_lock.go).
	GetContentsByLessonID(ctx context.Context, lessonID, userID uuid.UUID, isAdmin bool) ([]dto.LessonContentResponseDTO, error)
	// ReorderContents (M2-03, review vòng 3): thêm actorUserID/isAdmin — trước đây hàm này chỉ
	// validateLesson (kiểm TỒN TẠI), không kiểm CHỦ SỞ HỮU, khác với mọi CRUD content khác
	// (Create/Update/Delete đều gọi requireContentLessonOwnerOrAdmin/requireLessonCourseOwnerOrAdmin).
	ReorderContents(ctx context.Context, lessonID uuid.UUID, actorUserID uuid.UUID, isAdmin bool, req dto.ReorderDTO) error
}

type LessonContentService struct {
	lessonRepo         repository.LessonRepositoryInterface
	sectionRepo        repository.SectionRepositoryInterface
	courseRepo         repository.CourseRepositoryInterface
	enrollmentRepo     repository.EnrollmentRepositoryInterface
	videoUploadService VideoUploadServiceInterface
}

func NewLessonContentService(
	lessonRepo repository.LessonRepositoryInterface,
	sectionRepo repository.SectionRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	videoUploadService VideoUploadServiceInterface,
) *LessonContentService {
	return &LessonContentService{
		lessonRepo:         lessonRepo,
		sectionRepo:        sectionRepo,
		courseRepo:         courseRepo,
		enrollmentRepo:     enrollmentRepo,
		videoUploadService: videoUploadService,
	}
}

// requireContentLessonOwnerOrAdmin (H-05, review vòng 1): C-12 chỉ gate Lesson (tạo/sửa/xóa),
// bỏ sót LessonContentService — UpdateContent/DeleteContent trước đây chỉ nhận contentID, user
// bất kỳ đã đăng nhập sửa/xóa được nội dung khóa học của giảng viên khác, kể cả xóa video gốc
// trên MinIO (không khôi phục được). Dùng LẠI đúng logic ownership của Lesson
// (requireLessonCourseOwnerOrAdmin, lesson_service.go) thay vì viết lại — content không có
// course_id trực tiếp nên phải load lesson trước (content.LessonID -> lesson.SectionID ->
// section.CourseID -> course.InstructorID).
func (s *LessonContentService) requireContentLessonOwnerOrAdmin(ctx context.Context, content *model.LessonContent, actorUserID uuid.UUID, isAdmin bool) error {
	lesson, err := s.lessonRepo.GetByID(ctx, content.LessonID)
	if err != nil {
		return err
	}
	if lesson == nil {
		return errors.New("lesson not found")
	}
	return requireLessonCourseOwnerOrAdmin(ctx, s.sectionRepo, s.courseRepo, lesson, actorUserID, isAdmin)
}

// requireVideoUploadUsable: video_url trỏ vào một upload nội bộ (/hls/{upload_id}) thì upload đó phải
// do chính actor tải lên (hoặc actor là admin). Trước đây API cấp URL ký theo upload_id trong
// video_url mà không hỏi ai là chủ upload, nên giảng viên B gắn upload_id của A vào bài của mình là
// xem/tải được video của A. Lỗi: ErrUploadNotOwned (403) / ErrUploadNotFound (404).
//
// Giữ nguyên upload_id đang có trên content (currentURL) thì không kiểm lại: form sửa nội dung
// thường gửi lại nguyên video_url cũ, và dữ liệu có sẵn (kể cả seed) không được vì thế mà hỏng.
// video_url ngoài hệ thống (không có /hls/{uuid}) không phải tài nguyên của ta, bỏ qua.
func (s *LessonContentService) requireVideoUploadUsable(ctx context.Context, newURL, currentURL *string, actorUserID uuid.UUID, isAdmin bool) error {
	if newURL == nil || *newURL == "" {
		return nil
	}
	uploadID, ok := hlsauth.ExtractUploadID(*newURL)
	if !ok {
		return nil
	}
	if currentURL != nil {
		if cur, ok := hlsauth.ExtractUploadID(*currentURL); ok && cur == uploadID {
			return nil
		}
	}
	if s.videoUploadService == nil {
		return errors.New("cannot verify upload ownership: upload service unavailable")
	}
	return s.videoUploadService.RequireUploadUsableBy(ctx, uploadID, actorUserID, isAdmin)
}

func (s *LessonContentService) CreateContent(ctx context.Context, lessonID uuid.UUID, actorUserID uuid.UUID, isAdmin bool, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}
	if err := requireLessonCourseOwnerOrAdmin(ctx, s.sectionRepo, s.courseRepo, lesson, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.requireVideoUploadUsable(ctx, req.VideoURL, nil, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	content := &model.LessonContent{
		ID:           uuid.New(),
		LessonID:     lessonID,
		Type:         req.Type,
		Title:        req.Title,
		VideoURL:     req.VideoURL,
		Duration:     0,
		ExerciseID:   req.ExerciseID,
		IsMandatory:  true,
		DisplayOrder: 0,
		SubtitleURL:  req.SubtitleURL,
	}

	if req.Duration != nil {
		content.Duration = *req.Duration
	}
	if req.IsMandatory != nil {
		content.IsMandatory = *req.IsMandatory
	}
	if req.DisplayOrder != nil {
		content.DisplayOrder = *req.DisplayOrder
	}

	if err := s.lessonRepo.CreateContent(ctx, content); err != nil {
		return nil, err
	}

	// Đã qua requireLessonCourseOwnerOrAdmin ở trên: actor là chủ khoá / admin nên được xem file gốc.
	return s.toContentResponseDTO(content, videoViewer{userID: actorUserID, original: true}), nil
}

func (s *LessonContentService) GetContentByID(ctx context.Context, contentID uuid.UUID) (*dto.LessonContentResponseDTO, error) {
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	// Hàm này không nhận người gọi nên KHÔNG có căn cứ để cấp URL video — không ký gì cả.
	return s.toContentResponseDTO(content, withheldVideoViewer), nil
}

func (s *LessonContentService) GetContentsByLessonID(ctx context.Context, lessonID, userID uuid.UUID, isAdmin bool) ([]dto.LessonContentResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}

	// Phase 1 §2: chan noi dung bai bi khoa o CHINH tang API — contract ghi ro "khong chi chan
	// UI". Bai preview/mien phi bo qua nhanh nay (ResolveLessonLock tu tra locked=false).
	// CAO-4: chu so huu khoa hoc / admin duoc bypass qua BypassLock (gatherLessonLockInput),
	// nen van phai chay het nhanh nay (khong short-circuit rieng o day) de logic bypass nam
	// DUY NHAT o mot cho (lesson_lock.go), khong lech voi SectionService/LessonService.
	//
	// D4 (review PR #79): khoá học phải được tra cho CẢ bài preview — trước bản vá nhánh preview
	// bỏ qua mọi kiểm tra, nên người ngoài đọc được video_url bài preview của khoá nháp/chờ duyệt.
	// Bài preview chỉ công khai khi bản thân khoá được phép xem (canViewCourse).
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, ErrCourseHidden
	}
	bypass := isAdmin || course.InstructorID == userID
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, courseID, course.Sequential, bypass)
	if err != nil {
		return nil, err
	}
	if !canViewCourse(course, userID, isAdmin, lockInput.Enrolled) {
		return nil, ErrCourseHidden
	}
	if !lesson.IsPreview {
		// Quyết định team lead (review vòng 2): lessonID không thực sự thuộc LessonOrder của
		// courseID vừa suy ra (dữ liệu không nhất quán) không được ResolveLessonLock âm thầm mở
		// (idx==-1) hay khoá nhầm lý do — phải là lỗi rõ ràng. Xem EnsureLessonInCourse.
		if err := EnsureLessonInCourse(lessonID, lockInput.LessonOrder); err != nil {
			return nil, err
		}
		if locked, _, _ := ResolveLessonLock(lessonID, lockInput); locked {
			return nil, ErrLessonLocked
		}
	}

	contents, err := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	// Chỉ chủ khoá / admin (đúng cờ bypass đã tính ở trên) mới nhận URL file video gốc; học viên
	// đã ghi danh chỉ nhận URL HLS ký.
	viewer := videoViewer{userID: userID, original: bypass}
	result := make([]dto.LessonContentResponseDTO, len(contents))
	for i, c := range contents {
		result[i] = *s.toContentResponseDTO(&c, viewer)
	}

	return result, nil
}

// ErrLessonNotPreview: bài không phải preview/miễn phí — route công khai từ chối, không phân
// biệt với "khoá chưa published" (cả hai đều 404 ở handler) để tránh dò xem khoá nào tồn tại.
var ErrLessonNotPreview = errors.New("lesson is not a free preview")

// GetPreviewContentsByLessonID (F1, QA vòng 2 260929): route CÔNG KHAI cho khách CHƯA đăng nhập
// xem thử bài preview — trước đây `GET /lessons/:lesson_id/contents` (GetContentsByLessonID) là
// route DUY NHẤT, luôn nằm sau `auth` middleware, nên khách luôn nhận 401 kể cả với bài đã đánh
// dấu `is_preview=true` trên khoá đã published. Hàm này KHÔNG nhận userID/isAdmin (khách không có)
// nên không tái dùng gatherLessonLockInput/canViewCourse (cả hai đọc enrollment theo user) — tự
// đối chiếu course qua slug trên URL (không suy ngược từ lessonID, tránh nhầm giữa 2 khoá có
// lesson id trùng) và đòi hỏi CẢ HAI: course.Status == published VÀ lesson.IsPreview == true.
// Thiếu 1 trong 2 -> ErrCourseHidden/ErrLessonNotPreview, handler ánh xạ cả hai sang 404 như nhau
// để không lộ khoá nháp đang tồn tại nhưng bị từ chối vì lý do nào.
func (s *LessonContentService) GetPreviewContentsByLessonID(ctx context.Context, courseSlug string, lessonID uuid.UUID) ([]dto.LessonContentResponseDTO, error) {
	course, err := s.courseRepo.GetDetailBySlug(ctx, courseSlug)
	if err != nil {
		return nil, err
	}
	if course == nil || course.Status != model.CourseStatusPublished {
		return nil, ErrCourseHidden
	}

	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil || !lesson.IsPreview {
		return nil, ErrLessonNotPreview
	}

	section, err := s.sectionRepo.GetByID(ctx, lesson.SectionID)
	if err != nil {
		return nil, err
	}
	if section == nil || section.CourseID != course.ID {
		return nil, ErrLessonNotInCourse
	}

	contents, err := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonContentResponseDTO, len(contents))
	for i, c := range contents {
		result[i] = *s.toContentResponseDTO(&c, guestVideoViewer)
	}

	return result, nil
}

func (s *LessonContentService) UpdateContent(ctx context.Context, contentID, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}
	if err := s.requireContentLessonOwnerOrAdmin(ctx, content, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	if err := s.requireVideoUploadUsable(ctx, req.VideoURL, content.VideoURL, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	if req.Type != nil {
		content.Type = *req.Type
	}
	if req.Title != nil {
		content.Title = req.Title
	}
	if req.VideoURL != nil {
		content.VideoURL = req.VideoURL
	}
	if req.Duration != nil {
		content.Duration = *req.Duration
	}
	if req.ExerciseID != nil {
		content.ExerciseID = req.ExerciseID
	}
	if req.IsMandatory != nil {
		content.IsMandatory = *req.IsMandatory
	}
	if req.DisplayOrder != nil {
		content.DisplayOrder = *req.DisplayOrder
	}
	if req.SubtitleURL != nil {
		// Chuoi rong = go phu de dang co (contract §4), khac voi khong gui truong nay.
		if *req.SubtitleURL == "" {
			content.SubtitleURL = nil
		} else {
			content.SubtitleURL = req.SubtitleURL
		}
	}

	if err := s.lessonRepo.UpdateContent(ctx, content); err != nil {
		return nil, err
	}

	return s.toContentResponseDTO(content, videoViewer{userID: actorUserID, original: true}), nil
}

func (s *LessonContentService) DeleteContent(ctx context.Context, contentID, actorUserID uuid.UUID, isAdmin bool) error {
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return err
	}
	if content == nil {
		return errors.New("content not found")
	}
	if err := s.requireContentLessonOwnerOrAdmin(ctx, content, actorUserID, isAdmin); err != nil {
		return err
	}

	// Delete video upload and all associated files (original, HLS, thumbnail)
	// R6 (code-reviewer-260919-1557): actorUserID (nguoi dang thuc hien xoa content, da qua
	// requireContentLessonOwnerOrAdmin o tren) duoc truyen xuong lam ownerUserID — DeleteUpload
	// tu quyet dinh bo qua neu upload_id trich tu VideoURL khong thuoc ve actorUserID, thay vi
	// xoa mu quang video cua nguoi khac chi vi video_url tro sang do.
	if s.videoUploadService != nil && content.VideoURL != nil && *content.VideoURL != "" {
		// Try to extract upload_id from HLS URL pattern: /api/hls/{uploadId}/
		hlsPattern := regexp.MustCompile(`/hls/([a-f0-9-]{36})/`)
		if matches := hlsPattern.FindStringSubmatch(*content.VideoURL); len(matches) > 1 {
			if uploadID, err := uuid.Parse(matches[1]); err == nil {
				// Delete video upload (this deletes original video, HLS folder, thumbnail from MinIO)
				_ = s.videoUploadService.DeleteUpload(ctx, uploadID, actorUserID)
			}
		}
	}

	// Lo 1 (fullstack-verify-260922): KHONG xoa file theo content.VideoURL. URL do client tu dat qua
	// CreateContent/UpdateContent va khong co bang so huu file, nen goi DeleteByURL o day cho phep giang
	// vien tro video_url vao bat ky object nao trong bucket (anh khoa hoc, video goc cua nguoi khac) roi
	// xoa content de xoa file do: chinh lo C-14 ma DELETE /api/upload da khoa bang SYSTEM_SETTINGS_MANAGE.
	// Video qua luong upload duoc don qua DeleteUpload (co kiem chu so huu); file legacy tro thang MinIO
	// chap nhan de mo coi.

	return s.lessonRepo.DeleteContent(ctx, contentID)
}

// ReorderContents (M2-03, review vòng 3): trước đây chỉ validateLesson (bài học có tồn tại
// không) — bất kỳ giảng viên/user đăng nhập nào biết lessonID đều sắp xếp lại được nội dung bài
// học của khóa học người khác. Load lesson trực tiếp (thay vì chỉ check Exists) + kiểm chủ sở
// hữu bằng requireLessonCourseOwnerOrAdmin, khớp CreateContent.
func (s *LessonContentService) ReorderContents(ctx context.Context, lessonID uuid.UUID, actorUserID uuid.UUID, isAdmin bool, req dto.ReorderDTO) error {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return err
	}
	if lesson == nil {
		return errors.New("lesson not found")
	}
	if err := requireLessonCourseOwnerOrAdmin(ctx, s.sectionRepo, s.courseRepo, lesson, actorUserID, isAdmin); err != nil {
		return err
	}

	ids := make([]uuid.UUID, len(req.Items))
	items := make([]repository.ReorderItem, len(req.Items))
	for i, item := range req.Items {
		ids[i] = item.ID
		items[i] = repository.ReorderItem{
			ID:           item.ID,
			DisplayOrder: item.DisplayOrder,
		}
	}

	count, err := s.lessonRepo.CountContentsByIDsAndLesson(ctx, ids, lessonID)
	if err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return errors.New("one or more contents do not belong to this lesson")
	}

	return s.lessonRepo.ReorderContents(ctx, items)
}

func (s *LessonContentService) toContentResponseDTO(c *model.LessonContent, viewer videoViewer) *dto.LessonContentResponseDTO {
	resp := &dto.LessonContentResponseDTO{
		ID:          c.ID,
		LessonID:    c.LessonID,
		Type:        c.Type,
		Title:       c.Title,
		Duration:    c.Duration,
		ExerciseID:  c.ExerciseID,
		IsMandatory: c.IsMandatory,
		// N10 (review vòng 2, từ review web): xem chú thích tại model.LessonContent.
		LivestreamSessionID: c.LivestreamSessionID,
		DisplayOrder:        c.DisplayOrder,
		SubtitleURL:         c.SubtitleURL,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}

	// Thay URL video đã lưu bằng URL KÝ (video_hls_url), và chỉ chủ khoá/admin mới còn video_url
	// trỏ file gốc — xem applyVideoAccess.
	applyVideoAccess(resp, c.VideoURL, viewer)

	return resp
}
