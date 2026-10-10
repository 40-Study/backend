package service

import (
	"context"
	"errors"

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
	// ReorderLessons (QA vòng 2): trước đây không nhận người gọi nên không kiểm chủ sở hữu.
	ReorderLessons(ctx context.Context, sectionID, actorUserID uuid.UUID, isAdmin bool, req dto.ReorderDTO) error
}

type LessonService struct {
	lessonRepo     repository.LessonRepositoryInterface
	sectionRepo    repository.SectionRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
	// uploads: kiểm chủ file phụ đề khi ghi subtitle_url (review S1 M1). Gắn qua WithUploadOwnership
	// để không phải đổi chữ ký NewLessonService; nil chỉ trong test không ghi subtitle_url.
	uploads uploadOwnership
}

// WithUploadOwnership gắn bộ kiểm chủ upload dùng khi ghi subtitle_url.
func (s *LessonService) WithUploadOwnership(u uploadOwnership) *LessonService {
	s.uploads = u
	return s
}

func NewLessonService(
	lessonRepo repository.LessonRepositoryInterface,
	sectionRepo repository.SectionRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
) *LessonService {
	return &LessonService{
		lessonRepo:     lessonRepo,
		sectionRepo:    sectionRepo,
		courseRepo:     courseRepo,
		enrollmentRepo: enrollmentRepo,
	}
}

// checkSectionCourseOwnership tra ve section (neu ton tai) sau khi xac nhan actorUserID la
// giang vien so huu course chua section do (qua section.CourseID -> course.InstructorID) — hoặc
// admin khi isAdmin — và khoá không đang chờ duyệt (Q5, ensureCourseEditable).
func (s *LessonService) checkSectionCourseOwnership(ctx context.Context, sectionID, actorUserID uuid.UUID, isAdmin bool) (*model.Section, error) {
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
	if course.InstructorID != actorUserID && !isAdmin {
		return nil, ErrNotLessonCourseOwner
	}
	if err := ensureCourseEditable(course); err != nil {
		return nil, err
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
	// Q5: chặn ở ĐÂY là chặn luôn mọi thao tác ghi nội dung bài (LessonContentService dùng chung
	// hàm này cho create/update/delete/reorder content), không phải sửa từng service.
	return ensureCourseEditable(course)
}

func (s *LessonService) CreateLesson(ctx context.Context, sectionID, actorUserID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error) {
	section, err := s.checkSectionCourseOwnership(ctx, sectionID, actorUserID, false)
	if err != nil {
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

	// P2 QA 260927 teacher: total_lessons/total_duration_minutes không tự cập nhật — ghi lại
	// ngay sau khi thêm bài (xem RecalculateLessonStats).
	if err := s.courseRepo.RecalculateLessonStats(ctx, section.CourseID); err != nil {
		return nil, err
	}

	return s.toLessonResponseDTO(lesson, nil, withheldVideoViewer), nil
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
	// D4: danh sách bài của khoá chưa xuất bản chỉ dành cho chủ khoá/admin/người đã ghi danh.
	if course != nil && !canViewCourse(course, userID, isAdmin, lockInput.Enrolled) {
		return nil, ErrCourseHidden
	}

	lessons, err := s.lessonRepo.GetAllBySectionID(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	viewer := videoViewer{userID: userID, original: bypass}
	result := make([]dto.LessonResponseDTO, len(lessons))
	for i, les := range lessons {
		locked, reason, progress := ResolveLessonLock(les.ID, lockInput)
		if locked {
			// Bài bị khoá: KHÔNG truyền les.Contents — đó chính là đường lộ video_url mà bản vá
			// này đóng lại.
			result[i] = *s.toLessonResponseDTO(&les, nil, withheldVideoViewer)
		} else {
			result[i] = *s.toLessonResponseDTO(&les, les.Contents, viewer)
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
	locked, bypass, lockReason, progress, err := s.resolveLessonByIDLock(ctx, lesson, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	if locked {
		resp := s.toLessonResponseDTO(lesson, nil, withheldVideoViewer)
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

	resp := s.toLessonResponseDTO(lesson, contents, videoViewer{userID: userID, original: bypass})
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
// bypass (trả thêm) = người gọi là chủ khoá / admin: GetLessonByID dùng để quyết định có cấp URL
// video GỐC hay không (học viên chỉ nhận URL HLS ký).
func (s *LessonService) resolveLessonByIDLock(ctx context.Context, lesson *model.Lesson, userID uuid.UUID, isAdmin bool) (locked bool, bypass bool, reason *string, progress *dto.LessonProgressSummaryDTO, err error) {
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lesson.ID)
	if err != nil {
		return false, false, nil, nil, err
	}
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return false, false, nil, nil, err
	}
	sequential := course != nil && course.Sequential
	bypass = isAdmin || (course != nil && course.InstructorID == userID)
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, courseID, sequential, bypass)
	if err != nil {
		return false, false, nil, nil, err
	}
	// D4: bài của khoá chưa xuất bản — người ngoài nhận "không tồn tại", không phải metadata khoá.
	if course != nil && !canViewCourse(course, userID, isAdmin, lockInput.Enrolled) {
		return false, false, nil, nil, ErrCourseHidden
	}
	// Quyết định team lead (review vòng 2): xem chú thích tại EnsureLessonInCourse — lessonID
	// không thuộc LessonOrder của courseID vừa suy ra là lỗi rõ ràng, không mở lén.
	if err := EnsureLessonInCourse(lesson.ID, lockInput.LessonOrder); err != nil {
		return false, false, nil, nil, err
	}
	locked, reason, progress = ResolveLessonLock(lesson.ID, lockInput)
	return locked, bypass, reason, progress, nil
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
		// Kiểm chủ file phụ đề TRƯỚC KHI ghi bất cứ gì (cùng lý do với LESSON_HAS_NO_VIDEO ở trên).
		if err := requireSubtitleUsable(ctx, s.uploads, req.SubtitleURL, videoContent.SubtitleURL, actorUserID, isAdmin); err != nil {
			return nil, err
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

	// P2 QA 260927 teacher: duration_minutes có thể vừa đổi — ghi lại total_duration_minutes
	// của course. section đã được nạp ở checkLessonCourseOwnership nhưng không trả ra ngoài,
	// nên nạp lại section ở đây để lấy CourseID (1 query rẻ, không đáng gộp lại chỉ vì việc này).
	//
	// Review đối kháng PR #70 (MINOR): trước đây "sErr == nil && section != nil" NUỐT lỗi khi
	// refetch section thất bại — UpdateLesson vẫn báo thành công dù total_duration_minutes có
	// thể sai lệch, không nhất quán với DeleteLesson (cùng file, cùng tình huống) vốn trả lỗi
	// ngay ("Errors Over Silent Fallbacks"). Nay trả lỗi giống DeleteLesson thay vì im lặng bỏ
	// qua; section == nil (không lỗi nhưng không tìm thấy) vẫn bỏ qua recalculate như cũ — lesson
	// vừa GetByID thành công ở trên nên trường hợp này gần như không xảy ra trên đường thật.
	section, sErr := s.sectionRepo.GetByID(ctx, lesson.SectionID)
	if sErr != nil {
		return nil, sErr
	}
	if section != nil {
		if err := s.courseRepo.RecalculateLessonStats(ctx, section.CourseID); err != nil {
			return nil, err
		}
	}

	// UpdateLesson đã qua checkLessonCourseOwnership: actor là chủ khoá / admin.
	return s.toLessonResponseDTO(lesson, contents, videoViewer{userID: actorUserID, original: true}), nil
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

	// P2 QA 260927 teacher: nạp CourseID TRƯỚC khi xoá lesson (sau khi xoá, lesson.SectionID vẫn
	// còn trong biến Go nhưng section có thể đã trống bài — nạp trước để chắc chắn có CourseID
	// dùng cho RecalculateLessonStats bên dưới, không phụ thuộc thứ tự xoá).
	section, sErr := s.sectionRepo.GetByID(ctx, lesson.SectionID)
	if sErr != nil {
		return sErr
	}

	// Lo 1 (fullstack-verify-260922): KHONG xoa file theo content.VideoURL. URL do client tu dat qua
	// CreateContent/UpdateContent va khong co bang so huu file, nen goi DeleteByURL o day cho phep giang
	// vien tro video_url vao bat ky object nao trong bucket (anh khoa hoc, video goc cua nguoi khac) roi
	// xoa content de xoa file do: chinh lo C-14 ma DELETE /api/upload da khoa bang SYSTEM_SETTINGS_MANAGE.
	// Video qua luong upload duoc don qua DeleteUpload (co kiem chu so huu); file legacy tro thang MinIO
	// chap nhan de mo coi.

	if err := s.lessonRepo.Delete(ctx, lessonID); err != nil {
		return err
	}

	if section != nil {
		if err := s.courseRepo.RecalculateLessonStats(ctx, section.CourseID); err != nil {
			return err
		}
	}
	return nil
}

func (s *LessonService) ReorderLessons(ctx context.Context, sectionID, actorUserID uuid.UUID, isAdmin bool, req dto.ReorderDTO) error {
	if _, err := s.checkSectionCourseOwnership(ctx, sectionID, actorUserID, isAdmin); err != nil {
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

func (s *LessonService) toLessonResponseDTO(lesson *model.Lesson, contents []model.LessonContent, viewer videoViewer) *dto.LessonResponseDTO {
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
		resp.Contents = make([]dto.LessonContentResponseDTO, len(contents))
		for i := range contents {
			// URL video ký (video_hls_url); URL file gốc chỉ cho chủ khoá/admin — xem applyVideoAccess.
			item := lessonContentToDTO(&contents[i], viewer)
			// Phase 1 §4: web doc subtitle_url AUTHORITATIVE tu chinh lesson content (item o
			// tren); truong resp.SubtitleURL o cap lesson chi la BAN DU PHONG (web tu ghi chu
			// "giu field nay lam du phong neu curriculum cung tra kem") — gan tu content video.
			// Dùng bản ĐÃ KÝ của item (bucket video private), không phải URL thô đã lưu.
			if contents[i].Type == "video" && item.SubtitleURL != nil {
				resp.SubtitleURL = item.SubtitleURL
			}
			resp.Contents[i] = item
		}
	}

	return resp
}
