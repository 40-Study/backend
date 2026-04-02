package asynq_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"study.com/v1/internal/config"
)

// ===================== Frequency =====================

// Frequency là cron expression dùng cho recurring tasks.
type Frequency string

func DailyAt(hour, min int) Frequency {
	return Frequency(fmt.Sprintf("%d %d * * *", min, hour))
}

func WeeklyAt(weekday, hour, min int) Frequency {
	return Frequency(fmt.Sprintf("%d %d * * %d", min, hour, weekday))
}

// EveryNMinutes tạo frequency chạy mỗi N phút.
//
//	EveryNMinutes(5) → "*/5 * * * *"
func EveryNMinutes(n int) Frequency {
	return Frequency(fmt.Sprintf("*/%d * * * *", n))
}

// EveryNHours tạo frequency chạy mỗi N giờ (phút 0).
//
//	EveryNHours(2) → "0 */2 * * *"
func EveryNHours(n int) Frequency {
	return Frequency(fmt.Sprintf("0 */%d * * *", n))
}

// Cron tạo frequency từ cron expression tùy ý.
//
//	Cron("0 6,12,18 * * *") → chạy lúc 6h, 12h, 18h
func Cron(expr string) Frequency {
	return Frequency(expr)
}

type TaskFunc func(ctx context.Context, payload []byte) error

// Queue quản lý toàn bộ asynq: enqueue, schedule, và process tasks.
type Queue struct {
	client    *asynq.Client
	server    *asynq.Server
	scheduler *asynq.Scheduler
	mux       *asynq.ServeMux
}

// New tạo Queue mới, kết nối Redis.
func New(cfg *config.Config) *Queue {
	redisOpt := asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	// giờ Việt Nam (UTC+7) để scheduler chạy đúng giờ địa phương
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")

	return &Queue{
		client: asynq.NewClient(redisOpt),
		server: asynq.NewServer(redisOpt, asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical":      4,
				"notifications": 5,
				"low":           1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Printf("[asynq] ERROR task=%s err=%v", task.Type(), err)
			}),
		}),
		scheduler: asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{
			Location: loc,
		}),
		mux: asynq.NewServeMux(),
	}
}

func (q *Queue) Handle(taskType string, fn TaskFunc) {
	q.mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
		return fn(ctx, t.Payload())
	})
}

func (q *Queue) Schedule(taskType string, freq Frequency, fn TaskFunc, queue string) error {
	q.Handle(taskType, fn)

	_, err := q.scheduler.Register(
		string(freq),
		asynq.NewTask(taskType, nil),
		asynq.Queue(queue),
	)
	if err != nil {
		return fmt.Errorf("register periodic task %s: %w", taskType, err)
	}

	log.Printf("[asynq] Scheduled recurring: %s [%s] → queue=%s", taskType, string(freq), queue)
	return nil
}

func (q *Queue) ScheduleAt(taskType string, payload any, at time.Time, queue string) error {
	if at.Before(time.Now()) {
		return nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload for %s: %w", taskType, err)
	}

	info, err := q.client.Enqueue(
		asynq.NewTask(taskType, data),
		asynq.ProcessAt(at),
		asynq.Queue(queue),
		asynq.MaxRetry(3),
	)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", taskType, err)
	}

	log.Printf("[asynq] Scheduled: id=%s type=%s queue=%s at=%v", info.ID, taskType, queue, at)
	return nil
}

func (q *Queue) Enqueue(taskType string, payload any, queue string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload for %s: %w", taskType, err)
	}

	info, err := q.client.Enqueue(
		asynq.NewTask(taskType, data),
		asynq.Queue(queue),
		asynq.MaxRetry(3),
	)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", taskType, err)
	}

	log.Printf("[asynq] Enqueued: id=%s type=%s queue=%s", info.ID, taskType, queue)
	return nil
}

func (q *Queue) Start() error {
	q.mux.Use(func(h asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			start := time.Now()
			log.Printf("[asynq] ▶ Processing %s (payload=%d bytes)", t.Type(), len(t.Payload()))

			err := h.ProcessTask(ctx, t)
			elapsed := time.Since(start)
			if err != nil {
				log.Printf("[asynq] ✗ Failed %s: %v (elapsed=%v)", t.Type(), err, elapsed)
			} else {
				log.Printf("[asynq] ✓ Done %s (elapsed=%v)", t.Type(), elapsed)
			}
			return err
		})
	})

	go func() {
		if err := q.scheduler.Start(); err != nil {
			log.Printf("[asynq] Scheduler error: %v", err)
		}
	}()

	log.Println("[asynq] Starting worker server...")
	return q.server.Start(q.mux)
}

func (q *Queue) Stop() {
	q.scheduler.Shutdown()
	q.server.Stop()
	_ = q.client.Close()
	log.Println("[asynq] Stopped")
}
