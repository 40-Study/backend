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
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

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
	GetOrderByID(ctx context.Context, orderID uuid.UUID) (*dto.OrderResponse, error)
	GetOrderByNumber(ctx context.Context, orderNumber string) (*dto.OrderResponse, error)
	GetUserOrders(ctx context.Context, userID uuid.UUID, page, limit int, status string) (*dto.OrderListResponse, error)
	CancelOrder(ctx context.Context, userID, orderID uuid.UUID, reason string) error
	CompleteOrder(ctx context.Context, orderID uuid.UUID, paymentMethod, transactionID string) error
	ValidateIdempotencyKey(ctx context.Context, scope, key string, requestHash string) (*dto.OrderResponse, bool, error)
}

type OrderService struct {
	orderRepo          repository.OrderRepositoryInterface
	orderItemRepo      repository.OrderItemRepositoryInterface
	couponRepo         repository.CouponRepositoryInterface
	courseRepo         repository.CourseRepositoryInterface
	enrollmentRepo     repository.EnrollmentRepositoryInterface
	cartRepo           repository.CartItemRepositoryInterface
	orderHistoryRepo   repository.OrderStatusHistoryRepositoryInterface
	idempotencyKeyRepo repository.IdempotencyKeyRepositoryInterface
}

func NewOrderService(
	orderRepo repository.OrderRepositoryInterface,
	orderItemRepo repository.OrderItemRepositoryInterface,
	couponRepo repository.CouponRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	cartRepo repository.CartItemRepositoryInterface,
	orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface,
	idempotencyKeyRepo repository.IdempotencyKeyRepositoryInterface,
) *OrderService {
	return &OrderService{
		orderRepo:          orderRepo,
		orderItemRepo:      orderItemRepo,
		couponRepo:         couponRepo,
		courseRepo:         courseRepo,
		enrollmentRepo:     enrollmentRepo,
		cartRepo:           cartRepo,
		orderHistoryRepo:   orderHistoryRepo,
		idempotencyKeyRepo: idempotencyKeyRepo,
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

func (s *OrderService) CreateOrder(ctx context.Context, userID uuid.UUID, req dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	var courseIDs []uuid.UUID

	if req.Source == "cart" {
		cartItems, err := s.cartRepo.GetByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}

		courseIDs = make([]uuid.UUID, 0, len(cartItems))
		for _, item := range cartItems {
			courseIDs = append(courseIDs, item.CourseID)
		}
	} else if req.Source == "buy_now" {
		for _, idStr := range req.CourseIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				return nil, errors.New("invalid course id")
			}
			courseIDs = append(courseIDs, id)
		}
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

	// Calculate subtotal
	subtotal := decimal.Zero
	for _, course := range courses {
		price := course.Price
		subtotal = subtotal.Add(price)
	}

	var coupon *model.Coupon
	discountAmount := decimal.Zero

	if req.CouponCode != "" {
		coupon, discountAmount, err = s.couponRepo.ValidateCoupon(req.CouponCode, userID, courseIDs, subtotal)
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
		Status:         "pending",
		CouponID:       nil,
		Notes:          nil,
	}

	if coupon != nil {
		order.CouponID = &coupon.ID
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
	err = s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		if err := txRepo.Create(order); err != nil {
			return err
		}

		if err := s.orderItemRepo.CreateBatch(items); err != nil {
			return err
		}

		// Create status history
		history := &model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  time.Now(),
			OrderID:    order.ID,
			FromStatus: "",
			ToStatus:   "pending",
			Reason:     "Order created",
		}
		if err := s.orderHistoryRepo.Create(history); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if req.Source == "cart" {
		if err := s.cartRepo.DeleteByUserID(ctx, userID); err != nil {
			return nil, err
		}
	}

	return s.toOrderResponse(order, items), nil
}

// GetOrderByID - Get order by ID
func (s *OrderService) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*dto.OrderResponse, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
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

func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID uuid.UUID, reason string) error {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	if order.UserID != userID {
		return errors.New("unauthorized")
	}

	if !s.isValidTransition(order.Status, "cancelled") {
		return ErrInvalidStateTransition
	}

	oldStatus := order.Status

	return s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		// Update status
		if err := txRepo.UpdateStatusWithTx(&gorm.DB{}, orderID, "cancelled"); err != nil {
			return err
		}

		// Create history
		history := &model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  time.Now(),
			OrderID:    orderID,
			FromStatus: oldStatus,
			ToStatus:   "cancelled",
			Reason:     reason,
		}
		return s.orderHistoryRepo.Create(history)
	})
}

// CompleteOrder - Complete order after successful payment
func (s *OrderService) CompleteOrder(ctx context.Context, orderID uuid.UUID, paymentMethod, transactionID string) error {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	if order.Status != "processing" {
		return ErrInvalidStateTransition
	}

	oldStatus := order.Status

	return s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		// Update payment info
		if err := txRepo.UpdatePaymentInfo(orderID, paymentMethod, "internal", transactionID, time.Now()); err != nil {
			return err
		}

		// Create history
		history := &model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  time.Now(),
			OrderID:    orderID,
			FromStatus: oldStatus,
			ToStatus:   "completed",
			Reason:     "Payment completed",
		}
		if err := s.orderHistoryRepo.Create(history); err != nil {
			return err
		}

		// Create enrollments for each course
		items, err := s.orderItemRepo.GetByOrderID(orderID)
		if err != nil {
			return err
		}

		for _, item := range items {
			// Check if already enrolled
			existingEnrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, order.UserID, item.CourseID)
			if err == nil && existingEnrollment != nil {
				continue // Already enrolled, skip
			}

			// Create enrollment - Enrollment uses BaseModel which has ID
			enrollment := &model.Enrollment{
				UserID:     order.UserID,
				CourseID:   item.CourseID,
				EnrolledAt: time.Now(),
			}
			if err := s.enrollmentRepo.Create(ctx, enrollment); err != nil {
				return err
			}
		}

		// Update coupon usage if applicable
		if order.CouponID != nil {
			if err := s.couponRepo.IncrementUsageCount(*order.CouponID); err != nil {
				return err
			}

			usage := &model.CouponUsage{
				ID:             uuid.New(),
				CreatedAt:      time.Now(),
				CouponID:       *order.CouponID,
				UserID:         order.UserID,
				OrderID:        orderID,
				DiscountAmount: order.DiscountAmount,
			}
			if err := s.couponRepo.CreateUsage(usage); err != nil {
				return err
			}
		}

		return nil
	})
}

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
