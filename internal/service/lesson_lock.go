package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Hai gia tri lock_reason theo contract Phase 1 §2 — hang so o day de khong go nham chuoi rai
// rac o nhieu noi (course_service.go, section_service.go, lesson_content_service.go).
const (
	LockReasonNotEnrolled        = "not_enrolled"
	LockReasonPreviousIncomplete = "previous_incomplete"
)

// ErrLessonLocked (Phase 1 §2): bai hoc dang bi khoa doi voi nguoi dung hien tai — handler anh
// xa loi nay sang 403 {message: "LESSON_LOCKED"}, dung nhu contract yeu cau chan CA o tang GET
// noi dung bai, khong chi chan o UI.
var ErrLessonLocked = errors.New("lesson locked")

// ErrLessonNotInCourse (quyết định team lead, review vòng 2): lessonID không xuất hiện trong
// LessonOrder của khoá mà ResolveLessonLock đang xét — dữ liệu không nhất quán (hiếm khi xảy ra
// qua đường hợp lệ vì courseID luôn được suy TỪ lessonID ở mọi caller hiện có, xem
// EnsureLessonInCourse). Trường hợp này KHÔNG được coi là "mở" (idx==-1 trong ResolveLessonLock
// tự nhiên trả locked=false, vì luật 4 chỉ xét được "bài trước" khi biết vị trí bài) và cũng
// KHÔNG được coi là "khoá vì previous_incomplete" (không có cơ sở nào để gán lý do đó) — phải là
// lỗi rõ ràng, chặn ở handler bằng 404, không mở lén và không khoá nhầm lý do.
var ErrLessonNotInCourse = errors.New("lesson does not belong to this course")

// EnsureLessonInCourse (quyết định team lead, review vòng 2): xác nhận lessonID có mặt trong
// LessonOrder TRƯỚC khi gọi ResolveLessonLock — caller nào nhận lessonID làm tham số tuỳ ý
// (không phải lặp qua chính danh sách bài của khoá, như SectionService.GetAllSections) BẮT BUỘC
// gọi hàm này ngay sau gatherLessonLockInput và trả ErrLessonNotInCourse nếu false, thay vì để
// ResolveLessonLock âm thầm trả locked=false (idx==-1) cho một bài không thuộc khoá đang xét.
func EnsureLessonInCourse(lessonID uuid.UUID, order []repository.LessonOrderInfo) error {
	for _, item := range order {
		if item.ID == lessonID {
			return nil
		}
	}
	return ErrLessonNotInCourse
}

// LessonLockInput gom du lieu can co de tinh locked/lock_reason/progress cho MOT bai hoc,
// theo dung dinh nghia contract Phase 1 §2. Tach thanh struct rieng de dung CHUNG giua
// SectionService (tinh ca curriculum mot luc) va LessonContentService (chan noi dung MOT bai) —
// tranh viet lai luat khoa hai lan o hai noi va co nguy co lech nhau.
type LessonLockInput struct {
	// Enrolled: nguoi dung hien tai co ban ghi enrollment con hieu luc cho khoa nay khong.
	Enrolled bool
	// Sequential: course.Sequential — khoa co bat che do hoc tuan tu khong.
	Sequential bool
	// LessonOrder: id + is_preview cua TOAN BO bai trong khoa, DUNG THU TU hien thi
	// (section.display_order, lesson.display_order).
	LessonOrder []repository.LessonOrderInfo
	// Progress: tien do CUA CHINH nguoi dang xem, map theo lessonID. Co the rong (chua enroll
	// hoac chua hoc bai nao).
	Progress map[uuid.UUID]*model.LessonProgress
	// BypassLock (CAO-4 / reviewer C-1, review vòng 2): true khi người gọi là giảng viên sở hữu
	// khóa học chứa bài này, hoặc admin hệ thống. Trước bản vá này, giảng viên/admin đi qua
	// CÙNG một luật khóa tuần tự như học viên — một giảng viên xem lại chính khóa của mình (chưa
	// enroll, hoặc bài trước tự đánh dấu chưa completed vì họ không "học tuần tự") vẫn bị khóa,
	// không xem/kiểm duyệt được nội dung khóa mình dạy. Người sở hữu/admin KHÔNG BAO GIỜ bị khóa,
	// bất kể preview/enrolled/sequential.
	BypassLock bool
}

// ResolveLessonLock tinh locked/lock_reason/progress cho MOT bai hoc trong curriculum, theo
// dung 4 luat cua contract Phase 1 §2:
//
//  1. Bai preview/mien phi (IsPreview) khong bao gio bi khoa boi luat hoc tuan tu — va cung
//     KHONG duoc tinh la "bai truoc" ma bai ke phai doi hoan thanh (dung y "bo qua bai
//     preview/mien phi" trong contract).
//  2. Chua enroll: MOI bai khong phai preview deu khoa, ly do "not_enrolled".
//  3. Da enroll, khoa KHONG bat sequential: khong bai nao bi khoa boi luat nay.
//  4. Da enroll, khoa bat sequential: bai N (khong phai preview) bi khoa neu bai GATED gan nhat
//     dung TRUOC no (theo thu tu hien thi, bo qua moi bai preview xen giua) CHUA completed.
func ResolveLessonLock(lessonID uuid.UUID, in LessonLockInput) (locked bool, lockReason *string, progress *dto.LessonProgressSummaryDTO) {
	progress = toLessonProgressSummary(in.Progress[lessonID])

	// CAO-4 / C-1: chủ sở hữu khóa học / admin không bao giờ bị khóa — kiểm TRƯỚC luật preview
	// (luật 1) vì bypass này rộng hơn: preview chỉ mở MỘT bài, còn bypass mở TẤT CẢ bài của
	// đúng khóa học mà actor sở hữu/quản trị.
	if in.BypassLock {
		return false, nil, progress
	}

	idx := -1
	isPreview := false
	for i, item := range in.LessonOrder {
		if item.ID == lessonID {
			idx = i
			isPreview = item.IsPreview
			break
		}
	}

	if isPreview {
		return false, nil, progress
	}
	if !in.Enrolled {
		reason := LockReasonNotEnrolled
		return true, &reason, progress
	}
	if !in.Sequential || idx <= 0 {
		// idx<=0: bai dau tien cua khoa (hoac khong xac dinh duoc vi tri) khong co "bai truoc"
		// nen khong the bi khoa boi luat nay.
		return false, nil, progress
	}

	for i := idx - 1; i >= 0; i-- {
		prev := in.LessonOrder[i]
		if prev.IsPreview {
			continue // bo qua bai preview/mien phi, tim tiep ve truoc
		}
		prevStatus := ""
		if p := in.Progress[prev.ID]; p != nil {
			prevStatus = p.Status
		}
		if prevStatus != "completed" {
			reason := LockReasonPreviousIncomplete
			return true, &reason, progress
		}
		return false, nil, progress
	}
	// Moi bai truoc do deu la preview (khong co bai gated nao truoc): coi nhu dau chuoi gated.
	return false, nil, progress
}

func toLessonProgressSummary(p *model.LessonProgress) *dto.LessonProgressSummaryDTO {
	if p == nil {
		return &dto.LessonProgressSummaryDTO{Status: "not_started", WatchedPct: 0, LastPositionSeconds: 0}
	}
	pct, _ := p.WatchedPct.Float64()
	return &dto.LessonProgressSummaryDTO{
		Status:              p.Status,
		WatchedPct:          pct,
		LastPositionSeconds: p.LastPositionSeconds,
	}
}

// gatherLessonLockInput doc du lieu can thiet (enrollment, thu tu bai, tien do) cho MOT
// nguoi dung + MOT khoa hoc, dung chung boi SectionService, LessonContentService va
// LessonService.GetLessonByID.
//
// bypassLock (CAO-4, review vòng 2): caller tự tính (course.InstructorID == userID || isAdmin)
// TRƯỚC khi gọi — hàm này không tự tra courseRepo (chỉ nhận enrollmentRepo) nên không tự xác
// định được chủ sở hữu; truyền thẳng qua LessonLockInput.BypassLock để ResolveLessonLock áp dụng
// đồng nhất ở CẢ BA nơi gọi, tránh lệch luật giữa các endpoint.
func gatherLessonLockInput(
	ctx context.Context,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	userID, courseID uuid.UUID,
	sequential bool,
	bypassLock bool,
) (LessonLockInput, error) {
	enrollment, err := enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return LessonLockInput{}, err
	}

	order, err := enrollmentRepo.GetLessonOrderInfoByCourseID(ctx, courseID)
	if err != nil {
		return LessonLockInput{}, err
	}

	progress := map[uuid.UUID]*model.LessonProgress{}
	if enrollment != nil {
		progress, err = enrollmentRepo.GetLessonProgressMapByUserAndCourse(ctx, userID, courseID)
		if err != nil {
			return LessonLockInput{}, err
		}
	}

	return LessonLockInput{
		Enrolled:    enrollment != nil,
		Sequential:  sequential,
		LessonOrder: order,
		Progress:    progress,
		BypassLock:  bypassLock,
	}, nil
}
