package service

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrNotLessonCourseOwner dùng chung cho mọi thao tác ghi lesson — chỉ giảng viên sở hữu
// khóa học cha (qua section -> course.InstructorID) mới được tạo/sửa/xóa lesson (C-12).
var ErrNotLessonCourseOwner = errors.New("forbidden: not the owner")

// ErrLessonHasNoVideo (Phase 1 §4, bổ sung từ review web #17): PUT /lessons/:lessonId gửi
// subtitle_url cho một bài chưa có content video nào để gắn vào — handler ánh xạ sang 400
// {"message": "LESSON_HAS_NO_VIDEO"}, đúng contract, thay vì một message lỗi tự do.
var ErrLessonHasNoVideo = errors.New("lesson has no video content to attach a subtitle to")

type LessonServiceInterface interface {
	CreateLesson(ctx context.Context, sectionID, actorUserID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error)
	// GetAllLessons (C-1, review vòng 2): userID/isAdmin dùng để tính khoá cho TỪNG bài — trước
	// bản vá này hàm KHÔNG nhận userID và trả thẳng les.Contents cho bất kỳ ai đã đăng nhập, nên
	// GET /sections/:section_id/lessons là đường vòng lộ contents[].video_url bỏ qua hoàn toàn
	// ResolveLessonLock (đúng lớp lỗ của B-2, chỉ khác endpoint). Route này nằm sau
	// middleware.AuthMiddleware nên userID không bao giờ là uuid.Nil trên đường thật.
	GetAllLessons(ctx context.Context, sectionID, userID uuid.UUID, isAdmin bool) ([]dto.LessonResponseDTO, error)
	// GetLessonByID (B-2, review vòng 2): userID/isAdmin dùng để tính khoá — trước bản vá này
	// hàm KHÔNG nhận userID, gọi thẳng lessonRepo.GetContentsByLessonID (bỏ qua hoàn toàn
	// ResolveLessonLock), nên GET /lessons/:id là đường vòng lộ contents (bao gồm video_url,
	// hls url) của một bài đang bị khoá đối với chính người gọi. Khi khoá: trả metadata với
	// contents=[] + locked=true + lock_reason, KHÔNG trả lỗi (khác GetContentsByLessonID/
	// LessonContentService, nơi contract yêu cầu 403 LESSON_LOCKED — hai endpoint có ngữ nghĩa
	// khác nhau: cái này là "xem lesson" tổng quan, cái kia là "lấy nội dung để phát").
	GetLessonByID(ctx context.Context, lessonID, userID uuid.UUID, isAdmin bool) (*dto.LessonResponseDTO, error)
	UpdateLesson(ctx context.Context, lessonID, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error)
	DeleteLesson(ctx context.Context, lessonID, actorUserID uuid.UUID, isAdmin bool) error
	ReorderLessons(ctx context.Context, sectionID uuid.UUID, req dto.ReorderDTO) error
}

type LessonService struct {
	lessonRepo     repository.LessonRepositoryInterface
	sectionRepo    repository.SectionRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
	uploadService  UploadServiceInterface
}

func NewLessonService(
	lessonRepo repository.LessonRepositoryInterface,
	sectionRepo repository.SectionRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	uploadService UploadServiceInterface,
) *LessonService {
	return &LessonService{
		lessonRepo:     lessonRepo,
		sectionRepo:    sectionRepo,
		courseRepo:     courseRepo,
		enrollmentRepo: enrollmentRepo,
		uploadService:  uploadService,
	}
}

func (s *LessonService) validateSection(ctx context.Context, sectionID uuid.UUID) error {
	exists, err := s.sectionRepo.Exists(ctx, sectionID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("section not found")
	}
	return nil
}

// checkSectionCourseOwnership tra ve section (neu ton tai) sau khi xac nhan actorUserID la
// giang vien so huu course chua section do (qua section.CourseID -> course.InstructorID).
func (s *LessonService) checkSectionCourseOwnership(ctx context.Context, sectionID, actorUserID uuid.UUID) (*model.Section, error) {
	section, err := s.sectionRepo.GetByID(ctx, sectionID)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return nil, errors.New("section not found")
	}
	course, err := s.courseRepo.GetByID(ctx, section.CourseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, errors.New("course not found")
	}
	if course.InstructorID != actorUserID {
		return nil, ErrNotLessonCourseOwner
	}
	return section, nil
}

// checkLessonCourseOwnership giong checkSectionCourseOwnership nhung xuat phat tu lessonID
// (di qua lesson.SectionID -> section.CourseID -> course.InstructorID). isAdmin (vòng 2, tính
// sẵn ở handler qua PermissionChecker) cho phép SYSTEM_ADMIN override chủ sở hữu khi sửa/xóa.
func (s *LessonService) checkLessonCourseOwnership(ctx context.Context, lesson *model.Lesson, actorUserID uuid.UUID, isAdmin bool) error {
	return requireLessonCourseOwnerOrAdmin(ctx, s.sectionRepo, s.courseRepo, lesson, actorUserID, isAdmin)
}

// requireLessonCourseOwnerOrAdmin (H-05, review vòng 1): tách thành HÀM TỰ DO (không gắn với
// *LessonService) để LessonContentService dùng chung logic kiểm tra chủ sở hữu này — trước đây
// C-12 chỉ gate được Lesson (tạo/sửa/xóa), còn LessonContentService (nội dung BÊN TRONG lesson:
// video, bài tập...) không kiểm gì cả, kể cả DeleteContent xóa cả video gốc trên MinIO.
func requireLessonCourseOwnerOrAdmin(ctx context.Context, sectionRepo repository.SectionRepositoryInterface, courseRepo repository.CourseRepositoryInterface, lesson *model.Lesson, actorUserID uuid.UUID, isAdmin bool) error {
	section, err := sectionRepo.GetByID(ctx, lesson.SectionID)
	if err != nil {
		return err
	}
	if section == nil {
		return errors.New("section not found")
	}
	course, err := courseRepo.GetByID(ctx, section.CourseID)
	if err != nil {
		return err
	}
	if course == nil {
		return errors.New("course not found")
	}
	if course.InstructorID != actorUserID && !isAdmin {
		return ErrNotLessonCourseOwner
	}
	return nil
}

func (s *LessonService) CreateLesson(ctx context.Context, sectionID, actorUserID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error) {
	if _, err := s.checkSectionCourseOwnership(ctx, sectionID, actorUserID); err != nil {
		return nil, err
	}

	maxOrder, err := s.lessonRepo.GetMaxDisplayOrder(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	displayOrder := maxOrder + 1
	if req.DisplayOrder > 0 {
		displayOrder = req.DisplayOrder
	}

	lesson := &model.Lesson{
		ID:           uuid.New(),
		SectionID:    sectionID,
		Title:        req.Title,
		Description:  req.Description,
		DisplayOrder: displayOrder,
		IsMandatory:  true,
	}

	if req.DurationMins != nil {
		lesson.DurationMins = *req.DurationMins
	}
	if req.IsPreview != nil {
		lesson.IsPreview = *req.IsPreview
	}
	if req.IsMandatory != nil {
		lesson.IsMandatory = *req.IsMandatory
	}

	if err := s.lessonRepo.Create(ctx, lesson); err != nil {
		return nil, err
	}

	return s.toLessonResponseDTO(lesson, nil), nil
}

// GetAllLessons (C-1, review vòng 2): bản trước trả thẳng les.Contents cho MỌI người đã đăng nhập
// — không enroll, không kiểm khoá. Bản này dùng LẠI đúng đường khoá của GetLessonByID
// (gatherLessonLockInput + ResolveLessonLock — xem lesson_lock.go): bài bị khoá trả contents RỖNG
// kèm locked/lock_reason/progress, bài mở trả contents đầy đủ, nên giảng viên sở hữu khoá học
// (BypassLock) vẫn thấy video_url.
//
// KHÁC GetLessonByID ở chỗ KHÔNG có lessonID tuỳ ý để xác nhận: hàm này chỉ trả các bài nằm trong
// chính sectionID được hỏi, nên mọi bài đều thuộc khoá theo cấu trúc — không cần EnsureLessonInCourse
// (cùng lý do SectionService.GetAllSections không gọi nó).
func (s *LessonService) GetAllLessons(ctx context.Context, sectionID, userID uuid.UUID, isAdmin bool) ([]dto.LessonResponseDTO, error) {
	section, err := s.sectionRepo.GetByID(ctx, sectionID)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return nil, errors.New("section not found")
	}

	course, err := s.courseRepo.GetByID(ctx, section.CourseID)
	if err != nil {
		return nil, err
	}
	// course == nil (dữ liệu mồ côi) rơi về sequential=false + không bypass — tức làn "mở" của
	// luật khoá, giống hệt cách resolveLessonByIDLock xử lý course nil.
	var sequential bool
	bypass := isAdmin
	if course != nil {
		sequential = course.Sequential
		bypass = isAdmin || course.InstructorID == userID
	}
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, section.CourseID, sequential, bypass)
	if err != nil {
		return nil, err
	}

	lessons, err := s.lessonRepo.GetAllBySectionID(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonResponseDTO, len(lessons))
	for i, les := range lessons {
		locked, reason, progress := ResolveLessonLock(les.ID, lockInput)
		if locked {
			// Bài bị khoá: KHÔNG truyền les.Contents — đó chính là đường lộ video_url mà bản vá
			// này đóng lại.
			result[i] = *s.toLessonResponseDTO(&les, nil)
		} else {
			result[i] = *s.toLessonResponseDTO(&les, les.Contents)
		}
		result[i].Locked = locked
		result[i].LockReason = reason
		result[i].Progress = progress
	}

	return result, nil
}

func (s *LessonService) GetLessonByID(ctx context.Context, lessonID, userID uuid.UUID, isAdmin bool) (*dto.LessonResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}

	// B-2 (review vòng 2): tính khoá TRƯỚC khi quyết định có nạp contents hay không — bài bị
	// khoá đối với người gọi thì KHÔNG được nạp/tra contents (đó là đường lộ video_url/hls
	// url mà bản vá này đóng lại), dù response vẫn 200 kèm metadata.
	locked, lockReason, progress, err := s.resolveLessonByIDLock(ctx, lesson, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	if locked {
		resp := s.toLessonResponseDTO(lesson, nil)
		resp.Locked = true
		resp.LockReason = lockReason
		resp.Progress = progress
		resp.SubtitleURL = nil
		return resp, nil
	}

	contents, err := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	resp := s.toLessonResponseDTO(lesson, contents)
	resp.Locked = false
	resp.LockReason = nil
	resp.Progress = progress
	return resp, nil
}

// resolveLessonByIDLock (B-2 + CAO-4, review vòng 2): dùng LẠI đúng logic khoá của
// LessonContentService.GetContentsByLessonID (gatherLessonLockInput + ResolveLessonLock) thay vì
// viết lại — hai nơi lệch nhau là đúng thứ bug mà LessonLockInput được tách struct riêng để tránh
// (xem chú thích tại LessonLockInput). CAO-4: giảng viên sở hữu khoá học chứa bài này, hoặc admin
// hệ thống, KHÔNG BAO GIỜ bị khoá khỏi bài của chính khoá học đó — bypassLock truyền vào
// gatherLessonLockInput, ResolveLessonLock áp dụng thống nhất (xem lesson_lock.go).
func (s *LessonService) resolveLessonByIDLock(ctx context.Context, lesson *model.Lesson, userID uuid.UUID, isAdmin bool) (bool, *string, *dto.LessonProgressSummaryDTO, error) {
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lesson.ID)
	if err != nil {
		return false, nil, nil, err
	}
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return false, nil, nil, err
	}
	sequential := course != nil && course.Sequential
	bypass := isAdmin || (course != nil && course.InstructorID == userID)
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, courseID, sequential, bypass)
	if err != nil {
		return false, nil, nil, err
	}
	// Quyết định team lead (review vòng 2): xem chú thích tại EnsureLessonInCourse — lessonID
	// không thuộc LessonOrder của courseID vừa suy ra là lỗi rõ ràng, không mở lén.
	if err := EnsureLessonInCourse(lesson.ID, lockInput.LessonOrder); err != nil {
		return false, nil, nil, err
	}
	locked, reason, progress := ResolveLessonLock(lesson.ID, lockInput)
	return locked, reason, progress, nil
}

func (s *LessonService) UpdateLesson(ctx context.Context, lessonID, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}
	if err := s.checkLessonCourseOwnership(ctx, lesson, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	// (review vòng 2, TB) VALIDATE LESSON_HAS_NO_VIDEO TRƯỚC KHI GHI: nạp contents và tìm
	// content video một lần ở đây, TRƯỚC lessonRepo.Update(lesson) bên dưới. Trước bản vá này,
	// thứ tự là Update(lesson) rồi mới applySubtitleURL() tự nạp lại contents — nếu bài không có
	// video, request thất bại NHƯNG title/display_order/duration_minutes ĐÃ ghi xuống DB, để lại
	// một ghi write nửa vời (client thấy lỗi nhưng dữ liệu vẫn đổi).
	var videoContent *model.LessonContent
	var contents []model.LessonContent
	if req.SubtitleURL != nil {
		var err error
		contents, err = s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
		if err != nil {
			return nil, err
		}
		for i := range contents {
			if contents[i].Type == "video" {
				videoContent = &contents[i]
				break
			}
		}
		if videoContent == nil {
			return nil, ErrLessonHasNoVideo
		}
	}

	if req.Title != nil {
		lesson.Title = *req.Title
	}
	if req.Description != nil {
		lesson.Description = req.Description
	}
	if req.DisplayOrder != nil {
		lesson.DisplayOrder = *req.DisplayOrder
	}
	if req.DurationMins != nil {
		lesson.DurationMins = *req.DurationMins
	}
	if req.IsPreview != nil {
		lesson.IsPreview = *req.IsPreview
	}
	if req.IsMandatory != nil {
		lesson.IsMandatory = *req.IsMandatory
	}

	if err := s.lessonRepo.Update(ctx, lesson); err != nil {
		return nil, err
	}

	if req.SubtitleURL != nil {
		if err := s.applySubtitleURL(ctx, videoContent, req.SubtitleURL); err != nil {
			return nil, err
		}
	}

	contents, err = s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		contents = nil
	}

	return s.toLessonResponseDTO(lesson, contents), nil
}

// applySubtitleURL (Phase 1 §4): ghi subtitle_url vao dung content TYPE="video" cua bai — day
// la "duong lane" ma web da chot (PUT /lessons/:id -> backend PERSIST vao lesson_videos, tra
// lai o lesson content). Chuoi rong duoc coi la "go phu de" (chuyen ve nil), khac voi khong gui
// gi (req.SubtitleURL == nil, khong cham toi ham nay).
//
// video (review vòng 2, TB): TRUYỀN VÀO thay vì tự nạp lại — content video đã được tìm và xác
// nhận tồn tại ở UpdateLesson TRƯỚC KHI ghi bất cứ gì, để tránh ghi nửa vời (xem comment gọi).
func (s *LessonService) applySubtitleURL(ctx context.Context, video *model.LessonContent, subtitleURL *string) error {
	cleaned := subtitleURL
	if cleaned != nil && *cleaned == "" {
		cleaned = nil
	}
	video.SubtitleURL = cleaned
	return s.lessonRepo.UpdateContent(ctx, video)
}

func (s *LessonService) DeleteLesson(ctx context.Context, lessonID, actorUserID uuid.UUID, isAdmin bool) error {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return err
	}
	if lesson == nil {
		return errors.New("lesson not found")
	}
	if err := s.checkLessonCourseOwnership(ctx, lesson, actorUserID, isAdmin); err != nil {
		return err
	}

	// Delete all content videos from MinIO before deleting lesson
	if s.uploadService != nil {
		contents, _ := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
		for _, content := range contents {
			if content.VideoURL != nil && *content.VideoURL != "" {
				_ = s.uploadService.DeleteByURL(ctx, *content.VideoURL)
			}
		}
	}

	return s.lessonRepo.Delete(ctx, lessonID)
}

func (s *LessonService) ReorderLessons(ctx context.Context, sectionID uuid.UUID, req dto.ReorderDTO) error {
	if err := s.validateSection(ctx, sectionID); err != nil {
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

	count, err := s.lessonRepo.CountByIDsAndSection(ctx, ids, sectionID)
	if err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return errors.New("one or more lessons do not belong to this section")
	}

	return s.lessonRepo.Reorder(ctx, items)
}

func (s *LessonService) toLessonResponseDTO(lesson *model.Lesson, contents []model.LessonContent) *dto.LessonResponseDTO {
	resp := &dto.LessonResponseDTO{
		ID:           lesson.ID,
		SectionID:    lesson.SectionID,
		Title:        lesson.Title,
		Description:  lesson.Description,
		DisplayOrder: lesson.DisplayOrder,
		DurationMins: lesson.DurationMins,
		IsPreview:    lesson.IsPreview,
		IsMandatory:  lesson.IsMandatory,
		CreatedAt:    lesson.CreatedAt,
		UpdatedAt:    lesson.UpdatedAt,
	}

	if len(contents) > 0 {
		hlsPattern := regexp.MustCompile(`/hls/([a-f0-9-]{36})`)
		resp.Contents = make([]dto.LessonContentResponseDTO, len(contents))
		for i, c := range contents {
			item := dto.LessonContentResponseDTO{
				ID:       c.ID,
				LessonID: c.LessonID,
				Type:     c.Type,
				Title:    c.Title,
				VideoURL: c.VideoURL,
				Duration: c.Duration,
				// N10 (review vòng 2, từ review web): xem chú thích tại model.LessonContent.
				LivestreamSessionID: c.LivestreamSessionID,
				DisplayOrder:        c.DisplayOrder,
				SubtitleURL:         c.SubtitleURL,
				CreatedAt:           c.CreatedAt,
				UpdatedAt:           c.UpdatedAt,
			}
			// Phase 1 §4: web doc subtitle_url AUTHORITATIVE tu chinh lesson content (item o
			// tren); truong resp.SubtitleURL o cap lesson chi la BAN DU PHONG (web tu ghi chu
			// "giu field nay lam du phong neu curriculum cung tra kem") — gan tu content video.
			if c.Type == "video" && c.SubtitleURL != nil {
				resp.SubtitleURL = c.SubtitleURL
			}
			// Extract upload ID and generate HLS + fallback URLs
			if c.VideoURL != nil && *c.VideoURL != "" {
				if matches := hlsPattern.FindStringSubmatch(*c.VideoURL); len(matches) > 1 {
					uploadID := matches[1]
					hlsURL := "/api/hls/" + uploadID + "/master.m3u8"
					fallbackURL := "/api/hls/" + uploadID + "/video.mp4"
					item.VideoUploadID = &uploadID
					item.VideoHLSURL = &hlsURL
					item.VideoURL = &fallbackURL
				}
			}
			resp.Contents[i] = item
		}
	}

	return resp
}
