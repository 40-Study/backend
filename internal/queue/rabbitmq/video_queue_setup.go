package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// Exchange names
	VideoExchange = "video"
	DLXExchange   = "video.dlx" // Dead Letter Exchange

	// Queue names
	VideoProcessingQueue = "video.processing"
	VideoResultQueue     = "video.results"
	VideoCleanupQueue    = "video.cleanup"
	VideoDeadLetterQueue = "video.dead_letters"

	// Routing keys
	VideoProcessingRoutingKey = "video.process"
	VideoResultRoutingKey     = "video.result"
	VideoCleanupRoutingKey    = "video.cleanup"

	// Queue arguments
	MessageTTL            = 24 * 60 * 60 * 1000 // 24 hours in milliseconds
	MaxRetries            = 3
	RetryDelay            = 30 * 1000 // 30 seconds in milliseconds
	ConsumerPrefetchCount = 10        // Number of unacked messages per consumer
	ProcessingTimeout     = 30 * 60   // 30 minutes processing timeout
)

// VideoQueueInterface - Interface cho video queue, giúp dễ mock trong unit test
type VideoQueueInterface interface {
	PublishVideoProcessingMessage(ctx context.Context, message VideoProcessingMessage) error
}

// VideoQueueSetup - Wrapper quản lý topology (exchange/queue/binding) và publish/consume
// cho video processing. Tái sử dụng connection của RabbitMQService thay vì tạo connection riêng.
type VideoQueueSetup struct {
	service *RabbitMQService
}

// NewVideoQueueSetup tạo VideoQueueSetup từ RabbitMQService đã có sẵn.
// Không tạo connection mới — dùng chung channel với RabbitMQService.
func NewVideoQueueSetup(service *RabbitMQService) *VideoQueueSetup {
	return &VideoQueueSetup{service: service}
}

// getChannel trả về channel hiện tại từ RabbitMQService (thread-safe qua RWMutex).
func (v *VideoQueueSetup) getChannel() *amqp.Channel {
	return v.service.getChannel()
}

// SetupVideoQueues khai báo toàn bộ topology cần thiết cho video processing trên RabbitMQ.
// Cần gọi 1 lần khi khởi động app. Bao gồm:
//   - Dead Letter Exchange (DLX): nhận message bị reject / hết TTL
//   - Video Exchange (topic): fanout message tới các queue theo routing key
//   - Dead Letter Queue: lưu message lỗi để debug
//   - Video Processing Queue: queue chính xử lý upload video (có DLX + TTL)
//   - Video Result Queue: nhận kết quả sau khi xử lý xong
//   - Video Cleanup Queue: nhận lệnh dọn dẹp file tạm / upload hết hạn
//   - QoS: giới hạn số message unacked mỗi consumer tránh quá tải
func (v *VideoQueueSetup) SetupVideoQueues() error {
	// 1. Declare Dead Letter Exchange (DLX) first
	err := v.getChannel().ExchangeDeclare(
		DLXExchange, // name
		"direct",    // type
		true,        // durable
		false,       // auto-delete
		false,       // internal
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare DLX exchange: %w", err)
	}

	// 2. Declare main Video Exchange
	err = v.getChannel().ExchangeDeclare(
		VideoExchange, // name
		"topic",       // type - using topic for flexible routing
		true,          // durable
		false,         // auto-delete
		false,         // internal
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare video exchange: %w", err)
	}

	// 3. Declare Dead Letter Queue
	_, err = v.getChannel().QueueDeclare(
		VideoDeadLetterQueue, // name
		true,                 // durable
		false,                // auto-delete
		false,                // exclusive
		false,                // no-wait
		amqp.Table{
			"x-message-ttl": int32(MessageTTL), // Messages expire after 24h
		}, // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare dead letter queue: %w", err)
	}

	// Bind DLQ to DLX
	err = v.getChannel().QueueBind(
		VideoDeadLetterQueue, // queue name
		"#",                  // routing key (all messages)
		DLXExchange,          // exchange
		false,                // no-wait
		nil,                  // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind dead letter queue: %w", err)
	}

	// 4. Declare Video Processing Queue with DLX configuration
	_, err = v.getChannel().QueueDeclare(
		VideoProcessingQueue, // name
		true,                 // durable
		false,                // auto-delete
		false,                // exclusive
		false,                // no-wait
		amqp.Table{
			"x-message-ttl":             int32(ProcessingTimeout * 1000), // Message TTL
			"x-dead-letter-exchange":    DLXExchange,                     // DLX
			"x-dead-letter-routing-key": "video.dead",                    // DLX routing key
			"x-max-retries":             int32(MaxRetries),               // Custom retry limit
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare video processing queue: %w", err)
	}

	// Bind processing queue to main exchange
	err = v.getChannel().QueueBind(
		VideoProcessingQueue,      // queue name
		VideoProcessingRoutingKey, // routing key
		VideoExchange,             // exchange
		false,                     // no-wait
		nil,                       // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind video processing queue: %w", err)
	}

	// 5. Declare Video Result Queue (for processing completion notifications)
	_, err = v.getChannel().QueueDeclare(
		VideoResultQueue, // name
		true,             // durable
		false,            // auto-delete
		false,            // exclusive
		false,            // no-wait
		amqp.Table{
			"x-message-ttl":             int32(MessageTTL),
			"x-dead-letter-exchange":    DLXExchange,
			"x-dead-letter-routing-key": "video.result.dead",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare video result queue: %w", err)
	}

	err = v.getChannel().QueueBind(
		VideoResultQueue,      // queue name
		VideoResultRoutingKey, // routing key
		VideoExchange,         // exchange
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind video result queue: %w", err)
	}

	// 6. Declare Video Cleanup Queue (for maintenance tasks)
	_, err = v.getChannel().QueueDeclare(
		VideoCleanupQueue, // name
		true,              // durable
		false,             // auto-delete
		false,             // exclusive
		false,             // no-wait
		amqp.Table{
			"x-message-ttl":             int32(MessageTTL),
			"x-dead-letter-exchange":    DLXExchange,
			"x-dead-letter-routing-key": "video.cleanup.dead",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare video cleanup queue: %w", err)
	}

	err = v.getChannel().QueueBind(
		VideoCleanupQueue,      // queue name
		VideoCleanupRoutingKey, // routing key
		VideoExchange,          // exchange
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind video cleanup queue: %w", err)
	}

	// 7. Set QoS for the channel to prevent overwhelming consumers
	err = v.getChannel().Qos(
		ConsumerPrefetchCount, // prefetch count - max unacked messages per consumer
		0,                     // prefetch size (0 = no limit)
		false,                 // global (false = apply to current channel)
	)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	log.Println("Video processing queues setup completed successfully")
	return nil
}

// PublishVideoProcessingMessage gửi message yêu cầu xử lý video vào exchange.
// Dùng publisher confirm để đảm bảo RabbitMQ đã nhận message trước khi return.
// Timeout confirm sau 5 giây để tránh block vô hạn.
func (v *VideoQueueSetup) PublishVideoProcessingMessage(ctx context.Context, message VideoProcessingMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Enable publisher confirms for reliability
	err = v.getChannel().Confirm(false)
	if err != nil {
		return fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	// Create confirms channel
	confirms := v.getChannel().NotifyPublish(make(chan amqp.Confirmation, 1))

	err = v.getChannel().PublishWithContext(
		ctx,
		VideoExchange,             // exchange
		VideoProcessingRoutingKey, // routing key
		false,                     // mandatory
		false,                     // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			Body:          body,
			Timestamp:     time.Now(),
			MessageId:     message.UploadID.String(),
			CorrelationId: message.UploadID.String(),
			DeliveryMode:  amqp.Persistent, // Make message persistent
			Priority:      0,               // Normal priority
			Headers: amqp.Table{
				"upload_id":     message.UploadID.String(),
				"user_id":       message.UserID.String(),
				"resource_type": message.ResourceType,
				"retry_count":   message.RetryCount,
				"created_at":    message.CreatedAt,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Wait for publisher confirmation with timeout
	select {
	case confirm := <-confirms:
		if !confirm.Ack {
			return fmt.Errorf("message publish was not confirmed")
		}
		log.Printf("Video processing message published successfully for upload: %s", message.UploadID)
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("publisher confirmation timeout")
	case <-ctx.Done():
		return fmt.Errorf("context cancelled while waiting for confirmation: %w", ctx.Err())
	}
}

// PublishVideoResultMessage gửi kết quả xử lý video (thành công / thất bại) vào result queue.
// Dùng để notify các service khác (ví dụ: cập nhật DB, gửi notification cho user).
func (v *VideoQueueSetup) PublishVideoResultMessage(ctx context.Context, result VideoProcessingResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	err = v.getChannel().PublishWithContext(
		ctx,
		VideoExchange,         // exchange
		VideoResultRoutingKey, // routing key
		false,                 // mandatory
		false,                 // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			Body:          body,
			Timestamp:     time.Now(),
			MessageId:     result.UploadID.String(),
			CorrelationId: result.UploadID.String(),
			DeliveryMode:  amqp.Persistent,
			Headers: amqp.Table{
				"upload_id":    result.UploadID.String(),
				"success":      result.Success,
				"processed_at": result.ProcessedAt,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	log.Printf("Video processing result published for upload: %s", result.UploadID)
	return nil
}

// PublishCleanupMessage gửi lệnh dọn dẹp vào cleanup queue.
// Ví dụ: xóa file tạm, abort upload hết hạn, dọn HLS segment cũ.
func (v *VideoQueueSetup) PublishCleanupMessage(ctx context.Context, cleanup CleanupMessage) error {
	body, err := json.Marshal(cleanup)
	if err != nil {
		return fmt.Errorf("failed to marshal cleanup message: %w", err)
	}

	err = v.getChannel().PublishWithContext(
		ctx,
		VideoExchange,          // exchange
		VideoCleanupRoutingKey, // routing key
		false,                  // mandatory
		false,                  // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
			DeliveryMode: amqp.Persistent,
			Headers: amqp.Table{
				"task_type":  cleanup.TaskType,
				"created_at": cleanup.CreatedAt,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish cleanup message: %w", err)
	}

	log.Printf("Cleanup message published: %s", cleanup.TaskType)
	return nil
}

// StartVideoProcessingConsumer bắt đầu consume message từ VideoProcessingQueue.
// Mỗi message được parse thành VideoProcessingMessage rồi truyền vào handler.
// Nếu handler lỗi và chưa vượt MaxRetries: tăng RetryCount, re-publish sau RetryDelay.
// Nếu đã vượt MaxRetries: Nack không requeue → message rơi vào Dead Letter Queue.
func (v *VideoQueueSetup) StartVideoProcessingConsumer(ctx context.Context, handler func(VideoProcessingMessage) error) error {
	msgs, err := v.getChannel().Consume(
		VideoProcessingQueue, // queue
		"video-processor",    // consumer tag
		false,                // auto-ack (disabled for manual ack)
		false,                // exclusive
		false,                // no-local
		false,                // no-wait
		nil,                  // args
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	log.Println("Video processing consumer started. Waiting for messages...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Video processing consumer shutting down...")
				return
			case msg, ok := <-msgs:
				if !ok {
					log.Println("Video processing consumer channel closed")
					return
				}

				// Parse message
				var videoMsg VideoProcessingMessage
				if err := json.Unmarshal(msg.Body, &videoMsg); err != nil {
					log.Printf("Failed to parse video processing message: %v", err)
					msg.Nack(false, false) // Send to DLQ
					continue
				}

				// Process message
				err := handler(videoMsg)
				if err != nil {
					log.Printf("Failed to process video message for upload %s: %v", videoMsg.UploadID, err)

					// Check retry count
					retryCount := videoMsg.RetryCount
					if retryCount < MaxRetries {
						// Retry: reject and requeue with delay
						log.Printf("Retrying video processing for upload %s (attempt %d/%d)", videoMsg.UploadID, retryCount+1, MaxRetries)
						videoMsg.RetryCount++

						// Re-publish with incremented retry count after delay
						go func() {
							time.Sleep(time.Duration(RetryDelay) * time.Millisecond)
							if retryErr := v.PublishVideoProcessingMessage(ctx, videoMsg); retryErr != nil {
								log.Printf("Failed to retry video processing message: %v", retryErr)
							}
						}()
					}

					// Nack the current message (don't requeue since we're handling retry manually)
					msg.Nack(false, false)
				} else {
					// Success: acknowledge the message
					msg.Ack(false)
					log.Printf("Video processing completed successfully for upload: %s", videoMsg.UploadID)
				}
			}
		}
	}()

	return nil
}

// Close is a no-op: connection lifecycle is managed by RabbitMQService
func (v *VideoQueueSetup) Close() error {
	return nil
}

// HealthCheck delegates to the underlying RabbitMQService
func (v *VideoQueueSetup) HealthCheck() error {
	if v.service == nil {
		return fmt.Errorf("RabbitMQ service is nil")
	}
	if v.getChannel() == nil {
		return fmt.Errorf("RabbitMQ channel is nil")
	}
	return nil
}
