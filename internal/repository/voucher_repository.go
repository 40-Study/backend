package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

var (
	ErrVoucherNotFound = errors.New("voucher not found")
)

// DeletedMode - Mode for soft delete queries
type DeletedMode int

const (
	DeletedExclude DeletedMode = iota // Exclude deleted records
	DeletedOnly                      // Only deleted records
	DeletedWith                      // Include deleted records
)

// VoucherRepository - Repository for Voucher
type VoucherRepository struct {
	db *gorm.DB
}

func NewVoucherRepository(db *gorm.DB) *VoucherRepository {
	return &VoucherRepository{db: db}
}

// CreateVoucher - Create new voucher
func (r *VoucherRepository) CreateVoucher(ctx context.Context, voucher *model.Voucher) error {
	return r.db.WithContext(ctx).Create(voucher).Error
}

// GetVoucherByID - Get voucher by ID
func (r *VoucherRepository) GetVoucherByID(ctx context.Context, id uuid.UUID) (*model.Voucher, error) {
	var voucher model.Voucher
	if err := r.db.WithContext(ctx).First(&voucher, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}
	return &voucher, nil
}

// GetVoucherByCode - Get voucher by code
func (r *VoucherRepository) GetVoucherByCode(ctx context.Context, code string) (*model.Voucher, error) {
	var voucher model.Voucher
	if err := r.db.WithContext(ctx).First(&voucher, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}
	return &voucher, nil
}

// GetVoucherByName - Get voucher by name
func (r *VoucherRepository) GetVoucherByName(ctx context.Context, name string) (*model.Voucher, error) {
	var voucher model.Voucher
	if err := r.db.WithContext(ctx).First(&voucher, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}
	return &voucher, nil
}

// UpdateVoucher - Update voucher
func (r *VoucherRepository) UpdateVoucher(ctx context.Context, voucher *model.Voucher) error {
	return r.db.WithContext(ctx).Save(voucher).Error
}

// DeleteVoucher - Soft delete voucher
func (r *VoucherRepository) DeleteVoucher(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Voucher{}, "id = ?", id).Error
}

// RestoreVoucher - Restore soft deleted voucher
func (r *VoucherRepository) RestoreVoucher(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Model(&model.Voucher{}).Where("id = ?", id).Update("deleted_at", nil).Error
}

// HardDeleteVoucherCascade - Hard delete voucher and all related data
func (r *VoucherRepository) HardDeleteVoucherCascade(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete related records first
		if err := tx.Where("voucher_id = ?", id).Delete(&model.VoucherApplicability{}).Error; err != nil {
			return err
		}
		if err := tx.Where("voucher_id = ?", id).Delete(&model.VoucherLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("voucher_id = ?", id).Delete(&model.UserVoucher{}).Error; err != nil {
			return err
		}
		// Delete voucher
		return tx.Unscoped().Delete(&model.Voucher{}, "id = ?", id).Error
	})
}

// GetAllVouchers - Get all vouchers with filters
func (r *VoucherRepository) GetAllVouchers(ctx context.Context, limit, offset int, keyword string, startTime, endTime *time.Time, delMode DeletedMode) ([]*model.Voucher, int64, error) {
	var vouchers []*model.Voucher
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Voucher{})

	// Apply deleted mode
	switch delMode {
	case DeletedOnly:
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	case DeletedWith:
		query = query.Unscoped()
	default:
		query = query.Where("deleted_at IS NULL")
	}

	// Apply filters
	if keyword != "" {
		query = query.Where("code ILIKE ? OR name ILIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if startTime != nil {
		query = query.Where("start_date >= ?", startTime)
	}
	if endTime != nil {
		query = query.Where("end_date <= ?", endTime)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&vouchers).Error; err != nil {
		return nil, 0, err
	}

	return vouchers, total, nil
}

// GetPublicVouchers - Get public (active, not expired) vouchers
func (r *VoucherRepository) GetPublicVouchers(ctx context.Context, limit, offset int) ([]*model.Voucher, int64, error) {
	var vouchers []*model.Voucher
	var total int64

	now := time.Now()
	query := r.db.WithContext(ctx).Model(&model.Voucher{}).Where("is_active = ?", true).Where("deleted_at IS NULL")

	// Not expired
	query = query.Where("(end_date IS NULL OR end_date > ?)", now)
	// Has started
	query = query.Where("(start_date IS NULL OR start_date <= ?)", now)

	// Has usage limit not reached
	query = query.Where("(usage_limit = 0 OR used_count < usage_limit)")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&vouchers).Error; err != nil {
		return nil, 0, err
	}

	return vouchers, total, nil
}

// ============================================================
// USER VOUCHER
// ============================================================

// CreateUserVoucher - Create user voucher (save)
func (r *VoucherRepository) CreateUserVoucher(ctx context.Context, userVoucher *model.UserVoucher) error {
	return r.db.WithContext(ctx).Create(userVoucher).Error
}

// GetUserVoucherByUserAndVoucher - Get user's saved voucher
func (r *VoucherRepository) GetUserVoucherByUserAndVoucher(ctx context.Context, userID, voucherID uuid.UUID) (*model.UserVoucher, error) {
	var userVoucher model.UserVoucher
	err := r.db.WithContext(ctx).Where("user_id = ? AND voucher_id = ? AND deleted_at IS NULL", userID, voucherID).First(&userVoucher).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user voucher not found")
		}
		return nil, err
	}
	return &userVoucher, nil
}

// GetUserVouchers - Get user's saved vouchers
func (r *VoucherRepository) GetUserVouchers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.UserVoucher, int64, error) {
	var userVouchers []*model.UserVoucher
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UserVoucher{}).Where("user_id = ? AND deleted_at IS NULL", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("saved_at DESC").Find(&userVouchers).Error; err != nil {
		return nil, 0, err
	}

	return userVouchers, total, nil
}

// DeleteUserVoucher - Soft delete user voucher
func (r *VoucherRepository) DeleteUserVoucher(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.UserVoucher{}, "id = ?", id).Error
}

// ============================================================
// VOUCHER APPLICABILITY
// ============================================================

// CreateVoucherApplicability - Create applicability rule
func (r *VoucherRepository) CreateVoucherApplicability(ctx context.Context, applicability *model.VoucherApplicability) error {
	return r.db.WithContext(ctx).Create(applicability).Error
}

// GetVoucherApplicabilityByID - Get applicability by ID
func (r *VoucherRepository) GetVoucherApplicabilityByID(ctx context.Context, id uuid.UUID) (*model.VoucherApplicability, error) {
	var applicability model.VoucherApplicability
	err := r.db.WithContext(ctx).First(&applicability, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("applicability not found")
		}
		return nil, err
	}
	return &applicability, nil
}

// GetVoucherApplicabilitiesByVoucherID - Get all applicability rules for a voucher
func (r *VoucherRepository) GetVoucherApplicabilitiesByVoucherID(ctx context.Context, voucherID uuid.UUID, limit, offset int) ([]*model.VoucherApplicability, error) {
	var applicabilities []*model.VoucherApplicability
	err := r.db.WithContext(ctx).Where("voucher_id = ?", voucherID).Offset(offset).Limit(limit).Find(&applicabilities).Error
	return applicabilities, err
}

// UpdateVoucherApplicability - Update applicability rule
func (r *VoucherRepository) UpdateVoucherApplicability(ctx context.Context, applicability *model.VoucherApplicability) error {
	return r.db.WithContext(ctx).Save(applicability).Error
}

// DeleteVoucherApplicability - Delete applicability rule
func (r *VoucherRepository) DeleteVoucherApplicability(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.VoucherApplicability{}, "id = ?", id).Error
}

// DeleteAllVoucherApplicabilities - Delete all applicability rules for a voucher
func (r *VoucherRepository) DeleteAllVoucherApplicabilities(ctx context.Context, voucherID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("voucher_id = ?", voucherID).Delete(&model.VoucherApplicability{}).Error
}

// ============================================================
// VOUCHER LOGS
// ============================================================

// CreateVoucherLog - Create voucher usage log
func (r *VoucherRepository) CreateVoucherLog(ctx context.Context, log *model.VoucherLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetVoucherLogsByVoucherID - Get logs for a voucher
func (r *VoucherRepository) GetVoucherLogsByVoucherID(ctx context.Context, voucherID uuid.UUID, limit, offset int) ([]*model.VoucherLog, int64, error) {
	var logs []*model.VoucherLog
	var total int64

	query := r.db.WithContext(ctx).Model(&model.VoucherLog{}).Where("voucher_id = ?", voucherID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetVoucherLogsByUserID - Get logs for a user
func (r *VoucherRepository) GetVoucherLogsByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.VoucherLog, int64, error) {
	var logs []*model.VoucherLog
	var total int64

	query := r.db.WithContext(ctx).Model(&model.VoucherLog{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetVoucherLogsByOrderID - Get logs for an order
func (r *VoucherRepository) GetVoucherLogsByOrderID(ctx context.Context, orderID uuid.UUID) ([]*model.VoucherLog, error) {
	var logs []*model.VoucherLog
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Find(&logs).Error
	return logs, err
}

// ============================================================
// ANALYTICS
// ============================================================

// GetVoucherStats - Get voucher statistics
func (r *VoucherRepository) GetVoucherStats(ctx context.Context, voucherID uuid.UUID) (map[string]interface{}, error) {
	var result struct {
		TotalUsed     int64
		TotalDiscount int64
		UniqueUsers   int64
	}

	// Get usage count
	r.db.WithContext(ctx).Model(&model.VoucherLog{}).Where("voucher_id = ? AND action = ?", voucherID, "used").Count(&result.TotalUsed)

	// Get total discount
	r.db.WithContext(ctx).Model(&model.VoucherLog{}).Where("voucher_id = ? AND action = ?", voucherID, "used").Select("COALESCE(SUM(amount), 0)").Scan(&result.TotalDiscount)

	// Get unique users
	r.db.WithContext(ctx).Model(&model.VoucherLog{}).Where("voucher_id = ? AND action = ?", voucherID, "used").Distinct("user_id").Count(&result.UniqueUsers)

	return map[string]interface{}{
		"total_used":     result.TotalUsed,
		"total_discount": result.TotalDiscount,
		"unique_users":   result.UniqueUsers,
	}, nil
}

// GetTopVouchers - Get top vouchers by usage
func (r *VoucherRepository) GetTopVouchers(ctx context.Context, startDate, endDate time.Time, limit int) ([]map[string]interface{}, error) {
	type Result struct {
		VoucherID     uuid.UUID
		VoucherCode   string
		VoucherName   string
		UsageCount    int64
		TotalDiscount int64
		UniqueUsers   int64
	}

	var results []Result
	err := r.db.WithContext(ctx).
		Table("voucher_logs").
		Select(`
			voucher_id,
			voucher_code,
			MAX(voucher_code) as voucher_name,
			COUNT(*) as usage_count,
			COALESCE(SUM(amount), 0) as total_discount,
			COUNT(DISTINCT user_id) as unique_users
		`).
		Joins("JOIN vouchers ON vouchers.id = voucher_logs.voucher_id").
		Where("voucher_logs.created_at BETWEEN ? AND ?", startDate, endDate).
		Where("voucher_logs.action = ?", "used").
		Group("voucher_id").
		Order("usage_count DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	var response []map[string]interface{}
	for _, r := range results {
		response = append(response, map[string]interface{}{
			"voucher_id":     r.VoucherID,
			"voucher_code":   r.VoucherCode,
			"voucher_name":   r.VoucherName,
			"usage_count":    r.UsageCount,
			"total_discount": r.TotalDiscount,
			"unique_users":   r.UniqueUsers,
		})
	}

	return response, nil
}

// GetVoucherUsageTrend - Get usage trend over time
func (r *VoucherRepository) GetVoucherUsageTrend(ctx context.Context, voucherID uuid.UUID, startDate, endDate time.Time) ([]map[string]interface{}, error) {
	type Result struct {
		Date       time.Time
		UsageCount int64
		Discount   int64
		UserCount  int64
	}

	var results []Result
	err := r.db.WithContext(ctx).
		Table("voucher_logs").
		Select(`
			DATE(created_at) as date,
			COUNT(*) as usage_count,
			COALESCE(SUM(amount), 0) as discount,
			COUNT(DISTINCT user_id) as user_count
		`).
		Where("voucher_id = ?", voucherID).
		Where("created_at BETWEEN ? AND ?", startDate, endDate).
		Where("action = ?", "used").
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	var response []map[string]interface{}
	for _, r := range results {
		response = append(response, map[string]interface{}{
			"date":        r.Date,
			"usage_count": r.UsageCount,
			"discount":    r.Discount,
			"user_count":  r.UserCount,
		})
	}

	return response, nil
}
