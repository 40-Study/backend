package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

var (
	ErrCouponNotFound       = errors.New("coupon not found")
	ErrCouponInvalid        = errors.New("coupon invalid")
	ErrCouponExpired       = errors.New("coupon expired")
	ErrCouponUsageExceeded  = errors.New("coupon usage limit exceeded")
	ErrCouponPerUserExceeded = errors.New("coupon per-user limit exceeded")
	ErrCouponNotApplicable  = errors.New("coupon not applicable to selected courses")
)

// CouponRepository - Repository for Coupon
type CouponRepository struct {
	db *gorm.DB
}

func NewCouponRepository(db *gorm.DB) *CouponRepository {
	return &CouponRepository{db: db}
}

// Create - Create new coupon
func (r *CouponRepository) Create(coupon *model.Coupon) error {
	return r.db.Create(coupon).Error
}

// GetByID - Get coupon by ID
func (r *CouponRepository) GetByID(id uuid.UUID) (*model.Coupon, error) {
	var coupon model.Coupon
	if err := r.db.First(&coupon, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	return &coupon, nil
}

// GetByCode - Get coupon by code
func (r *CouponRepository) GetByCode(code string) (*model.Coupon, error) {
	var coupon model.Coupon
	if err := r.db.First(&coupon, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	return &coupon, nil
}

// ValidateCoupon - Validate coupon for user and course
func (r *CouponRepository) ValidateCoupon(code string, userID uuid.UUID, courseIDs []uuid.UUID, subtotal decimal.Decimal) (*model.Coupon, decimal.Decimal, error) {
	coupon, err := r.GetByCode(code)
	if err != nil {
		return nil, decimal.Zero, err
	}

	now := time.Now()

	// Check if active
	if !coupon.IsActive {
		return nil, decimal.Zero, ErrCouponInvalid
	}

	// Check start date
	if coupon.StartsAt != nil && now.Before(*coupon.StartsAt) {
		return nil, decimal.Zero, ErrCouponInvalid
	}

	// Check expiry date
	if coupon.ExpiresAt != nil && now.After(*coupon.ExpiresAt) {
		return nil, decimal.Zero, ErrCouponExpired
	}

	// Check usage limit
	if coupon.UsageLimit != nil && coupon.UsageCount >= *coupon.UsageLimit {
		return nil, decimal.Zero, ErrCouponUsageExceeded
	}

	// Check minimum purchase
	if coupon.MinPurchaseAmount != nil && subtotal.LessThan(*coupon.MinPurchaseAmount) {
		return nil, decimal.Zero, ErrCouponInvalid
	}

	// Check per-user limit
	var userUsageCount int64
	r.db.Model(&model.CouponUsage{}).
		Where("coupon_id = ? AND user_id = ?", coupon.ID, userID).
		Count(&userUsageCount)

	if int(userUsageCount) >= coupon.PerUserLimit {
		return nil, decimal.Zero, ErrCouponPerUserExceeded
	}

	// Check if coupon applies to selected courses
	if len(coupon.ApplicableCourseIDs) > 0 {
		courseIDSet := make(map[uuid.UUID]bool)
		for _, id := range courseIDs {
			courseIDSet[id] = true
		}

		applicable := false
		for _, applicableIDStr := range coupon.ApplicableCourseIDs {
			applicableID, err := uuid.Parse(applicableIDStr)
			if err != nil {
				continue
			}
			if courseIDSet[applicableID] {
				applicable = true
				break
			}
		}
		if !applicable {
			return nil, decimal.Zero, ErrCouponNotApplicable
		}
	}

	// Calculate discount
	discountAmount := decimal.Zero

	if coupon.DiscountType == "percentage" {
		discountAmount = subtotal.Mul(coupon.DiscountValue).Div(decimal.NewFromInt(100))
		if coupon.MaxDiscountAmount != nil && discountAmount.GreaterThan(*coupon.MaxDiscountAmount) {
			discountAmount = *coupon.MaxDiscountAmount
		}
	} else if coupon.DiscountType == "fixed_amount" {
		discountAmount = coupon.DiscountValue
		if discountAmount.GreaterThan(subtotal) {
			discountAmount = subtotal
		}
	}

	return coupon, discountAmount, nil
}

// IncrementUsageCount - Increment coupon usage count
func (r *CouponRepository) IncrementUsageCount(couponID uuid.UUID) error {
	return r.db.Model(&model.Coupon{}).
		Where("id = ?", couponID).
		Update("usage_count", gorm.Expr("usage_count + 1")).Error
}

// CreateUsage - Create coupon usage record
func (r *CouponRepository) CreateUsage(usage *model.CouponUsage) error {
	return r.db.Create(usage).Error
}

// GetUsageByUserAndCoupon - Get coupon usage by user and coupon
func (r *CouponRepository) GetUsageByUserAndCoupon(couponID, userID uuid.UUID) ([]model.CouponUsage, error) {
	var usages []model.CouponUsage
	if err := r.db.Where("coupon_id = ? AND user_id = ?", couponID, userID).Find(&usages).Error; err != nil {
		return nil, err
	}
	return usages, nil
}

// GetAll - Get all coupons with pagination
func (r *CouponRepository) GetAll(page, limit int, isActive *bool) ([]model.Coupon, int64, error) {
	var coupons []model.Coupon
	var total int64

	query := r.db.Model(&model.Coupon{})

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Find(&coupons).Error; err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}

// Update - Update coupon
func (r *CouponRepository) Update(coupon *model.Coupon) error {
	return r.db.Save(coupon).Error
}

// Delete - Delete coupon
func (r *CouponRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.Coupon{}, "id = ?", id).Error
}
