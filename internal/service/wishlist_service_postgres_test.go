package service

// F-10 (QA vòng 2 261013): wishlist API thêm/bỏ/liệt kê cho user hiện tại. Bảng `wishlists` đã có (seed 64 dòng)
// nhưng không có code nào dùng. Postgres thật vì các luật cần pin nằm ở SQL: thêm hai lần không nhân đôi dòng
// (unique index + ON CONFLICT), khoá không còn xuất bản / đã xoá mềm tự rơi khỏi danh sách, và wishlist của user
// này không đụng wishlist của user kia.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type wishlistWorld struct {
	t       *testing.T
	f       *s2Fixture
	svc     *WishlistService
	teacher model.User
	alice   model.User
	bob     model.User
}

func newWishlistWorld(t *testing.T) *wishlistWorld {
	t.Helper()
	db := isolatedAPISchema(t)
	f := &s2Fixture{t: t, db: db}
	return &wishlistWorld{
		t: t, f: f,
		svc:     NewWishlistService(repository.NewWishlistRepository(db), repository.NewCourseRepository(db)),
		teacher: f.user("teacher"), alice: f.user("alice"), bob: f.user("bob"),
	}
}

func (w *wishlistWorld) rows(user, course uuid.UUID) int64 {
	var n int64
	w.f.db.Model(&model.Wishlist{}).Where("user_id = ? AND course_id = ?", user, course).Count(&n)
	return n
}

func requireKnownStatus(t *testing.T, err error, want int) {
	t.Helper()
	var known *apperr.KnownError
	if !errors.As(err, &known) || known.Status != want {
		t.Fatalf("muon KnownError %d, duoc %v", want, err)
	}
}

func TestWishlist_AddIdempotentVaKhongNhanDoiDong(t *testing.T) {
	w := newWishlistWorld(t)
	course := w.f.course(w.teacher)
	ctx := context.Background()

	first, created, err := w.svc.Add(ctx, w.alice.ID, course.ID)
	if err != nil || !created {
		t.Fatalf("lan 1: created=%v err=%v", created, err)
	}
	if first.CourseID != course.ID || first.Course == nil || first.Course.Title != course.Title {
		t.Fatalf("item thieu thong tin khoa hoc: %+v", first)
	}
	again, created, err := w.svc.Add(ctx, w.alice.ID, course.ID)
	if err != nil || created {
		t.Fatalf("lan 2 phai idempotent: created=%v err=%v", created, err)
	}
	if again.ID != first.ID {
		t.Errorf("lan 2 phai tra lai cung item: %s != %s", again.ID, first.ID)
	}
	if n := w.rows(w.alice.ID, course.ID); n != 1 {
		t.Fatalf("muon dung 1 dong, co %d", n)
	}
}

func TestWishlist_ChiThemDuocKhoaDaXuatBan(t *testing.T) {
	w := newWishlistWorld(t)
	ctx := context.Background()
	for _, status := range []string{model.CourseStatusDraft, model.CourseStatusPendingReview, model.CourseStatusRejected} {
		c := w.f.course(w.teacher)
		w.f.db.Model(&c).Update("status", status)
		_, _, err := w.svc.Add(ctx, w.alice.ID, c.ID)
		requireKnownStatus(t, err, http.StatusNotFound)
		if n := w.rows(w.alice.ID, c.ID); n != 0 {
			t.Errorf("status %s: khong duoc co dong, co %d", status, n)
		}
	}
	_, _, err := w.svc.Add(ctx, w.alice.ID, uuid.New())
	requireKnownStatus(t, err, http.StatusNotFound)
}

func TestWishlist_ListBoQuaKhoaNgungXuatBanVaDaXoa_ChiCuaMinh(t *testing.T) {
	w := newWishlistWorld(t)
	ctx := context.Background()
	keep, archived, deleted := w.f.course(w.teacher), w.f.course(w.teacher), w.f.course(w.teacher)
	for _, c := range []model.Course{keep, archived, deleted} {
		if _, _, err := w.svc.Add(ctx, w.alice.ID, c.ID); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if _, _, err := w.svc.Add(ctx, w.bob.ID, keep.ID); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	w.f.db.Model(&archived).Update("status", model.CourseStatusArchived)
	w.f.db.Delete(&deleted)

	list, err := w.svc.List(ctx, w.alice.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.TotalItem != 1 || len(list.Items) != 1 || list.Items[0].CourseID != keep.ID {
		t.Fatalf("muon chi khoa 'keep', duoc %+v", list)
	}
	if list.Items[0].UserID != w.alice.ID {
		t.Errorf("item khong phai cua alice")
	}
	ids, err := w.svc.CourseIDs(ctx, w.alice.ID)
	if err != nil || len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("CourseIDs = %v, %v", ids, err)
	}
	empty, _ := w.svc.List(ctx, w.f.user("nobody").ID)
	if empty.Items == nil || len(empty.Items) != 0 {
		t.Errorf("danh sach rong phai la [] khong phai null: %#v", empty.Items)
	}
}

func TestWishlist_RemoveIdempotentVaCoLap(t *testing.T) {
	w := newWishlistWorld(t)
	ctx := context.Background()
	c := w.f.course(w.teacher)
	_, _, _ = w.svc.Add(ctx, w.alice.ID, c.ID)
	_, _, _ = w.svc.Add(ctx, w.bob.ID, c.ID)

	if err := w.svc.Remove(ctx, w.alice.ID, c.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := w.svc.Remove(ctx, w.alice.ID, c.ID); err != nil {
		t.Fatalf("Remove lan 2 phai khong loi: %v", err)
	}
	if in, _ := w.svc.Check(ctx, w.alice.ID, c.ID); in {
		t.Error("alice van con trong wishlist")
	}
	if in, _ := w.svc.Check(ctx, w.bob.ID, c.ID); !in {
		t.Error("remove cua alice lam mat wishlist cua bob")
	}
}
