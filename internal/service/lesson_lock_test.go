package service

import (
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
