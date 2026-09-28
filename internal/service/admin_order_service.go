package service

import (
	"context"
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

// Sentinel errors cho luồng hoàn tiền admin (POST /orders/admin/:id/refund) — tách khỏi
// ErrInvalidStateTransition dùng chung cho CancelOrder vì handler cần phân biệt 400 (chưa thanh
// toán, không hoàn được) và 409 (đã hoàn rồi, idempotency) theo đúng contract phase-02.
var (
	ErrOrderNotRefundable   = errors.New("only completed orders can be refunded")
	ErrOrderAlreadyRefunded = errors.New("order already refunded")
)

// AdminOrderServiceInterface — xem context đầy đủ ở phase-02-orders-refund.md (đơn hàng admin +
// hoàn tiền + báo cáo doanh thu nền tảng thật).
type AdminOrderServiceInterface interface {
	ListOrders(ctx context.Context, filter repository.AdminOrderFilter) (*dto.AdminOrderListResponse, error)
	RefundOrder(ctx context.Context, actorID, orderID uuid.UUID, reason, refundMethod string) (*dto.RefundOrderResponse, error)
	GetRevenueReport(ctx context.Context, from, to time.Time) (*dto.RevenueReportResponse, error)
}

type AdminOrderService struct {
	orderRepo      repository.OrderRepositoryInterface
	orderItemRepo  repository.OrderItemRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
}

func NewAdminOrderService(
	orderRepo repository.OrderRepositoryInterface,
	orderItemRepo repository.OrderItemRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
) *AdminOrderService {
	return &AdminOrderService{
		orderRepo:      orderRepo,
		orderItemRepo:  orderItemRepo,
		enrollmentRepo: enrollmentRepo,
		courseRepo:     courseRepo,
	}
}

// ListOrders - GET /api/orders/admin.
func (s *AdminOrderService) ListOrders(ctx context.Context, filter repository.AdminOrderFilter) (*dto.AdminOrderListResponse, error) {
	orders, total, err := s.orderRepo.ListAdmin(filter)
	if err != nil {
		return nil, err
	}

	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	items := make([]dto.AdminOrderListItem, 0, len(orders))
	for _, order := range orders {
		itemBriefs := make([]dto.AdminOrderItemBrief, 0, len(order.Items))
		for _, it := range order.Items {
			itemBriefs = append(itemBriefs, dto.AdminOrderItemBrief{
				CourseID:    it.CourseID,
				CourseTitle: it.Course.Title,
				FinalPrice:  it.FinalPrice,
			})
		}
		items = append(items, dto.AdminOrderListItem{
			ID:            order.ID,
			OrderNumber:   order.OrderNumber,
			UserID:        order.UserID,
			UserEmail:     order.User.Email,
			TotalAmount:   order.TotalAmount,
			Currency:      order.Currency,
			Status:        order.Status,
			PaymentMethod: order.PaymentMethod,
			CreatedAt:     order.CreatedAt,
			PaidAt:        order.PaidAt,
			Items:         itemBriefs,
		})
	}

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	return &dto.AdminOrderListResponse{
		Items:      items,
		TotalCount: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// RefundOrder - POST /api/orders/admin/:id/refund.
//
// Hoàn tiền = admin xác nhận ĐÃ chuyển khoản thủ công NGOÀI hệ thống (quyết định chủ dự án #1) —
// hàm này KHÔNG chuyển tiền, chỉ ghi nhận đã hoàn + thu hồi ghi danh + trừ doanh thu giáo viên
// (tự động, vì WalletRepository.GetTeacherEarnings chỉ SUM đơn status='completed' — đơn vừa
// refunded rơi khỏi tổng, không cần bút toán trừ riêng).
//
// Khoá + double-check TRÁNH TOCTOU (phase-02 contract, khác kiểu với releaseOrderAndTransition
// dùng cho CancelOrder): GetForUpdate khoá dòng NGAY ĐẦU transaction (SELECT ... FOR UPDATE) —
// 2 request hoàn cùng lúc cho CÙNG đơn tuần tự hoá qua khoá hàng: request thắng commit trước,
// request thua chờ khoá nhả rồi đọc lại status ĐÃ LÀ "refunded" -> ErrOrderAlreadyRefunded (409),
// không có khoảng hở đọc-trước-ghi-sau giữa 2 transaction như UPDATE-có-điều-kiện-không-khoá.
func (s *AdminOrderService) RefundOrder(ctx context.Context, actorID, orderID uuid.UUID, reason, refundMethod string) (*dto.RefundOrderResponse, error) {
	var result *dto.RefundOrderResponse

	err := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		txDB := txRepo.TxDB()

		order, err := txRepo.GetForUpdate(txDB, orderID)
		if err != nil {
			if errors.Is(err, repository.ErrOrderNotFound) {
				return ErrOrderNotFound
			}
			return err
		}

		if order.Status == "refunded" {
			return ErrOrderAlreadyRefunded
		}
		if order.Status != "completed" {
			return ErrOrderNotRefundable
		}

		// Phase 4: khoá hồ sơ các giảng viên của đơn để hoàn tiền không chen giữa lúc admin đang
		// duyệt/đánh dấu đã chuyển 1 yêu cầu rút (xem WithdrawalService.transition).
		if err := repository.LockTeacherProfilesOfOrder(ctx, txDB, order.ID); err != nil {
			return err
		}

		now := time.Now()
		applied, err := txRepo.RefundOrder(order.ID, reason, refundMethod, now, actorID)
		if err != nil {
			return err
		}
		if !applied {
			return ErrOrderAlreadyRefunded
		}

		history := &model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  now,
			OrderID:    order.ID,
			FromStatus: "completed",
			ToStatus:   "refunded",
			Reason:     fmt.Sprintf("Refund by admin %s (method=%s): %s", actorID, refundMethod, reason),
		}
		if err := repository.NewOrderStatusHistoryRepository(txDB).Create(history); err != nil {
			return err
		}

		if err := s.revokeEnrollmentsForRefund(ctx, txDB, order); err != nil {
			return err
		}

		result = &dto.RefundOrderResponse{ID: order.ID, Status: "refunded", RefundedAt: now}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// revokeEnrollmentsForRefund thu hồi enrollment cho từng khoá trong đơn — CHỈ khi đây là đơn
// HOÀN TẤT DUY NHẤT còn lại của (user, course) đó. Enrollment KHÔNG mang OrderID (một enrollment
// có thể "sống nhờ" nhiều đơn nếu user từng mua đi mua lại cùng khoá — completeOrderFulfillment
// bỏ qua nếu đã enroll active), nên không thể tra "đơn nào tạo ra enrollment này" trực tiếp;
// thay vào đó suy luận ĐÚNG bằng cách đếm CÁC ĐƠN "completed" KHÁC (order.ID khác) của cùng
// user+course NGAY TRONG transaction đang khoá đơn hiện tại — còn ít nhất 1 đơn "completed" khác
// thì khoá học đó vẫn "sống", không đụng enrollment; ngược lại mới soft-delete + giảm
// total_students đối xứng với lúc tạo (completeOrderFulfillment).
func (s *AdminOrderService) revokeEnrollmentsForRefund(ctx context.Context, txDB *gorm.DB, order *model.Order) error {
	items, err := repository.NewOrderItemRepository(txDB).GetByOrderID(order.ID)
	if err != nil {
		return err
	}

	enrollmentRepoTx := repository.NewEnrollmentRepository(txDB)
	courseRepoTx := repository.NewCourseRepository(txDB)

	for _, item := range items {
		var otherCompleted int64
		if err := txDB.Model(&model.Order{}).
			Joins("JOIN order_items oi ON oi.order_id = orders.id").
			Where("orders.user_id = ? AND oi.course_id = ? AND orders.status = 'completed' AND orders.id <> ?",
				order.UserID, item.CourseID, order.ID).
			Count(&otherCompleted).Error; err != nil {
			return err
		}
		if otherCompleted > 0 {
			continue // course còn "sống" nhờ đơn hoàn tất KHÁC — không đụng enrollment (item 24/26 tinh thần).
		}

		enrollment, err := enrollmentRepoTx.GetByUserAndCourse(ctx, order.UserID, item.CourseID)
		if err != nil {
			return err
		}
		if enrollment == nil {
			continue // chưa/không còn enrollment active — không có gì để thu hồi.
		}
		if err := enrollmentRepoTx.Delete(ctx, enrollment.ID); err != nil {
			return err
		}
		if err := courseRepoTx.IncrementTotalStudents(ctx, item.CourseID, -1); err != nil {
			return err
		}
	}
	return nil
}

// GetRevenueReport - GET /api/admin/reports/revenue. Đọc trực tiếp (không mở transaction — thao
// tác chỉ ĐỌC, không có side-effect ghi nào cần tính nguyên tử) qua orderRepo.TxDB(), vốn trả về
// đúng *gorm.DB kết nối gốc khi gọi trên repo KHÔNG nằm trong closure WithTransaction (xem
// OrderRepository.TxDB()).
func (s *AdminOrderService) GetRevenueReport(ctx context.Context, from, to time.Time) (*dto.RevenueReportResponse, error) {
	db := s.orderRepo.TxDB().WithContext(ctx)

	type aggRow struct {
		GrossRevenue      decimal.Decimal
		RefundAmount      decimal.Decimal
		PlatformFeeAmount decimal.Decimal
		CompletedCount    int64
		RefundedCount     int64
		FailedCount       int64
		CancelledCount    int64
		ExpiredCount      int64
		PendingCount      int64
		ProcessingCount   int64
		TransactionCount  int64
	}
	var agg aggRow
	if err := db.Model(&model.Order{}).
		Where("created_at BETWEEN ? AND ?", from, to).
		Select(`
			COALESCE(SUM(total_amount) FILTER (WHERE status = 'completed'), 0) AS gross_revenue,
			COALESCE(SUM(total_amount) FILTER (WHERE status = 'refunded'), 0) AS refund_amount,
			COALESCE(SUM(platform_fee_amount) FILTER (WHERE status = 'completed'), 0) AS platform_fee_amount,
			COUNT(*) FILTER (WHERE status = 'completed') AS completed_count,
			COUNT(*) FILTER (WHERE status = 'refunded') AS refunded_count,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
			COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled_count,
			COUNT(*) FILTER (WHERE status = 'expired') AS expired_count,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
			COUNT(*) FILTER (WHERE status = 'processing') AS processing_count,
			COUNT(*) AS transaction_count
		`).
		Scan(&agg).Error; err != nil {
		return nil, err
	}

	report := &dto.RevenueReportResponse{
		GrossRevenue:       agg.GrossRevenue,
		RefundAmount:       agg.RefundAmount,
		NetRevenue:         agg.GrossRevenue.Sub(agg.RefundAmount),
		PlatformFeeAmount:  agg.PlatformFeeAmount,
		TeacherShareAmount: agg.GrossRevenue.Sub(agg.PlatformFeeAmount),
		TransactionCount:   agg.TransactionCount,
		CompletedCount:     agg.CompletedCount,
		RefundedCount:      agg.RefundedCount,
		Currency:           "VND",
		ByStatus: map[string]int64{
			"pending":    agg.PendingCount,
			"processing": agg.ProcessingCount,
			"completed":  agg.CompletedCount,
			"failed":     agg.FailedCount,
			"refunded":   agg.RefundedCount,
			"cancelled":  agg.CancelledCount,
			"expired":    agg.ExpiredCount,
		},
	}

	report.SuccessRate = calculateSuccessRate(agg.CompletedCount, agg.FailedCount, agg.CancelledCount, agg.ExpiredCount)

	return report, nil
}

// calculateSuccessRate (phase-02 contract) — tách thành hàm THUẦN (không side-effect, không DB)
// để unit test độc lập, cùng pattern shouldAlertFulfillmentFailure (payment_service.go):
// completed / (completed+failed+cancelled+expired) * 100 — loại pending/processing khỏi mẫu số
// (đơn CHƯA XONG không tính thành/bại). KHÔNG đánh đồng cancelled với refunded (lỗi đang có ở FE
// cũ, xem context link phase-02) — refunded cũng KHÔNG nằm trong mẫu số này (đơn refunded từng
// "completed" thật sự, tính riêng ở refund_amount, không lẫn vào success_rate của kỳ báo cáo).
// Mẫu số rỗng (chưa có đơn nào xong/thất bại trong kỳ) -> 0, không chia cho 0.
func calculateSuccessRate(completed, failed, cancelled, expired int64) float64 {
	denom := completed + failed + cancelled + expired
	if denom == 0 {
		return 0
	}
	return float64(completed) / float64(denom) * 100
}
