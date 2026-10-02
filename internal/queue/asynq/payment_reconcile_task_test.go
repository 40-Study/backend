package asynq_queue

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// L8 mục 2: lịch và tuỳ chọn retry của job quét đối chiếu thanh toán chưa có test (đổi MaxRetry(0) thành mặc
// định 25 lần, hay "@every" thành cron "*/N", không test nào đỏ). asynq không cho đọc lại tuỳ chọn đã đăng ký
// nên test kiểm hàm dựng spec, rồi kiểm đường đăng ký thật nhận đúng spec đó.

func TestPaymentReconcileSchedule_EveryNMinutesIsEvenForAnyN(t *testing.T) {
	// 7, 45, 59 không chia hết 60: cron "*/N" sẽ chạy không đều, "@every" thì không.
	for _, n := range []int{1, 7, 10, 45, 59} {
		freq, _, _ := paymentReconcileSchedule(n)
		spec := string(freq)
		if !strings.HasPrefix(spec, "@every ") {
			t.Fatalf("N=%d: lịch %q không phải dạng @every (cron */N chạy không đều khi N không chia hết 60)", n, spec)
		}
		got, err := time.ParseDuration(strings.TrimPrefix(spec, "@every "))
		if err != nil || got != time.Duration(n)*time.Minute {
			t.Fatalf("N=%d: lịch %q = %v (err=%v), muốn đúng %d phút", n, spec, got, err, n)
		}
	}
}

func TestPaymentReconcileSchedule_NeverRetriesAndUsesLowQueue(t *testing.T) {
	_, queue, opts := paymentReconcileSchedule(10)
	if queue != "low" {
		t.Fatalf("hàng đợi = %q, muốn low", queue)
	}
	retries := 0
	for _, o := range opts {
		if o.Type() != asynq.MaxRetryOpt {
			continue
		}
		retries++
		if v, ok := o.Value().(int); !ok || v != 0 {
			t.Fatalf("MaxRetry = %v, muốn 0: asynq mặc định retry 25 lần sẽ chạy lại lượt quét nặng ngay lúc ngân hàng đang chết", o.Value())
		}
	}
	if retries != 1 {
		t.Fatalf("có %d tuỳ chọn MaxRetry, muốn đúng 1 (thiếu thì asynq dùng mặc định 25 lần)", retries)
	}
}

// Đường đăng ký thật: scheduler của asynq tự parse spec (lịch sai thì Register trả lỗi) mà không cần Redis.
func TestRegisterPaymentReconcile_AcceptsScheduleAndRequiresRunFunc(t *testing.T) {
	// ServeMux panic khi đăng ký trùng loại task, nên mỗi N một Queue riêng.
	newQueue := func() *Queue {
		return &Queue{
			scheduler: asynq.NewScheduler(asynq.RedisClientOpt{Addr: "127.0.0.1:1"}, nil),
			mux:       asynq.NewServeMux(),
		}
	}
	for _, n := range []int{1, 7, 45} {
		if err := RegisterPaymentReconcile(newQueue(), n, func(context.Context) error { return nil }); err != nil {
			t.Fatalf("N=%d: đăng ký lỗi: %v", n, err)
		}
	}
	if err := RegisterPaymentReconcile(newQueue(), 10, nil); err == nil {
		t.Fatal("thiếu hàm run mà vẫn đăng ký được")
	}
}
