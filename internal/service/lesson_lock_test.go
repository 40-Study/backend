package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Test cho ResolveLessonLock (Phase 1 §2) — 4 luat cua contract: bai preview khong bao gio bi
// khoa VA khong duoc tinh la "bai truoc"; chua enroll thi khoa het (tru preview); khoa khong
// sequential thi khong khoa bai nao; khoa sequential thi bai N khoa neu bai GATED gan nhat truoc
// no (bo qua preview) chua completed.

func lessonOrder(items ...repository.LessonOrderInfo) []repository.LessonOrderInfo {
	return items
}

func gated(id uuid.UUID) repository.LessonOrderInfo {
	return repository.LessonOrderInfo{ID: id, IsPreview: false}
}

func preview(id uuid.UUID) repository.LessonOrderInfo {
	return repository.LessonOrderInfo{ID: id, IsPreview: true}
}

func progressOf(status string) *model.LessonProgress {
	return &model.LessonProgress{Status: status}
}

// TestResolveLessonLock_ChuaEnroll_BaiKhongPhaiPreviewBiKhoa: nguoi dung chua enroll thi MOI
// bai khong phai preview deu khoa, ly do "not_enrolled" — bat ke khoa co sequential hay khong.
func TestResolveLessonLock_ChuaEnroll_BaiKhongPhaiPreviewBiKhoa(t *testing.T) {
	bai1 := uuid.New()
	in := LessonLockInput{
		Enrolled:    false,
		Sequential:  true,
		LessonOrder: lessonOrder(gated(bai1)),
	}

	locked, reason, progress := ResolveLessonLock(bai1, in)

	if !locked {
		t.Fatal("chua enroll thi bai khong phai preview phai bi khoa")
	}
	if reason == nil || *reason != LockReasonNotEnrolled {
		t.Fatalf("lock_reason = %v, muon %q", reason, LockReasonNotEnrolled)
	}
	// T-2 (review vòng 2): khẳng định THÊM bằng chuỗi LITERAL "not_enrolled" — không chỉ bằng
	// chính hằng số LockReasonNotEnrolled sinh ra nó (so một giá trị với chính hằng số sinh ra
	// nó thì luôn đúng bất kể hằng số mang giá trị gì). Web GHIM cứng đúng chuỗi này
	// (web/src/lib/lesson-lock.ts: not_enrolled: "Bạn cần tham gia khoá học để mở bài này") —
	// đổi giá trị hằng số ở backend mà không đổi ở web thì học viên nhận sai thông điệp và mất
	// nút ghi danh, và test so-với-chính-hằng-số không bao giờ bắt được việc này.
	if reason == nil || *reason != "not_enrolled" {
		t.Fatalf("lock_reason = %v, muon chuỗi literal \"not_enrolled\" (giá trị web đã ghim, không phải chỉ đúng TÊN hằng số)", reason)
	}
	if progress == nil || progress.Status != "not_started" {
		t.Fatalf("progress = %+v, muon not_started (chua co ban ghi)", progress)
	}
}

// TestResolveLessonLock_ChuaEnroll_BaiPreviewVanMo: bai preview/mien phi luon mo, ke ca khi
// chua enroll — day la chinh dieu kien "bo qua bai preview" trong contract.
func TestResolveLessonLock_ChuaEnroll_BaiPreviewVanMo(t *testing.T) {
	baiPreview := uuid.New()
	in := LessonLockInput{
		Enrolled:    false,
		Sequential:  true,
		LessonOrder: lessonOrder(preview(baiPreview)),
	}

	locked, reason, _ := ResolveLessonLock(baiPreview, in)

	if locked {
		t.Fatal("bai preview khong duoc khoa du chua enroll")
	}
	if reason != nil {
		t.Fatalf("lock_reason = %v, muon nil", reason)
	}
}

// TestResolveLessonLock_KhoaKhongSequential_KhongKhoaBaiNao: da enroll, khoa KHONG bat
// sequential thi moi bai deu mo, du bai truoc chua hoc gi.
func TestResolveLessonLock_KhoaKhongSequential_KhongKhoaBaiNao(t *testing.T) {
	bai1, bai2 := uuid.New(), uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  false,
		LessonOrder: lessonOrder(gated(bai1), gated(bai2)),
		Progress:    map[uuid.UUID]*model.LessonProgress{}, // bai1 chua hoc gi
	}

	if locked, _, _ := ResolveLessonLock(bai2, in); locked {
		t.Fatal("khoa khong sequential thi bai 2 phai mo du bai 1 chua hoc")
	}
}

// TestResolveLessonLock_BaiDauTien_KhongCoBaiTruocNenKhongKhoa: bai dau tien cua khoa (idx=0)
// khong co "bai truoc" nen khong the bi khoa boi luat tuan tu, du khoa co bat sequential.
func TestResolveLessonLock_BaiDauTien_KhongCoBaiTruocNenKhongKhoa(t *testing.T) {
	bai1 := uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  true,
		LessonOrder: lessonOrder(gated(bai1)),
	}

	if locked, reason, _ := ResolveLessonLock(bai1, in); locked || reason != nil {
		t.Fatalf("locked=%v reason=%v, muon locked=false reason=nil", locked, reason)
	}
}

// TestResolveLessonLock_Sequential_BaiTruocChuaCompleted_BiKhoa: khoa sequential, bai N-1
// (gated) chua completed thi bai N phai khoa, ly do previous_incomplete.
func TestResolveLessonLock_Sequential_BaiTruocChuaCompleted_BiKhoa(t *testing.T) {
	bai1, bai2 := uuid.New(), uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  true,
		LessonOrder: lessonOrder(gated(bai1), gated(bai2)),
		Progress: map[uuid.UUID]*model.LessonProgress{
			bai1: progressOf("in_progress"),
		},
	}

	locked, reason, _ := ResolveLessonLock(bai2, in)

	if !locked {
		t.Fatal("bai 1 chua completed thi bai 2 phai bi khoa")
	}
	if reason == nil || *reason != LockReasonPreviousIncomplete {
		t.Fatalf("lock_reason = %v, muon %q", reason, LockReasonPreviousIncomplete)
	}
	// T-2 (review vòng 2): xem chú thích tại TestResolveLessonLock_ChuaEnroll_BaiKhongPhaiPreviewBiKhoa
	// — web ghim literal "previous_incomplete" (lesson-lock.ts).
	if reason == nil || *reason != "previous_incomplete" {
		t.Fatalf("lock_reason = %v, muon chuỗi literal \"previous_incomplete\"", reason)
	}
}

// TestResolveLessonLock_Sequential_BaiTruocCompleted_DuocMo: bai N-1 da completed thi bai N mo.
func TestResolveLessonLock_Sequential_BaiTruocCompleted_DuocMo(t *testing.T) {
	bai1, bai2 := uuid.New(), uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  true,
		LessonOrder: lessonOrder(gated(bai1), gated(bai2)),
		Progress: map[uuid.UUID]*model.LessonProgress{
			bai1: progressOf("completed"),
		},
	}

	if locked, reason, _ := ResolveLessonLock(bai2, in); locked || reason != nil {
		t.Fatalf("locked=%v reason=%v, muon locked=false reason=nil (bai 1 da completed)", locked, reason)
	}
}

// TestResolveLessonLock_Sequential_BoQuaBaiPreviewXenGiua: chuoi [bai1(gated,chua completed),
// baiPreview, bai3(gated)] — bai3 phai nhin XUYEN QUA baiPreview de thay bai1 chua completed va
// bi khoa. Day la phep thu chinh cho "bo qua bai preview/mien phi" khi tim bai truoc.
func TestResolveLessonLock_Sequential_BoQuaBaiPreviewXenGiua(t *testing.T) {
	bai1, baiPreview, bai3 := uuid.New(), uuid.New(), uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  true,
		LessonOrder: lessonOrder(gated(bai1), preview(baiPreview), gated(bai3)),
		Progress: map[uuid.UUID]*model.LessonProgress{
			bai1: progressOf("in_progress"),
		},
	}

	locked, reason, _ := ResolveLessonLock(bai3, in)

	if !locked {
		t.Fatal("bai 3 phai bi khoa vi bai GATED gan nhat truoc no (bai 1, bo qua preview xen giua) chua completed")
	}
	if reason == nil || *reason != LockReasonPreviousIncomplete {
		t.Fatalf("lock_reason = %v, muon %q", reason, LockReasonPreviousIncomplete)
	}

	// Doi bai 1 thanh completed: bai 3 phai mo, van phai bo qua preview xen giua khi tim bai
	// truoc (khong phai vo tinh khop nham voi baiPreview - bai preview khong co progress).
	in.Progress[bai1] = progressOf("completed")
	if locked, reason, _ := ResolveLessonLock(bai3, in); locked || reason != nil {
		t.Fatalf("locked=%v reason=%v, muon mo khi bai 1 da completed", locked, reason)
	}
}

// TestResolveLessonLock_Sequential_MoiBaiTruocDeuLaPreview_KhongKhoa: chuoi toan bai preview
// truoc bai dang xet — khong co bai GATED nao de doi, coi nhu dau chuoi gated nen khong khoa.
func TestResolveLessonLock_Sequential_MoiBaiTruocDeuLaPreview_KhongKhoa(t *testing.T) {
	p1, p2, bai3 := uuid.New(), uuid.New(), uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  true,
		LessonOrder: lessonOrder(preview(p1), preview(p2), gated(bai3)),
	}

	if locked, reason, _ := ResolveLessonLock(bai3, in); locked || reason != nil {
		t.Fatalf("locked=%v reason=%v, muon mo (khong co bai gated nao truoc de doi)", locked, reason)
	}
}

// TestResolveLessonLock_ProgressTraDungGiaTri: progress tra ve trong ket qua phai la CUA CHINH
// bai dang xet, khong phai bai khac — va gia tri phai khop voi ban ghi da luu.
func TestResolveLessonLock_ProgressTraDungGiaTri(t *testing.T) {
	bai1 := uuid.New()
	p := &model.LessonProgress{
		Status:              "in_progress",
		WatchedPct:          decimal.NewFromFloat(42.5),
		LastPositionSeconds: 300,
	}
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  false,
		LessonOrder: lessonOrder(gated(bai1)),
		Progress:    map[uuid.UUID]*model.LessonProgress{bai1: p},
	}

	_, _, progress := ResolveLessonLock(bai1, in)

	if progress == nil {
		t.Fatal("progress khong duoc nil khi da co ban ghi")
	}
	if progress.Status != "in_progress" || progress.WatchedPct != 42.5 || progress.LastPositionSeconds != 300 {
		t.Fatalf("progress = %+v, khong khop ban ghi da luu", progress)
	}
}

// ---------------------------------------------------------------------------
// gatherLessonLockInput (review vòng 2, bổ sung theo yêu cầu team lead — trước đây hàm này
// KHÔNG có test trực tiếp nào, dù đây chính là nơi CAO-4 nằm).
// ---------------------------------------------------------------------------

type fakeEnrollmentRepoForGather struct {
	repository.EnrollmentRepositoryInterface
	enrollment *model.Enrollment
	order      []repository.LessonOrderInfo
	progress   map[uuid.UUID]*model.LessonProgress

	gotUserID   uuid.UUID
	gotCourseID uuid.UUID
	// progressCalls (review vòng 2): đếm số lần GetLessonProgressMapByUserAndCourse được gọi —
	// dùng để khẳng định hàm KHÔNG gọi truy vấn tiến độ khi chưa enroll (tối ưu đã có từ trước,
	// chưa từng có test khẳng định).
	progressCalls int
}

func (f *fakeEnrollmentRepoForGather) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	f.gotUserID = userID
	f.gotCourseID = courseID
	return f.enrollment, nil
}

func (f *fakeEnrollmentRepoForGather) GetLessonOrderInfoByCourseID(ctx context.Context, courseID uuid.UUID) ([]repository.LessonOrderInfo, error) {
	return f.order, nil
}

func (f *fakeEnrollmentRepoForGather) GetLessonProgressMapByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (map[uuid.UUID]*model.LessonProgress, error) {
	f.progressCalls++
	return f.progress, nil
}

// TestGatherLessonLockInput_DaEnroll_TapHopDungDuLieu: enrollment khac nil -> Enrolled=true,
// LessonOrder/Progress lay dung tu repo, userID/courseID truyen dung xuong repo.
func TestGatherLessonLockInput_DaEnroll_TapHopDungDuLieu(t *testing.T) {
	userID, courseID, bai1 := uuid.New(), uuid.New(), uuid.New()
	order := []repository.LessonOrderInfo{{ID: bai1}}
	progress := map[uuid.UUID]*model.LessonProgress{bai1: progressOf("in_progress")}
	repo := &fakeEnrollmentRepoForGather{
		enrollment: &model.Enrollment{UserID: userID, CourseID: courseID},
		order:      order,
		progress:   progress,
	}

	in, err := gatherLessonLockInput(context.Background(), repo, userID, courseID, true, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if !in.Enrolled {
		t.Error("Enrolled = false, muon true (enrollment khac nil)")
	}
	if !in.Sequential {
		t.Error("Sequential khong duoc truyen dung tu tham so")
	}
	if len(in.LessonOrder) != 1 || in.LessonOrder[0].ID != bai1 {
		t.Errorf("LessonOrder = %+v, muon dung tu repo", in.LessonOrder)
	}
	if in.Progress[bai1] == nil || in.Progress[bai1].Status != "in_progress" {
		t.Errorf("Progress = %+v, muon dung tu repo", in.Progress)
	}
	if repo.gotUserID != userID || repo.gotCourseID != courseID {
		t.Errorf("GetByUserAndCourse nhan userID=%s courseID=%s, muon %s/%s", repo.gotUserID, repo.gotCourseID, userID, courseID)
	}
	if repo.progressCalls != 1 {
		t.Errorf("GetLessonProgressMapByUserAndCourse goi %d lan, muon 1 (da enroll)", repo.progressCalls)
	}
}

// TestGatherLessonLockInput_ChuaEnroll_KhongGoiTruyVanTienDo: enrollment=nil -> Enrolled=false,
// Progress la map RONG (khong phai nil, tranh panic khi ResolveLessonLock doc in.Progress[id]),
// va KHONG goi GetLessonProgressMapByUserAndCourse (khong co ly do truy van tien do cua nguoi
// chua enroll).
func TestGatherLessonLockInput_ChuaEnroll_KhongGoiTruyVanTienDo(t *testing.T) {
	repo := &fakeEnrollmentRepoForGather{enrollment: nil}

	in, err := gatherLessonLockInput(context.Background(), repo, uuid.New(), uuid.New(), true, false)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if in.Enrolled {
		t.Error("Enrolled = true, muon false (enrollment = nil)")
	}
	if in.Progress == nil {
		t.Error("Progress = nil, muon map rong (khong phai nil)")
	}
	if repo.progressCalls != 0 {
		t.Errorf("GetLessonProgressMapByUserAndCourse goi %d lan, muon 0 (chua enroll)", repo.progressCalls)
	}
}

// TestGatherLessonLockInput_BypassLock_TruyenNguyenXuongLessonLockInput (CAO-4): tham so
// bypassLock phai duoc truyen NGUYEN xuong LessonLockInput.BypassLock — day chinh la noi CAO-4
// (C-1 cua reviewer) nam, va truoc yeu cau bo sung nay ham gatherLessonLockInput khong co test
// truc tiep nao ca.
func TestGatherLessonLockInput_BypassLock_TruyenNguyenXuongLessonLockInput(t *testing.T) {
	repo := &fakeEnrollmentRepoForGather{enrollment: nil} // chua enroll — truong hop bypass phai thang

	in, err := gatherLessonLockInput(context.Background(), repo, uuid.New(), uuid.New(), true, true)
	if err != nil {
		t.Fatalf("khong mong doi loi: %v", err)
	}
	if !in.BypassLock {
		t.Fatal("BypassLock = false, muon true (tham so bypassLock=true phai duoc truyen nguyen xuong)")
	}
}

// ---------------------------------------------------------------------------
// EnsureLessonInCourse (quyet dinh team lead, review vong 2): lessonID khong thuoc LessonOrder
// cua khoa dang xet phai la LOI RO RANG, khong duoc ResolveLessonLock am tham mo (idx==-1) hay
// khoa nham ly do previous_incomplete.
// ---------------------------------------------------------------------------

func TestEnsureLessonInCourse_LessonThuocKhoa_KhongLoi(t *testing.T) {
	bai1 := uuid.New()
	if err := EnsureLessonInCourse(bai1, lessonOrder(gated(bai1))); err != nil {
		t.Fatalf("loi = %v, muon nil (lessonID co trong LessonOrder)", err)
	}
}

func TestEnsureLessonInCourse_LessonKhongThuocKhoa_TraLoiRoRang(t *testing.T) {
	baiLa, baiKhongThuoc := uuid.New(), uuid.New()
	err := EnsureLessonInCourse(baiKhongThuoc, lessonOrder(gated(baiLa)))
	if err != ErrLessonNotInCourse {
		t.Fatalf("loi = %v, muon ErrLessonNotInCourse", err)
	}
}

// TestResolveLessonLock_LessonKhongThuocKhoa_KhongDuocMoLen: lam ro hanh vi HIEN TAI cua chinh
// ResolveLessonLock khi khong co EnsureLessonInCourse dung truoc no — idx==-1 (lessonID khong co
// trong LessonOrder) roi vao nhanh "idx<=0" va tra locked=false. Day CHINH LA ly do
// EnsureLessonInCourse phai duoc goi TRUOC o moi caller nhan lessonID tuy y (xem
// lesson_content_service.go, lesson_service.go) — ResolveLessonLock mot minh KHONG phan biet
// duoc "bai dau khoa" voi "bai khong thuoc khoa nay".
func TestResolveLessonLock_LessonKhongThuocKhoa_KhongDuocMoLen(t *testing.T) {
	baiLa, baiKhongThuoc := uuid.New(), uuid.New()
	in := LessonLockInput{
		Enrolled:    true,
		Sequential:  true,
		LessonOrder: lessonOrder(gated(baiLa)),
	}

	// Ghi nhan hien trang: ResolveLessonLock don thuan tra locked=false (KHONG phai vi bai nay
	// hop le, ma vi no khong tim thay idx). Test nay la tai lieu cho quyet dinh EnsureLessonInCourse
	// phai chan TRUOC — nếu chỉ dựa vào ResolveLessonLock, bài lạ sẽ "mở lén" đúng như report ghi.
	locked, reason, _ := ResolveLessonLock(baiKhongThuoc, in)
	if locked || reason != nil {
		t.Fatalf("locked=%v reason=%v — xac nhan hien trang: ResolveLessonLock mo bai la (idx==-1), do la ly do EnsureLessonInCourse phai chan TRUOC no", locked, reason)
	}
}
