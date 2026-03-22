package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

var (
	ErrVoucherInactive        = errors.New("voucher is inactive")
	ErrVoucherNotStarted     = errors.New("voucher is not started yet")
	ErrVoucherExpired        = errors.New("voucher is expired")
	ErrVoucherMinPurchase    = errors.New("order does not meet minimum purchase requirement")
	ErrVoucherPaymentNotAllow = errors.New("payment method is not accepted by voucher")
	ErrInvalidNumeric        = errors.New("invalid numeric value")
)

type VoucherServiceInterface interface {
	// Voucher Management
	CreateVoucher(ctx context.Context, req *dto.CreateVoucherRequest) (*model.Voucher, error)
	GetVoucherByID(ctx context.Context, voucherID uuid.UUID) (*model.Voucher, error)
	GetVoucherByCode(ctx context.Context, code string) (*model.Voucher, error)
	UpdateVoucher(ctx context.Context, voucherID uuid.UUID, req *dto.UpdateVoucherRequest) (*model.Voucher, error)
	DeleteVoucher(ctx context.Context, voucherID uuid.UUID) error
	RestoreVoucher(ctx context.Context, voucherID uuid.UUID) error
	HardDeleteVoucher(ctx context.Context, voucherID uuid.UUID) error
	GetAllVouchers(ctx context.Context, req *dto.GetVouchersRequest) ([]*model.Voucher, int64, error)
	ActivateVoucher(ctx context.Context, voucherID uuid.UUID) error
	DeactivateVoucher(ctx context.Context, voucherID uuid.UUID) error

	// Apply voucher
	ApplyVoucher(ctx context.Context, userID uuid.UUID, subTotal *int64, paymentMethod string, voucherCode string) (uuid.UUID, *string, error)

	// User Voucher (Bookmark/Save)
	SaveVoucher(ctx context.Context, userID uuid.UUID, req *dto.SaveVoucherRequest) (*model.UserVoucher, error)
	UnsaveVoucher(ctx context.Context, userID uuid.UUID, voucherID uuid.UUID) error
	GetUserSavedVouchers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.UserVoucher, int64, error)

	// Voucher Applicability
	CreateVoucherApplicability(ctx context.Context, voucherID uuid.UUID, req *dto.CreateApplicabilityRequest) (*model.VoucherApplicability, error)
	GetVoucherApplicabilities(ctx context.Context, voucherID uuid.UUID, limit, offset int) ([]*model.VoucherApplicability, error)
	UpdateVoucherApplicability(ctx context.Context, applicabilityID uuid.UUID, req *dto.UpdateApplicabilityRequest) (*model.VoucherApplicability, error)
	DeleteVoucherApplicability(ctx context.Context, applicabilityID uuid.UUID) error
	DeleteAllVoucherApplicabilities(ctx context.Context, voucherID uuid.UUID) error

	// Voucher Logs
	GetVoucherUsageHistory(ctx context.Context, voucherID uuid.UUID, limit, offset int) ([]*model.VoucherLog, int64, error)
	GetUserVoucherUsageHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.VoucherLog, int64, error)
	GetOrderVoucherLogs(ctx context.Context, orderID uuid.UUID) ([]*model.VoucherLog, error)

	// Analytics
	GetVoucherStats(ctx context.Context, voucherID uuid.UUID) (map[string]interface{}, error)
	GetTopVouchers(ctx context.Context, startDate, endDate time.Time, limit int) ([]map[string]interface{}, error)
	GetVoucherUsageTrend(ctx context.Context, voucherID uuid.UUID, startDate, endDate time.Time) ([]map[string]interface{}, error)
	GetPublicVouchers(ctx context.Context, limit, offset int) ([]*model.Voucher, int64, error)

	// Calculate discount
	CalculateDiscountAmount(voucher *model.Voucher, orderAmount int64) (int64, error)
}

type VoucherService struct {
	vr *repository.VoucherRepository
	ur *repository.UserRepository
}

func NewVoucherService(vr *repository.VoucherRepository, ur *repository.UserRepository) *VoucherService {
	return &VoucherService{
		vr: vr,
		ur: ur,
	}
}

func (vs *VoucherService) validateVoucherRequest(req *dto.CreateVoucherRequest) error {
	if req.StartDate != nil && req.EndDate != nil {
		if req.EndDate.Before(*req.StartDate) {
			return errors.New("end_date must be after start_date")
		}
	}

	if req.DiscountPercent != nil && (*req.DiscountPercent < 0 || *req.DiscountPercent > 100) {
		return errors.New("discount_percent must be between 0 and 100")
	}

	return nil
}

func (vs *VoucherService) CreateVoucher(ctx context.Context, req *dto.CreateVoucherRequest) (*model.Voucher, error) {
	// Check code exists
	if existingVoucher, _ := vs.vr.GetVoucherByCode(ctx, req.Code); existingVoucher != nil {
		return nil, errors.New("voucher code already exists")
	}

	// Check name exists
	if existingVoucher, _ := vs.vr.GetVoucherByName(ctx, req.Name); existingVoucher != nil {
		return nil, errors.New("voucher name already exists")
	}

	if err := vs.validateVoucherRequest(req); err != nil {
		return nil, err
	}

	voucher := &model.Voucher{
		Code:                    req.Code,
		Name:                    req.Name,
		Description:             req.Description,
		DiscountUnit:            model.DiscountUnit(req.DiscountUnit),
		DiscountMethod:          model.DiscountMethod(req.DiscountMethod),
		AcceptAllPaymentMethods: req.AcceptAllPaymentMethods,
		PaymentMethodsAccepted:  req.PaymentMethodsAccepted,
		CanStack:                false,
		IsActive:                true,
	}

	// Set discount based on unit + method
	switch model.DiscountUnit(req.DiscountUnit) {
	case model.DiscountUnitMoney:
		switch model.DiscountMethod(req.DiscountMethod) {
		case model.DiscountMethodFixed:
			if req.DiscountAmountMoney == nil {
				return nil, errors.New("discount_amount_money is required for MONEY + FIXED")
			}
			d := decimal.NewFromFloat(*req.DiscountAmountMoney)
			voucher.DiscountAmountMoney = &d

		case model.DiscountMethodPercent:
			if req.DiscountPercent == nil {
				return nil, errors.New("discount_percent is required for MONEY + PERCENT")
			}
			d := decimal.NewFromFloat(*req.DiscountPercent)
			voucher.DiscountPercent = &d
			if req.MaxDiscountMoney != nil {
				md := decimal.NewFromFloat(*req.MaxDiscountMoney)
				voucher.MaxDiscountMoney = &md
			}
		}

	case model.DiscountUnitPoint:
		switch model.DiscountMethod(req.DiscountMethod) {
		case model.DiscountMethodFixed:
			if req.DiscountAmountPoints == nil {
				return nil, errors.New("discount_amount_points is required for POINT + FIXED")
			}
			voucher.DiscountAmountPoints = *req.DiscountAmountPoints

		case model.DiscountMethodPercent:
			if req.DiscountPercent == nil {
				return nil, errors.New("discount_percent is required for POINT + PERCENT")
			}
			d := decimal.NewFromFloat(*req.DiscountPercent)
			voucher.DiscountPercent = &d
			if req.MaxDiscountPoints != nil {
				voucher.MaxDiscountPoints = *req.MaxDiscountPoints
			}
		}
	}

	// Optional fields
	if req.MinPurchaseMoney != nil {
		d := decimal.NewFromFloat(*req.MinPurchaseMoney)
		voucher.MinPurchaseMoney = &d
	}
	if req.MinPurchasePoints != nil {
		voucher.MinPurchasePoints = *req.MinPurchasePoints
	}
	if req.UsageLimit != nil {
		voucher.UsageLimit = *req.UsageLimit
	}
	if req.UsagePerUser != nil {
		voucher.UsagePerUser = *req.UsagePerUser
	}
	if req.CanStack != nil {
		voucher.CanStack = *req.CanStack
	}
	if req.StartDate != nil {
		voucher.StartDate = req.StartDate
	}
	if req.EndDate != nil {
		voucher.EndDate = req.EndDate
	}
	if req.IsActive != nil {
		voucher.IsActive = *req.IsActive
	}

	if err := vs.vr.CreateVoucher(ctx, voucher); err != nil {
		return nil, err
	}

	return voucher, nil
}

func (vs *VoucherService) GetVoucherByID(ctx context.Context, voucherID uuid.UUID) (*model.Voucher, error) {
	return vs.vr.GetVoucherByID(ctx, voucherID)
}

func (vs *VoucherService) GetVoucherByCode(ctx context.Context, code string) (*model.Voucher, error) {
	return vs.vr.GetVoucherByCode(ctx, code)
}

func (vs *VoucherService) UpdateVoucher(ctx context.Context, voucherID uuid.UUID, req *dto.UpdateVoucherRequest) (*model.Voucher, error) {
	voucher, err := vs.vr.GetVoucherByID(ctx, voucherID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		voucher.Name = *req.Name
	}
	if req.Description != nil {
		voucher.Description = *req.Description
	}

	if req.DiscountAmountMoney != nil {
		d := decimal.NewFromFloat(*req.DiscountAmountMoney)
		voucher.DiscountAmountMoney = &d
	}
	if req.DiscountAmountPoints != nil {
		voucher.DiscountAmountPoints = *req.DiscountAmountPoints
	}
	if req.DiscountPercent != nil {
		d := decimal.NewFromFloat(*req.DiscountPercent)
		voucher.DiscountPercent = &d
	}

	if req.MaxDiscountMoney != nil {
		d := decimal.NewFromFloat(*req.MaxDiscountMoney)
		voucher.MaxDiscountMoney = &d
	}
	if req.MaxDiscountPoints != nil {
		voucher.MaxDiscountPoints = *req.MaxDiscountPoints
	}

	if req.MinPurchaseMoney != nil {
		d := decimal.NewFromFloat(*req.MinPurchaseMoney)
		voucher.MinPurchaseMoney = &d
	}
	if req.MinPurchasePoints != nil {
		voucher.MinPurchasePoints = *req.MinPurchasePoints
	}

	if req.UsageLimit != nil {
		voucher.UsageLimit = *req.UsageLimit
	}
	if req.UsagePerUser != nil {
		voucher.UsagePerUser = *req.UsagePerUser
	}
	if req.CanStack != nil {
		voucher.CanStack = *req.CanStack
	}

	if req.StartDate != nil {
		voucher.StartDate = req.StartDate
	}
	if req.EndDate != nil {
		voucher.EndDate = req.EndDate
	}
	if voucher.StartDate != nil && voucher.EndDate != nil {
		if !voucher.StartDate.Before(*voucher.EndDate) {
			return nil, errors.New("start_date must be before end_date")
		}
	}

	if req.IsActive != nil {
		voucher.IsActive = *req.IsActive
	}

	if err := vs.vr.UpdateVoucher(ctx, voucher); err != nil {
		return nil, err
	}
	return voucher, nil
}

func (vs *VoucherService) DeleteVoucher(ctx context.Context, voucherID uuid.UUID) error {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return err
	}
	return vs.vr.DeleteVoucher(ctx, voucherID)
}

func (vs *VoucherService) RestoreVoucher(ctx context.Context, voucherID uuid.UUID) error {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return errors.New("voucher not found in deleted records")
	}
	return vs.vr.RestoreVoucher(ctx, voucherID)
}

func (vs *VoucherService) HardDeleteVoucher(ctx context.Context, voucherID uuid.UUID) error {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return err
	}
	return vs.vr.HardDeleteVoucherCascade(ctx, voucherID)
}

func (vs *VoucherService) GetAllVouchers(ctx context.Context, req *dto.GetVouchersRequest) ([]*model.Voucher, int64, error) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	if req.StartTime != nil && req.EndTime != nil {
		if req.StartTime.After(*req.EndTime) {
			return nil, 0, errors.New("startTime must be <= endTime")
		}
	}

	fmt.Print(req.StartTime)
	fmt.Print(req.EndTime)

	delMode := repository.DeletedMode(req.DelMode)
	if delMode == 0 {
		delMode = repository.DeletedExclude
	}

	return vs.vr.GetAllVouchers(ctx, req.Limit, req.Offset, req.Keyword, req.StartTime, req.EndTime, delMode)
}

func (vs *VoucherService) ActivateVoucher(ctx context.Context, voucherID uuid.UUID) error {
	voucher, err := vs.vr.GetVoucherByID(ctx, voucherID)
	if err != nil {
		return err
	}
	voucher.IsActive = true
	return vs.vr.UpdateVoucher(ctx, voucher)
}

func (vs *VoucherService) DeactivateVoucher(ctx context.Context, voucherID uuid.UUID) error {
	voucher, err := vs.vr.GetVoucherByID(ctx, voucherID)
	if err != nil {
		return err
	}
	voucher.IsActive = false
	return vs.vr.UpdateVoucher(ctx, voucher)
}

func (vs *VoucherService) ApplyVoucher(ctx context.Context, userID uuid.UUID, subTotal *int64, paymentMethod string, voucherCode string) (uuid.UUID, *string, error) {
	voucher, err := vs.vr.GetVoucherByCode(ctx, voucherCode)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if voucher == nil {
		return uuid.Nil, nil, errors.New("voucher not found")
	}

	discountAmount, err := vs.CalculateDiscountAmount(voucher, *subTotal)
	if err != nil {
		return uuid.Nil, nil, err
	}

	discountStr := fmt.Sprintf("%d", discountAmount)
	return voucher.ID, &discountStr, nil
}

func (vs *VoucherService) CalculateDiscountAmount(voucher *model.Voucher, orderAmount int64) (int64, error) {
	var discountAmount int64 = 0

	switch voucher.DiscountMethod {
	case model.DiscountMethodFixed:
		if voucher.DiscountUnit == model.DiscountUnitMoney && voucher.DiscountAmountMoney != nil {
			discountAmount = voucher.DiscountAmountMoney.IntPart()
		}
		if voucher.DiscountUnit == model.DiscountUnitPoint && voucher.DiscountAmountPoints != 0 {
			discountAmount = int64(voucher.DiscountAmountPoints)
		}

	case model.DiscountMethodPercent:
		if voucher.DiscountPercent != nil {
			percent := voucher.DiscountPercent.IntPart()
			discountAmount = (orderAmount * percent) / 100
			if voucher.MaxDiscountMoney != nil {
				maxAmount := voucher.MaxDiscountMoney.IntPart()
				fmt.Printf("DEBUG: maxAmount=%d\n", maxAmount)
				if discountAmount > maxAmount {
					discountAmount = maxAmount
				}
			}
		}

	default:
		return 0, errors.New("unknown discount method")
	}
	return discountAmount, nil
}

// ============================================================
// USER VOUCHER (Bookmark/Save)
// ============================================================

func (vs *VoucherService) SaveVoucher(ctx context.Context, userID uuid.UUID, req *dto.SaveVoucherRequest) (*model.UserVoucher, error) {
	if _, err := vs.ur.FindUserByID(ctx, userID); err != nil {
		return nil, err
	}
	voucher, err := vs.vr.GetVoucherByCode(ctx, req.VoucherCode)
	if err != nil {
		return nil, err
	}
	userVoucher := &model.UserVoucher{
		UserID:    userID,
		VoucherID: voucher.ID,
		Source:    req.Source,
		SavedAt:   time.Now(),
		Notes:     req.Notes,
	}
	if err := vs.vr.CreateUserVoucher(ctx, userVoucher); err != nil {
		return nil, err
	}

	return vs.vr.GetUserVoucherByUserAndVoucher(ctx, userID, voucher.ID)
}

func (vs *VoucherService) UnsaveVoucher(ctx context.Context, userID uuid.UUID, voucherID uuid.UUID) error {
	if _, err := vs.ur.FindUserByID(ctx, userID); err != nil {
		return errors.New("user not found")
	}
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return errors.New("voucher not found")
	}
	userVoucher, err := vs.vr.GetUserVoucherByUserAndVoucher(ctx, userID, voucherID)
	if err != nil {
		return errors.New("user voucher not found")
	}
	return vs.vr.DeleteUserVoucher(ctx, userVoucher.ID)
}

func (vs *VoucherService) GetUserSavedVouchers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.UserVoucher, int64, error) {
	if _, err := vs.ur.FindUserByID(ctx, userID); err != nil {
		return nil, 0, err
	}
	return vs.vr.GetUserVouchers(ctx, userID, limit, offset)
}

// ============================================================
// VOUCHER APPLICABILITY
// ============================================================

func (vs *VoucherService) CreateVoucherApplicability(ctx context.Context, voucherID uuid.UUID, req *dto.CreateApplicabilityRequest) (*model.VoucherApplicability, error) {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return nil, err
	}
	newVoucherApp := &model.VoucherApplicability{
		VoucherID:      voucherID,
		ApplicableType: model.VoucherApplicableType(req.ApplicableType),
		ApplicableID:   req.ApplicableID,
	}
	if err := vs.vr.CreateVoucherApplicability(ctx, newVoucherApp); err != nil {
		return nil, err
	}
	return newVoucherApp, nil
}

func (vs *VoucherService) GetVoucherApplicabilities(ctx context.Context, voucherID uuid.UUID, limit, offset int) ([]*model.VoucherApplicability, error) {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return nil, err
	}
	return vs.vr.GetVoucherApplicabilitiesByVoucherID(ctx, voucherID, limit, offset)
}

func (vs *VoucherService) UpdateVoucherApplicability(ctx context.Context, applicabilityID uuid.UUID, req *dto.UpdateApplicabilityRequest) (*model.VoucherApplicability, error) {
	voucherApp, err := vs.vr.GetVoucherApplicabilityByID(ctx, applicabilityID)
	if err != nil {
		return nil, err
	}

	if req.ApplicableType != nil {
		voucherApp.ApplicableType = model.VoucherApplicableType(*req.ApplicableType)
	}
	if req.ApplicableID != nil {
		voucherApp.ApplicableID = *req.ApplicableID
	}

	if err := vs.vr.UpdateVoucherApplicability(ctx, voucherApp); err != nil {
		return nil, err
	}
	return voucherApp, nil
}

func (vs *VoucherService) DeleteVoucherApplicability(ctx context.Context, applicabilityID uuid.UUID) error {
	if _, err := vs.vr.GetVoucherApplicabilityByID(ctx, applicabilityID); err != nil {
		return err
	}
	return vs.vr.DeleteVoucherApplicability(ctx, applicabilityID)
}

func (vs *VoucherService) DeleteAllVoucherApplicabilities(ctx context.Context, voucherID uuid.UUID) error {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return err
	}
	return vs.vr.DeleteAllVoucherApplicabilities(ctx, voucherID)
}

// ============================================================
// VOUCHER LOGS
// ============================================================

func (vs *VoucherService) GetVoucherUsageHistory(ctx context.Context, voucherID uuid.UUID, limit, offset int) ([]*model.VoucherLog, int64, error) {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return vs.vr.GetVoucherLogsByVoucherID(ctx, voucherID, limit, offset)
}

func (vs *VoucherService) GetUserVoucherUsageHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.VoucherLog, int64, error) {
	if _, err := vs.ur.FindUserByID(ctx, userID); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return vs.vr.GetVoucherLogsByUserID(ctx, userID, limit, offset)
}

func (vs *VoucherService) GetOrderVoucherLogs(ctx context.Context, orderID uuid.UUID) ([]*model.VoucherLog, error) {
	return vs.vr.GetVoucherLogsByOrderID(ctx, orderID)
}

// ============================================================
// ANALYTICS
// ============================================================

func (vs *VoucherService) GetVoucherStats(ctx context.Context, voucherID uuid.UUID) (map[string]interface{}, error) {
	voucher, err := vs.vr.GetVoucherByID(ctx, voucherID)
	if err != nil {
		return nil, err
	}
	stats, err := vs.vr.GetVoucherStats(ctx, voucherID)
	if err != nil {
		return nil, err
	}

	stats["voucher_id"] = voucher.ID
	stats["voucher_code"] = voucher.Code
	stats["voucher_name"] = voucher.Name
	if voucher.UsageLimit != 0 {
		stats["usage_limit"] = voucher.UsageLimit
	}
	return stats, nil
}

func (vs *VoucherService) GetTopVouchers(ctx context.Context, startDate, endDate time.Time, limit int) ([]map[string]interface{}, error) {
	if startDate.After(endDate) {
		return nil, errors.New("start_date must be before end_date")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	return vs.vr.GetTopVouchers(ctx, startDate, endDate, limit)
}

func (vs *VoucherService) GetVoucherUsageTrend(ctx context.Context, voucherID uuid.UUID, startDate, endDate time.Time) ([]map[string]interface{}, error) {
	if _, err := vs.vr.GetVoucherByID(ctx, voucherID); err != nil {
		return nil, err
	}
	if startDate.After(endDate) {
		return nil, errors.New("start_date must be before end_date")
	}
	return vs.vr.GetVoucherUsageTrend(ctx, voucherID, startDate, endDate)
}

func (vs *VoucherService) GetPublicVouchers(ctx context.Context, limit, offset int) ([]*model.Voucher, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return vs.vr.GetPublicVouchers(ctx, limit, offset)
}
