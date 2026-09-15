package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ClassLessonContentServiceInterface interface {
	AssignClassToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, isAdmin bool, req dto.AssignClassToContentDTO) (*dto.ClassLessonContentResponseDTO, error)
	UpdateClassContentSchedule(ctx context.Context, contentID, classID, userID uuid.UUID, isAdmin bool, req dto.UpdateClassContentScheduleDTO) (*dto.ClassLessonContentResponseDTO, error)
	RemoveClassFromContent(ctx context.Context, contentID, classID, userID uuid.UUID, isAdmin bool) error
	GetClassesForContent(ctx context.Context, contentID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error)
	GetContentScheduleForClass(ctx context.Context, classID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error)
	BulkAssignClassesToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, isAdmin bool, req dto.BulkAssignClassesToContentDTO) ([]dto.ClassLessonContentResponseDTO, error)
}

type ClassLessonContentService struct {
	clcRepo        repository.ClassLessonContentRepositoryInterface
	classRepo      repository.ClassRepositoryInterface
	// courseRepo (V3-7, issue #58): can de kiem instructor cua khoa chua lop — xem
	// class_access.go. Truoc day service nay khong he biet khoa nao day lop nao.
	courseRepo     repository.CourseRepositoryInterface
	lessonRepo     repository.LessonRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
	livestreamSvc  LivestreamServiceInterface
}

func NewClassLessonContentService(
	clcRepo repository.ClassLessonContentRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	livestreamSvc LivestreamServiceInterface,
) *ClassLessonContentService {
	return &ClassLessonContentService{
		clcRepo:        clcRepo,
		classRepo:      classRepo,
		courseRepo:     courseRepo,
		lessonRepo:     lessonRepo,
		enrollmentRepo: enrollmentRepo,
		livestreamSvc:  livestreamSvc,
	}
}

func (s *ClassLessonContentService) AssignClassToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, isAdmin bool, req dto.AssignClassToContentDTO) (*dto.ClassLessonContentResponseDTO, error) {
	// Validate content exists
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	// Validate class exists
	class, err := s.classRepo.GetByID(ctx, req.ClassID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// V3-7 (issue #58): truoc day bat ky user dang nhap nao cung gan duoc mot lesson content vao
	// BAT KY lop nao cua cung khoa hoc — ke ca hoc sinh. Gio chi giao vien cua lop / instructor
	// cua khoa chua lop (hoac admin) moi duoc gan lich.
	if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, userID, req.ClassID, isAdmin); err != nil {
		return nil, err
	}

	// Validate class belongs to the same course as the content
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, content.LessonID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve course from lesson: %w", err)
	}
	if class.CourseID == nil || *class.CourseID != courseID {
		return nil, errors.New("class does not belong to this course")
	}

	// Check duplicate
	exists, err := s.clcRepo.Exists(ctx, req.ClassID, contentID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("class is already assigned to this content")
	}

	// Parse dates
	clc := &model.ClassLessonContent{
		ID:              uuid.New(),
		ClassID:         req.ClassID,
		LessonContentID: contentID,
		Status:          "scheduled",
	}

	if err := s.parseDates(req.OpenDate, req.DueDate, req.ScheduledAt, req.EndAt, clc); err != nil {
		return nil, err
	}

	// Livestream conflict check
	if content.Type == "livestream" && clc.ScheduledAt != nil && clc.EndAt != nil {
		overlap, err := s.clcRepo.HasOverlappingLivestream(ctx, req.ClassID, *clc.ScheduledAt, *clc.EndAt, nil)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, errors.New("class has an overlapping livestream session at this time")
		}
	}

	if err := s.clcRepo.Create(ctx, clc); err != nil {
		return nil, err
	}

	// Auto-create LivestreamSession for livestream content
	if content.Type == "livestream" && clc.ScheduledAt != nil {
		s.createLivestreamSession(ctx, clc, content, class, courseID, userID)
	}

	// Reload with preloaded relationships
	created, err := s.clcRepo.GetByClassAndContent(ctx, req.ClassID, contentID)
	if err != nil {
		return nil, err
	}

	return s.toResponseDTO(created), nil
}

func (s *ClassLessonContentService) UpdateClassContentSchedule(ctx context.Context, contentID, classID, userID uuid.UUID, isAdmin bool, req dto.UpdateClassContentScheduleDTO) (*dto.ClassLessonContentResponseDTO, error) {
	// V3-7 (issue #58): doi lich hoc cua lop la thao tac GHI len lop — truoc day khong kiem quyen.
	if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, userID, classID, isAdmin); err != nil {
		return nil, err
	}

	clc, err := s.clcRepo.GetByClassAndContent(ctx, classID, contentID)
	if err != nil {
		return nil, err
	}
	if clc == nil {
		return nil, errors.New("class-content assignment not found")
	}

	if req.OpenDate != nil {
		t, err := time.Parse(time.RFC3339, *req.OpenDate)
		if err != nil {
			return nil, errors.New("invalid open_date format, expected RFC3339")
		}
		clc.OpenDate = &t
	}
	if req.DueDate != nil {
		t, err := time.Parse(time.RFC3339, *req.DueDate)
		if err != nil {
			return nil, errors.New("invalid due_date format, expected RFC3339")
		}
		clc.DueDate = &t
	}
	if req.ScheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ScheduledAt)
		if err != nil {
			return nil, errors.New("invalid scheduled_at format, expected RFC3339")
		}
		clc.ScheduledAt = &t
	}
	if req.EndAt != nil {
		t, err := time.Parse(time.RFC3339, *req.EndAt)
		if err != nil {
			return nil, errors.New("invalid end_at format, expected RFC3339")
		}
		clc.EndAt = &t
	}
	if req.Status != nil {
		clc.Status = *req.Status
	}

	// Livestream conflict check if times changed
	if clc.LessonContent.Type == "livestream" && clc.ScheduledAt != nil && clc.EndAt != nil {
		overlap, err := s.clcRepo.HasOverlappingLivestream(ctx, classID, *clc.ScheduledAt, *clc.EndAt, &contentID)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, errors.New("class has an overlapping livestream session at this time")
		}
	}

	if err := s.clcRepo.Update(ctx, clc); err != nil {
		return nil, err
	}

	// Reload
	updated, err := s.clcRepo.GetByClassAndContent(ctx, classID, contentID)
	if err != nil {
		return nil, err
	}

	return s.toResponseDTO(updated), nil
}

func (s *ClassLessonContentService) RemoveClassFromContent(ctx context.Context, contentID, classID, userID uuid.UUID, isAdmin bool) error {
	// V3-7 (issue #58): go mot lesson content khoi lop la thao tac GHI (xoa lich cua ca lop).
	if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, userID, classID, isAdmin); err != nil {
		return err
	}

	clc, err := s.clcRepo.GetByClassAndContent(ctx, classID, contentID)
	if err != nil {
		return err
	}
	if clc == nil {
		return errors.New("class-content assignment not found")
	}

	return s.clcRepo.Delete(ctx, classID, contentID)
}

func (s *ClassLessonContentService) GetClassesForContent(ctx context.Context, contentID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error) {
	// Validate content exists
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := s.clcRepo.GetByContentID(ctx, contentID, page, pageSize)
	if err != nil {
		return nil, err
	}

	// V3-7 (issue #58): truoc day bat ky user dang nhap nao cung doc duoc danh sach lop duoc gan
	// vao mot lesson content. Quyen duoc xet tren dung tap lop cua trang nay: nguoi goi phai la
	// thanh vien (giao vien lop/instructor khoa/hoc sinh) cua IT NHAT MOT lop trong so do.
	// Danh doi da biet: mot giao vien co lop nam ngoai trang hien tai se nhan 403 thay vi thay
	// dung phan cua minh — fail-closed (khong ro ri), va so lop cua mot lesson content thuong
	// nam gon trong mot trang.
	classIDs := make([]uuid.UUID, len(items))
	for i, item := range items {
		classIDs[i] = item.ClassID
	}
	if err := ensureAnyClassView(ctx, s.classRepo, s.courseRepo, userID, isAdmin, classIDs); err != nil {
		return nil, err
	}

	dtos := make([]dto.ClassLessonContentResponseDTO, len(items))
	for i, item := range items {
		dtos[i] = *s.toResponseDTO(&item)
	}

	return &dto.ClassLessonContentListResponseDTO{
		Items:    dtos,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassLessonContentService) GetContentScheduleForClass(ctx context.Context, classID, userID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error) {
	// Validate class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// V3-7 (issue #58): thoi khoa bieu cua mot lop chi duoc xem boi giao vien lop/instructor khoa,
	// hoc sinh cua lop, hoac admin — truoc day bat ky user dang nhap nao cung xem duoc.
	if err := ensureClassView(ctx, s.classRepo, s.courseRepo, userID, classID, isAdmin); err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := s.clcRepo.GetByClassID(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.ClassLessonContentResponseDTO, len(items))
	for i, item := range items {
		dtos[i] = *s.toResponseDTO(&item)
	}

	return &dto.ClassLessonContentListResponseDTO{
		Items:    dtos,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassLessonContentService) BulkAssignClassesToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, isAdmin bool, req dto.BulkAssignClassesToContentDTO) ([]dto.ClassLessonContentResponseDTO, error) {
	// Validate content exists
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	// Resolve courseID
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, content.LessonID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve course from lesson: %w", err)
	}

	// Determine class list
	var classIDs []uuid.UUID
	if len(req.ClassIDs) > 0 {
		classIDs = req.ClassIDs
	} else {
		// Get all classes in course
		classes, err := s.classRepo.GetByCourseID(ctx, courseID)
		if err != nil {
			return nil, err
		}
		for _, c := range classes {
			classIDs = append(classIDs, c.ID)
		}
	}

	if len(classIDs) == 0 {
		return nil, errors.New("no classes found for this course")
	}

	var clcs []*model.ClassLessonContent
	for _, classID := range classIDs {
		// Validate class belongs to course
		class, err := s.classRepo.GetByID(ctx, classID)
		if err != nil {
			return nil, err
		}
		if class == nil {
			return nil, fmt.Errorf("class not found: %s", classID)
		}
		if class.CourseID == nil || *class.CourseID != courseID {
			return nil, fmt.Errorf("class %s does not belong to this course", classID)
		}

		// V3-7 (issue #58): gom phep kiem quyen cua tung lop vao chinh vong lap da co san (vong
		// nay da load `class` cho muc dich khac) — khong ton them truy van nao. Duong
		// `req.ClassIDs` rong chi goi ham nay KHONG tu kiem quyen gi ca (khac voi
		// AssignClassToContent), nen neu bo qua cho nay thi "gan ca khoa" van la mot lo hong mo.
		if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, userID, classID, isAdmin); err != nil {
			return nil, err
		}

		// Skip if already assigned
		exists, err := s.clcRepo.Exists(ctx, classID, contentID)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}

		clc := &model.ClassLessonContent{
			ID:              uuid.New(),
			ClassID:         classID,
			LessonContentID: contentID,
			Status:          "scheduled",
		}

		if err := s.parseDates(req.OpenDate, req.DueDate, req.ScheduledAt, req.EndAt, clc); err != nil {
			return nil, err
		}

		// Livestream conflict check
		if content.Type == "livestream" && clc.ScheduledAt != nil && clc.EndAt != nil {
			overlap, err := s.clcRepo.HasOverlappingLivestream(ctx, classID, *clc.ScheduledAt, *clc.EndAt, nil)
			if err != nil {
				return nil, err
			}
			if overlap {
				return nil, fmt.Errorf("class %s has an overlapping livestream session at this time", class.Name)
			}
		}

		clcs = append(clcs, clc)
	}

	if len(clcs) == 0 {
		return nil, errors.New("all classes are already assigned to this content")
	}

	if err := s.clcRepo.CreateBatch(ctx, clcs); err != nil {
		return nil, err
	}

	// Build response
	result := make([]dto.ClassLessonContentResponseDTO, len(clcs))
	for i, clc := range clcs {
		loaded, err := s.clcRepo.GetByClassAndContent(ctx, clc.ClassID, contentID)
		if err != nil || loaded == nil {
			continue
		}
		result[i] = *s.toResponseDTO(loaded)
	}

	return result, nil
}

// Helpers

func (s *ClassLessonContentService) parseDates(openDate, dueDate, scheduledAt, endAt *string, clc *model.ClassLessonContent) error {
	if openDate != nil {
		t, err := time.Parse(time.RFC3339, *openDate)
		if err != nil {
			return errors.New("invalid open_date format, expected RFC3339")
		}
		clc.OpenDate = &t
	}
	if dueDate != nil {
		t, err := time.Parse(time.RFC3339, *dueDate)
		if err != nil {
			return errors.New("invalid due_date format, expected RFC3339")
		}
		clc.DueDate = &t
	}
	if scheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *scheduledAt)
		if err != nil {
			return errors.New("invalid scheduled_at format, expected RFC3339")
		}
		clc.ScheduledAt = &t
	}
	if endAt != nil {
		t, err := time.Parse(time.RFC3339, *endAt)
		if err != nil {
			return errors.New("invalid end_at format, expected RFC3339")
		}
		clc.EndAt = &t
	}
	return nil
}

func (s *ClassLessonContentService) createLivestreamSession(ctx context.Context, clc *model.ClassLessonContent, content *model.LessonContent, class *model.Class, courseID uuid.UUID, userID uuid.UUID) {
	title := "Livestream"
	if content.Title != nil {
		title = *content.Title
	}
	scheduledAt := ""
	if clc.ScheduledAt != nil {
		scheduledAt = clc.ScheduledAt.Format(time.RFC3339)
	}

	livestreamReq := dto.CreateLivestreamDTO{
		Title:           fmt.Sprintf("%s - %s", title, class.Name),
		ClassID:         clc.ClassID.String(),
		CourseID:        courseID.String(),
		LessonContentID: clc.LessonContentID.String(),
		MaxViewers:      1000,
		IsRecorded:      true,
		ScheduledAt:     scheduledAt,
	}

	// N10 (review vòng 2, từ review web): trước đây phiên tạo xong rồi bỏ qua (`_, _ =`),
	// không có cách nào từ content lấy lại được id phiên — web mở phòng theo id lesson_content
	// (`/rooms/<lesson_content_id>`) nên join luôn hỏng vì đó không phải RoomName/session id
	// thật. Ghi lại content.LivestreamSessionID khi tạo phiên thành công để
	// LessonContentResponseDTO trả đúng id phiên cho web. Lỗi tạo phiên hoặc ghi lại vẫn không
	// làm hỏng luồng gán lịch (giữ nguyên hành vi cũ: không throw ra ngoài AssignClassToContent)
	// — nhưng không còn im lặng nuốt kết quả nữa.
	session, err := s.livestreamSvc.Create(ctx, userID, livestreamReq)
	if err != nil || session == nil {
		// V3-2 (review vòng 3): không nuốt im lặng — thường là 403 do người gán lịch
		// (admin/org-owner) không phải GV lớp/instructor khoá (kiểm quyền N1), khiến
		// content có lịch nhưng livestream_session_id null vĩnh viễn.
		log.Printf("[WARN] createLivestreamSession: content=%s class=%s user=%s: %v",
			clc.LessonContentID, clc.ClassID, userID, err)
		return
	}
	content.LivestreamSessionID = &session.ID
	if err := s.lessonRepo.UpdateContent(ctx, content); err != nil {
		log.Printf("[WARN] createLivestreamSession: ghi livestream_session_id thất bại content=%s session=%s: %v",
			clc.LessonContentID, session.ID, err)
	}
}

func (s *ClassLessonContentService) toResponseDTO(clc *model.ClassLessonContent) *dto.ClassLessonContentResponseDTO {
	resp := &dto.ClassLessonContentResponseDTO{
		ID:              clc.ID,
		ClassID:         clc.ClassID,
		LessonContentID: clc.LessonContentID,
		OpenDate:        clc.OpenDate,
		DueDate:         clc.DueDate,
		ScheduledAt:     clc.ScheduledAt,
		EndAt:           clc.EndAt,
		Status:          clc.Status,
		CreatedAt:       clc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       clc.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	// From preloaded relationships
	resp.ClassName = clc.Class.Name
	resp.ContentType = clc.LessonContent.Type
	resp.ContentTitle = clc.LessonContent.Title
	// N10 (review vòng 2, bổ sung theo yêu cầu mở rộng của team-lead sang mapper "class lesson
	// content"): xem chú thích tại model.LessonContent.LivestreamSessionID.
	resp.LivestreamSessionID = clc.LessonContent.LivestreamSessionID

	return resp
}
