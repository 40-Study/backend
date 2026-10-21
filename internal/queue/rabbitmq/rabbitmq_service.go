package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"study.com/v1/internal/config"
	"study.com/v1/internal/utils"
)

/*
RabbitMQService

Thiết kế theo pattern:

Application
    ↓
RabbitMQService (Wrapper)
    ↓
Connection (TCP)
    ↓
Channel (AMQP session)
    ↓
Exchange → Queue → Consumer

Lý do tách connect(), handleReconnect():
- Connection có thể chết bất ngờ (network drop, broker restart)
- Khi chết phải reconnect + tạo lại channel
- Nếu không, publish/consume sẽ fail âm thầm
*/

type RabbitMQServiceInterface interface {
	SetupQueue(exchange, queue, routingKey string) error
	PublishMessage(ctx context.Context, exchange, routingKey string, message interface{}) error
	ConsumeMessages(ctx context.Context, queue string, handler func(context.Context, []byte) error) error
	Close() error
}

type RabbitMQService struct {
	conn    *amqp.Connection
	channel *amqp.Channel

	url string

	mu sync.RWMutex // bảo vệ channel/connection khi reconnect

	isClosing       bool
	notifyConnClose chan *amqp.Error
}

//
// =======================
// Constructor
// =======================
//

func (r *RabbitMQService) connect() error {

	var conn *amqp.Connection
	var err error

	// retry 5 lần
	for i := range 5 {
		i++
		conn, err = amqp.Dial(r.url)
		if err == nil {
			break
		}
		log.Printf("[RabbitMQ] Connect failed, retrying %d...", i)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}

	// lock khi gán vì có thể đang reconnect
	r.mu.Lock()
	r.conn = conn
	r.channel = ch
	r.mu.Unlock()

	// đăng ký listener khi connection chết
	r.conn.NotifyClose(r.notifyConnClose)

	log.Println("[RabbitMQ] Connected successfully")

	return nil
}

func buildAMQPURL(cfg *config.Config) string {

	scheme := "amqp"

	username := url.QueryEscape(cfg.RabbitMQUser)
	password := url.QueryEscape(cfg.RabbitMQPassword)

	vhost := cfg.RabbitMQVHost
	if vhost == "" {
		vhost = "/"
	}

	return fmt.Sprintf(
		"%s://%s:%s@%s:%s/%s",
		scheme,
		username,
		password,
		cfg.RabbitMQHost,
		cfg.RabbitMQPort,
		vhost,
	)
}

func NewRabbitMQService(cfg *config.Config) (*RabbitMQService, error) {

	r := &RabbitMQService{
		url:             buildAMQPURL(cfg),
		notifyConnClose: make(chan *amqp.Error),
	}

	if err := r.connect(); err != nil {
		return nil, err
	}

	go r.handleReconnect()

	return r, nil
}

func (r *RabbitMQService) handleReconnect() {
	for {
		err := <-r.notifyConnClose

		if r.isClosing {
			log.Println("[RabbitMQ] Closed intentionally")
			return
		}

		log.Printf("[RabbitMQ] Connection lost: %v", err)

		for {
			time.Sleep(3 * time.Second)

			if err := r.connect(); err != nil {
				log.Println("[RabbitMQ] Reconnect failed, retrying...")
				continue
			}

			log.Println("[RabbitMQ] Reconnected successfully")
			break
		}
	}
}

//
// =======================
// getChannel()
// =======================
//
// Trả về channel thread-safe
// Vì reconnect có thể thay đổi channel pointer
//

func (r *RabbitMQService) getChannel() *amqp.Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channel
}

// openChannel mở một channel RIÊNG trên connection hiện tại.
//
// Vì sao không dùng channel chung cho lệnh đồng bộ (declare, bind, qos, consume)? amqp091-go không khoá cặp
// "gửi yêu cầu, chờ trả lời" của một channel (Channel.call): hai goroutine cùng gọi lệnh đồng bộ trên CÙNG
// channel có thể nhận nhầm trả lời của nhau và nhận ErrCommandInvalid — Exception (503) "unexpected command
// received". Lúc khởi động app.go chạy SetupQueue trên goroutine chính trong khi worker gọi ConsumeMessages
// trên goroutine khác, nên mỗi lệnh đồng bộ phải có channel của riêng nó. Publish vẫn dùng channel chung
// (Publish tự khoá channel và không chờ trả lời).
func (r *RabbitMQService) openChannel() (*amqp.Channel, error) {
	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("connection not ready")
	}
	return conn.Channel()
}

//
// =======================
// SetupQueue()
// =======================
//
// Khai báo Exchange + Queue + Bind
//
// Tại sao cần?
// - RabbitMQ không tự tạo exchange nếu chưa tồn tại
// - Sau reconnect phải declare lại topology
//

func (r *RabbitMQService) SetupQueue(
	exchange string,
	queue string,
	routingKey string,
) error {

	// Channel riêng, đóng khi xong: xem openChannel.
	ch, err := r.openChannel()
	if err != nil {
		return fmt.Errorf("open setup channel: %w", err)
	}
	defer ch.Close()

	// Declare exchange
	if err := ch.ExchangeDeclare(
		exchange, // tên exchange
		"direct", // loại exchange
		true,     // durable: restart broker không mất exchange
		false,    // autoDelete: false vì muốn exchange tồn tại lâu dài
		false,    // internal: false vì producer/consumer có thể publish/consume trực tiếp
		false,    // noWait: false vì muốn chờ RabbitMQ confirm đã declare xong
		nil,      // args
	); err != nil {
		return err
	}

	// Declare queue
	_, err = ch.QueueDeclare(
		queue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	// Bind queue với exchange
	return ch.QueueBind(
		queue,
		routingKey,
		exchange,
		false,
		nil,
	)
}

//
// =======================
// PublishMessage()
// =======================
//
// Gửi message vào exchange
// Dùng PublishWithContext để có timeout/cancel
//

func (r *RabbitMQService) PublishMessage(
	ctx context.Context,
	exchange string,
	routingKey string,
	message interface{},
) error {

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	ch := r.getChannel()
	if ch == nil {
		return fmt.Errorf("channel not ready")
	}

	return ch.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // message không mất khi restart
			Timestamp:    time.Now(),
		},
	)
}

//
// =======================
// ConsumeMessages()
// =======================
//
// Manual ACK pattern:
// - Thành công → Ack
// - Lỗi → Nack + requeue
//

func (r *RabbitMQService) ConsumeMessages(
	ctx context.Context,
	queue string,
	handler func(context.Context, []byte) error,
) error {

	// Mỗi consumer một channel riêng (xem openChannel); Qos cũng vì thế áp dụng đúng cho riêng consumer này.
	ch, err := r.openChannel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}

	// Giới hạn xử lý 1 message mỗi lần
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		return err
	}

	msgs, err := ch.Consume(
		queue,
		"",
		false, // manual ack
		false, // exclusive: false vì có thể có nhiều consumer
		false, // noLocal: false vì consumer có thể nhận message do chính nó publish
		false, // noWait: false vì muốn chờ RabbitMQ confirm đã setup xong consumer
		nil,
	)
	if err != nil {
		ch.Close()
		return err
	}

	// M-05 (audit 260909 vòng 2): bọc SafeGo — panic trong handler(ctx, d.Body) của 1 message
	// (business logic tuỳ consumer) trước đây sập cả process, kéo theo mọi queue/worker khác
	// đang chạy chung. Sau recover, riêng consumer loop này dừng — vẫn tốt hơn sập cả server.
	utils.SafeGo(func() {
		defer ch.Close()
		for {
			select {
			case <-ctx.Done():
				return

			case d, ok := <-msgs:
				if !ok {
					return
				}

				err := handler(ctx, d.Body)
				if err != nil {
					d.Nack(false, true) // retry
				} else {
					d.Ack(false)
				}
			}
		}
	})

	return nil
}

//
// =======================
// Close()
// =======================
//
// Đóng connection gracefully
//

func (r *RabbitMQService) Close() error {

	r.isClosing = true

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channel != nil {
		r.channel.Close()
	}

	if r.conn != nil {
		return r.conn.Close()
	}

	return nil
}
