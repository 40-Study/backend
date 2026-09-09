package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrOrderForbidden (H-06, audit 260909 vòng 2): GetOrder/Cancel/PaymentIntent/PaymentStatus/
// CheckPayment trước đây không kiểm tra order.UserID với actor gọi API — bất kỳ user đăng
// nhập nào biết orderID (UUID có thể đoán được qua thứ tự tạo hoặc rò rỉ) đều xem/hủy/thao
// tác thanh toán đơn hàng của người khác. Dùng chung 1 error cho mọi service (order/payment)
// vì cùng 1 khái niệm "không phải chủ đơn hàng, không phải admin".
var ErrOrderForbidden = errors.New("forbidden: not the order owner")

var (
	ErrOrderNotFound              = errors.New("order not found")
	ErrInvalidStateTransition     = errors.New("invalid state transition")
	ErrOrderAlreadyCompleted      = errors.New("order already completed")
	ErrOrderAlreadyCancelled      = errors.New("order already cancelled")
	ErrDuplicateOrderItem         = errors.New("duplicate order item")
	ErrAmountMismatch             = errors.New("amount mismatch")
	ErrAlreadyEnrolled            = errors.New("already enrolled in course")
	ErrCouponInvalid              = errors.New("invalid coupon")
	ErrCourseNotFound             = errors.New("course not found")
	ErrIdempotencyPayloadMismatch = errors.New("idempotency payload mismatch")
	ErrIdempotencyKeyNotFound     = errors.New("idempotency key not found")
	ErrIdempotencyKeyExpired      = errors.New("idempotency key expired")
)

type OrderServiceInterface interface {
	CreateOrder(ctx context.Context, userID uuid.UUID, req dto.CreateOrderRequest) (*dto.OrderResponse, error)
	GetOrderByID(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.OrderResponse, error)
	GetOrderByNumber(ctx context.Context, orderNumber string) (*dto.OrderResponse, error)
	GetUserOrders(ctx context.Context, userID uuid.UUID, page, limit int, status string) (*dto.OrderListResponse, error)
	CancelOrder(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, reason string) error
	ValidateIdempotencyKey(ctx context.Context, scope, key string, requestHash string) (*dto.OrderResponse, bool, error)
}

type OrderService struct {
	orderRepo          repository.OrderRepositoryInterface
	orderItemRepo      repository.OrderItemRepositoryInterface
	courseRepo         repository.CourseRepositoryInterface
	cartRepo           repository.CartItemRepositoryInterface
	idempotencyKeyRepo repository.IdempotencyKeyRepositoryInterface
	// voucherService (item 24, review web vòng 1): CreateOrder validate/áp mã giảm giá qua
	// bảng vouchers thay vì coupons.
	voucherService VoucherServiceInterface
}

// M3-09 (review vòng 3b, bổ sung vòng 4): TRƯỚC ĐÂY NewOrderService còn nhận couponRepo/
// enrollmentRepo/orderHistoryRepo — cả 3 đã 0 lần được đọc trong order_service.go (grep xác
// nhận): couponRepo bỏ hẳn sau khi CompleteOrder (dùng flow coupon cũ) bị xóa ở vòng 3b;
// enrollmentRepo (bare, không tx-bound) cùng số phận — mọi thao tác enrollment giờ đi qua
// enrollmentRepoTx dựng từ txDB (H2-06/H2-03, vòng 3); orderHistoryRepo (bare) tương tự — mọi
// ghi history giờ đi qua orderHistoryRepoTx. Giữ lại field/tham số DEAD chỉ để "tương thích" là
// mầm mống nhầm lẫn cho người đọc sau (tưởng còn được dùng) — xóa hẳn khỏi cả struct lẫn
// constructor, cập nhật app/services.go cùng lượt.
func NewOrderService(
	orderRepo repository.OrderRepositoryInterface,
	orderItemRepo repository.OrderItemRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	cartRepo repository.CartItemRepositoryInterface,
	idempotencyKeyRepo repository.IdempotencyKeyRepositoryInterface,
	voucherService VoucherServiceInterface,
) *OrderService {
	return &OrderService{
		orderRepo:          orderRepo,
		orderItemRepo:      orderItemRepo,
		courseRepo:         courseRepo,
		cartRepo:           cartRepo,
		idempotencyKeyRepo: idempotencyKeyRepo,
		voucherService:     voucherService,
	}
}

// Order status transitions
var validTransitions = map[string][]string{
	"pending":    {"processing", "cancelled"},
	"processing": {"completed", "failed", "cancelled"},
	"completed":  {"refunded"},
	"failed":     {"pending"}, // Allow retry
	"cancelled":  {},
	"refunded":   {},
	// "expired" (M3-04, review vòng 4): TRƯỚC ĐÂY không có key này — isValidTransition("expired",
	// ...) luôn false một cách NGẪU NHIÊN (không có key -> map trả zero-value {}), không phải
	// chủ đích. Sau khi B3-01 được vá, đơn thực sự chuyển "expired" (CheckAndProcessPayment hết
	// hạn mã thanh toán, hoặc lazy sweep của CreateOrder — cả hai đều đi qua
	// releaseOrderAndTransition, không qua isValidTransition/CancelOrder). Khai báo TƯỜNG MINH
	// là trạng thái CHỐT (không cho chuyển tiếp qua CancelOrder) — hiện hệ thống chưa có luồng
	// "tạo lại payment intent cho đơn đã expired", nên KHÔNG mở "pending" ở đây để tránh CancelOrder
	// vô tình cho phép 1 hành động chưa có route/luồng nào hỗ trợ. Đây cũng là lý do CancelOrder
	// không double-release voucher sau khi đơn đã expired — chốt chặn có chủ đích, không phải may.
	"expired": {},
}

func (s *OrderService) isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, status := range allowed {
		if status == to {
			return true
		}
	}
	return false
}

// pendingOrderDefaultTTL (H3-01b, review vòng 4): hạn mặc định cho đơn "pending" CHƯA từng tạo
// payment intent (payment_code_expired_at còn NULL) — dùng làm ngưỡng lazy-sweep trong
// sweepExpiredHeldOrders. Khớp đúng hạn 24h đã dùng sẵn cho payment intent thật
// (CreatePaymentIntent, payment_service.go) và cho ExpiresAt hiển thị ở toOrderResponse bên
// dưới — không phát minh một con số mới, giữ nhất quán với quy ước đã có trong codebase.
const pendingOrderDefaultTTL = 24 * time.Hour

func (s *OrderService) CreateOrder(ctx context.Context, userID uuid.UUID, req dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	// H3-01b (review vòng 4): quét lazy — trước khi tạo đơn mới, chuyển các đơn pending/
	// processing CỦA USER ĐÓ đã quá hạn sang "expired" + hoàn used_count voucher đã reserve.
	// KHÔNG cần cron/worker riêng: mỗi lần user tạo đơn mới là một cơ hội dọn các đơn cũ CHÍNH
	// HỌ đã bỏ rơi (tạo đơn xong đóng tab, không bao giờ bấm thanh toán) — đặt NGAY ĐẦU hàm, TRƯỚC
	// bước tính usage_per_user (ValidateAndApplyVoucher bên dưới) để voucher vừa được giải
	// phóng có thể dùng lại được luôn trong CHÍNH lần tạo đơn này. Lỗi sweep KHÔNG bị nuốt — nếu
	// sweep lỗi, dừng hẳn CreateOrder thay vì tạo đơn mới trên trạng thái voucher có thể sai.
	if err := s.sweepExpiredHeldOrders(ctx, userID); err != nil {
		return nil, err
	}

	var courseIDs []uuid.UUID
	// selectedCourseIDs (item 26, review web vòng 1): khi client gửi kèm course_ids CÙNG với
	// source="cart" (chọn một phần giỏ hàng để checkout thay vì cả giỏ), nhớ lại tập đã CHỌN
	// để lúc dọn giỏ hàng chỉ xóa đúng các item này — trước đây CreateOrder BỎ QUA hoàn toàn
	// req.CourseIDs khi source="cart" (luôn lấy TOÀN BỘ giỏ hàng), khiến UI hiện giá của vài
	// khóa được chọn nhưng đơn tạo ra + số tiền chuyển khoản lại tính trên CẢ giỏ hàng.
	var selectedCourseIDs []uuid.UUID
	if len(req.CourseIDs) > 0 {
		selectedCourseIDs = make([]uuid.UUID, 0, len(req.CourseIDs))
		for _, idStr := range req.CourseIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				return nil, errors.New("invalid course id")
			}
			selectedCourseIDs = append(selectedCourseIDs, id)
		}
	}

	if req.Source == "cart" {
		cartItems, err := s.cartRepo.GetByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}

		if len(selectedCourseIDs) > 0 {
			// Chỉ lấy các item trong giỏ TRÙNG với course_ids client chọn — không lấy cả giỏ.
			selectedSet := make(map[uuid.UUID]bool, len(selectedCourseIDs))
			for _, id := range selectedCourseIDs {
				selectedSet[id] = true
			}
			courseIDs = make([]uuid.UUID, 0, len(selectedCourseIDs))
			for _, item := range cartItems {
				if selectedSet[item.CourseID] {
					courseIDs = append(courseIDs, item.CourseID)
				}
			}
		} else {
			courseIDs = make([]uuid.UUID, 0, len(cartItems))
			for _, item := range cartItems {
				courseIDs = append(courseIDs, item.CourseID)
			}
		}
	} else if req.Source == "buy_now" {
		courseIDs = selectedCourseIDs
	} else {
		return nil, errors.New("invalid source")
	}

	if len(courseIDs) == 0 {
		return nil, errors.New("no courses selected")
	}

	// Get course details and calculate pricing
	courses, err := s.getCoursesByIDs(ctx, courseIDs)
	if err != nil {
		return nil, err
	}

	if len(courses) != len(courseIDs) {
		return nil, ErrCourseNotFound
	}

	subtotal := decimal.Zero
	for _, course := range courses {
		price := course.Price
		subtotal = subtotal.Add(price)
	}

	// item 24 (review web vòng 1): validate/áp mã giảm giá qua VOUCHERS thay vì COUPONS —
	// bảng coupons không còn route/handler nào tạo dữ liệu (đã grep xác nhận), web tra mã bằng
	// GET /vouchers/code/:code nên mã người dùng nhập luôn thuộc bảng vouchers. Giữ tên field
	// DTO "coupon_code" để không phá tương thích ngược với web (web vẫn gửi coupon_code).
	var voucher *model.Voucher
	discountAmount := decimal.Zero

	if req.CouponCode != "" {
		voucher, discountAmount, err = s.voucherService.ValidateAndApplyVoucher(ctx, req.CouponCode, userID, subtotal, "")
		if err != nil {
			return nil, ErrCouponInvalid
		}
	}

	// Calculate tax (0% for now - can be configured)
	taxAmount := decimal.Zero

	// Calculate total
	totalAmount := subtotal.Sub(discountAmount).Add(taxAmount)
	if totalAmount.LessThan(decimal.Zero) {
		totalAmount = decimal.Zero
	}

	// Generate order number
	orderNumber := s.generateOrderNumber()

	// item 14 (review vòng 1): đơn 0đ (khóa miễn phí, hoặc voucher giảm 100%) đánh dấu
	// "completed" NGAY khi tạo — trước đây LUÔN đặt "pending" bất kể totalAmount, nhưng
	// enrollment chỉ được tạo trong CheckAndProcessPayment sau khi có giao dịch ngân hàng
	// khớp mã thanh toán, mà đơn 0đ không bao giờ có giao dịch ngân hàng nào cả -> học viên
	// không bao giờ được ghi danh dù web đã điều hướng sang trang "thành công".
	status := "pending"
	var paidAt *time.Time
	isFreeOrder := totalAmount.IsZero()
	if isFreeOrder {
		status = "completed"
		now := time.Now()
		paidAt = &now
	}

	// Create order
	order := &model.Order{
		ID:             uuid.New(),
		UserID:         userID,
		OrderNumber:    orderNumber,
		Subtotal:       subtotal,
		DiscountAmount: discountAmount,
		TaxAmount:      taxAmount,
		TotalAmount:    totalAmount,
		Currency:       "VND",
		Status:         status,
		PaidAt:         paidAt,
		CouponID:       nil,
		VoucherID:      nil,
		Notes:          nil,
	}

	if voucher != nil {
		order.VoucherID = &voucher.ID
	}

	if req.Note != "" {
		order.Notes = &req.Note
	}

	// Create order items with price snapshot
	items := make([]model.OrderItem, 0, len(courses))
	for _, course := range courses {
		price := course.Price

		item := model.OrderItem{
			ID:             uuid.New(),
			CreatedAt:      time.Now(),
			OrderID:        order.ID,
			CourseID:       course.ID,
			Price:          price,
			DiscountAmount: decimal.Zero,
			FinalPrice:     price,
		}

		// Apply pro-rated discount
		if discountAmount.GreaterThan(decimal.Zero) && subtotal.GreaterThan(decimal.Zero) {
			item.DiscountAmount = price.Mul(discountAmount).Div(subtotal)
			item.FinalPrice = price.Sub(item.DiscountAmount)
		}

		items = append(items, item)
	}

	// Save order and items in transaction
	//
	// H2-06 (review vòng 3): TRƯỚC ĐÂY chỉ txRepo.Create(order) chạy trong transaction —
	// s.orderItemRepo/s.orderHistoryRepo dùng CONNECTION GỐC (không phải tx), nên lỗi ở bất kỳ
	// bước nào sau order_items/history KHÔNG rollback được order đã insert. Dựng orderItemRepo/
	// orderHistoryRepo TX-BOUND qua txRepo.TxDB() để toàn bộ order + order_items + history +
	// voucher used_count + (đơn 0đ) fulfillment CÙNG rollback nếu bất kỳ bước nào lỗi.
	err = s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		txDB := txRepo.TxDB()

		if err := txRepo.Create(order); err != nil {
			return err
		}

		orderItemRepoTx := repository.NewOrderItemRepository(txDB)
		if err := orderItemRepoTx.CreateBatch(items); err != nil {
			return err
		}

		// Create status history — item 14: đơn 0đ ghi thẳng ToStatus="completed" (khớp
		// order.Status vừa set ở trên), không phải "pending" cứng như trước.
		reason := "Order created"
		if isFreeOrder {
			reason = "Free order auto-completed (0đ)"
		}
		history := &model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  time.Now(),
			OrderID:    order.ID,
			FromStatus: "",
			ToStatus:   status,
			Reason:     reason,
		}
		orderHistoryRepoTx := repository.NewOrderStatusHistoryRepository(txDB)
		if err := orderHistoryRepoTx.Create(history); err != nil {
			return err
		}

		// H2-05 (review vòng 3): reserve used_count NGAY khi tạo đơn (áp dụng cho MỌI đơn có
		// voucher, không chỉ đơn 0đ) — trước đây chỉ tăng lúc đơn HOÀN TẤT
		// (completeOrderFulfillment), để hở khoảng trống: nhiều đơn "pending" cùng lúc dùng
		// chung 1 voucher có thể vượt usage_limit vì chưa ai bị trừ lúc tạo đơn. Hoàn lại ở
		// CancelOrder khi đơn bị hủy trước khi hoàn tất (và ở CheckAndProcessPayment khi mã
		// thanh toán hết hạn — M2-02).
		if voucher != nil {
			if err := s.voucherService.ReserveVoucherUsage(ctx, txDB, voucher.ID); err != nil {
				return err
			}
		}

		// H2-03 (review vòng 3): đơn 0đ (khóa miễn phí/voucher giảm 100%) hoàn tất fulfillment
		// (enrollment + total_students) NGAY TRONG transaction này — lỗi ở đây rollback TOÀN BỘ
		// (order, items, history, voucher reserve) thay vì để lại đơn "completed" không có
		// enrollment như trước (fulfillment TRƯỚC ĐÂY chạy sau khi transaction đã commit).
		if isFreeOrder {
			enrollmentRepoTx := repository.NewEnrollmentRepository(txDB)
			courseRepoTx := repository.NewCourseRepository(txDB)
			if err := completeOrderFulfillment(ctx, txDB, enrollmentRepoTx, courseRepoTx, s.voucherService, items, order); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// item 26 (review web vòng 1): chỉ xóa khỏi giỏ hàng đúng các course ĐÃ CHỌN — trước đây
	// luôn DeleteByUserID (xóa TOÀN BỘ giỏ) dù client chỉ chọn một phần. Không có course_ids
	// (checkout cả giỏ, hành vi mặc định cũ) vẫn xóa toàn bộ như trước.
	if req.Source == "cart" {
		if len(selectedCourseIDs) > 0 {
			for _, courseID := range selectedCourseIDs {
				if err := s.cartRepo.Delete(ctx, userID, courseID); err != nil {
					return nil, err
				}
			}
		} else {
			if err := s.cartRepo.DeleteByUserID(ctx, userID); err != nil {
				return nil, err
			}
		}
	}

	// item 14/H2-03: fulfillment cho đơn 0đ giờ chạy BÊN TRONG transaction phía trên (cùng
	// order/items/history/voucher-reserve) — xem comment "H2-03" ở khối WithTransaction. Không
	// còn gọi completeOrderFulfillment ở đây (ngoài transaction) nữa.

	return s.toOrderResponse(order, items), nil
}

// GetOrderByID - Get order by ID
func (s *OrderService) GetOrderByID(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.OrderResponse, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	// H-06: chỉ chủ đơn hàng hoặc admin mới xem được — đơn hàng chứa thông tin nhạy cảm
	// (giá, mã giảm giá, phương thức thanh toán).
	if order.UserID != actorUserID && !isAdmin {
		return nil, ErrOrderForbidden
	}

	items, err := s.orderItemRepo.GetByOrderID(orderID)
	if err != nil {
		return nil, err
	}

	return s.toOrderResponse(order, items), nil
}

// GetOrderByNumber - Get order by order number
func (s *OrderService) GetOrderByNumber(ctx context.Context, orderNumber string) (*dto.OrderResponse, error) {
	order, err := s.orderRepo.GetByOrderNumber(orderNumber)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	items, err := s.orderItemRepo.GetByOrderID(order.ID)
	if err != nil {
		return nil, err
	}

	return s.toOrderResponse(order, items), nil
}

// GetUserOrders - Get orders for user with pagination
func (s *OrderService) GetUserOrders(ctx context.Context, userID uuid.UUID, page, limit int, status string) (*dto.OrderListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	orders, total, err := s.orderRepo.GetByUserID(userID, page, limit, status)
	if err != nil {
		return nil, err
	}

	orderResponses := make([]dto.OrderResponse, 0, len(orders))
	for _, order := range orders {
		items, _ := s.orderItemRepo.GetByOrderID(order.ID)
		orderResponses = append(orderResponses, *s.toOrderResponse(&order, items))
	}

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	return &dto.OrderListResponse{
		Orders:     orderResponses,
		TotalCount: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, reason string) error {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	// H-06: chỉ chủ đơn hàng hoặc admin mới hủy được.
	if order.UserID != userID && !isAdmin {
		return ErrOrderForbidden
	}

	if !s.isValidTransition(order.Status, "cancelled") {
		return ErrInvalidStateTransition
	}

	var applied bool
	err = s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		var txErr error
		// M3-02/H3-01c (review vòng 4): releaseOrderAndTransition dùng UPDATE CÓ ĐIỀU KIỆN
		// (WHERE status = order.Status vừa đọc ở trên) + kiểm RowsAffected — chặn race 2 request
		// đồng thời (2 tab bấm "Hủy đơn", hoặc 1 tab hủy + 1 tab poll trúng lúc mã hết hạn) cùng
		// vượt qua isValidTransition() (đọc TRƯỚC transaction, có thể đã stale) và cùng gọi
		// ReleaseVoucherUsage — trước đây UpdateStatus/UpdateStatusWithTx chạy VÔ ĐIỀU KIỆN nên
		// cả 2 request đều "thắng", trừ used_count 2 lần cho 1 lần reserve.
		applied, txErr = releaseOrderAndTransition(ctx, txRepo, s.voucherService, order, []string{order.Status}, "cancelled", reason)
		return txErr
	})
	if err != nil {
		return err
	}
	if !applied {
		// RowsAffected == 0: trạng thái order đã đổi (bởi request khác) kể từ lúc đọc ở trên —
		// không còn đúng "order.Status" đã kiểm isValidTransition, coi như transition thất bại.
		return ErrInvalidStateTransition
	}
	return nil
}

// releaseOrderAndTransition (H3-01c/M3-02, review vòng 4): chuyển order sang targetStatus bằng
// UPDATE CÓ ĐIỀU KIỆN (status hiện tại phải nằm trong fromStatuses) + kiểm RowsAffected — chỉ
// ghi history + release voucher NẾU RowsAffected == 1 (đúng 1 request "thắng" cuộc đua chuyển
// trạng thái). Dùng chung cho CancelOrder (chuyển "cancelled"), OrderService.CreateOrder's lazy
// sweep VÀ PaymentService.CheckAndProcessPayment's nhánh hết hạn mã thanh toán (cả hai chuyển
// "expired") — tránh 3 nơi tự viết lại cùng 1 pattern UPDATE-có-điều-kiện rồi lệch nhau (đúng
// bài học M3-05 review vòng 4: 3 bản sao điều kiện usage_limit đã lệch nhau vì copy tay).
// applied=false nghĩa là KHÔNG có gì thay đổi (status đã bị request khác đổi trước) — caller tự
// quyết định coi đó là lỗi (CancelOrder) hay bỏ qua êm (sweep/expire, best-effort).
func releaseOrderAndTransition(
	ctx context.Context,
	txRepo *repository.OrderRepository,
	voucherService VoucherServiceInterface,
	order *model.Order,
	fromStatuses []string,
	targetStatus string,
	reason string,
) (applied bool, err error) {
	txDB := txRepo.TxDB()

	result := txDB.Model(&model.Order{}).
		Where("id = ? AND status IN ?", order.ID, fromStatuses).
		Update("status", targetStatus)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}

	history := &model.OrderStatusHistory{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		OrderID:    order.ID,
		FromStatus: order.Status,
		ToStatus:   targetStatus,
		Reason:     reason,
	}
	orderHistoryRepoTx := repository.NewOrderStatusHistoryRepository(txDB)
	if err := orderHistoryRepoTx.Create(history); err != nil {
		return false, err
	}

	if order.VoucherID != nil && voucherService != nil {
		if err := voucherService.ReleaseVoucherUsage(ctx, txDB, *order.VoucherID); err != nil {
			return false, err
		}
	}

	return true, nil
}

// H2-06 vòng 3b: CompleteOrder (flow COUPON cũ, dùng payment_method/transaction_id nhận trực
// tiếp thay vì đi qua PaymentService/gRPC) đã bị XÓA — grep xác nhận 0 caller (interface, handler,
// test) ở cả vòng 1/2/3, dùng flow coupons đã deprecated (bảng "coupons" không còn route/handler
// nào tạo dữ liệu — xem comment couponRepo phía trên), và có cùng lớp lỗi H2-04
// (GetByUserAndCourse có scope + Create vô điều kiện, không IncrementTotalStudents) như
// completeOrderFulfillment TRƯỚC KHI được sửa ở vòng 3. Quyết định team-lead vòng 3b: xóa hẳn
// thay vì sửa code chết. Luồng thật đi qua PaymentService.CheckAndProcessPayment.

// ValidateIdempotencyKey - Check if request is idempotent
func (s *OrderService) ValidateIdempotencyKey(ctx context.Context, scope, key string, requestHash string) (*dto.OrderResponse, bool, error) {
	if key == "" {
		return nil, false, nil
	}

	idemKey, err := s.idempotencyKeyRepo.GetByScopeAndKey(scope, key)
	if err != nil {
		if errors.Is(err, repository.ErrIdempotencyKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	// Check if expired
	if time.Now().After(idemKey.ExpiresAt) {
		return nil, false, nil
	}

	// Check if request hash matches
	if idemKey.RequestHash != requestHash {
		return nil, false, ErrIdempotencyPayloadMismatch
	}

	// Return cached response if successful
	if idemKey.ResponseCode >= 200 && idemKey.ResponseCode < 300 && idemKey.ResponseBody != "" {
		// Parse response body to order response
		// For simplicity, we return nil and let the handler reconstruct
		return nil, true, nil
	}

	return nil, true, nil
}

// sweepExpiredHeldOrders (H3-01b, review vòng 4) — xem comment gọi ở đầu CreateOrder. Mỗi đơn
// quá hạn được chuyển "expired" trong TRANSACTION RIÊNG (không gộp chung 1 transaction cho cả
// batch) qua releaseOrderAndTransition — 1 đơn lỗi/đã bị request khác xử lý trước (applied=false,
// bỏ qua êm) không chặn việc dọn các đơn còn lại trong batch.
func (s *OrderService) sweepExpiredHeldOrders(ctx context.Context, userID uuid.UUID) error {
	staleOrders, err := s.orderRepo.GetExpiredHeldOrdersForUser(userID, pendingOrderDefaultTTL)
	if err != nil {
		return err
	}
	for i := range staleOrders {
		order := &staleOrders[i]
		if err := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
			_, txErr := releaseOrderAndTransition(ctx, txRepo, s.voucherService, order, []string{order.Status}, "expired", "Order expired (lazy sweep on new order creation)")
			return txErr
		}); err != nil {
			return err
		}
	}
	return nil
}

// Helper functions

func (s *OrderService) generateOrderNumber() string {
	timestamp := time.Now().Format("20060102150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("ORD%s%s", timestamp, randomHex)
}

func (s *OrderService) getCoursesByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Course, error) {
	var courses []model.Course
	for _, id := range ids {
		course, err := s.courseRepo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		courses = append(courses, *course)
	}
	return courses, nil
}

func (s *OrderService) toOrderResponse(order *model.Order, items []model.OrderItem) *dto.OrderResponse {
	itemResponses := make([]dto.OrderItemResponse, 0, len(items))
	for _, item := range items {
		itemResponses = append(itemResponses, dto.OrderItemResponse{
			ID:             item.ID,
			CourseID:       item.CourseID,
			CourseName:     "", // Will be populated if needed
			Price:          item.Price,
			DiscountAmount: item.DiscountAmount,
			FinalPrice:     item.FinalPrice,
		})
	}

	now := time.Now()
	response := &dto.OrderResponse{
		ID:             order.ID,
		OrderNumber:    order.OrderNumber,
		Subtotal:       order.Subtotal,
		DiscountAmount: order.DiscountAmount,
		TaxAmount:      order.TaxAmount,
		TotalAmount:    order.TotalAmount,
		Currency:       order.Currency,
		Status:         order.Status,
		PaymentMethod:  order.PaymentMethod,
		PaymentGateway: order.PaymentGateway,
		PaidAt:         order.PaidAt,
		CouponID:       order.CouponID,
		Notes:          order.Notes,
		Items:          itemResponses,
		CreatedAt:      now,
	}

	// Set expiration (24 hours for pending orders)
	if order.Status == "pending" {
		expiresAt := now.Add(24 * time.Hour)
		response.ExpiresAt = &expiresAt
	}

	return response
}
