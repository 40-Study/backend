package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type EnrollmentRepositoryInterface interface {
	Create(ctx context.Context, enrollment *model.Enrollment) error
	GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
	// GetByUserAndCourseUnscoped giống GetByUserAndCourse nhưng bao gồm cả bản ghi đã soft-delete
	// (dùng để phát hiện re-enroll sau khi Unenroll — xem C-06 audit 260909).
	//
	// HỢP ĐỒNG (review 260912, finding N1): bản ghi trả về ĐÃ Preload("LessonProgress"). Nhánh
	// re-enroll của EnrollmentService.Enroll cộng dồn video_watched_seconds từ đây để trả về cùng
	// con số với GET /my-enrollments — bỏ Preload đi thì enrollment.LessonProgress luôn nil và
	// endpoint lặng lẽ trả watched_seconds = 0 dù client vừa xem xong. Caller nào chỉ cần biết
	// bản ghi có deleted_at hay không thì vẫn dùng được như cũ.
	GetByUserAndCourseUnscoped(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
	// Restore khôi phục một enrollment đã soft-delete (deleted_at = NULL).
	Restore(ctx context.Context, id uuid.UUID) error
	// RestoreAndReactivate (H-02, review vòng 1): gộp restore (deleted_at = NULL) và reset các
	// field tiến trình học vào ĐÚNG MỘT câu UPDATE, tránh lỗi Save() ghi đè deleted_at cũ khi
	// re-enroll — xem comment tại EnrollmentService.Enroll để biết bối cảnh đầy đủ.
	RestoreAndReactivate(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error)
	GetByCourseID(ctx context.Context, courseID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error)
	GetByCourseIDIncludeDeleted(ctx context.Context, courseID uuid.UUID) ([]model.Enrollment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, enrollment *model.Enrollment) error

	// Lookup
	GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error)
	// GetLessonIDsByCourseID tra ve id cac bai hoc cua khoa theo dung thu tu hien thi
	// (section.display_order, lesson.display_order) — xem comment tai ham impl.
	GetLessonIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error)
	// GetLessonOrderInfoByCourseID (Phase 1 §2, khoa hoc tuan tu): giong GetLessonIDsByCourseID
	// nhung kem ca IsPreview — can de tim "bai truoc" MA BO QUA bai preview/mien phi (contract
	// §2). Tach rieng khoi GetLessonIDsByCourseID (dang duoc §1 dung va da co test) thay vi doi
	// chu ky ham do, tranh dong cham vao duong da on dinh.
	GetLessonOrderInfoByCourseID(ctx context.Context, courseID uuid.UUID) ([]LessonOrderInfo, error)
	// GetLessonProgressMapByUserAndCourse (Phase 1 §2): tra ve TOAN BO tien do da co cua MOT
	// nguoi dung trong MOT khoa, dang map[lessonID]. Dung khi tinh locked/progress cho CA
	// curriculum trong MOT lan doc, tranh N+1 (moi lesson mot query rieng).
	GetLessonProgressMapByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (map[uuid.UUID]*model.LessonProgress, error)
	GetEnrolledUserIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error)

	// LessonProgress
	// InsertLessonProgressIfAbsent (F2, review 260917): INSERT ... ON CONFLICT (user_id, lesson_id)
	// WHERE deleted_at IS NULL DO NOTHING. inserted=false nghia la request khac vua tao ban ghi
	// nay truoc; caller KHONG duoc coi do la loi, ma phai hop nhat len ban ghi do (xem
	// EnrollmentService.UpdateLessonProgress). Truoc day la Save() tho: 2 request dau tien song
	// song cho cung bai thi request thu hai tra 400 "duplicated key not allowed" va mat khoang da xem.
	InsertLessonProgressIfAbsent(ctx context.Context, progress *model.LessonProgress) (inserted bool, err error)
	// UpdateLessonProgressFields (review 260912, finding #2): UPDATE chỉ đúng các cột trong
	// updates (map) cho bản ghi lesson_progress của (userID, lessonID) — đọc lại bản ghi SAU khi
	// ghi. Trả về (nil, nil) khi không tìm thấy bản ghi nào để UPDATE.
	//
	// Trước đây đường ghi tiến trình dùng db.Save(progress): Save() ghi ĐÈ TOÀN BỘ struct, nên
	// (a) một request đọc-trước-ghi-sau có thể hạ video_watched_seconds xuống, và (b) request đó
	// có thể ghi đè luôn status/completed_at mà request song song vừa ghi — đúng lớp bug mà cột
	// chỉ-tăng sinh ra để diệt. Ghi bằng map chỉ chạm đúng các cột được yêu cầu.
	//
	// watchedSeconds: khi khác nil, cột video_watched_seconds KHÔNG được set thẳng mà dùng
	// GREATEST(video_watched_seconds, ?) ngay trong SQL, để phép max là nguyên tử ở tầng DB thay
	// vì so sánh read-then-write ở Go (hai request song song vẫn có thể làm giá trị giảm).
	UpdateLessonProgressFields(ctx context.Context, userID, lessonID uuid.UUID, updates map[string]interface{}, watchedSeconds *int) (*model.LessonProgress, error)
	GetLessonProgress(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error)
	// WithLessonProgressLock (F2, review 260917; tai hien that: 8 request dong thoi deu 200 nhung DB
	// chi giu 2/8 khoang): mo transaction, khoa dong lesson_progress (userID, lessonID) bang
	// SELECT ... FOR UPDATE, roi goi fn voi mot repo gan vao CHINH transaction do va ban ghi vua khoa
	// (nil neu chua co). Doc -> hop nhat played_ranges o Go -> ghi phai nam tron trong fn, de hai
	// request song song (heartbeat + beacon, nhieu tab) tuan tu hoa thay vi cung doc mot ban cu roi
	// ghi de nhau. fn tra loi -> rollback toan bo.
	WithLessonProgressLock(ctx context.Context, userID, lessonID uuid.UUID, fn func(repo EnrollmentRepositoryInterface, locked *model.LessonProgress) error) error
	CountCompletedMandatory(ctx context.Context, enrollmentID uuid.UUID) (int64, error)
	CountTotalMandatory(ctx context.Context, courseID uuid.UUID) (int64, error)
	UpdateEnrollmentProgress(ctx context.Context, enrollmentID uuid.UUID, progress decimal.Decimal, completedLessons, totalLessons int, lastAccessedAt time.Time) error
	// SumWatchedSecondsByEnrollmentIDs cong don video_watched_seconds theo tung enrollment
	// bang DUNG MOT cau GROUP BY (tranh N+1 khi liet ke danh sach ghi danh).
	SumWatchedSecondsByEnrollmentIDs(ctx context.Context, enrollmentIDs []uuid.UUID) (map[uuid.UUID]int, error)
	// GetPendingAssignmentsByCourseIDs tra ve cac assignment chua hoan thanh (chua co submission
	// accepted) cho MOT nguoi dung trong NHIEU khoa, nhom theo course_id. Tranh N+1 khi liet ke
	// danh sach ghi danh.
	GetPendingAssignmentsByCourseIDs(ctx context.Context, userID uuid.UUID, courseIDs []uuid.UUID) (map[uuid.UUID][]PendingAssignmentInfo, error)
}

type EnrollmentRepository struct {
	db *gorm.DB
}

func NewEnrollmentRepository(db *gorm.DB) *EnrollmentRepository {
	return &EnrollmentRepository{db: db}
}

func (r *EnrollmentRepository) Create(ctx context.Context, enrollment *model.Enrollment) error {
	return r.db.WithContext(ctx).Create(enrollment).Error
}

func (r *EnrollmentRepository) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

// GetByUserAndCourseUnscoped tìm enrollment kể cả đã soft-delete (Unscoped) — dùng để phân
// biệt "chưa từng enroll" với "đã unenroll trước đó" khi xử lý re-enroll (C-06).
//
// Preload("LessonProgress") (review 260912, finding N1): nhánh re-enroll cần cộng dồn
// video_watched_seconds để trả về ĐÚNG con số mà GET /my-enrollments trả cho cùng enrollment.
// Trước đây hàm này không Preload, còn GORM không tự preload (model không có tag preload), nên
// enrollment.LessonProgress luôn nil => endpoint trả watched_seconds = 0 trong khi danh sách trả
// 5640 — hai con số mâu thuẫn cho cùng một ghi danh.
//
// Preload KHÔNG làm mất cờ Unscoped: GORM truyền Statement.Unscoped xuống query preload
// (callbacks/preload.go:181, gorm v1.30.0 — bản đang dùng trong go.mod), nên các dòng
// lesson_progress đã soft-delete vẫn được đọc. Đó chính là tập dòng mà
// SumWatchedSecondsByEnrollmentIDs dùng cho GET /my-enrollments: câu GROUP BY thô ở đó không hề
// có mệnh đề deleted_at, tức nó cũng bỏ qua soft-delete. Hai đường đọc vì vậy khớp nhau về ngữ
// nghĩa, không chỉ khớp ở trường hợp thường gặp.
func (r *EnrollmentRepository) GetByUserAndCourseUnscoped(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_id = ? AND course_id = ?", userID, courseID).
		Preload("LessonProgress").
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

// Restore khôi phục enrollment đã soft-delete (deleted_at = NULL).
func (r *EnrollmentRepository) Restore(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Model(&model.Enrollment{}).Where("id = ?", id).Update("deleted_at", nil).Error
}

// RestoreAndReactivate (H-02, review vòng 1): trước đây service gọi Restore() rồi gọi tiếp
// Update() (db.Save()) trên struct đã load TRƯỚC Restore — struct đó vẫn giữ DeletedAt cũ
// trong bộ nhớ, nên Save() ghi đè lại đúng giá trị deleted_at vừa xóa, vô hiệu hóa Restore().
// Gộp restore + set field vào MỘT lệnh Updates() duy nhất (map, không qua struct) để tránh
// hoàn toàn vấn đề stale-in-memory-field.
// buildRestoreAndReactivateQuery (M2-05, review vòng 3): tách phần XÂY câu UPDATE ra khỏi phần
// đọc .Error, để test DryRun (enrollment_repository_test.go) gọi được ĐÚNG hàm sản xuất thật
// thay vì hand-roll lại câu query trong test — xóa/sửa sai hàm này sẽ làm test đỏ.
func (r *EnrollmentRepository) buildRestoreAndReactivateQuery(ctx context.Context, id uuid.UUID, updates map[string]interface{}) *gorm.DB {
	updates["deleted_at"] = nil
	return r.db.WithContext(ctx).Unscoped().Model(&model.Enrollment{}).Where("id = ?", id).Updates(updates)
}

func (r *EnrollmentRepository) RestoreAndReactivate(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	return r.buildRestoreAndReactivateQuery(ctx, id, updates).Error
}

func (r *EnrollmentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Preload("Course").
		Where("id = ?", id).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

func (r *EnrollmentRepository) GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Preload("Course").
		Preload("LessonProgress").
		Where("id = ?", id).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

func (r *EnrollmentRepository) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error) {
	var enrollments []model.Enrollment
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Enrollment{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Preload("Course").
		Preload("Course.Category").
		Preload("Course.Instructor").
		Order("enrolled_at DESC").
		Find(&enrollments).Error; err != nil {
		return nil, 0, err
	}

	return enrollments, total, nil
}

// SumWatchedSecondsByEnrollmentIDs - xem ghi chu tren interface.
func (r *EnrollmentRepository) SumWatchedSecondsByEnrollmentIDs(ctx context.Context, enrollmentIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	totals := make(map[uuid.UUID]int, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return totals, nil
	}

	var rows []struct {
		EnrollmentID uuid.UUID
		Total        int
	}
	if err := r.db.WithContext(ctx).
		Model(&model.LessonProgress{}).
		// ::bigint la BAT BUOC, khong phai trang tri: video_watched_seconds la bigint nen
		// SUM() tra ve numeric, va viec scan numeric -> int cua Go phu thuoc vao driver.
		// Ep kieu o SQL cho ket qua xac dinh.
		Select("enrollment_id, COALESCE(SUM(video_watched_seconds), 0)::bigint AS total").
		Where("enrollment_id IN ?", enrollmentIDs).
		Group("enrollment_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		totals[row.EnrollmentID] = row.Total
	}
	return totals, nil
}

func (r *EnrollmentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Enrollment{}, "id = ?", id).Error
}

func (r *EnrollmentRepository) Update(ctx context.Context, enrollment *model.Enrollment) error {
	return r.db.WithContext(ctx).Save(enrollment).Error
}

// Lookup

func (r *EnrollmentRepository) GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error) {
	var result struct {
		CourseID uuid.UUID
	}
	err := r.db.WithContext(ctx).
		Model(&model.Section{}).
		Select("sections.course_id").
		Joins("JOIN lessons ON lessons.section_id = sections.id").
		Where("lessons.id = ?", lessonID).
		Scan(&result).Error
	if err != nil {
		return uuid.Nil, err
	}
	if result.CourseID == uuid.Nil {
		return uuid.Nil, errors.New("lesson not found in any course")
	}
	return result.CourseID, nil
}

// GetLessonIDsByCourseID tra ve id cac bai hoc cua mot khoa, SAP XEP theo dung thu tu
// curriculum hien thi: (section.display_order, lesson.display_order).
//
// Dung cho Phase 1 §1 (`next_lesson_unlocked`) va §2 (khoa tuan tu — bai N phu thuoc bai N-1).
// Tra ve day du trong MOT cau truy van thay vi de tang tren do chuoi section/lesson: ham tinh
// khoa can nhin thay TOAN BO chuoi cung luc, va mot vong lap N+1 o day se chay o MOI request
// tien do (heartbeat 10 giay/lan/nguoi hoc).
func (r *EnrollmentRepository) GetLessonIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Select("lessons.id").
		Joins("JOIN sections ON sections.id = lessons.section_id").
		Where("sections.course_id = ?", courseID).
		Order("sections.display_order ASC, lessons.display_order ASC").
		Pluck("lessons.id", &ids).Error
	return ids, err
}

// LessonOrderInfo la MOT dong cua GetLessonOrderInfoByCourseID: id bai hoc kem is_preview,
// theo dung thu tu hien thi trong khoa.
type LessonOrderInfo struct {
	ID        uuid.UUID
	IsPreview bool
}

// PendingAssignmentInfo: bai tap chua hoan thanh cua nguoi dung trong mot khoa.
type PendingAssignmentInfo struct {
	ID         uuid.UUID
	CourseID   uuid.UUID
	Title      string
	EndTime    *time.Time
	CourseName string
	LessonID   *uuid.UUID
}

// GetLessonOrderInfoByCourseID (Phase 1 §2): xem comment tai interface. Cung JOIN nhu
// GetLessonIDsByCourseID nhung lay them is_preview trong MOT truy van, thay vi query rieng
// cho tung bai — logic khoa tuan tu can nhin thay CA chuoi bai (id + is_preview) cung luc de
// tim "bai truoc" bo qua preview.
func (r *EnrollmentRepository) GetLessonOrderInfoByCourseID(ctx context.Context, courseID uuid.UUID) ([]LessonOrderInfo, error) {
	var rows []LessonOrderInfo
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Select("lessons.id AS id, lessons.is_preview AS is_preview").
		Joins("JOIN sections ON sections.id = lessons.section_id").
		Where("sections.course_id = ?", courseID).
		Order("sections.display_order ASC, lessons.display_order ASC").
		Scan(&rows).Error
	return rows, err
}

// GetLessonProgressMapByUserAndCourse (Phase 1 §2): xem comment tai interface. Khong tim thay
// enrollment (nguoi dung chua enroll khoa nay) tra ve map RONG, khong phai loi — "chua enroll"
// la mot trang thai hop le ma caller (tinh lock/progress) phai tu xu ly rieng.
func (r *EnrollmentRepository) GetLessonProgressMapByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (map[uuid.UUID]*model.LessonProgress, error) {
	enrollment, err := r.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return map[uuid.UUID]*model.LessonProgress{}, nil
	}

	var records []model.LessonProgress
	if err := r.db.WithContext(ctx).
		Where("enrollment_id = ?", enrollment.ID).
		Find(&records).Error; err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]*model.LessonProgress, len(records))
	for i := range records {
		out[records[i].LessonID] = &records[i]
	}
	return out, nil
}

func (r *EnrollmentRepository) GetEnrolledUserIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error) {
	var userIDs []uuid.UUID
	err := r.db.WithContext(ctx).
		Model(&model.Enrollment{}).
		Where("course_id = ?", courseID).
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

// GetByCourseID returns all enrollments for a course (for instructor view)
func (r *EnrollmentRepository) GetByCourseID(ctx context.Context, courseID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error) {
	var enrollments []model.Enrollment
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Enrollment{}).Where("course_id = ?", courseID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Preload("User").
		Order("enrolled_at DESC").
		Find(&enrollments).Error; err != nil {
		return nil, 0, err
	}

	return enrollments, total, nil
}

// GetByCourseIDIncludeDeleted returns all enrollments including soft-deleted (for debugging)
func (r *EnrollmentRepository) GetByCourseIDIncludeDeleted(ctx context.Context, courseID uuid.UUID) ([]model.Enrollment, error) {
	var enrollments []model.Enrollment
	err := r.db.WithContext(ctx).
		Unscoped(). // Include soft-deleted records
		Where("course_id = ?", courseID).
		Preload("User").
		Find(&enrollments).Error
	return enrollments, err
}

// LessonProgress

// InsertLessonProgressIfAbsent: xem comment tai interface. TargetWhere bat buoc vi idx_user_lesson la
// unique index PARTIAL (WHERE deleted_at IS NULL, migrations.go); Postgres chi chap nhan ON CONFLICT
// khop dung predicate do.
func (r *EnrollmentRepository) InsertLessonProgressIfAbsent(ctx context.Context, progress *model.LessonProgress) (bool, error) {
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "user_id"}, {Name: "lesson_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
		DoNothing:   true,
	}).Create(progress)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// buildUpdateLessonProgressFieldsQuery (review 260912, finding #2) — tach phan XAY cau UPDATE ra
// khoi phan doc .Error, theo dung pattern cua buildRestoreAndReactivateQuery: test DryRun
// (enrollment_repository_test.go) goi duoc DUNG ham san xuat that va doc Statement.SQL, thay vi
// hand-roll lai cau query trong test (xoa/sua sai ham nay se lam test do).
func (r *EnrollmentRepository) buildUpdateLessonProgressFieldsQuery(
	ctx context.Context,
	userID, lessonID uuid.UUID,
	updates map[string]interface{},
	watchedSeconds *int,
) *gorm.DB {
	// Clone map truoc khi enrich: caller (service) co the dang giu va tai su dung map nay.
	cols := make(map[string]interface{}, len(updates)+1)
	for k, v := range updates {
		cols[k] = v
	}
	if watchedSeconds != nil {
		// GREATEST(...) chu khong phai gia tri tho: phep max phai la NGUYEN TU o tang DB. Doc row
		// -> so sanh o Go -> ghi de (cach cu) khong nguyen tu: hai request song song cung doc 400,
		// request 450 ghi truoc, request 430 ghi sau => DB con 430, dung bug can diet.
		cols["video_watched_seconds"] = gorm.Expr("GREATEST(video_watched_seconds, ?)", *watchedSeconds)
	}

	// UpdateColumns (khong phai Updates): bo qua hook BeforeUpdate/UpdatedAt cua GORM nen map duoc
	// dung NGUYEN VEN lam danh sach cot. Updates(map) cua GORM tu them updated_at => 2 nguon
	// quyet dinh cot, kho kiem chung va khong ghi duoc updated_at khi caller KHONG yeu cau.
	// updated_at vi vay la trach nhiem cua caller (service luon set).
	return r.db.WithContext(ctx).
		Model(&model.LessonProgress{}).
		Where("user_id = ? AND lesson_id = ?", userID, lessonID).
		UpdateColumns(cols)
}

func (r *EnrollmentRepository) UpdateLessonProgressFields(
	ctx context.Context,
	userID, lessonID uuid.UUID,
	updates map[string]interface{},
	watchedSeconds *int,
) (*model.LessonProgress, error) {
	res := r.buildUpdateLessonProgressFieldsQuery(ctx, userID, lessonID, updates, watchedSeconds)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// Khong co ban ghi nao de UPDATE: KHONG tao moi o day (nhanh tao moi di qua InsertLessonProgressIfAbsent).
		// Tra (nil, nil) de service biet day la duong "ban ghi da bien mat giua hai buoc" va tra loi
		// loi thay vi tra ve mot DTO mang gia tri chua he duoc ghi xuong DB.
		return nil, nil
	}
	return r.GetLessonProgress(ctx, userID, lessonID)
}

func (r *EnrollmentRepository) GetLessonProgress(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error) {
	var progress model.LessonProgress
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND lesson_id = ?", userID, lessonID).
		First(&progress).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &progress, nil
}

// getLessonProgressForUpdate: giong GetLessonProgress, them FOR UPDATE. CHI goi ben trong
// WithLessonProgressLock: tren connection goc moi cau SQL tu commit nen khoa nha ngay, vo tac dung.
func (r *EnrollmentRepository) getLessonProgressForUpdate(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error) {
	var progress model.LessonProgress
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND lesson_id = ?", userID, lessonID).
		First(&progress).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &progress, nil
}

// WithLessonProgressLock: xem comment tai interface.
func (r *EnrollmentRepository) WithLessonProgressLock(ctx context.Context, userID, lessonID uuid.UUID, fn func(repo EnrollmentRepositoryInterface, locked *model.LessonProgress) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &EnrollmentRepository{db: tx}
		locked, err := txRepo.getLessonProgressForUpdate(ctx, userID, lessonID)
		if err != nil {
			return err
		}
		return fn(txRepo, locked)
	})
}

func (r *EnrollmentRepository) CountCompletedMandatory(ctx context.Context, enrollmentID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.LessonProgress{}).
		Joins("JOIN lessons ON lessons.id = lesson_progress.lesson_id").
		Where("lesson_progress.enrollment_id = ? AND lesson_progress.status = 'completed' AND lessons.is_mandatory = true", enrollmentID).
		Count(&count).Error
	return count, err
}

func (r *EnrollmentRepository) CountTotalMandatory(ctx context.Context, courseID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Joins("JOIN sections ON sections.id = lessons.section_id").
		Where("sections.course_id = ? AND lessons.is_mandatory = true", courseID).
		Count(&count).Error
	return count, err
}

func (r *EnrollmentRepository) UpdateEnrollmentProgress(ctx context.Context, enrollmentID uuid.UUID, progress decimal.Decimal, completedLessons, totalLessons int, lastAccessedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.Enrollment{}).
		Where("id = ?", enrollmentID).
		Updates(map[string]interface{}{
			"progress_percentage": progress,
			"completed_lessons":   completedLessons,
			"total_lessons":       totalLessons,
			"last_accessed_at":    &lastAccessedAt,
		}).Error
}

// GetPendingAssignmentsByCourseIDs tra ve cac bai tap chua hoan thanh cho mot nguoi dung
// trong nhieu khoa, nhom theo course_id. Assignment duoc coi la "pending" khi:
// - is_published = true
// - Nguoi dung CHUA co submission nao verdict = 'accepted'
//
// Path: Assignment -> Class (class.course_id) HOAC Assignment -> Session (session.course_id
// hoac session.class_id -> class.course_id).
func (r *EnrollmentRepository) GetPendingAssignmentsByCourseIDs(ctx context.Context, userID uuid.UUID, courseIDs []uuid.UUID) (map[uuid.UUID][]PendingAssignmentInfo, error) {
	result := make(map[uuid.UUID][]PendingAssignmentInfo)
	if len(courseIDs) == 0 {
		return result, nil
	}

	// Query: tim tat ca assignment published ma user chua co accepted submission,
	// thuoc ve cac course trong danh sach (qua class hoac session).
	var rows []struct {
		ID         uuid.UUID  `gorm:"column:id"`
		CourseID   uuid.UUID  `gorm:"column:course_id"`
		Title      string     `gorm:"column:title"`
		EndTime    *time.Time `gorm:"column:end_time"`
		CourseName string     `gorm:"column:course_name"`
		LessonID   *uuid.UUID `gorm:"column:lesson_id"`
	}

	// ponytail: COALESCE 3 nguon course_id (class truc tiep, session truc tiep, session->class)
	// trong mot subquery de tranh 3 LEFT JOIN rieng biet.
	// lesson_id: session.lesson_content_id -> lesson_contents.lesson_id
	err := r.db.WithContext(ctx).
		Table("assignments a").
		Select(`a.id, a.title, a.end_time,
			COALESCE(c.course_id, s.course_id, sc.course_id) AS course_id,
			co.title AS course_name,
			lc.lesson_id AS lesson_id`).
		Joins("LEFT JOIN classes c ON a.class_id = c.id").
		Joins("LEFT JOIN livestream_sessions s ON a.session_id = s.id").
		Joins("LEFT JOIN classes sc ON s.class_id = sc.id").
		Joins("LEFT JOIN courses co ON co.id = COALESCE(c.course_id, s.course_id, sc.course_id)").
		Joins("LEFT JOIN lesson_contents lc ON lc.id = s.lesson_content_id").
		Where("a.is_published = ?", true).
		Where("COALESCE(c.course_id, s.course_id, sc.course_id) IN ?", courseIDs).
		Where(`NOT EXISTS (
			SELECT 1 FROM submissions sub
			WHERE sub.assignment_id = a.id
			  AND sub.user_id = ?
			  AND sub.verdict = 'accepted'
		)`, userID).
		Order("a.end_time ASC NULLS LAST").
		Scan(&rows).Error

	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.CourseID] = append(result[row.CourseID], PendingAssignmentInfo{
			ID:         row.ID,
			CourseID:   row.CourseID,
			Title:      row.Title,
			EndTime:    row.EndTime,
			CourseName: row.CourseName,
			LessonID:   row.LessonID,
		})
	}

	return result, nil
}
