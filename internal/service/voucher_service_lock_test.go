package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
	"study.com/v1/internal/model"
)

// TestLockAndCheckUsagePerUser_ZeroMeansUnlimited (I-02, review vòng 5 — chỉ đạo team-lead:
// "usage_per_user = 0 nghĩa là không giới hạn — GIỮ, nhưng ghi comment rõ + test") — voucher có
// UsagePerUser<=0 phải bỏ qua HẲN việc khoá+đếm, KHÔNG chạm vào tx. Truyền tx=nil để chứng minh:
// nếu hàm lỡ đi vào nhánh khoá/đếm (repository.NewVoucherRepository(nil).LockVoucherForUpdate(...))
// sẽ PANIC ngay (gọi method trên *gorm.DB nil) — test không panic tức là nhánh "unlimited" đã
// return SỚM, đúng như comment.
func TestLockAndCheckUsagePerUser_ZeroMeansUnlimited(t *testing.T) {
	vs := &VoucherService{}
	voucher := &model.Voucher{ID: uuid.New(), UsagePerUser: 0}

	err := vs.LockAndCheckUsagePerUser(context.Background(), nil, voucher, uuid.New())
	if err != nil {
		t.Errorf("expected nil error for UsagePerUser=0 (unlimited), got %v", err)
	}

	// Âm cũng phải coi là "không giới hạn" giống 0 — cùng điều kiện `<= 0` trong code sản xuất.
	voucherNegative := &model.Voucher{ID: uuid.New(), UsagePerUser: -1}
	if err := vs.LockAndCheckUsagePerUser(context.Background(), nil, voucherNegative, uuid.New()); err != nil {
		t.Errorf("expected nil error for UsagePerUser<0 (treated as unlimited), got %v", err)
	}
}

// TestLockAndCheckUsagePerUser_PositiveUsagePerUser_LocksAndCounts (I-02, review vòng 5) — pin
// lại: khi UsagePerUser > 0, hàm PHẢI thực sự khoá dòng voucher (SELECT ... FOR UPDATE) trước khi
// đếm — khác hẳn nhánh UsagePerUser<=0 ở trên. Chạy trên DummyDialector DryRun (không có DB
// thật): CountUserVoucherUsage/CountUserHeldOrders trên DryRun LUÔN trả count=0 (không có kết
// nối thật để scan hàng), nên completedCount+heldCount=0 < UsagePerUser -> hàm PHẢI trả nil, KHÔNG
// phải vì bỏ qua check (như nhánh UsagePerUser<=0) mà vì count thật sự = 0 trên DryRun — phân
// biệt 2 lý do "trả nil" bằng cách bắt SQL thực sự chạy qua callback (không phải suy luận từ giá
// trị trả về).
//
// Tự kiểm chứng: sửa tạm LockAndCheckUsagePerUser bỏ nhánh `if voucher.UsagePerUser <= 0` (luôn
// chạy khoá+đếm) — test TestLockAndCheckUsagePerUser_ZeroMeansUnlimited sẽ PANIC (gọi trên tx=nil)
// đúng như kỳ vọng; revert lại — PASS. Không giữ mutation trong working tree.
func TestLockAndCheckUsagePerUser_PositiveUsagePerUser_LocksAndCounts(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})

	var capturedSelects []string
	if err := dryRunDB.Callback().Query().After("gorm:query").
		Register("test:capture_sql", func(tx *gorm.DB) {
			capturedSelects = append(capturedSelects, tx.Statement.SQL.String())
		}); err != nil {
		t.Fatalf("failed to register capture callback: %v", err)
	}

	vs := &VoucherService{}
	voucher := &model.Voucher{ID: uuid.New(), UsagePerUser: 1}

	if err := vs.LockAndCheckUsagePerUser(context.Background(), dryRunDB, voucher, uuid.New()); err != nil {
		t.Fatalf("expected nil error on DryRun (count luôn = 0 < UsagePerUser), got %v", err)
	}

	if len(capturedSelects) < 3 {
		t.Fatalf("expected ít nhất 3 câu SELECT (lock + 2 count), captured %d: %v", len(capturedSelects), capturedSelects)
	}

	foundLock := false
	for _, sql := range capturedSelects {
		if strings.Contains(sql, "FOR UPDATE") && strings.Contains(sql, "vouchers") {
			foundLock = true
			break
		}
	}
	if !foundLock {
		t.Errorf("expected một câu SELECT ... vouchers ... FOR UPDATE trong danh sách đã chạy, got: %v", capturedSelects)
	}
}

// Không có test riêng cho nhánh "LockVoucherForUpdate trả lỗi thật (vd ErrRecordNotFound)" —
// đã thử bằng DummyDialector KHÔNG DryRun (Take() thật) nhưng gormtests.DummyDialector panic
// (nil pointer, không có driver/connection thật để Exec — cùng giới hạn đã ghi nhận ở vòng 4b
// cho db.Create()) thay vì trả lỗi như kỳ vọng. Không có sqlmock/sqlite trong go.sum để giả lập
// lỗi DB thật một cách an toàn — hành vi "lỗi khoá thì trả nguyên lỗi đó, không tiếp tục đếm"
// chỉ còn được đảm bảo bằng ĐỌC CODE (early return ngay khi err != nil, xem
// voucher_service.go LockAndCheckUsagePerUser) chứ không có test tự động pin lại được trong môi
// trường hiện tại — ghi nhận hạn chế này thay vì giả vờ đã test.
