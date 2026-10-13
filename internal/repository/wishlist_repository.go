package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// maxWishlistItems chặn một lần liệt kê trả quá nhiều dòng; wishlist không phân trang (như giỏ hàng).
const maxWishlistItems = 200

type WishlistRepositoryInterface interface {
	// Add chèn (user, course); created=false khi dòng đã có (unique idx_user_course_wishlist), item luôn là dòng hiện hành.
	Add(ctx context.Context, userID, courseID uuid.UUID) (item *model.Wishlist, created bool, err error)
	Delete(ctx context.Context, userID, courseID uuid.UUID) error
	// ListPublished: chỉ khoá đang published và chưa xoá mềm, mới nhất trước, kèm Course.Instructor.
	ListPublished(ctx context.Context, userID uuid.UUID) ([]model.Wishlist, error)
	Exists(ctx context.Context, userID, courseID uuid.UUID) (bool, error)
}

type WishlistRepository struct {
	db *gorm.DB
}

func NewWishlistRepository(db *gorm.DB) *WishlistRepository {
	return &WishlistRepository{db: db}
}

func (r *WishlistRepository) Add(ctx context.Context, userID, courseID uuid.UUID) (*model.Wishlist, bool, error) {
	item := &model.Wishlist{UserID: userID, CourseID: courseID}
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "course_id"}}, DoNothing: true}).
		Create(item)
	if res.Error != nil {
		return nil, false, res.Error
	}
	created := res.RowsAffected == 1
	if !created {
		// Dòng đã có (hoặc vừa được request song song chèn): lấy dòng hiện hành thay vì trả item rỗng ID.
		if err := r.db.WithContext(ctx).Where("user_id = ? AND course_id = ?", userID, courseID).First(item).Error; err != nil {
			return nil, false, err
		}
	}
	return item, created, nil
}

func (r *WishlistRepository) Delete(ctx context.Context, userID, courseID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		Delete(&model.Wishlist{}).Error
}

func (r *WishlistRepository) ListPublished(ctx context.Context, userID uuid.UUID) ([]model.Wishlist, error) {
	var items []model.Wishlist
	err := r.db.WithContext(ctx).
		Joins("JOIN courses ON courses.id = wishlists.course_id AND courses.deleted_at IS NULL AND courses.status = ?", model.CourseStatusPublished).
		Where("wishlists.user_id = ?", userID).
		Preload("Course").
		Preload("Course.Instructor").
		Order("wishlists.created_at DESC").
		Limit(maxWishlistItems).
		Find(&items).Error
	return items, err
}

func (r *WishlistRepository) Exists(ctx context.Context, userID, courseID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Wishlist{}).
		Joins("JOIN courses ON courses.id = wishlists.course_id AND courses.deleted_at IS NULL AND courses.status = ?", model.CourseStatusPublished).
		Where("wishlists.user_id = ? AND wishlists.course_id = ?", userID, courseID).Count(&n).Error
	return n > 0, err
}
