package queue

// Lúc khởi động, app.go chạy SetupQueue (exercise, certificate...) trên goroutine chính trong khi worker
// invitation/video gọi ConsumeMessages (Qos + Consume) trên goroutine khác. amqp091-go KHÔNG khoá cặp
// gửi-yêu-cầu/chờ-trả-lời của một channel (Channel.call), nên hai lệnh đồng bộ chạy song song trên CÙNG channel
// có thể nhận nhầm trả lời của nhau và trả ErrCommandInvalid: Exception (503) "unexpected command received" —
// đúng lỗi QA thấy ở boot. Test này dựng lại tình huống đó trên RabbitMQ thật. Không kết nối được: Skip ở local,
// FAIL khi CI=true (CI có service RabbitMQ, cùng quy ước pgtest.MissingDBAction).

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"study.com/v1/internal/config"
	"study.com/v1/internal/testutil/pgtest"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// testRabbitConfig đọc RABBITMQ_* từ môi trường, mặc định giống .env.example dev.
func testRabbitConfig() *config.Config {
	return &config.Config{
		RabbitMQHost:     envOr("RABBITMQ_HOST", "localhost"),
		RabbitMQPort:     envOr("RABBITMQ_PORT", "5672"),
		RabbitMQUser:     envOr("RABBITMQ_USER", "guest"),
		RabbitMQPassword: envOr("RABBITMQ_PASSWORD", "guest"),
		RabbitMQVHost:    envOr("RABBITMQ_VHOST", "/"),
	}
}

func TestRabbitMQService_ConcurrentSetupAndConsumeDoNotShareRPCs(t *testing.T) {
	cfg := testRabbitConfig()
	url := buildAMQPURL(cfg)
	probe, err := amqp.Dial(url)
	if err != nil {
		msg := fmt.Sprintf("không kết nối được RabbitMQ %s:%s (%v)", cfg.RabbitMQHost, cfg.RabbitMQPort, err)
		if pgtest.MissingDBAction(os.Getenv("CI")) == "fail" {
			t.Fatalf("CI=true nhưng %s", msg)
		}
		t.Skip(msg + ": bỏ qua test tích hợp")
	}
	cleanupCh, err := probe.Channel()
	if err != nil {
		probe.Close()
		t.Fatalf("mở channel dọn dẹp: %v", err)
	}

	const workers = 16
	exchange := "test.concurrency." + uuid.NewString()[:8]
	queues := make([]string, workers)
	for i := range queues {
		queues[i] = fmt.Sprintf("%s.q%d", exchange, i)
	}
	t.Cleanup(func() {
		for _, q := range queues {
			_, _ = cleanupCh.QueueDelete(q, false, false, false)
		}
		_ = cleanupCh.ExchangeDelete(exchange, false, false)
		_ = probe.Close()
	})

	svc, err := NewRabbitMQService(cfg)
	if err != nil {
		t.Fatalf("NewRabbitMQService: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	received := make(chan string, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		q := queues[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := svc.SetupQueue(exchange, q, q); err != nil {
				errs <- fmt.Errorf("SetupQueue %s: %w", q, err)
				return
			}
			if err := svc.ConsumeMessages(ctx, q, func(_ context.Context, body []byte) error {
				received <- q
				return nil
			}); err != nil {
				errs <- fmt.Errorf("ConsumeMessages %s: %w", q, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	// Mọi consumer phải thật sự nhận được tin: setup và consume thành công không chỉ trên giấy.
	for _, q := range queues {
		if err := svc.PublishMessage(ctx, exchange, q, map[string]string{"queue": q}); err != nil {
			t.Fatalf("PublishMessage %s: %v", q, err)
		}
	}
	got := map[string]bool{}
	timeout := time.After(10 * time.Second)
	for len(got) < workers {
		select {
		case q := <-received:
			got[q] = true
		case <-timeout:
			t.Fatalf("chỉ %d/%d consumer nhận được tin trong 10s: %v", len(got), workers, got)
		}
	}
}
