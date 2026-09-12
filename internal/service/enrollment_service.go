package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrPaymentRequired: C-06 (audit 260909) — trước đây Enroll ghi danh được khóa trả phí mà
// không kiểm tra course.Price/IsFree hay đơn hàng đã thanh toán. Handler map lỗi này sang 402.
var ErrPaymentRequired = errors.New("payment required for this course")

type EnrollmentServiceInterface interface {
	Enroll(ctx context.Context, userID, courseID uuid.UUID) (*dto.EnrollmentResponseDTO, error)
	Unenroll(ctx context.Context, userID, courseID uuid.UUID) error
	GetMyEnrollments(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.EnrollmentListResponseDTO, error)
	GetEnrollmentDetail(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*dto.EnrollmentDetailDTO, error)
	UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO) (*dto.LessonProgressResponseDTO, error)
	GetCourseEnrollments(ctx context.Context, courseID uuid.UUID, page, pageSize int) (*dto.CourseEnrollmentListDTO, error)
	DebugGetCourseEnrollments(ctx context.Context, courseID uuid.UUID) ([]dto.DebugEnrollmentDTO, error)
}

type EnrollmentService struct {
	enrollmentRepo repository.EnrollmentRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
	lessonRepo    repository.LessonRepositoryInterface
}

func NewEnrollmentService(
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
) *EnrollmentService {
	return &EnrollmentService{
		enrollmentRepo: enrollmentRepo,
		courseRepo:     courseRepo,
		lessonRepo:    lessonRepo,
	}
}

func (s *EnrollmentService) Enroll(ctx context.Context, userID, courseID uuid.UUID) (*dto.EnrollmentResponseDTO, error) {
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, errors.New("course not found")
	}
	// C-06: endpoint tự-ghi-danh chỉ dành cho khóa MIỄN PHÍ. Khóa trả phí phải đi qua
	// payment_service.CheckAndProcessPayment (lane khác) sau khi đơn hàng completed.
	//
	// M-02/23b (review vòng 1): TRƯỚC ĐÂY điều kiện là "!course.IsFree && Price > 0" (AND) —
	// hai cột IsFree/Price độc lập, không có ràng buộc DB nào bắt chúng nhất quán, nên dữ liệu
	// IsFree=true nhưng Price>0 (hoàn toàn có thể xảy ra) sẽ ghi danh MIỄN PHÍ một khóa trả
	// phí. Bỏ hẳn nhánh IsFree khỏi điều kiện — chỉ dựa vào MỘT nguồn sự thật duy nhất là
	// Price (decimal.Decimal, không phải con trỏ nên không có rủi ro nil): khóa được coi là
	// miễn phí khi và chỉ khi Price.IsZero(). Không tin cột IsFree cho quyết định enroll-trực-
	// tiếp-hay-không nữa (IsFree vẫn có thể dùng để hiển thị UI, không phải nguồn sự thật).
	if !course.Price.IsZero() {
		return nil, ErrPaymentRequired
	}

	// Dùng bản Unscoped để phân biệt "chưa từng enroll" với "đã unenroll trước đó" — trước
	// đây GetByUserAndCourse (có scope mặc định, loại soft-delete) khiến re-enroll sau khi
	// Unenroll bị lỗi unique constraint (idx_user_course) vì bản ghi cũ vẫn còn trong DB.
	existing, err := s.enrollmentRepo.GetByUserAndCourseUnscoped(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	if existing != nil && !existing.DeletedAt.Valid {
		return nil, errors.New("already enrolled in this course")
	}

	if existing != nil && existing.DeletedAt.Valid {
		// Re-enroll: khôi phục bản ghi cũ thay vì INSERT mới (tránh vi phạm unique index),
		// đồng thời reset tiến trình học về trạng thái ban đầu.
		//
		// H-02 (review vòng 1): TRƯỚC ĐÂY gọi Restore() (đặt deleted_at=NULL) rồi gọi tiếp
		// enrollmentRepo.Update(existing) — Update dùng db.Save(), mà "existing" là struct đã
		// load TRƯỚC khi Restore chạy nên vẫn giữ DeletedAt.Valid=true trong bộ nhớ; Save() ghi
		// đè NGUYÊN struct (kể cả deleted_at cũ) → enrollment bị soft-delete lại NGAY LẬP TỨC dù
		// API trả 200 "đã ghi danh". Sửa: gộp restore + reset field vào MỘT UPDATE bằng map,
		// không đi qua struct đã stale.
		now := time.Now()
		updates := map[string]interface{}{
			"enrolled_at":         now,
			"completed_at":        nil,
			"last_accessed_at":    nil,
			"progress_percentage": decimal.Zero,
		}
		if err := s.enrollmentRepo.RestoreAndReactivate(ctx, existing.ID, updates); err != nil {
			return nil, err
		}
		existing.EnrolledAt = now
		existing.CompletedAt = nil
		existing.LastAccessedAt = nil
		existing.ProgressPercent = decimal.Zero
		if err := s.courseRepo.IncrementTotalStudents(ctx, courseID, 1); err != nil {
			return nil, err
		}
		// Re-enroll vua reset tien do hoc tren bang enrollments, nhung KHONG xoa lesson_progress cu
		// (cac dong do thuoc ban ghi enrollment duoc khoi phuc) — nguoi hoc quay lai van dang co
		// 5640s da xem, tra 0 o day la noi doi.
		//
		// Con so nay chi that su cong don duoc vi GetByUserAndCourseUnscoped o tren Preload
		// "LessonProgress" (review 260912, finding N1): bo Preload do di thi existing.LessonProgress
		// luon nil, sumWatchedSeconds luon = 0, va endpoint tra mot con so MAU THUAN voi
		// GET /my-enrollments cho cung ghi danh do — dung lop loi "comment noi mot dang, code lam
		// mot neo" ma khong test nao bat duoc. Test khoa lai: TestEnroll_ReEnrollTraWatchedSecondsThat.
		return setWatchedSeconds(s.toEnrollmentResponseDTO(existing), sumWatchedSeconds(existing.LessonProgress)), nil
	}

	enrollment := &model.Enrollment{
		UserID:     userID,
		CourseID:   courseID,
		EnrolledAt: time.Now(),
	}

	if err := s.enrollmentRepo.Create(ctx, enrollment); err != nil {
		return nil, err
	}
	if err := s.courseRepo.IncrementTotalStudents(ctx, courseID, 1); err != nil {
		return nil, err
	}

	// Ghi danh vua tao: chua the co ban ghi lesson_progress nao, nhung van gan tuong minh qua
	// setWatchedSeconds de moi noi tao DTO deu di qua cung mot duong.
	return setWatchedSeconds(s.toEnrollmentResponseDTO(enrollment), 0), nil
}

func (s *EnrollmentService) Unenroll(ctx context.Context, userID, courseID uuid.UUID) error {
	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return errors.New("not enrolled in this course")
	}

	if err := s.enrollmentRepo.Delete(ctx, enrollment.ID); err != nil {
		return err
	}
	return s.courseRepo.IncrementTotalStudents(ctx, courseID, -1)
}

func (s *EnrollmentService) GetMyEnrollments(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.EnrollmentListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	enrollments, total, err := s.enrollmentRepo.GetByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	enrollmentIDs := make([]uuid.UUID, len(enrollments))
	for i := range enrollments {
		enrollmentIDs[i] = enrollments[i].ID
	}
	// Khong nuot loi: neu khong cong don duoc thoi gian xem thi tra loi that, vi web dung
	// truong nay de hien thi chi so "thoi gian hoc" cho nguoi dung.
	watchedByEnrollment, err := s.enrollmentRepo.SumWatchedSecondsByEnrollmentIDs(ctx, enrollmentIDs)
	if err != nil {
		return nil, err
	}

	result := make([]dto.EnrollmentResponseDTO, len(enrollments))
	for i := range enrollments {
		// Danh sach: mot cau GROUP BY duy nhat cho TAT CA ghi danh (chan N+1 — xem
		// SumWatchedSecondsByEnrollmentIDs), khong dung sumWatchedSeconds() vi Course.LessonProgress
		// khong duoc Preload o day.
		d := setWatchedSeconds(s.toEnrollmentResponseDTO(&enrollments[i]), watchedByEnrollment[enrollments[i].ID])
		d.CourseTitle = enrollments[i].Course.Title
		d.CourseSlug = enrollments[i].Course.Slug
		d.CourseThumbnail = enrollments[i].Course.ThumbnailURL
		if enrollments[i].Course.Category != nil {
			d.CourseCategory = enrollments[i].Course.Category.Name
		}
		result[i] = *d
	}

	return &dto.EnrollmentListResponseDTO{
		Enrollments: result,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *EnrollmentService) GetEnrollmentDetail(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*dto.EnrollmentDetailDTO, error) {
	enrollment, err := s.enrollmentRepo.GetDetailByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, errors.New("enrollment not found")
	}
	if enrollment.UserID != userID {
		return nil, errors.New("enrollment not found")
	}

	detail := &dto.EnrollmentDetailDTO{
		// GetDetailByID da Preload("LessonProgress") => cong don tu du lieu co san, khong them
		// truy van nao (xem sumWatchedSeconds).
		EnrollmentResponseDTO: *setWatchedSeconds(s.toEnrollmentResponseDTO(enrollment), sumWatchedSeconds(enrollment.LessonProgress)),
	}
	detail.CourseTitle = enrollment.Course.Title
	detail.CourseThumbnail = enrollment.Course.ThumbnailURL

	progressDTOs := make([]dto.LessonProgressResponseDTO, len(enrollment.LessonProgress))
	for i, lp := range enrollment.LessonProgress {
		progressDTOs[i] = *s.toLessonProgressResponseDTO(&lp)
	}
	detail.LessonProgress = progressDTOs

	return detail, nil
}

func (s *EnrollmentService) UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO) (*dto.LessonProgressResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}

	// Find courseID via lesson -> section -> course (single JOIN query)
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	// Verify user is enrolled in this course
	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, errors.New("not enrolled in the course containing this lesson")
	}

	progress, err := s.enrollmentRepo.GetLessonProgress(ctx, userID, lessonID)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	// Duong INSERT: ban ghi chua ton tai thi tao moi bang Save() nhu cu (khong co UPDATE de
	// GREATEST() dua vao, va khong co gi de ghi de). Ban ghi DA ton tai thi di duong UPDATE
	// nguyen tu ben duoi.
	if progress == nil {
		progress = &model.LessonProgress{
			UserID:         userID,
			LessonID:       lessonID,
			EnrollmentID:   enrollment.ID,
			Status:         "not_started",
			LastAccessedAt: now,
		}
		if req.Status != nil {
			progress.Status = *req.Status
			if *req.Status == "completed" {
				progress.CompletedAt = &now
			}
		}
		if req.ProgressPercent != nil {
			progress.ProgressPercent = *req.ProgressPercent
		}
		if req.VideoWatchedSecs != nil {
			progress.VideoWatchedSecs = *req.VideoWatchedSecs
		}
		if err := s.enrollmentRepo.UpsertLessonProgress(ctx, progress); err != nil {
			return nil, err
		}
		return s.finishLessonProgressUpdate(ctx, enrollment, progress)
	}

	// Trinh phat gui VI TRI phat hien tai (currentTime), khong phai so giay cong don.
	// Neu ghi de thang thi xem lai bai tu dau se GHI DE mot gia tri lon bang mot gia tri
	// nho, va chi so "thoi gian hoc" tren web tu dung giam. Cot nay ten la
	// video_watched_seconds nen ngu nghia dung la "da xem toi giay thu may" => chi tang.
	// Vi tri tua-lai co cot rieng (last_position_seconds), khong dung cot nay.
	//
	// Cac cot duoc ghi qua map nay: Save() cu ghi DE TOAN BO struct nen ghi de ca status/
	// completed_at ma request song song vua ghi. Map duoi day CHI chua dung cac cot thuc su doi,
	// va video_watched_seconds duoc day xuong SQL duoi dang GREATEST(...) de phep max la nguyen tu.
	updates := map[string]interface{}{
		"last_accessed_at": now,
		// updated_at: UpdateColumns bo qua hook tu dong cua GORM nen phai set tuong minh, neu khong
		// cot nay dung yen mai mai.
		"updated_at": now,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
		if *req.Status == "completed" && progress.CompletedAt == nil {
			updates["completed_at"] = now
		}
	}
	if req.ProgressPercent != nil {
		updates["progress_percentage"] = *req.ProgressPercent
	}

	// watchedSeconds duoc truyen RIENG chu khong nam trong map: gia tri cua no phai di qua
	// GREATEST() trong cau UPDATE, khong duoc set tho.
	var watchedSeconds *int
	if req.VideoWatchedSecs != nil {
		watchedSeconds = req.VideoWatchedSecs
	}

	updated, err := s.enrollmentRepo.UpdateLessonProgressFields(ctx, userID, lessonID, updates, watchedSeconds)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		// Bản ghi bi xoa giua luc doc va luc ghi — khong co dong nao duoc UPDATE. Tra loi that thay
		// vi tra ve mot DTO mang gia tri chua he duoc ghi xuong DB.
		return nil, errors.New("lesson progress not found")
	}

	return s.finishLessonProgressUpdate(ctx, enrollment, updated)
}

// finishLessonProgressUpdate tinh lai tien do ghi danh va tra ve DTO cua ban ghi vua ghi.
func (s *EnrollmentService) finishLessonProgressUpdate(ctx context.Context, enrollment *model.Enrollment, progress *model.LessonProgress) (*dto.LessonProgressResponseDTO, error) {
	if err := s.recalculateProgress(ctx, enrollment); err != nil {
		return nil, err
	}
	return s.toLessonProgressResponseDTO(progress), nil
}

func (s *EnrollmentService) recalculateProgress(ctx context.Context, enrollment *model.Enrollment) error {
	completed, err := s.enrollmentRepo.CountCompletedMandatory(ctx, enrollment.ID)
	if err != nil {
		return err
	}

	total, err := s.enrollmentRepo.CountTotalMandatory(ctx, enrollment.CourseID)
	if err != nil {
		return err
	}

	var progressPercent decimal.Decimal
	if total > 0 {
		progressPercent = decimal.NewFromInt(completed).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(total))
	}

	enrollment.ProgressPercent = progressPercent
	if err := s.enrollmentRepo.UpdateEnrollmentProgress(ctx, enrollment.ID, progressPercent); err != nil {
		return err
	}

	// Auto-complete enrollment when 100%
	if progressPercent.Equal(decimal.NewFromInt(100)) {
		now := time.Now()
		enrollment.CompletedAt = &now
		if err := s.enrollmentRepo.Update(ctx, enrollment); err != nil {
			return err
		}
	}

	return nil
}

// setWatchedSeconds gan WatchedSeconds cho mot DTO ghi danh.
//
// Ly do ton tai (review 260912, finding #3): WatchedSeconds la `int` KHONG `omitempty`, nen MOI
// noi tao EnrollmentResponseDTO deu phai gan. Truoc day chi GetMyEnrollments gan, con Enroll
// (ca hai nhanh) va GetEnrollmentDetail tra ve 0 — client khong phan biet duoc "chua xem gi"
// voi "endpoint nay khong tinh". Giu duong ghi nay o DUNG MOT cho de khong lap lai loi do.
func setWatchedSeconds(d *dto.EnrollmentResponseDTO, seconds int) *dto.EnrollmentResponseDTO {
	d.WatchedSeconds = seconds
	return d
}

// sumWatchedSeconds cong don video_watched_seconds cua cac ban ghi tien do DA CO SAN trong bo nho.
//
// Dung cho endpoint chi tiet: GetDetailByID da Preload("LessonProgress") nen du lieu nam san
// trong enrollment.LessonProgress — khong can them truy van nao, va cung khong vi pham chan N+1
// (khong co vong lap tren nhieu ghi danh o day; endpoint danh sach VAN dung mot cau GROUP BY
// duy nhat qua SumWatchedSecondsByEnrollmentIDs).
func sumWatchedSeconds(progresses []model.LessonProgress) int {
	total := 0
	for i := range progresses {
		total += progresses[i].VideoWatchedSecs
	}
	return total
}

func (s *EnrollmentService) toEnrollmentResponseDTO(enrollment *model.Enrollment) *dto.EnrollmentResponseDTO {
	resp := &dto.EnrollmentResponseDTO{
		ID:              enrollment.ID,
		UserID:          enrollment.UserID,
		CourseID:        enrollment.CourseID,
		EnrolledAt:      enrollment.EnrolledAt.Format("2006-01-02T15:04:05Z"),
		ProgressPercent: enrollment.ProgressPercent,
	}
	if enrollment.CompletedAt != nil {
		formatted := enrollment.CompletedAt.Format("2006-01-02T15:04:05Z")
		resp.CompletedAt = &formatted
	}
	if enrollment.LastAccessedAt != nil {
		formatted := enrollment.LastAccessedAt.Format("2006-01-02T15:04:05Z")
		resp.LastAccessedAt = &formatted
	}
	return resp
}

func (s *EnrollmentService) toLessonProgressResponseDTO(lp *model.LessonProgress) *dto.LessonProgressResponseDTO {
	resp := &dto.LessonProgressResponseDTO{
		ID:               lp.ID,
		UserID:           lp.UserID,
		LessonID:         lp.LessonID,
		EnrollmentID:     lp.EnrollmentID,
		Status:           lp.Status,
		ProgressPercent:  lp.ProgressPercent,
		VideoWatchedSecs: lp.VideoWatchedSecs,
		LastAccessedAt:   lp.LastAccessedAt.Format("2006-01-02T15:04:05Z"),
	}
	if lp.CompletedAt != nil {
		formatted := lp.CompletedAt.Format("2006-01-02T15:04:05Z")
		resp.CompletedAt = &formatted
	}
	return resp
}

// GetCourseEnrollments returns all enrollments for a course (instructor view)
func (s *EnrollmentService) GetCourseEnrollments(ctx context.Context, courseID uuid.UUID, page, pageSize int) (*dto.CourseEnrollmentListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	enrollments, total, err := s.enrollmentRepo.GetByCourseID(ctx, courseID, page, pageSize)
	if err != nil {
		return nil, err
	}

	items := make([]dto.CourseEnrollmentItemDTO, len(enrollments))
	for i, e := range enrollments {
		items[i] = dto.CourseEnrollmentItemDTO{
			ID:              e.ID,
			UserID:          e.UserID,
			EnrolledAt:      e.EnrolledAt.Format("2006-01-02T15:04:05Z"),
			ProgressPercent: e.ProgressPercent,
		}
		if e.User.Email != "" {
			items[i].UserEmail = e.User.Email
		}
		if e.User.FullName != nil {
			items[i].UserName = *e.User.FullName
		}
		if e.CompletedAt != nil {
			formatted := e.CompletedAt.Format("2006-01-02T15:04:05Z")
			items[i].CompletedAt = &formatted
		}
	}

	return &dto.CourseEnrollmentListDTO{
		Enrollments: items,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

// DebugGetCourseEnrollments returns all enrollments including soft-deleted (for debugging)
func (s *EnrollmentService) DebugGetCourseEnrollments(ctx context.Context, courseID uuid.UUID) ([]dto.DebugEnrollmentDTO, error) {
	enrollments, err := s.enrollmentRepo.GetByCourseIDIncludeDeleted(ctx, courseID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.DebugEnrollmentDTO, len(enrollments))
	for i, e := range enrollments {
		result[i] = dto.DebugEnrollmentDTO{
			ID:         e.ID,
			UserID:     e.UserID,
			CourseID:   e.CourseID,
			EnrolledAt: e.EnrolledAt.Format("2006-01-02T15:04:05Z"),
			IsDeleted:  e.DeletedAt.Valid,
		}
		if e.User.Email != "" {
			result[i].UserEmail = e.User.Email
		}
		if e.DeletedAt.Valid {
			formatted := e.DeletedAt.Time.Format("2006-01-02T15:04:05Z")
			result[i].DeletedAt = &formatted
		}
	}

	return result, nil
}
