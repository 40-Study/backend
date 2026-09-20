package service

import (
	"context"
	"errors"
	"log"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrPaymentRequired: C-06 (audit 260909) — trước đây Enroll ghi danh được khóa trả phí mà
// không kiểm tra course.Price/IsFree hay đơn hàng đã thanh toán. Handler map lỗi này sang 402.
var ErrPaymentRequired = errors.New("payment required for this course")

// lessonProgressStatusRank (HIGH-2, review 260915): status chỉ được đi LÊN, không bao giờ đi
// LÙI. Beacon `POST /api/progress` (sendBeacon khi đóng tab) luôn gửi cứng "in_progress" —
// không phân biệt được người học đang xem LẦN ĐẦU hay xem LẠI một bài đã `completed`. Ghi đè vô
// điều kiện thì mỗi lần đóng tab một bài đã hoàn thành sẽ hạ nó về in_progress, và
// CountCompletedMandatory (đếm theo status='completed') kéo tụt % tiến độ khoá học đang hiển thị
// cho phụ huynh/học viên.
//
// N8 (review vòng 2, 260915): giá trị status lạ (không có trong map) nhận rank 0 — thấp nhất —
// nên thực tế bị CHẶN ghi khi progress.Status hiện tại đã ≥ "in_progress" (rank 0 < 1). Map này
// không phải "cho phép enum lạ đi qua" mà ngược lại: bất kỳ giá trị nào ngoài ba key trên coi
// như thấp nhất và chỉ được ghi khi progress hiện tại còn ở "not_started". Vô hại hiện nay vì cả
// hai DTO liên quan (UpdateLessonProgressDTO, beacon) đều đã validate `oneof=not_started
// in_progress completed`, nên request tới được đây luôn có status hợp lệ — nhưng nếu enum status
// mở rộng sau này mà quên cập nhật map, giá trị mới sẽ bị coi là rank 0 và im lặng không ghi
// được (trừ khi progress đang not_started), không phải "được chấp nhận" như comment cũ ngụ ý.
var lessonProgressStatusRank = map[string]int{
	"not_started": 0,
	"in_progress": 1,
	"completed":   2,
}

type EnrollmentServiceInterface interface {
	Enroll(ctx context.Context, userID, courseID uuid.UUID) (*dto.EnrollmentResponseDTO, error)
	Unenroll(ctx context.Context, userID, courseID uuid.UUID) error
	GetMyEnrollments(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.EnrollmentListResponseDTO, error)
	GetEnrollmentDetail(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*dto.EnrollmentDetailDTO, error)
	// UpdateLessonProgress ghi tien do xem video cho mot bai hoc.
	//
	// Phase 1 §1 (chong tua): ngoai `status`/`video_watched_seconds` nhu truoc, request co the mang
	// them `position_seconds`, `duration_seconds` va `played_ranges`. Khi co `played_ranges` +
	// `duration_seconds`:
	//   - hop nhat khoang vua phat vao khoang da luu (MergePlayedRanges),
	//   - `watched_seconds` = tong do dai sau merge (KHONG phai so client tu khai),
	//   - `watched_pct`   = round(watched_seconds / duration * 100, 1),
	//   - tu chot `completed` khi watched_pct >= course.min_video_pct (mac dinh 90).
	//
	// `completed` do CLIENT gui len bi BO QUA: neu khong thi chi can keo thanh tua toi cuoi video
	// la co ngay 100% — dung cai lo hong ma cot played_ranges sinh ra de bit.
	//
	// Khong co `played_ranges` (client cu) thi ham chay y nguyen nhu truoc: chi ghi
	// status/video_watched_seconds theo duong GREATEST() cu.
	//
	// Tra ve LessonProgressStateDTO (contract §1) chu khong phai LessonProgressResponseDTO:
	// web doc thang shape nay trong services/enrollment.service.ts.
	// isAdmin (F3, audit 260917-live): actor la SYSTEM_ADMIN hay khong, do handler tinh qua
	// isAdminActor va truyen xuong — dung de bypass luat khoa tuan tu, giong het cach
	// LessonContentService.GetContentsByLessonID lam. Xem chu thich tai impl.
	UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO, isAdmin bool) (*dto.LessonProgressStateDTO, error)
	GetCourseEnrollments(ctx context.Context, courseID uuid.UUID, page, pageSize int) (*dto.CourseEnrollmentListDTO, error)
	DebugGetCourseEnrollments(ctx context.Context, courseID uuid.UUID) ([]dto.DebugEnrollmentDTO, error)
}

type EnrollmentService struct {
	enrollmentRepo repository.EnrollmentRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
	lessonRepo    repository.LessonRepositoryInterface
	// videoUploadRepo (BLOCKER-1, review vòng 3): chỉ dùng để tự chữa lesson_contents.duration
	// từ video_uploads.duration khi cột đó còn 0 — xem healDurationFromVideoUpload. Nil được
	// chấp nhận ở tầng thân hàm (đường chữa tự tắt), nhưng vẫn là tham số bắt buộc của
	// constructor để nơi gọi phải ý thức được sự phụ thuộc này.
	videoUploadRepo repository.VideoUploadRepositoryInterface
}

func NewEnrollmentService(
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
	videoUploadRepo repository.VideoUploadRepositoryInterface,
) *EnrollmentService {
	return &EnrollmentService{
		enrollmentRepo:  enrollmentRepo,
		courseRepo:      courseRepo,
		lessonRepo:      lessonRepo,
		videoUploadRepo: videoUploadRepo,
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
	courseIDs := make([]uuid.UUID, len(enrollments))
	for i := range enrollments {
		enrollmentIDs[i] = enrollments[i].ID
		courseIDs[i] = enrollments[i].CourseID
	}
	// Khong nuot loi: neu khong cong don duoc thoi gian xem thi tra loi that, vi web dung
	// truong nay de hien thi chi so "thoi gian hoc" cho nguoi dung.
	watchedByEnrollment, err := s.enrollmentRepo.SumWatchedSecondsByEnrollmentIDs(ctx, enrollmentIDs)
	if err != nil {
		return nil, err
	}

	// Pending assignments: mot cau query cho TAT CA khoa (chan N+1).
	pendingByCourse, err := s.enrollmentRepo.GetPendingAssignmentsByCourseIDs(ctx, userID, courseIDs)
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
		// Pending assignments cho khoa nay.
		d.PendingAssignments = toPendingAssignmentDTOs(pendingByCourse[enrollments[i].CourseID])
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

func (s *EnrollmentService) UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO, isAdmin bool) (*dto.LessonProgressStateDTO, error) {
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

	// Cau hinh khoa hoc (Phase 1 §2): `sequential` quyet dinh next_lesson_unlocked,
	// `min_video_pct` quyet dinh nguong tu chot completed. Doc TRUOC khi ghi: neu buoc doc nay
	// loi thi khong duoc de lai mot ban ghi da ghi xong nhung response bao loi.
	course, err := s.courseRepo.GetByID(ctx, courseID)
	if err != nil {
		return nil, err
	}
	minVideoPct := defaultMinVideoPct
	sequential := false
	if course != nil {
		sequential = course.Sequential
		if course.MinVideoPct > 0 {
			minVideoPct = course.MinVideoPct
		}
	}

	// F3 (audit 260917-live, review report review-260917-phase1-merged): endpoint nay (ca
	// PUT /lessons/:lessonId/progress lan beacon POST /api/progress, hai handler deu goi ham
	// nay) TRUOC ban va nay khong kiem tra locked/lock_reason gi ca — mot hoc vien vuot qua
	// bai truoc chua hoan thanh (goi thang API, khong qua UI/lesson-content-guard) van GHI
	// DUOC tien do cho bai dang bi khoa boi luat hoc tuan tu. Ap dung DUNG luat khoa nhu doc
	// noi dung bai (LessonContentService.GetContentsByLessonID) — chu so huu khoa
	// (course.InstructorID) va admin duoc bypass toan bo, dung tham so isAdmin tu handler qua
	// isAdminActor, de logic bypass nam DUY NHAT o mot cho (lesson_lock.go), khong lech giua
	// hai endpoint. Kiem TRUOC moi thao tac doc/ghi tien do phia duoi.
	bypass := isAdmin || (course != nil && course.InstructorID == userID)
	lockInput, err := gatherLessonLockInput(ctx, s.enrollmentRepo, userID, courseID, sequential, bypass)
	if err != nil {
		return nil, err
	}
	if locked, _, _ := ResolveLessonLock(lessonID, lockInput); locked {
		return nil, ErrLessonLocked
	}

	// Thu tu bai hoc trong khoa, de biet bai ke tiep la bai nao (next_lesson_unlocked).
	// Cung la du lieu ma §2 dung lai cho khoa tuan tu.
	lessonOrder, err := s.enrollmentRepo.GetLessonIDsByCourseID(ctx, courseID)
	if err != nil {
		return nil, err
	}

	serverDuration, err := s.resolveServerVideoDuration(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	// F2 (review 260917): doc -> hop nhat -> ghi chay trong transaction, dong lesson_progress bi khoa
	// FOR UPDATE, nen heartbeat + beacon + nhieu tab tuan tu hoa thay vi ghi de khoang cua nhau.
	var saved *model.LessonProgress
	write := func() error {
		return s.enrollmentRepo.WithLessonProgressLock(ctx, userID, lessonID, func(repo repository.EnrollmentRepositoryInterface, progress *model.LessonProgress) error {
			p, err := s.writeLessonProgressLocked(ctx, repo, progress, userID, lessonID, enrollment, req, serverDuration, minVideoPct, now)
			if err != nil {
				return err
			}
			saved = p
			return nil
		})
	}
	err = write()
	if errors.Is(err, errLessonProgressInsertRace) {
		// Chi can MOT lan: ban ghi da ton tai, lan nay FOR UPDATE khoa duoc no va di nhanh UPDATE.
		err = write()
	}
	if err != nil {
		return nil, err
	}
	return s.finishLessonProgressStateUpdate(ctx, enrollment, saved, nextLessonUnlocked(lessonOrder, lessonID, sequential, saved.Status))
}

// errLessonProgressInsertRace: nhanh INSERT cua writeLessonProgressLocked thua race tao ban ghi.
// Chi dung noi bo UpdateLessonProgress de rollback roi chay lai dung MOT lan.
var errLessonProgressInsertRace = errors.New("lesson progress created concurrently")

// writeLessonProgressLocked: doc -> hop nhat played_ranges -> ghi cho MOT ban ghi lesson_progress.
// CHI goi ben trong EnrollmentRepository.WithLessonProgressLock: progress la ban ghi da khoa FOR
// UPDATE (nil neu chua co), repo gan vao chinh transaction do. Truoc ban va F2 (review 260917) doan
// nay chay khong khoa: 8 request dong thoi deu 200 nhung DB chi giu 2/8 khoang da xem.
func (s *EnrollmentService) writeLessonProgressLocked(
	ctx context.Context,
	repo repository.EnrollmentRepositoryInterface,
	progress *model.LessonProgress,
	userID, lessonID uuid.UUID,
	enrollment *model.Enrollment,
	req dto.UpdateLessonProgressDTO,
	serverDuration, minVideoPct int,
	now time.Time,
) (*model.LessonProgress, error) {
	// ——— Hop nhat khoang da phat (chong tua) ———
	var storedRanges model.PlayedRanges
	storedFallbackDuration := 0
	if progress != nil {
		storedRanges = progress.PlayedRanges
		storedFallbackDuration = progress.FallbackDurationSeconds
	}

	// B-1 (review vòng 2, BLOCKER): nguồn sự thật thời lượng LÀ SERVER (lesson_contents, sau đó
	// lesson_videos) — KHÔNG BAO GIỜ tin duration_seconds client tự khai khi server đã biết,
	// đóng đường tấn công "khai duration=10 trên bài 1200s" biến 10 giây xem thật thành watched_pct
	// ≈100%. Chỉ khi server = 0 (bài chưa gắn content video, dữ liệu thiếu) mới rơi về
	// duration_seconds client khai, và giá trị đó BẮT BUỘC lưu STICKY-MAX (fallback_duration_seconds,
	// xem model.LessonProgress) — một request sau đó khai duration NHỎ HƠN không được phép hạ mẫu
	// số xuống (mất dữ liệu/pct nhảy lùi).
	clientDuration := 0
	if req.DurationSeconds != nil {
		clientDuration = *req.DurationSeconds
	}
	durationSeconds := serverDuration
	usingFallbackDuration := serverDuration <= 0
	if usingFallbackDuration {
		durationSeconds = storedFallbackDuration
		if clientDuration > durationSeconds {
			durationSeconds = clientDuration
		}
	}

	// Chi hop nhat khi client THAT SU gui khoang moi VA co thoi luong lam mau so. Client cu
	// (chi gui status + video_watched_seconds) di nguyen duong cu ben duoi.
	hasRangePayload := len(req.PlayedRanges) > 0 && durationSeconds > 0
	var mergedRanges model.PlayedRanges
	var mergedSeconds int
	if hasRangePayload {
		mergedRanges, mergedSeconds = MergePlayedRanges(storedRanges, toModelPlayedRanges(req.PlayedRanges), durationSeconds)
	}

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
		if req.PositionSeconds != nil {
			progress.LastPositionSeconds = *req.PositionSeconds
		}
		if usingFallbackDuration && durationSeconds > 0 {
			// Ban ghi moi tao: chua co gia tri fallback cu nao de GREATEST, ghi thang durationSeconds
			// (da la max(0, clientDuration) o tren).
			progress.FallbackDurationSeconds = durationSeconds
		}
		if hasRangePayload {
			progress.PlayedRanges = mergedRanges
			progress.VideoWatchedSecs = mergedSeconds
			if pct, ok := WatchedPercent(mergedSeconds, durationSeconds); ok {
				progress.WatchedPct = pct
			}
		} else if req.VideoWatchedSecs != nil {
			// TB (review vòng 2): clamp theo duration server-truth — khong cho client tu khai
			// mot so giay da xem VUOT QUA do dai that cua video (chi so "thoi gian hoc" khong duoc
			// phinh to vo han qua duong cu nay).
			progress.VideoWatchedSecs = clampToDuration(*req.VideoWatchedSecs, durationSeconds)
		}

		// C-2 (review vòng 2, BLOCKER): !usingFallbackDuration là điều kiện "mẫu số đáng tin" —
		// xem resolveLessonStatus. Vẫn ghi watched_pct/played_ranges như cũ, chỉ KHÔNG cấp completed.
		status := resolveLessonStatus("not_started", req.Status, progress.WatchedPct, minVideoPct, !usingFallbackDuration)
		progress.Status = status
		if status == "completed" {
			progress.CompletedAt = &now
		}
		if req.ProgressPercent != nil {
			progress.ProgressPercent = *req.ProgressPercent
		}

		// F2: ON CONFLICT DO NOTHING thay cho Save() tho. inserted=false = request song song vua tao
		// ban ghi truoc (FOR UPDATE o tren khong khoa duoc dong chua ton tai): bao caller chay lai,
		// lan sau khoa duoc ban ghi do va di nhanh UPDATE, hop nhat len tren thay vi tra 400.
		inserted, err := repo.InsertLessonProgressIfAbsent(ctx, progress)
		if err != nil {
			return nil, err
		}
		if !inserted {
			return nil, errLessonProgressInsertRace
		}
		return progress, nil
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

	// ——— status: sticky, tu chot theo nguong, client khong duoc tu phong ———
	// watchedPctMoi la pct SAU khi hop nhat (hoac giu nguyen pct cu neu lan nay khong gui khoang).
	watchedPct := progress.WatchedPct
	if hasRangePayload {
		if pct, ok := WatchedPercent(mergedSeconds, durationSeconds); ok {
			watchedPct = pct
			updates["watched_pct"] = pct
		}
	}
	// C-2 (review vòng 2, BLOCKER): xem chú thích ở nhánh INSERT phía trên — cùng một điều kiện,
	// hai nhánh ghi phải luật giống nhau.
	status := resolveLessonStatus(progress.Status, req.Status, watchedPct, minVideoPct, !usingFallbackDuration)
	if status != progress.Status {
		updates["status"] = status
		if status == "completed" && progress.CompletedAt == nil {
			updates["completed_at"] = now
		}
	}
	if req.ProgressPercent != nil {
		updates["progress_percentage"] = *req.ProgressPercent
	}

	if hasRangePayload {
		// played_ranges duoc GHI DE (khong phai GREATEST): no la hop nhat cua khoang cu + khoang
		// moi, nen gia tri moi DA bao gom gia tri cu. Dung GREATEST o day la vo nghia.
		updates["played_ranges"] = mergedRanges
	}
	if req.PositionSeconds != nil {
		updates["last_position_seconds"] = *req.PositionSeconds
	}
	if usingFallbackDuration && clientDuration > 0 {
		// B-1 (review vòng 2): GREATEST() ngay trong SQL — cùng lý do với video_watched_seconds
		// bên dưới, phép max phải NGUYÊN TỬ ở tầng DB, không phải đọc-so sánh-ghi ở tầng Go (hai
		// request song song đọc cùng storedFallbackDuration cũ thì request ghi SAU sẽ đè mất giá
		// trị lớn hơn của request ghi TRƯỚC nếu làm ở Go).
		updates["fallback_duration_seconds"] = gorm.Expr("GREATEST(fallback_duration_seconds, ?)", clientDuration)
	}

	// watchedSeconds duoc truyen RIENG chu khong nam trong map: gia tri cua no phai di qua
	// GREATEST() trong cau UPDATE, khong duoc set tho.
	var watchedSeconds *int
	if hasRangePayload {
		// Tong do dai sau merge, KHONG phai con so client tu khai.
		watchedSeconds = &mergedSeconds
	} else if req.VideoWatchedSecs != nil {
		// TB (review vòng 2): clamp truoc khi day vao GREATEST() — khong clamp o day thi mot gia
		// tri client khai vuot duration van thang GREATEST va lam phinh video_watched_seconds vo
		// han qua duong cu nay.
		clamped := clampToDuration(*req.VideoWatchedSecs, durationSeconds)
		watchedSeconds = &clamped
	}

	updated, err := repo.UpdateLessonProgressFields(ctx, userID, lessonID, updates, watchedSeconds)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		// Bản ghi bi xoa giua luc doc va luc ghi — khong co dong nao duoc UPDATE. Tra loi that thay
		// vi tra ve mot DTO mang gia tri chua he duoc ghi xuong DB.
		return nil, errors.New("lesson progress not found")
	}

	return updated, nil
}

// resolveServerVideoDuration (B-1, review vòng 2): nguồn sự thật thời lượng LÀ SERVER — ưu tiên
// lesson_contents (content Type="video".Duration, đây là loại content video THỰC SỰ được đọc/ghi
// bởi mọi service khác trong hệ thống), sau đó lesson_videos (bảng legacy — AutoMigrate vẫn tạo
// nhưng không repository/service nào khác đọc/ghi; tra theo đúng quyết định review dù trong thực
// tế gần như luôn trả 0). Trả 0 khi CẢ HAI đều không có duration dương (bài chưa gắn content
// video nào) — lúc đó caller (UpdateLessonProgress) mới được rơi về duration_seconds CLIENT khai.
func (s *EnrollmentService) resolveServerVideoDuration(ctx context.Context, lessonID uuid.UUID) (int, error) {
	contents, err := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		return 0, err
	}
	for _, c := range contents {
		if c.Type == "video" && c.Duration > 0 {
			return c.Duration, nil
		}
	}

	// BLOCKER-1 (review vòng 3). lesson_contents.duration do người tạo content tự điền, nhưng
	// web KHÔNG gửi trường này (lesson-content.service.ts: CreateVideoContentDTO có `duration?`
	// nhưng call site duy nhất ở trang quản lý khoá không truyền) — nên trên thực tế gần như mọi
	// bài đều là 0. Khi đó C-2 chặn luôn đường `completed` HỢP LỆ, và với sequential=true khoá
	// học kẹt vĩnh viễn ở bài đầu.
	//
	// Thời lượng THẬT có trong video_uploads.duration, nhưng video worker chỉ ghi nó SAU khi
	// ffmpeg xử lý xong (video_processor.go), mà CreateContent lại chạy trước đó — web hiện
	// toast thành công ngay khi upload xong rồi giáo viên mới bấm tạo nội dung. Vì vậy backfill
	// một chiều lúc tạo content sẽ đọc phải nil và không chữa được gì.
	//
	// Tự chữa ở ĐÂY thay vì lúc tạo: đây là thời điểm ĐỌC duration (lúc ghi tiến độ), lúc đó
	// video đã xử lý xong nên không còn race — và nó chữa được cả những bài đã tạo từ trước.
	if healed := s.healDurationFromVideoUpload(ctx, contents); healed > 0 {
		return healed, nil
	}

	legacy, err := s.lessonRepo.GetLegacyVideoDurationByLessonID(ctx, lessonID)
	if err != nil {
		return 0, err
	}
	return legacy, nil
}

// healDurationFromVideoUpload chữa lesson_contents.duration = 0 bằng thời lượng thật của video
// gốc trên video_uploads (khớp qua upload id nhúng trong URL HLS), rồi ghi lại để các nhịp
// heartbeat sau không phải tra lại. Trả 0 khi không chữa được — mọi trường hợp không chữa được
// đều là "không biết", không phải lỗi, nên không làm hỏng request tiến độ đang chạy.
func (s *EnrollmentService) healDurationFromVideoUpload(ctx context.Context, contents []model.LessonContent) int {
	if s.videoUploadRepo == nil {
		return 0
	}
	for i := range contents {
		c := &contents[i]
		if c.Type != "video" || c.Duration > 0 || c.VideoURL == nil {
			continue
		}
		m := hlsUploadIDPattern.FindStringSubmatch(*c.VideoURL)
		if len(m) < 2 {
			continue
		}
		uploadID, err := uuid.Parse(m[1])
		if err != nil {
			continue
		}
		upload, err := s.videoUploadRepo.GetUploadByID(ctx, uploadID)
		if err != nil || upload == nil || upload.Duration == nil || *upload.Duration <= 0 {
			continue
		}
		healed := int(*upload.Duration)
		// Ghi lại để lần sau rẻ. Dùng UpdateContentDuration (một cột) chứ KHÔNG dùng
		// UpdateContent — UpdateContent là db.Save() nên ghi đè MỌI cột từ struct vừa nạp, sẽ
		// nuốt im lặng thay đổi của request song song (C-5). Ghi hỏng thì vẫn trả duration vừa
		// tìm được: không đánh đổi lợi ích cache lấy việc chặn tiến độ học — nhưng phải LOG,
		// không được nuốt.
		if err := s.lessonRepo.UpdateContentDuration(ctx, c.ID, healed); err != nil {
			log.Printf("[WARN] Khong ghi nguoc duoc duration cho lesson_content %s: %v", c.ID, err)
		}
		return healed
	}
	return 0
}

// hlsUploadIDPattern khớp upload id trong URL HLS dạng "/api/hls/{uploadId}/master.m3u8" —
// cùng dạng mà lesson_content_service.go dùng để dựng lại URL.
var hlsUploadIDPattern = regexp.MustCompile(`/hls/([a-f0-9-]{36})`)

// clampToDuration (B-1/TB, review vòng 2): CẮT (không xoá) một giá trị giây về duration khi vượt
// quá — dùng cho đường watched-seconds cũ (video_watched_seconds) để một client khai một số giây
// lớn hơn độ dài thật của video không thể phình "thời gian học" vô hạn. duration<=0 (chưa biết
// duration nào, kể cả fallback) nghĩa là không có gì để clamp — trả nguyên giá trị.
func clampToDuration(seconds, duration int) int {
	if duration > 0 && seconds > duration {
		return duration
	}
	return seconds
}

// defaultMinVideoPct la nguong watched_pct mac dinh (contract §1) khi course.min_video_pct
// chua duoc set (0 tren cac dong cu vua duoc AutoMigrate them cot).
const defaultMinVideoPct = 90

// resolveLessonStatus quyet dinh status SAU cung cua mot lan ghi tien do.
//
// Ba luat, theo dung thu tu:
//  1. `completed` la BAT BIEN — mot khi da completed thi khong bao gio ha cap (status chi di len,
//     xem lessonProgressStatusRank). Beacon dong tab luon gui cung "in_progress", ghi de vo dieu
//     kien se lam CountCompletedMandatory tut so va % tien do khoa hoc giam moi lan xem lai bai cu.
//  2. Server TU chot `completed` khi watched_pct >= minVideoPct — NHUNG chi khi mẫu số của
//     watched_pct là sự thật phía server (trustedDuration, xem luật 2b bên dưới).
//  3. `completed` do CLIENT gui len bi BO QUA (contract §1): no chi duoc chap nhan khi chinh server
//     cung tinh ra pct dat nguong (truong hop 2). Neu khong thi keo thanh tua toi cuoi video la
//     xong bai — dung lo hong ma played_ranges sinh ra de bit.
//
// Cac gia tri status khac (not_started/in_progress) van theo luat cu: chi ghi khi cap bac moi >=
// cap bac hien tai.
//
// LUAT 2b (C-2, review vòng 2 — BLOCKER): trustedDuration=false nghia la KHONG co duration nao
// phia server (lesson_contents.lesson_videos deu 0), nen mẫu số của watched_pct chinh là
// duration_seconds do CLIENT tu khai (xem usingFallbackDuration ở UpdateLessonProgress). Lúc đó
// watched_pct là một con số do client kiểm soát: request {"duration_seconds":10,
// "played_ranges":[[0,10]]} cho watched_pct=100 trên một bài dài 1200s và tự chốt completed —
// mà completed là sticky nên không thu hồi được (sticky-max của fallback_duration_seconds chỉ bảo
// vệ các request SAU, không rút lại lần hoàn thành đã cấp). Vì vậy khi mẫu số chưa phải
// server-truth thì KHÔNG cho watched_pct tự chốt completed; played_ranges/watched_pct vẫn được
// ghi như thường nên không mất dữ liệu, chỉ khoản "cấp completed" bị giữ lại cho tới khi biết
// duration thật của video. KHONG phai phép đảo luật 1: current=="completed" vẫn giữ nguyên.
func resolveLessonStatus(current string, requested *string, watchedPct decimal.Decimal, minVideoPct int, trustedDuration bool) string {
	reachedThreshold := trustedDuration && minVideoPct > 0 && watchedPct.GreaterThanOrEqual(decimal.NewFromInt(int64(minVideoPct)))

	next := current
	if reachedThreshold {
		next = "completed"
	}

	if requested == nil {
		return next
	}
	if *requested == "completed" {
		// Chi nhan khi server cung da ket luan dat nguong. Xem luat 3.
		return next
	}
	if lessonProgressStatusRank[*requested] >= lessonProgressStatusRank[next] {
		return *requested
	}
	return next
}

// nextLessonUnlocked tra ve bai ke tiep theo thu tu hien thi da MO chua, sau khi bai hien tai
// duoc ghi (contract §1, `next_lesson_unlocked`).
//
// Khoa khong bat `sequential` thi moi bai deu mo, nen chi can biet co bai ke tiep hay khong.
// Khoa bat `sequential` thi bai ke tiep chi mo khi bai hien tai da completed — day chinh la
// dieu kien ma §2 dung lai de tinh `locked` cho curriculum.
//
// Bai cuoi cua khoa tra ve false: khong co bai nao de mo them.
func nextLessonUnlocked(lessonOrder []uuid.UUID, currentLessonID uuid.UUID, sequential bool, currentStatus string) bool {
	idx := -1
	for i, id := range lessonOrder {
		if id == currentLessonID {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(lessonOrder) {
		return false
	}
	if !sequential {
		return true
	}
	return currentStatus == "completed"
}

// toModelPlayedRanges doi khuon DTO sang kieu cua model (dto khong duoc biet ve model).
func toModelPlayedRanges(in []dto.PlayedRangeDTO) []model.PlayedRange {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.PlayedRange, 0, len(in))
	for _, r := range in {
		out = append(out, model.PlayedRange{Start: r.Start, End: r.End})
	}
	return out
}

// watchedSecondsCuaBanGhi tra ve "da xem bao nhieu giay" theo contract §1.
//
// Uu tien played_ranges: do la du lieu chong tua, va tong do dai sau merge moi la con so dung.
// Ban ghi cu (ghi truoc Phase 1, chua co khoang nao) van phai doc duoc so cu tu
// video_watched_seconds, neu khong man hinh chi tiet se hien "0 giay da xem" cho nguoi hoc da
// hoc tu truoc.
func watchedSecondsCuaBanGhi(p *model.LessonProgress) int {
	if len(p.PlayedRanges) > 0 {
		return totalPlayedSeconds(p.PlayedRanges)
	}
	return p.VideoWatchedSecs
}

// finishLessonProgressStateUpdate tinh lai tien do ghi danh va tra ve DTO trang thai (contract §1).
func (s *EnrollmentService) finishLessonProgressStateUpdate(
	ctx context.Context,
	enrollment *model.Enrollment,
	progress *model.LessonProgress,
	nextUnlocked bool,
) (*dto.LessonProgressStateDTO, error) {
	if err := s.recalculateProgress(ctx, enrollment); err != nil {
		return nil, err
	}
	courseCompleted := enrollment.ProgressPercent.Equal(decimal.NewFromInt(100))
	return toLessonProgressStateDTO(progress, nextUnlocked, courseCompleted), nil
}

func toLessonProgressStateDTO(p *model.LessonProgress, nextUnlocked bool, courseCompleted bool) *dto.LessonProgressStateDTO {
	watchedPct, _ := p.WatchedPct.Float64()
	out := &dto.LessonProgressStateDTO{
		LessonID:            p.LessonID,
		Status:              p.Status,
		WatchedSeconds:      watchedSecondsCuaBanGhi(p),
		WatchedPct:          watchedPct,
		LastPositionSeconds: p.LastPositionSeconds,
		NextLessonUnlocked:  nextUnlocked,
		CourseCompleted:     courseCompleted,
	}
	if p.CompletedAt != nil {
		// .UTC() BAT BUOC: Format voi layout "...15:04:05Z" chi la mot CHU CAI 'Z' theo dung nghia
		// den, khong phai chi thi UTC — neu p.CompletedAt la gio dia phuong (khong phai UTC), no
		// van bi gan hau to Z, ngu y SAI la UTC. time.RFC3339 + .UTC() moi dung.
		formatted := p.CompletedAt.UTC().Format(time.RFC3339)
		out.CompletedAt = &formatted
	}
	return out
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

	now := time.Now()
	enrollment.ProgressPercent = progressPercent
	enrollment.CompletedLessons = int(completed)
	enrollment.TotalLessons = int(total)
	enrollment.LastAccessedAt = &now
	if err := s.enrollmentRepo.UpdateEnrollmentProgress(ctx, enrollment.ID, progressPercent, int(completed), int(total), now); err != nil {
		return err
	}

	// Auto-complete enrollment when 100%
	if progressPercent.Equal(decimal.NewFromInt(100)) {
		enrollment.CompletedAt = &now
		if err := s.enrollmentRepo.Update(ctx, enrollment); err != nil {
			return err
		}
	}

	return nil
}

// toPendingAssignmentDTOs chuyen doi tu repository type sang DTO.
func toPendingAssignmentDTOs(infos []repository.PendingAssignmentInfo) []dto.PendingAssignmentDTO {
	if len(infos) == 0 {
		return []dto.PendingAssignmentDTO{}
	}
	result := make([]dto.PendingAssignmentDTO, len(infos))
	for i, info := range infos {
		result[i] = dto.PendingAssignmentDTO{
			ID:         info.ID,
			Title:      info.Title,
			CourseName: info.CourseName,
			LessonID:   info.LessonID,
		}
		if info.EndTime != nil {
			formatted := info.EndTime.UTC().Format(time.RFC3339)
			result[i].DueDate = &formatted
		}
	}
	return result
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
		ID:               enrollment.ID,
		UserID:           enrollment.UserID,
		CourseID:         enrollment.CourseID,
		EnrolledAt:       enrollment.EnrolledAt.Format("2006-01-02T15:04:05Z"),
		ProgressPercent:  enrollment.ProgressPercent,
		CompletedLessons: enrollment.CompletedLessons,
		TotalLessons:     enrollment.TotalLessons,
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
