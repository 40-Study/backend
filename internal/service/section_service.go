package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrNotSectionCourseOwner dùng chung cho mọi thao tác ghi section — chỉ giảng viên sở hữu
// khóa học cha (course.InstructorID) mới được tạo/sửa/xóa section (C-12).
var ErrNotSectionCourseOwner = errors.New("forbidden: not the owner")

type SectionServiceInterface interface {
	CreateSection(ctx context.Context, courseID, actorUserID uuid.UUID, req dto.CreateSectionDTO) (*dto.SectionResponseDTO, error)
	// GetAllSections (Phase 1 §2): userID dung de tinh locked/lock_reason/progress cho TUNG bai
	// theo dung nguoi dang xem — route nay luon di kem auth nen userID khong bao gio la uuid.Nil
	// tren duong that; xem sectionForbiddenResponse/handler. isAdmin (CAO-4, review vòng 2): chủ
	// khoá học / admin xem curriculum của chính khoá mình không bị khoá bài nào (BypassLock).
	GetAllSections(ctx context.Context, courseID, userID uuid.UUID, isAdmin bool) ([]dto.SectionResponseDTO, error)
	// GetSectionByID (D4, QA vòng 2): viewerID/isAdmin để ẩn chương của khoá chưa xuất bản với
	// người ngoài (canViewCourse) — trước đây route này trả chương của bất kỳ khoá nào.
	GetSectionByID(ctx context.Context, sectionID, viewerID uuid.UUID, isAdmin bool) (*dto.SectionResponseDTO, error)
	UpdateSection(ctx context.Context, courseID, sectionID, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateSectionDTO) (*dto.SectionResponseDTO, error)
	DeleteSection(ctx context.Context, courseID, sectionID, actorUserID uuid.UUID, isAdmin bool) error
	// ReorderSections (QA vòng 2): trước đây không nhận người gọi nên KHÔNG kiểm chủ sở hữu —
	// ai đăng nhập cũng sắp lại được chương của khoá người khác.
	ReorderSections(ctx context.Context, courseID, actorUserID uuid.UUID, isAdmin bool, req dto.ReorderDTO) error
}

type SectionService struct {
	sectionRepo    repository.SectionRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
}

func NewSectionService(
	sectionRepo repository.SectionRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
) *SectionService {
	return &SectionService{
		sectionRepo:    sectionRepo,
		courseRepo:     courseRepo,
		enrollmentRepo: enrollmentRepo,
	}
}

// validateCourseOwnership kiểm course tồn tại, actorUserID là giảng viên sở hữu course (C-12),
// và khoá không đang chờ duyệt (Q5 — ensureCourseEditable) — dùng cho mọi thao tác GHI trên section.
// isAdmin (vòng 2, đã tính sẵn ở handler qua PermissionChecker) cho phép SYSTEM_ADMIN
// override chủ sở hữu; CreateSection luôn gọi với isAdmin=false (admin không tạo section hộ
// giảng viên khác, chỉ được sửa/xóa nội dung vi phạm — đúng phạm vi C-12 vòng 2).
func (s *SectionService) validateCourseOwnership(ctx context.Context, courseID, actorUserID uuid.UUID, isAdmin bool) error {
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return err
	}
	if course == nil {
		return errors.New("course not found")
	}
	if course.InstructorID != actorUserID && !isAdmin {
		return ErrNotSectionCourseOwner
	}
	return ensureCourseEditable(course)
}

func (s *SectionService) CreateSection(ctx context.Context, courseID, actorUserID uuid.UUID, req dto.CreateSectionDTO) (*dto.SectionResponseDTO, error) {
	if err := s.validateCourseOwnership(ctx, courseID, actorUserID, false); err != nil {
		return nil, err
	}
	// Determine display order
	maxOrder, err := s.sectionRepo.GetMaxDisplayOrder(ctx, courseID)
	if err != nil {
		return nil, err
	}

	displayOrder := maxOrder + 1
	if req.DisplayOrder > 0 {
		displayOrder = req.DisplayOrder
	}

	section := &model.Section{
		CourseID:     courseID,
		Title:        req.Title,
		Description:  req.Description,
		DisplayOrder: displayOrder,
	}

	if err := s.sectionRepo.Create(ctx, section); err != nil {
		return nil, err
	}

	return s.toSectionResponseDTO(section, nil), nil
}

// GetAllSections (Phase 1 §2): "fetcher server" ma trang hoc (web) dung de lay curriculum —
// day la endpoint DUY NHAT can tinh day du locked/lock_reason/progress theo NGUOI DANG XEM,
// khac voi CourseService.GetCourseBySlug (public, khong biet nguoi xem la ai).
func (s *SectionService) GetAllSections(ctx context.Context, courseID, userID uuid.UUID, isAdmin bool) ([]dto.SectionResponseDTO, error) {
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, ErrCourseHidden
	}

	sections, err := s.sectionRepo.GetAllByCourseID(ctx, courseID)
	if err != nil {
		return nil, err
	}

	// CAO-4: giảng viên sở hữu chính khoá học này, hoặc admin hệ thống, xem curriculum không
	// bị khoá bài nào (BypassLock — xem lesson_lock.go).
	bypass := isAdmin || course.InstructorID == userID
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, courseID, course.Sequential, bypass)
	if err != nil {
		return nil, err
	}
	if !canViewCourse(course, userID, isAdmin, lockInput.Enrolled) {
		return nil, ErrCourseHidden
	}

	result := make([]dto.SectionResponseDTO, len(sections))
	for i, sec := range sections {
		lessons := make([]dto.LessonResponseDTO, len(sec.Lessons))
		for j, les := range sec.Lessons {
			lessons[j] = s.toLessonResponseDTO(&les, nil)
			locked, reason, progress := ResolveLessonLock(les.ID, lockInput)
			lessons[j].Locked = locked
			lessons[j].LockReason = reason
			lessons[j].Progress = progress
		}
		result[i] = *s.toSectionResponseDTO(&sec, lessons)
	}

	return result, nil
}

func (s *SectionService) GetSectionByID(ctx context.Context, sectionID, viewerID uuid.UUID, isAdmin bool) (*dto.SectionResponseDTO, error) {
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
		return nil, ErrCourseHidden
	}
	enrolled, err := isEnrolledInCourse(ctx, s.enrollmentRepo, viewerID, course.ID)
	if err != nil {
		return nil, err
	}
	if !canViewCourse(course, viewerID, isAdmin, enrolled) {
		return nil, ErrCourseHidden
	}

	lessons := make([]dto.LessonResponseDTO, len(section.Lessons))
	for j, les := range section.Lessons {
		lessons[j] = s.toLessonResponseDTO(&les, nil)
	}

	return s.toSectionResponseDTO(section, lessons), nil
}

func (s *SectionService) UpdateSection(ctx context.Context, courseID, sectionID, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateSectionDTO) (*dto.SectionResponseDTO, error) {
	if err := s.validateCourseOwnership(ctx, courseID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	belongs, err := s.sectionRepo.BelongsToCourse(ctx, sectionID, courseID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("section not found in this course")
	}

	section, err := s.sectionRepo.GetByID(ctx, sectionID)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return nil, errors.New("section not found")
	}

	if req.Title != nil {
		section.Title = *req.Title
	}
	if req.Description != nil {
		section.Description = req.Description
	}
	if req.DisplayOrder != nil {
		section.DisplayOrder = *req.DisplayOrder
	}

	if err := s.sectionRepo.Update(ctx, section); err != nil {
		return nil, err
	}

	lessons := make([]dto.LessonResponseDTO, len(section.Lessons))
	for j, les := range section.Lessons {
		lessons[j] = s.toLessonResponseDTO(&les, nil)
	}

	return s.toSectionResponseDTO(section, lessons), nil
}

func (s *SectionService) DeleteSection(ctx context.Context, courseID, sectionID, actorUserID uuid.UUID, isAdmin bool) error {
	if err := s.validateCourseOwnership(ctx, courseID, actorUserID, isAdmin); err != nil {
		return err
	}

	belongs, err := s.sectionRepo.BelongsToCourse(ctx, sectionID, courseID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("section not found in this course")
	}

	if err := s.sectionRepo.Delete(ctx, sectionID); err != nil {
		return err
	}

	// P2 QA 260927 teacher: xoá cả chương kéo theo xoá mọi bài trong đó (cascade) — ghi lại
	// total_lessons/total_duration_minutes của course cho khớp.
	return s.courseRepo.RecalculateLessonStats(ctx, courseID)
}

func (s *SectionService) ReorderSections(ctx context.Context, courseID, actorUserID uuid.UUID, isAdmin bool, req dto.ReorderDTO) error {
	if err := s.validateCourseOwnership(ctx, courseID, actorUserID, isAdmin); err != nil {
		return err
	}

	items := make([]repository.ReorderItem, len(req.Items))
	for i, item := range req.Items {
		belongs, err := s.sectionRepo.BelongsToCourse(ctx, item.ID, courseID)
		if err != nil {
			return err
		}
		if !belongs {
			return errors.New("section " + item.ID.String() + " does not belong to this course")
		}
		items[i] = repository.ReorderItem{
			ID:           item.ID,
			DisplayOrder: item.DisplayOrder,
		}
	}

	return s.sectionRepo.Reorder(ctx, items)
}

func (s *SectionService) toSectionResponseDTO(section *model.Section, lessons []dto.LessonResponseDTO) *dto.SectionResponseDTO {
	return &dto.SectionResponseDTO{
		ID:           section.ID,
		CourseID:     section.CourseID,
		Title:        section.Title,
		Description:  section.Description,
		DisplayOrder: section.DisplayOrder,
		Lessons:      lessons,
		CreatedAt:    section.CreatedAt,
		UpdatedAt:    section.UpdatedAt,
	}
}

func (s *SectionService) toLessonResponseDTO(lesson *model.Lesson, contents []model.LessonContent) dto.LessonResponseDTO {
	resp := dto.LessonResponseDTO{
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
			// Hiện mọi caller đều truyền contents=nil; nếu sau này có caller truyền contents, video
			// nội bộ KHÔNG được lộ URL chưa ký ở đây — withheldVideoViewer không cấp URL nào.
			resp.Contents[i] = lessonContentToDTO(&contents[i], withheldVideoViewer)
		}
	}

	return resp
}
