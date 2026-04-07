package queue

import (
	"context"
	"log"
	"time"
)

//
// ===== WORKER INTERFACES =====
//

// Worker - Interface chung cho tất cả workers
// Mọi worker implementation phải implement interface này
type Worker interface {
	Start(ctx context.Context) error
	Stop() error
	HealthCheck() HealthCheckResult
}

// HealthCheckResult - Kết quả health check của worker
type HealthCheckResult struct {
	Running     bool      `json:"running"`       // Worker đang chạy hay không
	QueueName   string    `json:"queue_name"`    // Tên queue đang consume
	RetryCount  int64     `json:"retry_count"`   // Số lần retry đã thực hiện
	LastError   string    `json:"last_error"`    // Error cuối cùng (nếu có)
	LastCheckAt time.Time `json:"last_check_at"` // Thời gian check
}
type MessageHandler func(ctx context.Context, messageData []byte) error

// WorkerConfig - Cấu hình cho worker
type WorkerConfig struct {
	QueueName    string        // Tên queue cần consume
	MaxRetries   int           // Số lần retry tối đa (default: 3)
	RetryBackoff time.Duration // Thời gian chờ giữa các retry (default: 5s)
}

//
// ===== BASE WORKER IMPLEMENTATION =====
//

// BaseWorker - Struct chứa ALL queue orchestration logic
// Responsibilities:
type BaseWorker struct {
	queueService *RabbitMQService
	queueName    string
	handler      MessageHandler // Callback để process message
	running      bool
	stopChan     chan struct{}

	// Retry configuration
	maxRetries   int
	retryBackoff time.Duration

	// Metrics
	retryCount int64
	lastError  string
}

// NewBaseWorker - Tạo BaseWorker mới
// queueService: RabbitMQ service để consume messages
// config: Cấu hình worker (queue name, retry settings)
// handler: Callback function xử lý business logic
func NewBaseWorker(
	queueService *RabbitMQService,
	config WorkerConfig,
	handler MessageHandler,
) *BaseWorker {
	// Set defaults nếu không có config
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBackoff == 0 {
		config.RetryBackoff = 5 * time.Second
	}

	return &BaseWorker{
		queueService: queueService,
		queueName:    config.QueueName,
		handler:      handler,
		running:      false,
		stopChan:     make(chan struct{}),
		maxRetries:   config.MaxRetries,
		retryBackoff: config.RetryBackoff,
		retryCount:   0,
		lastError:    "",
	}
}

// Start - Bắt đầu consume messages từ queue
// Implement Worker interface
func (w *BaseWorker) Start(ctx context.Context) error {
	if w.running {
		return nil
	}

	log.Printf("[BaseWorker] Starting worker for queue: %s", w.queueName)
	w.running = true

	// Start consuming messages với handler wrapper
	err := w.queueService.ConsumeMessages(ctx, w.queueName, w.processMessage)
	if err != nil {
		w.running = false
		return err
	}

	go func() {
		<-w.stopChan
		w.running = false
		log.Printf("[BaseWorker] Worker stopped for queue: %s", w.queueName)
	}()

	log.Printf("[BaseWorker] Worker started successfully for queue: %s", w.queueName)
	return nil
}

// Stop - Dừng worker gracefully
// Implement Worker interface
func (w *BaseWorker) Stop() error {
	if !w.running {
		return nil
	}

	log.Printf("[BaseWorker] Stopping worker for queue: %s", w.queueName)
	close(w.stopChan)
	return nil
}

// HealthCheck - Kiểm tra trạng thái worker
// Implement Worker interface
func (w *BaseWorker) HealthCheck() HealthCheckResult {
	return HealthCheckResult{
		Running:     w.running,
		QueueName:   w.queueName,
		RetryCount:  w.retryCount,
		LastError:   w.lastError,
		LastCheckAt: time.Now(),
	}
}

// processMessage - Internal method xử lý message với retry logic
// Đây là nơi tập trung ALL retry/DLQ handling
func (w *BaseWorker) processMessage(ctx context.Context, messageData []byte) error {
	var lastErr error

	// Retry loop với exponential backoff
	for attempt := 0; attempt <= w.maxRetries; attempt++ {
		if attempt > 0 {
			backoffDuration := time.Duration(attempt) * w.retryBackoff
			log.Printf("[BaseWorker] Retrying message (attempt %d/%d) after %v...",
				attempt, w.maxRetries, backoffDuration)
			time.Sleep(backoffDuration)
			w.retryCount++
		}

		// Gọi handler để xử lý message
		err := w.handler(ctx, messageData)
		if err == nil {
			// Success - return ngay
			return nil
		}

		// Log error và chuẩn bị retry
		lastErr = err
		w.lastError = err.Error()
		log.Printf("[BaseWorker] Message processing failed (attempt %d/%d): %v",
			attempt+1, w.maxRetries+1, err)
	}

	// Đã hết retry, message sẽ bị reject và đi vào DLQ (nếu có)
	log.Printf("[BaseWorker] Message failed after %d attempts. Moving to DLQ. Error: %v",
		w.maxRetries+1, lastErr)

	return lastErr
}