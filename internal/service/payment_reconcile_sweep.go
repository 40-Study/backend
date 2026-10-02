package service

// payment_reconcile_sweep.go — L6 mục 4 và 5.
//
// Trước đây đối chiếu ngân hàng chỉ chạy khi có người gọi endpoint kiểm thanh toán (L1 review MINOR
// 5): khách chuyển hai lần rồi đóng trình duyệt thì khoản thừa không bao giờ được thấy, và đơn đã
// hoàn tất không bao giờ tra lại ngân hàng. Job nền (asynq, chu kỳ lấy từ config) gọi lại ĐÚNG luồng
// đối chiếu sẵn có cho:
//   - đơn đang chờ (processing) trong cửa sổ reconcileProcessingWindow: hoàn tất / chốt hết hạn / gắn
//     cờ hoàn tiền như khi người dùng bấm kiểm tra;
//   - đơn vừa hoàn tất (completed) trong reconcileCompletedWindow: khoản chuyển THÊM vào cùng mã sau
//     khi đơn đã hoàn tất được gắn cờ cần hoàn tiền.
//
// Idempotent vì mọi bước ghi đã idempotent theo mã giao dịch (bank_transaction_usages, dedupe cờ
// payment_after_expiry). Không chạy chồng: khoá trong tiến trình (TryLock) cộng khoá Redis cho nhiều
// instance (asynq scheduler đăng ký cùng cron ở mỗi instance nên mỗi chu kỳ sinh nhiều task).

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
)

const (
	// reconcileProcessingWindow — đơn chờ cũ hơn mốc này không quét nữa. Đơn chờ sống tối đa khoảng
	// 24h giữ đơn + 24h hạn mã + 24h chốt "chưa xác minh" (~72h) nên 4 ngày là dư, đơn kẹt lâu hơn vẫn
	// đối chiếu được khi người dùng/admin gọi tay.
	reconcileProcessingWindow = 4 * 24 * time.Hour
	// reconcileCompletedWindow — đơn hoàn tất được tra lại để bắt khoản chuyển dư. Khách chuyển dư
	// thường làm ngay sau lần đầu; cửa sổ ngắn để không gọi ngân hàng (mbbank) hàng trăm lần mỗi đơn.
	reconcileCompletedWindow = 6 * time.Hour
	// reconcileMaxLookupsPerSweep — trần số đơn mỗi lượt, để một lượt không kéo dài quá chu kỳ khi có
	// bất thường (mỗi lần tra cứu tối đa bankLookupTimeout).
	reconcileMaxLookupsPerSweep = 150

	// reconcileMaxConsecutiveBankErrors — số lần tra ngân hàng lỗi LIÊN TIẾP thì dừng lượt quét. Python/ngân hàng
	// chết thì mỗi lời gọi chờ tới bankLookupTimeout (10s) rồi lỗi: không dừng thì lượt quét kéo dài tới
	// reconcileSweepTimeout và mỗi chu kỳ lặp lại vô ích. Chu kỳ sau thử lại từ đầu.
	reconcileMaxConsecutiveBankErrors = 3

	reconcileLockKey = "payment:reconcile-sweep:lock"
	// reconcileLockTTL — khoá tự hết hạn để tiến trình chết giữa chừng không chặn mãi.
	reconcileLockTTL = 10 * time.Minute
	// reconcileSweepTimeout — một lượt phải xong trước khi khoá hết hạn.
	reconcileSweepTimeout = reconcileLockTTL - time.Minute
	// defaultReconcileSweepInterval — chu kỳ khi không cấu hình (SetSweepInterval).
	defaultReconcileSweepInterval = 10 * time.Minute
)

// errBankLookupFailed — lần tra ngân hàng không cho kết quả tin được (gRPC lỗi, timeout, Python báo error).
var errBankLookupFailed = errors.New("bank lookup failed")

// SweepLocker — khoá phân tán cho job quét. TryLock trả ok=false khi nơi khác đang giữ khoá.
type SweepLocker interface {
	TryLock(ctx context.Context, key string, ttl time.Duration) (release func(), ok bool, err error)
}

// RedisSweepLocker — SweepLocker bằng Redis SET NX, nhả khoá chỉ khi còn là chủ (so token).
type RedisSweepLocker struct{ rdb *redis.Client }

func NewRedisSweepLocker(rdb *redis.Client) *RedisSweepLocker { return &RedisSweepLocker{rdb: rdb} }

var releaseIfOwnerScript = redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)

func (l *RedisSweepLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	if l == nil || l.rdb == nil {
		return nil, false, errors.New("redis client unavailable")
	}
	token := uuid.NewString()
	ok, err := l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	return func() {
		// ctx của lượt quét có thể đã hết hạn lúc nhả: dùng ctx riêng.
		relCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = releaseIfOwnerScript.Run(relCtx, l.rdb, []string{key}, token).Err()
	}, true, nil
}

// SetSweepLocker gắn khoá phân tán cho RunReconcileSweep. Không gắn thì chỉ có khoá trong tiến trình.
func (s *PaymentService) SetSweepLocker(l SweepLocker) { s.sweepLocker = l }

// SetSweepInterval cho biết chu kỳ của job (cùng giá trị đăng ký asynq) để khoá phân tán theo khung chu kỳ.
func (s *PaymentService) SetSweepInterval(d time.Duration) { s.sweepInterval = d }

// ReconcileSweepResult — số liệu một lượt quét (để log và test).
type ReconcileSweepResult struct {
	Processing int // đơn chờ đã đối chiếu
	Completed  int // đơn hoàn tất đã tra lại
	// CompletedWithoutCode — đơn hoàn tất trong cửa sổ nhưng không còn mã thanh toán (hoàn tất trước khi có
	// cột payment_code): không tra được nên bỏ qua.
	CompletedWithoutCode int
	ExtraFlagged         int // khoản chuyển dư mới được gắn cờ ở đơn hoàn tất
	// Errors — số lần đối chiếu lỗi, gồm cả ngân hàng không trả lời (BankErrors) và lỗi DB.
	Errors     int
	BankErrors int
	// BankDown — lượt quét dừng sớm vì ngân hàng lỗi reconcileMaxConsecutiveBankErrors lần liên tiếp.
	BankDown         bool
	LimitReached     bool
	SkippedNoBankSvc bool
}

// sweepSlotKey — khoá theo KHUNG chu kỳ: mọi instance trong cùng một khung chu kỳ cho cùng một key nên chỉ
// instance đến trước quét. asynq scheduler đăng ký cron ở mỗi instance nên mỗi chu kỳ sinh nhiều task, và
// khoá "đang chạy" (nhả khi xong) không chặn được task đến SAU khi lượt đầu đã xong.
func sweepSlotKey(now time.Time, interval time.Duration) string {
	return fmt.Sprintf("%s:slot:%d", reconcileLockKey, now.Unix()/int64(interval.Seconds()))
}

// RunReconcileSweep — điểm vào của job nền: khoá chống chạy chồng rồi ReconcileSweep. Khoá đang bị
// giữ thì bỏ qua lượt này (không phải lỗi).
func (s *PaymentService) RunReconcileSweep(ctx context.Context) error {
	if !s.sweepMu.TryLock() {
		log.Printf("[PAYMENT-SWEEP] bỏ qua: lượt trước trong tiến trình này còn chạy")
		return nil
	}
	defer s.sweepMu.Unlock()

	if s.sweepLocker != nil {
		interval := s.sweepInterval
		if interval < time.Minute {
			interval = defaultReconcileSweepInterval
		}
		// Khoá khung chu kỳ: KHÔNG nhả khi xong (tự hết hạn cùng khung), nên task đến sau trong cùng chu kỳ bỏ qua.
		if _, ok, err := s.sweepLocker.TryLock(ctx, sweepSlotKey(time.Now(), interval), interval); err != nil {
			return fmt.Errorf("payment reconcile sweep slot lock: %w", err)
		} else if !ok {
			log.Printf("[PAYMENT-SWEEP] bỏ qua: chu kỳ này đã được một instance quét")
			return nil
		}
		release, ok, err := s.sweepLocker.TryLock(ctx, reconcileLockKey, reconcileLockTTL)
		if err != nil {
			return fmt.Errorf("payment reconcile sweep lock: %w", err)
		}
		if !ok {
			log.Printf("[PAYMENT-SWEEP] bỏ qua: instance khác đang quét")
			return nil
		}
		defer release()
	}

	sweepCtx, cancel := context.WithTimeout(ctx, reconcileSweepTimeout)
	defer cancel()
	res, err := s.ReconcileSweep(sweepCtx, time.Now())
	log.Printf("[PAYMENT-SWEEP] xong: đơn chờ=%d đơn hoàn tất=%d (không còn mã=%d) khoản dư mới=%d lỗi=%d (ngân hàng=%d) chạm trần=%t err=%v",
		res.Processing, res.Completed, res.CompletedWithoutCode, res.ExtraFlagged, res.Errors, res.BankErrors, res.LimitReached, err)
	if res.BankDown {
		log.Printf("[PAYMENT-SWEEP] CẢNH BÁO: ngân hàng/dịch vụ giao dịch lỗi %d lần liên tiếp, lượt quét dừng sớm; chu kỳ sau sẽ thử lại", reconcileMaxConsecutiveBankErrors)
	}
	return err
}

// ReconcileSweep quét một lượt tại thời điểm now. Lỗi từng đơn chỉ được đếm và log: một đơn hỏng
// không được chặn các đơn còn lại.
func (s *PaymentService) ReconcileSweep(ctx context.Context, now time.Time) (ReconcileSweepResult, error) {
	var res ReconcileSweepResult
	if s.transactionService == nil {
		res.SkippedNoBankSvc = true
		return res, nil
	}
	db := s.orderRepo.TxDB()
	if db == nil {
		return res, errors.New("order repository has no database handle")
	}

	var processing []model.Order
	if err := db.WithContext(ctx).
		Where("status = ? AND payment_transaction_id IS NOT NULL AND payment_transaction_id <> '' AND created_at > ?",
			"processing", now.Add(-reconcileProcessingWindow)).
		Order("created_at ASC").Limit(reconcileMaxLookupsPerSweep).Find(&processing).Error; err != nil {
		return res, fmt.Errorf("list processing orders: %w", err)
	}
	// Đơn hoàn tất phải tra bằng MÃ THANH TOÁN (payment_code): payment_transaction_id của chúng là mã giao dịch
	// ngân hàng, không nằm trong nội dung chuyển khoản nên ngân hàng luôn trả not_found.
	var completed []model.Order
	completedWindow := now.Add(-reconcileCompletedWindow)
	if err := db.WithContext(ctx).
		Where("status = ? AND payment_code IS NOT NULL AND payment_code <> '' AND paid_at > ?", "completed", completedWindow).
		Order("paid_at ASC").Limit(reconcileMaxLookupsPerSweep).Find(&completed).Error; err != nil {
		return res, fmt.Errorf("list completed orders: %w", err)
	}
	var lost int64
	if err := db.WithContext(ctx).Model(&model.Order{}).
		Where("status = ? AND (payment_code IS NULL OR payment_code = '') AND paid_at > ?", "completed", completedWindow).
		Count(&lost).Error; err == nil && lost > 0 {
		res.CompletedWithoutCode = int(lost)
		log.Printf("[PAYMENT-SWEEP] %d đơn hoàn tất trong cửa sổ không còn mã thanh toán (hoàn tất trước khi có payment_code): không tra lại được, bỏ qua", lost)
	}

	// bankFailed ghi nhận một lần tra ngân hàng lỗi và trả true khi đã tới ngưỡng ngắt mạch.
	consecutiveBankErrors := 0
	bankFailed := func() bool {
		res.Errors++
		res.BankErrors++
		consecutiveBankErrors++
		if consecutiveBankErrors >= reconcileMaxConsecutiveBankErrors {
			res.BankDown = true
			return true
		}
		return false
	}

	budget := reconcileMaxLookupsPerSweep
	for i := range processing {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if budget == 0 {
			res.LimitReached = true
			return res, nil
		}
		budget--
		o := &processing[i]
		// Đúng luồng người dùng bấm "kiểm tra thanh toán" (hoàn tất, chốt hết hạn, gắn cờ...), quyền
		// admin vì đây là tác vụ hệ thống.
		resp, err := s.CheckAndProcessPayment(ctx, o.ID, o.UserID, true)
		res.Processing++
		switch {
		case resp != nil && resp.BankUnavailable:
			// Ngân hàng lỗi KHÔNG trả error mà trả BankUnavailable: phải đếm ở đây.
			if bankFailed() {
				return res, nil
			}
		case err != nil && !isBenignSweepError(err):
			res.Errors++
			log.Printf("[PAYMENT-SWEEP] order=%s đối chiếu lỗi: %v", o.ID, err)
		default:
			consecutiveBankErrors = 0
		}
	}
	for i := range completed {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if budget == 0 {
			res.LimitReached = true
			return res, nil
		}
		budget--
		flagged, err := s.reconcileCompletedExtras(ctx, &completed[i])
		res.ExtraFlagged += flagged
		res.Completed++
		switch {
		case errors.Is(err, errBankLookupFailed):
			log.Printf("[PAYMENT-SWEEP] order=%s tra lại đơn hoàn tất lỗi: %v", completed[i].ID, err)
			if bankFailed() {
				return res, nil
			}
		case err != nil:
			res.Errors++
			log.Printf("[PAYMENT-SWEEP] order=%s tra lại đơn hoàn tất lỗi: %v", completed[i].ID, err)
		default:
			consecutiveBankErrors = 0
		}
	}
	return res, nil
}

// isBenignSweepError — kết quả đối chiếu bình thường, không phải lỗi của job: đơn vừa hoàn tất ở
// request khác, hoặc khách chuyển sai số tiền khi mã còn hạn (đơn giữ nguyên chờ chuyển đúng).
func isBenignSweepError(err error) bool {
	return errors.Is(err, ErrPaymentAlreadyDone) || errors.Is(err, ErrPaymentAmountMismatch)
}

// flagExtraTransfers gắn cờ cần hoàn tiền cho mọi giao dịch trong txs khác giao dịch đã dùng để hoàn
// tất đơn. Trả số khoản vừa được gắn cờ mới (dedupe theo mã giao dịch nằm trong recordRefundFlag).
func (s *PaymentService) flagExtraTransfers(ctx context.Context, order *model.Order, usedTxID string, txs []*grpc.CheckTransactionResult) int {
	completed := *order
	completed.Status = "completed" // recordRefundFlag ghi trạng thái đơn vào history/log
	flagged := 0
	for _, other := range txs {
		if other.TransactionID == usedTxID {
			continue
		}
		if s.recordRefundFlag(ctx, &completed, other) {
			flagged++
			log.Printf("[PAYMENT-EXTRA-TRANSFER] order=%s dùng tx=%s, giao dịch thừa tx=%s amount=%s cùng mã: đã bật cờ cần hoàn tiền", order.ID, usedTxID, other.TransactionID, other.Amount)
		}
	}
	return flagged
}

// reconcileCompletedExtras tra lại ngân hàng cho một đơn ĐÃ hoàn tất: giao dịch khớp mã mà đơn không
// dùng (không có trong bank_transaction_usages của đơn này) là khoản chuyển thêm → cờ cần hoàn tiền.
// Trả số khoản vừa gắn cờ mới.
//
// An toàn: đơn hoàn tất không có bản ghi usage nào (đơn cũ trước M-06, hoặc hoàn tất bằng đường khác)
// thì KHÔNG biết giao dịch nào là khoản đã thanh toán, nên không gắn cờ gì — tránh bật cờ hoàn tiền
// nhầm cho chính khoản đã trả.
func (s *PaymentService) reconcileCompletedExtras(ctx context.Context, order *model.Order) (int, error) {
	// Mã thanh toán (không phải payment_transaction_id, xem paymentCodeOf): đơn mất mã thì không tra được.
	if order.Status != "completed" || paymentCodeOf(order) == "" {
		return 0, nil
	}
	var usedIDs []string
	if err := s.orderRepo.TxDB().WithContext(ctx).Model(&model.BankTransactionUsage{}).
		Where("reference_type = ? AND reference_id = ?", "order", order.ID).
		Pluck("bank_transaction_id", &usedIDs).Error; err != nil {
		return 0, fmt.Errorf("read bank transaction usages: %w", err)
	}
	if len(usedIDs) == 0 {
		log.Printf("[PAYMENT-SWEEP] order=%s hoàn tất nhưng không có bank_transaction_usage: không xác định được giao dịch thừa, bỏ qua", order.ID)
		return 0, nil
	}

	result, err := s.lookupBankTransaction(ctx, order, time.Now())
	if err != nil || result == nil || result.Status == "error" {
		return 0, fmt.Errorf("%w: %s", errBankLookupFailed, bankErrorDetail(err, result))
	}
	if !result.Found {
		return 0, nil
	}
	used := map[string]bool{}
	for _, id := range usedIDs {
		used[id] = true
	}
	completed := *order
	flagged := 0
	for _, tx := range transactionResults(result) {
		if tx.TransactionID == "" || used[tx.TransactionID] {
			continue
		}
		if s.recordRefundFlag(ctx, &completed, tx) {
			flagged++
			log.Printf("[PAYMENT-EXTRA-TRANSFER] order=%s đã hoàn tất, nhận thêm tx=%s amount=%s cùng mã: đã bật cờ cần hoàn tiền", order.ID, tx.TransactionID, tx.Amount)
		}
	}
	return flagged, nil
}
