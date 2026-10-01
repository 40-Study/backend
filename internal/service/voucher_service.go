package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

var (
	ErrVoucherInactive        = errors.New("voucher is inactive")
	ErrVoucherNotStarted      = errors.New("voucher is not started yet")
	ErrVoucherExpired         = errors.New("voucher is expired")
	ErrVoucherMinPurchase     = errors.New("order does not meet minimum purchase requirement")
	ErrVoucherPaymentNotAllow = errors.New("payment method is not accepted by voucher")
	ErrInvalidNumeric         = errors.New("invalid numeric value")
	// ErrVoucherUsageLimitExceeded/ErrVoucherPerUserLimitExceeded (item 24, review web +
	// review vòng 1): dùng cho ValidateAndApplyVoucher — tương đương
	// ErrCouponUsageExceeded/ErrCouponPerUserExceeded của coupon_repository.go, viết lại ở
	// đây vì logic kiểm tra nằm ở service layer (đúng vị trí CalculateDiscountAmount hiện có),
	// không phải repository layer như CouponRepository.
	ErrVoucherUsageLimitExceeded   = errors.New("voucher usage limit exceeded")
	ErrVoucherPerUserLimitExceeded = errors.New("voucher usage limit per user exceeded")
	ErrVoucherNotFoundByCode       = errors.New("voucher not found")
	// ErrVoucherNotMoneyUnit (H2-01, review vòng 3): voucher discount_unit != MONEY (đổi bằng
	// điểm) không áp dụng được cho đơn hàng tiền mặt — web từ chối rõ ràng
	// (errorMessage "Voucher này đổi bằng điểm..."), backend trước đây ÂM THẦM trả discount=0
	// cho nhánh PERCENT (vẫn coi là "áp dụng thành công", chỉ là giảm 0đ) thay vì từ chối hẳn.
	ErrVoucherNotMoneyUnit = errors.New("voucher is point-based, not applicable to cash payment")
	// ErrVoucherNotApplicable (H3-03, review vòng 4): discount tính ra <= 0 (vd PERCENT trên
	// subtotal quá nhỏ, Floor về 0; hoặc FIXED với discount_amount_money null/0) — khớp nhánh
	// "discount <= 0" của web (voucher.service.ts), từ chối HẲN thay vì áp dụng "thành công"
	// với giảm giá 0đ (tránh tiêu một lượt used_count vô ích — xem H2-05/H3-01).
	ErrVoucherNotApplicable = errors.New("voucher is not applicable to this order (discount would be zero)")
)

type VoucherServiceInterface interface {
	// Voucher Management
	CreateVoucher(ctx context.Context, req *dto.CreateVoucherRequest) (*model.Voucher, error)
	GetVoucherByID(ctx context.Context, voucherID uuid.UUID) (*model.Voucher, error)
	GetVoucherByCode(ctx context.Context, code string) (*model.Voucher, error)
	// GetVoucherByCodeForViewer: tra cứu công khai theo mã có xét holders_only (viewerID = uuid.Nil
	// cho khách). Voucher dành riêng mà người xem chưa giữ trả lỗi như mã không tồn tại.
	GetVoucherByCodeForViewer(ctx context.Context, code string, viewerID uuid.UUID) (*model.Voucher, error)
	UpdateVoucher(ctx context.Context, voucherID uuid.UUID, req *dto.UpdateVoucherRequest) (*model.Voucher, error)
	DeleteVoucher(ctx context.Context, voucherID uuid.UUID) error
	RestoreVoucher(ctx context.Context, voucherID uuid.UUID) error
	HardDeleteVoucher(ctx context.Context, voucherID uuid.UUID) error
	GetAllVouchers(ctx context.Context, req *dto.GetVouchersRequest) ([]*model.Voucher, int64, error)
	ActivateVoucher(ctx context.Context, voucherID uuid.UUID) error
	DeactivateVoucher(ctx context.Context, voucherID uuid.UUID) error

	// Apply voucher
	ApplyVoucher(ctx context.Context, userID uuid.UUID, subTotal *int64, paymentMethod string, voucherCode string) (uuid.UUID, *string, error)

	// ValidateAndApplyVoucher (item 24, review web + review vòng 1): kiểm tra đầy đủ điều
	// kiện áp dụng (active, ngày hiệu lực, usage_limit, min_purchase, usage_per_user, payment
	// method) rồi tính discount bằng decimal.Decimal — dùng thay CouponRepository.ValidateCoupon
	// trong order_service.CreateOrder vì bảng coupons không còn được tạo dữ liệu qua route/
	// handler nào (đã grep xác nhận). paymentMethod rỗng khi chưa biết ở bước tạo đơn (bỏ qua
	// kiểm tra payment method trong trường hợp đó).
	ValidateAndApplyVoucher(ctx context.Context, code string, userID uuid.UUID, subtotal decimal.Decimal, paymentMethod string) (*model.Voucher, decimal.Decimal, error)
	// IncrementUsedCount + RecordUsageLog: gọi lúc đơn hàng HOÀN TẤT (không phải lúc tạo đơn)
	// — cùng thời điểm CouponRepository.IncrementUsageCount/CreateUsage trước đây được gọi.
	//
	// H2-05 (review vòng 3): IncrementUsedCount hiện KHÔNG còn được gọi ở completeOrderFulfillment
	// nữa (used_count giờ được "reserve" (tăng) ngay lúc TẠO đơn — xem ReserveVoucherUsage —
	// để chặn oversell khi nhiều đơn "pending" cùng tồn tại chưa ai hoàn tất). Giữ nguyên method
	// này trong interface (không xoá) vì có thể còn dùng cho thao tác thủ công/tương lai; chỉ
	// đổi ĐIỂM GỌI trong luồng order.
	IncrementUsedCount(ctx context.Context, voucherID uuid.UUID) error
	RecordUsageLog(ctx context.Context, voucherID, userID, orderID uuid.UUID, discountAmount decimal.Decimal) error
	// RecordUsageLogTx (H2-06, review vòng 3b): giống RecordUsageLog nhưng ghi VoucherLog TRÊN
	// "tx" được truyền vào (nil => dùng connection gốc, hành vi giống RecordUsageLog) — cho phép
	// completeOrderFulfillment tham gia CÙNG transaction với enrollment/total_students khi caller
	// có sẵn transaction đang mở (CheckAndProcessPayment nhánh trả phí, CreateOrder nhánh 0đ).
	RecordUsageLogTx(ctx context.Context, tx *gorm.DB, voucherID, userID, orderID uuid.UUID, discountAmount decimal.Decimal) error
	// ReserveVoucherUsage / ReleaseVoucherUsage (H2-05, review vòng 3): tăng/giảm used_count
	// bằng UPDATE có điều kiện chạy TRÊN "tx" được truyền vào (không phải trên vs.vr — connection
	// gốc) để tham gia CÙNG một database transaction với việc tạo/hủy đơn hàng — xem
	// OrderService.CreateOrder (reserve lúc tạo đơn) và OrderService.CancelOrder /
	// PaymentService.CheckAndProcessPayment (release khi đơn bị hủy/mã thanh toán hết hạn).
	// Dùng CÙNG điều kiện chống race đã có ở IncrementUsedCount (usage_limit <= 0 OR used_count
	// < usage_limit), không viết lại logic mới.
	ReserveVoucherUsage(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) error
	ReleaseVoucherUsage(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) error
	// LockAndCheckUsagePerUser (I-02, review vòng 5): khoá dòng voucher (SELECT ... FOR UPDATE)
	// rồi mới đếm usage_per_user (CountUserVoucherUsage + CountUserHeldOrders) — PHẢI gọi TRONG
	// transaction tạo đơn (tx khác nil), NGAY TRƯỚC ReserveVoucherUsage. Trước vòng 5, phần đếm
	// này nằm trong ValidateAndApplyVoucher và chạy NGOÀI transaction — 2 request đồng thời của
	// CÙNG user có thể cùng đọc heldCount cũ rồi cùng vượt qua (TOCTOU). usagePerUser <= 0 nghĩa
	// là "không giới hạn số lượt/user" (voucher chỉ còn bị chặn bởi usage_limit TOÀN CỤC qua
	// ReserveVoucherUsage) — bỏ qua khoá+đếm hẳn trong trường hợp này để không trả giá SELECT...
	// FOR UPDATE vô ích trên voucher không giới hạn per-user.
	LockAndCheckUsagePerUser(ctx context.Context, tx *gorm.DB, voucher *model.Voucher, userID uuid.UUID) error

	// User Voucher (Bookmark/Save)
	SaveVoucher(ctx context.Context, userID uuid.UUID, req *dto.SaveVoucherRequest) (*model.UserVoucher, error)
	UnsaveVoucher(ctx context.Context, userID uuid.UUID, voucherID uuid.UUID) error
	GetUserSavedVouchers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*model.UserVoucher, int64, error)
	// GrantVoucherTx (contract "Cuộc thi" §6): phát voucher vào ví user TRÊN "tx" của caller — xem
	// chú thích tại hàm.
	GrantVoucherTx(ctx context.Context, tx *gorm.DB, userID, voucherID uuid.UUID, source, notes string) (*model.UserVoucher, error)

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
	if req.HoldersOnly != nil {
		voucher.HoldersOnly = *req.HoldersOnly
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

// voucherUsableBy — người xem này có được biết/dùng voucher không. Voucher công khai: ai cũng được.
// Voucher holders_only: chỉ user đã được cấp hoặc đã lưu (user_vouchers); khách (uuid.Nil) thì không.
// Mọi chỗ chặn holders_only đi qua hàm này để một quy ước "người giữ" duy nhất.
func (vs *VoucherService) voucherUsableBy(ctx context.Context, v *model.Voucher, userID uuid.UUID) (bool, error) {
	if !v.HoldersOnly {
		return true, nil
	}
	if userID == uuid.Nil {
		return false, nil
	}
	return vs.vr.UserHoldsVoucher(ctx, userID, v.ID)
}

// GetVoucherByCodeForViewer — tra cứu công khai theo mã (GET /vouchers/code/:code, khách hoặc user
// đăng nhập). Voucher holders_only mà người xem chưa giữ trả CHÍNH lỗi của mã không tồn tại
// (repository.ErrVoucherNotFound) để không lộ mã có thật. viewerID = uuid.Nil khi là khách.
func (vs *VoucherService) GetVoucherByCodeForViewer(ctx context.Context, code string, viewerID uuid.UUID) (*model.Voucher, error) {
	voucher, err := vs.vr.GetVoucherByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	ok, err := vs.voucherUsableBy(ctx, voucher, viewerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, repository.ErrVoucherNotFound
	}
	return voucher, nil
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
	if req.HoldersOnly != nil {
		voucher.HoldersOnly = *req.HoldersOnly
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
	usable, err := vs.voucherUsableBy(ctx, voucher, userID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if !usable {
		return uuid.Nil, nil, repository.ErrVoucherNotFound
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

// calculateVoucherDiscountDecimal (item 24 vòng 2b, H2-01/H2-02 review vòng 3) tính discount
// bằng decimal.Decimal — bản decimal-native của CalculateDiscountAmount (vốn dùng int64, phù
// hợp cho luồng xu) để khớp với subtotal decimal.Decimal mà order_service.go đang dùng. Công
// thức PHẢI khớp ĐÚNG calculateVoucherDiscount phía web (voucher.service.ts):
//
//  1. discount_unit phải là MONEY — web chặn CẢ HAI method (PERCENT lẫn FIXED) ngay từ đầu
//     hàm, TRƯỚC khi xét discount_method. H2-01 (review vòng 3): bản Go trước đây chỉ kiểm
//     DiscountUnit ở nhánh FIXED, nhánh PERCENT không kiểm gì — voucher POINT+PERCENT vẫn áp
//     và giảm tiền thật dù web đã từ chối, tạo lệch hợp đồng backend/web.
//  2. PERCENT: subtotal * percent / 100, clamp bằng max_discount_money (chỉ khi cap > 0).
//     FIXED (mọi method khác PERCENT): dùng thẳng discount_amount_money.
//  3. H2-02 (review vòng 3): web dùng Math.floor(discount) TRƯỚC khi clamp về subtotal — bản
//     Go trước đây giữ nguyên phần thập phân của decimal.Decimal, nên với subtotal lẻ (vd
//     199.999 × 10% = 19.999,9) số tiền backend trừ khác số web hiển thị. Dùng Floor().
func calculateVoucherDiscountDecimal(voucher *model.Voucher, subtotal decimal.Decimal) decimal.Decimal {
	// (1) discount_unit phải là MONEY cho CẢ HAI method — khớp thứ tự kiểm của web.
	if voucher.DiscountUnit != model.DiscountUnitMoney {
		return decimal.Zero
	}

	discount := decimal.Zero
	switch voucher.DiscountMethod {
	case model.DiscountMethodPercent:
		if voucher.DiscountPercent != nil {
			discount = subtotal.Mul(*voucher.DiscountPercent).Div(decimal.NewFromInt(100))
			if voucher.MaxDiscountMoney != nil && voucher.MaxDiscountMoney.GreaterThan(decimal.Zero) &&
				discount.GreaterThan(*voucher.MaxDiscountMoney) {
				discount = *voucher.MaxDiscountMoney
			}
		}
	default: // FIXED — khớp nhánh "else" của web (mọi method không phải PERCENT)
		if voucher.DiscountAmountMoney != nil {
			discount = *voucher.DiscountAmountMoney
		}
	}

	// (3) Floor TRƯỚC khi clamp về subtotal/0 — đúng thứ tự web: Math.max(0, Math.min(Math.floor(discount), subtotal)).
	discount = discount.Floor()
	if discount.GreaterThan(subtotal) {
		discount = subtotal
	}
	if discount.LessThan(decimal.Zero) {
		discount = decimal.Zero
	}
	return discount
}

// ValidateAndApplyVoucher (item 24, review web + review vòng 1) — xem comment interface.
func (vs *VoucherService) ValidateAndApplyVoucher(ctx context.Context, code string, userID uuid.UUID, subtotal decimal.Decimal, paymentMethod string) (*model.Voucher, decimal.Decimal, error) {
	voucher, err := vs.vr.GetVoucherByCode(ctx, code)
	if err != nil {
		return nil, decimal.Zero, err
	}
	if voucher == nil {
		return nil, decimal.Zero, ErrVoucherNotFoundByCode
	}

	// holders_only: kiểm TRƯỚC mọi điều kiện khác (hết hạn, tắt, đủ lượt...) và trả CHÍNH lỗi mà
	// GetVoucherByCode trả cho mã không tồn tại (repository.ErrVoucherNotFound) — lỗi khác đi thì
	// người ngoài dò ra được mã có thật.
	usable, err := vs.voucherUsableBy(ctx, voucher, userID)
	if err != nil {
		return nil, decimal.Zero, err
	}
	if !usable {
		return nil, decimal.Zero, repository.ErrVoucherNotFound
	}

	if !voucher.IsActive {
		return nil, decimal.Zero, ErrVoucherInactive
	}

	now := time.Now()
	if voucher.StartDate != nil && now.Before(*voucher.StartDate) {
		return nil, decimal.Zero, ErrVoucherNotStarted
	}
	if voucher.EndDate != nil && now.After(*voucher.EndDate) {
		return nil, decimal.Zero, ErrVoucherExpired
	}

	// H2-01: chặn hẳn voucher đổi bằng điểm (POINT) trước khi tính discount — khớp web, tránh
	// trả về "áp dụng thành công, discount=0" gây hiểu nhầm mã hợp lệ.
	if voucher.DiscountUnit != model.DiscountUnitMoney {
		return nil, decimal.Zero, ErrVoucherNotMoneyUnit
	}

	if voucher.MinPurchaseMoney != nil && subtotal.LessThan(*voucher.MinPurchaseMoney) {
		return nil, decimal.Zero, ErrVoucherMinPurchase
	}

	if voucher.IsUsageLimitReached() {
		return nil, decimal.Zero, ErrVoucherUsageLimitExceeded
	}

	// I-02 (review vòng 5): phần đếm usage_per_user (CountUserVoucherUsage + CountUserHeldOrders)
	// TRƯỚC ĐÂY nằm ở đây (H3-01a, review vòng 4) — chạy NGOÀI transaction tạo đơn nên vẫn TOCTOU
	// được: 2 request đồng thời của CÙNG user cùng đọc heldCount cũ rồi cùng vượt qua, trước khi
	// bên nào kịp tạo đơn/reserve. Đã CHUYỂN HẲN vào LockAndCheckUsagePerUser (bên dưới) — gọi
	// TRONG transaction tạo đơn, SAU khi đã SELECT ... FOR UPDATE khoá dòng voucher, để tuần tự
	// hoá đúng 2 giao dịch đồng thời thay vì đọc song song không khoá. ValidateAndApplyVoucher từ
	// đây chỉ còn làm các kiểm tra THUẦN (không phụ thuộc lock/đếm) để trả lỗi sớm cho UX — kết
	// quả CHƯA phải quyết định cuối cùng cho usage_per_user, quyết định thật nằm ở
	// LockAndCheckUsagePerUser bên trong transaction.

	if paymentMethod != "" && !voucher.AcceptAllPaymentMethods {
		allowed := false
		for _, m := range voucher.PaymentMethodsAccepted {
			if m == paymentMethod {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, decimal.Zero, ErrVoucherPaymentNotAllow
		}
	}

	discount := calculateVoucherDiscountDecimal(voucher, subtotal)
	// H3-03 (review vòng 4): discount <= 0 phải bị TỪ CHỐI (khớp web voucher.service.ts:
	// "if (discount <= 0) return { ok:false, ... }") — trước đây backend trả (voucher, 0, nil),
	// tức "áp dụng thành công, giảm 0đ", khiến CreateOrder vẫn gán order.VoucherID + gọi
	// ReserveVoucherUsage (H2-05), TIÊU MỘT LƯỢT used_count cho một mã không giảm được đồng
	// nào (vd PERCENT 10% trên subtotal=5 → Floor(0.5)=0; hoặc FIXED với discount_amount_money
	// null/0).
	if discount.LessThanOrEqual(decimal.Zero) {
		return nil, decimal.Zero, ErrVoucherNotApplicable
	}
	return voucher, discount, nil
}

// IncrementUsedCount — xem comment interface.
func (vs *VoucherService) IncrementUsedCount(ctx context.Context, voucherID uuid.UUID) error {
	return vs.vr.IncrementUsedCount(ctx, voucherID)
}

// ReserveVoucherUsage — xem comment interface. Chạy trực tiếp trên "tx" (không qua vs.vr, vốn
// luôn cầm connection gốc) để lời gọi này tham gia đúng transaction của caller.
//
// M3-05 (review vòng 4): TRƯỚC ĐÂY copy tay chuỗi điều kiện "usage_limit <= 0 OR used_count <
// usage_limit" thay vì gọi lại buildIncrementUsedCountQuery (repository.VoucherRepository) —
// dùng repository.VoucherUsageAvailableCondition (hằng số dùng chung, cũng thêm "usage_limit IS
// NULL" mà bản copy cũ thiếu) thay vì viết lại chuỗi, tránh lệch nhau lần nữa.
// buildReserveVoucherUsageQuery (M3-06, review vòng 4): tách phần XÂY câu UPDATE ra khỏi phần
// map RowsAffected -> error — cùng mẫu buildIncrementUsedCountQuery/buildRestoreAndReactivateQuery
// đã dùng ở repository layer, để test DryRun (voucher_service_reserve_release_test.go) gọi được
// ĐÚNG hàm sản xuất thật thay vì hand-roll lại câu query trong test.
func (vs *VoucherService) buildReserveVoucherUsageQuery(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) *gorm.DB {
	return tx.WithContext(ctx).Model(&model.Voucher{}).
		Where("id = ? AND ("+repository.VoucherUsageAvailableCondition+")", voucherID).
		Update("used_count", gorm.Expr("used_count + 1"))
}

func (vs *VoucherService) ReserveVoucherUsage(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) error {
	result := vs.buildReserveVoucherUsageQuery(ctx, tx, voucherID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrVoucherUsageLimitExceeded
	}
	return nil
}

// buildReleaseVoucherUsageQuery (M3-06, review vòng 4) — xem comment buildReserveVoucherUsageQuery.
func (vs *VoucherService) buildReleaseVoucherUsageQuery(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) *gorm.DB {
	return tx.WithContext(ctx).Model(&model.Voucher{}).
		Where("id = ? AND used_count > 0", voucherID).
		Update("used_count", gorm.Expr("used_count - 1"))
}

// ReleaseVoucherUsage — hoàn lại 1 lượt used_count đã reserve khi đơn hàng bị hủy/hết hạn trước
// khi hoàn tất. Điều kiện "used_count > 0" tránh giảm xuống âm nếu bị gọi trùng lặp.
func (vs *VoucherService) ReleaseVoucherUsage(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) error {
	return vs.buildReleaseVoucherUsageQuery(ctx, tx, voucherID).Error
}

// LockAndCheckUsagePerUser — xem comment interface. tx != nil BẮT BUỘC (gọi ngoài transaction
// không có tác dụng khoá gì — LockVoucherForUpdate tự nhả lock ngay sau câu SQL đơn lẻ đó).
func (vs *VoucherService) LockAndCheckUsagePerUser(ctx context.Context, tx *gorm.DB, voucher *model.Voucher, userID uuid.UUID) error {
	if !voucher.HasPerUserLimit() {
		// UsagePerUser = model.VoucherUnlimitedUsage (0/âm): không giới hạn số lượt cho từng user
		// (quyết định sản phẩm 09/2026, pin bằng TestLockAndCheckUsagePerUser_ZeroMeansUnlimited).
		// Admin vẫn có thể giới hạn TOÀN CỤC qua UsageLimit/ReserveVoucherUsage; usage_per_user
		// chỉ kiểm soát riêng số lượt của TỪNG user.
		return nil
	}

	txVr := repository.NewVoucherRepository(tx)
	if err := txVr.LockVoucherForUpdate(ctx, voucher.ID); err != nil {
		return err
	}
	completedCount, err := txVr.CountUserVoucherUsage(ctx, userID, voucher.ID)
	if err != nil {
		return err
	}
	heldCount, err := txVr.CountUserHeldOrders(ctx, userID, voucher.ID)
	if err != nil {
		return err
	}
	if completedCount+heldCount >= int64(voucher.UsagePerUser) {
		return ErrVoucherPerUserLimitExceeded
	}
	return nil
}

// RecordUsageLogTx — xem comment interface (RecordUsageLogTx/RecordUsageLog). Tự tra lại
// voucher.Code (VoucherLog.VoucherCode not-null) thay vì bắt caller truyền vào, tránh caller
// phải tự query/preload thêm. Amount trong VoucherLog là int64 (không có phần thập phân với
// VND) nên dùng IntPart().
//
// H2-06 vòng 3b: "tx" != nil -> ghi VoucherLog TRÊN chính transaction đó (tham gia cùng
// transaction với completeOrderFulfillment) — dùng cho CheckAndProcessPayment/CreateOrder khi đã
// có sẵn transaction đang mở. "tx" == nil -> ghi qua vs.vr (connection gốc) như hành vi cũ.
func (vs *VoucherService) RecordUsageLogTx(ctx context.Context, tx *gorm.DB, voucherID, userID, orderID uuid.UUID, discountAmount decimal.Decimal) error {
	voucher, err := vs.vr.GetVoucherByID(ctx, voucherID)
	if err != nil {
		return err
	}
	code := ""
	if voucher != nil {
		code = voucher.Code
	}
	log := &model.VoucherLog{
		UserID:      userID,
		VoucherID:   voucherID,
		VoucherCode: code,
		OrderID:     orderID,
		OrderType:   "course_order",
		Action:      "used",
		Amount:      discountAmount.IntPart(),
	}
	if tx != nil {
		return tx.WithContext(ctx).Create(log).Error
	}
	return vs.vr.CreateVoucherLog(ctx, log)
}

// RecordUsageLog — giữ nguyên chữ ký cũ (không tx) cho tương thích ngược, ủy quyền thẳng tới
// RecordUsageLogTx(ctx, nil, ...).
func (vs *VoucherService) RecordUsageLog(ctx context.Context, voucherID, userID, orderID uuid.UUID, discountAmount decimal.Decimal) error {
	return vs.RecordUsageLogTx(ctx, nil, voucherID, userID, orderID, discountAmount)
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
	// holders_only: không tự lưu được — "lưu" là con đường để người ngoài tự cấp quyền cho mình,
	// nên chỉ người ĐÃ giữ (được cấp) mới đi tiếp. Người khác nhận lỗi như mã không tồn tại.
	usable, err := vs.voucherUsableBy(ctx, voucher, userID)
	if err != nil {
		return nil, err
	}
	if !usable {
		return nil, repository.ErrVoucherNotFound
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

// Lý do không phát được voucher làm giải (VoucherGrantError.Reason), viết sẵn cho admin đọc.
const (
	VoucherGrantReasonNotFound     = "không tồn tại hoặc đã bị xoá"
	VoucherGrantReasonInactive     = "đang tắt"
	VoucherGrantReasonExpired      = "đã hết hạn"
	VoucherGrantReasonUsageLimit   = "đã hết tổng lượt dùng"
	VoucherGrantReasonNotStarted   = "chưa tới ngày hiệu lực"
	VoucherGrantReasonPerUserLimit = "người thắng đã dùng hết lượt của voucher này (tính cả đơn đang chờ thanh toán)"
)

// VoucherGrantError (re-review vòng 2, chủ dự án chốt 29/09): lần chốt vẫn bị chặn khi một voucher
// giải không phát được, nhưng lỗi phải nói rõ NGƯỜI THẮNG nào và VOUCHER nào để admin sửa giải rồi
// chốt lại. Unwrap về ErrVoucherUnavailableForGrant nên mọi chỗ đang dùng errors.Is vẫn chạy;
// ContestService dùng errors.As để lấy chi tiết. UserName/Rank do IssueAwardTx điền thêm.
type VoucherGrantError struct {
	UserID, VoucherID uuid.UUID
	VoucherCode       string
	Reason            string
	UserName          string
	Rank              *int
}

func (e *VoucherGrantError) Unwrap() error { return ErrVoucherUnavailableForGrant }

// Error là câu hiển thị cho admin, ví dụ: Không phát được voucher GIAI1 cho người thắng Nguyễn An
// (hạng 1, user ...): đã hết tổng lượt dùng. Sửa giải hoặc voucher rồi chốt lại.
func (e *VoucherGrantError) Error() string {
	who := e.UserID.String()
	if e.UserName != "" {
		who = e.UserName + " (" + who + ")"
	}
	if e.Rank != nil {
		who = fmt.Sprintf("%s, hạng %d", who, *e.Rank)
	}
	code := e.VoucherCode
	if code == "" {
		code = e.VoucherID.String()
	}
	return fmt.Sprintf("Không phát được voucher %s cho người thắng %s: %s. Sửa giải hoặc voucher rồi chốt lại.", code, who, e.Reason)
}

// voucherGrantBlockReason: lý do voucher không phát được cho BẤT KỲ ai; "" = phát được.
func voucherGrantBlockReason(v *model.Voucher, now time.Time) string {
	switch {
	case !v.IsActive:
		return VoucherGrantReasonInactive
	case v.EndDate != nil && !v.EndDate.After(now):
		return VoucherGrantReasonExpired
	case v.IsUsageLimitReached():
		return VoucherGrantReasonUsageLimit
	case v.StartDate != nil && v.StartDate.After(now):
		return VoucherGrantReasonNotStarted
	}
	return ""
}

// GrantVoucherTx (contract "Cuộc thi" §5, §6): ghi user_vouchers cho userID TRÊN "tx" được truyền
// vào, để việc phát voucher nằm CHUNG transaction chốt kết quả — lỗi ở bất kỳ người đạt giải nào
// làm rollback toàn bộ, không để lại voucher phát dở cho một nửa bảng xếp hạng.
//
// Voucher phải còn dùng được: tồn tại, chưa xoá mềm (scope DeletedAt của GORM tự loại), is_active,
// start_date NULL hoặc đã tới, end_date NULL hoặc còn ở tương lai, chưa hết tổng lượt dùng và chưa
// hết lượt dùng của chính user đó. Ngược lại trả ErrVoucherUnavailableForGrant (409
// CONTEST_VOUCHER_UNAVAILABLE). Khoá FOR SHARE dòng voucher để một thao tác tắt/xoá voucher chạy
// song song phải chờ transaction này kết thúc, tránh vừa kiểm "đang bật" xong thì voucher bị tắt.
// Không động tới used_count/usage_limit: đó là lượt DÙNG khi thanh toán, còn đây chỉ là đưa voucher
// vào ví (giống SaveVoucher).
func (vs *VoucherService) GrantVoucherTx(ctx context.Context, tx *gorm.DB, userID, voucherID uuid.UUID, source, notes string) (*model.UserVoucher, error) {
	if tx == nil {
		return nil, errors.New("GrantVoucherTx requires a transaction")
	}
	var voucher model.Voucher
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("id = ?", voucherID).Take(&voucher).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &VoucherGrantError{UserID: userID, VoucherID: voucherID, Reason: VoucherGrantReasonNotFound}
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	// Voucher tắt / hết hạn / hết tổng lượt (review PR #80, F4) / chưa tới start_date (chốt 28/09):
	// người thắng sẽ cầm voucher không dùng được, nên từ chối ngay để admin biết lúc chốt.
	if reason := voucherGrantBlockReason(&voucher, now); reason != "" {
		return nil, &VoucherGrantError{UserID: userID, VoucherID: voucherID, VoucherCode: voucher.Code, Reason: reason}
	}
	// Giới hạn lượt theo từng user (usage_per_user): đếm đúng luật của LockAndCheckUsagePerUser
	// (đã dùng + đang giữ trong đơn chờ) nhưng KHÔNG khoá FOR UPDATE — tx chốt đã giữ FOR SHARE ở
	// trên; hai lần chốt dùng chung voucher cùng nâng khoá lên FOR UPDATE sẽ deadlock.
	if voucher.HasPerUserLimit() {
		txVr := repository.NewVoucherRepository(tx)
		used, err := txVr.CountUserVoucherUsage(ctx, userID, voucherID)
		if err != nil {
			return nil, err
		}
		held, err := txVr.CountUserHeldOrders(ctx, userID, voucherID)
		if err != nil {
			return nil, err
		}
		if used+held >= int64(voucher.UsagePerUser) {
			return nil, &VoucherGrantError{UserID: userID, VoucherID: voucherID, VoucherCode: voucher.Code, Reason: VoucherGrantReasonPerUserLimit}
		}
	}
	// Idempotent (chủ dự án chốt): user_vouchers không có unique (user_id, voucher_id). Nếu học viên
	// đã tự lưu voucher này (SaveVoucher) thì dùng lại dòng cũ, không tạo dòng trùng trong ví.
	var existing model.UserVoucher
	err = tx.WithContext(ctx).Where("user_id = ? AND voucher_id = ?", userID, voucherID).
		Order("saved_at ASC").Take(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	userVoucher := &model.UserVoucher{
		UserID:    userID,
		VoucherID: voucherID,
		Source:    source,
		SavedAt:   now,
		Notes:     notes,
	}
	if err := tx.WithContext(ctx).Create(userVoucher).Error; err != nil {
		return nil, err
	}
	return userVoucher, nil
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
	if voucher.HasUsageLimit() {
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
