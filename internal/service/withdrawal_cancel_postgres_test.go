package service

// Q2 (QA vòng 2, 28/09/2026): giảng viên tự huỷ yêu cầu rút còn pending — test Postgres THẬT, cùng
// fixture schema tạm với withdrawal_service_postgres_test.go (dữ liệu COMMIT thật vì test race cần
// nhiều kết nối thấy nhau, schema bị DROP khi xong).

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

func TestWithdrawalCancel_Rules(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	admin := uuid.New()
	teacher, other := f.teacher(true), f.teacher(true)
	f.order("completed", 0, orderLine{f.course(teacher), 1000000})

	w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(400000))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wallet, _ := f.wallet.GetTeacherWallet(ctx, teacher)
	mustEqualDec(t, "available_balance khi đang chờ", wallet.AvailBalance, 600000)

	// Giảng viên khác: 404 như không tồn tại, và yêu cầu vẫn pending.
	if _, err := f.svc.Cancel(ctx, other, w.ID); !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("GV khác huỷ: err = %v, muốn ErrWithdrawalNotFound", err)
	}
	if _, err := f.svc.Cancel(ctx, teacher, uuid.New()); !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("id không tồn tại: err = %v", err)
	}
	if n := f.countPayouts(teacher, model.PayoutStatusPending); n != 1 {
		t.Fatalf("sau 2 lần huỷ sai, còn %d yêu cầu pending, muốn 1", n)
	}

	res, err := f.svc.Cancel(ctx, teacher, w.ID)
	if err != nil || res.Status != model.PayoutStatusCancelled {
		t.Fatalf("chủ yêu cầu huỷ: res=%v err=%v", res, err)
	}
	var saved model.InstructorPayout
	f.db.Where("id = ?", w.ID).First(&saved)
	if saved.Status != model.PayoutStatusCancelled || saved.ProcessedAt == nil || saved.Notes == nil {
		t.Fatalf("huỷ không lưu status/processed_at/notes: %+v", saved)
	}

	// Số dư trả lại ngay, không còn "đang xử lý".
	wallet, _ = f.wallet.GetTeacherWallet(ctx, teacher)
	mustEqualDec(t, "available_balance sau khi huỷ", wallet.AvailBalance, 1000000)
	if wallet.HasOpenWithdrawal {
		t.Fatal("has_open_withdrawal vẫn true sau khi huỷ")
	}

	// Huỷ lần 2: không còn pending -> 409 kèm trạng thái hiện tại.
	_, err = f.svc.Cancel(ctx, teacher, w.ID)
	if !errors.Is(err, ErrWithdrawalInvalidTransition) || ruleData(t, err)["current_status"] != model.PayoutStatusCancelled {
		t.Fatalf("huỷ lần 2: err = %v", err)
	}
	// Admin không duyệt được yêu cầu đã huỷ.
	if _, err := f.svc.Approve(ctx, admin, w.ID); !errors.Is(err, ErrWithdrawalInvalidTransition) {
		t.Fatalf("approve yêu cầu đã huỷ: err = %v", err)
	}

	// Gửi lại được ngay (cancelled không tính là đang mở), rút hết số dư đã được trả.
	w2, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(1000000))
	if err != nil {
		t.Fatalf("gửi lại sau khi huỷ: %v", err)
	}

	// Yêu cầu đã duyệt thì KHÔNG huỷ được (tiền đang chờ admin chuyển).
	if _, err := f.svc.Approve(ctx, admin, w2.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	_, err = f.svc.Cancel(ctx, teacher, w2.ID)
	if !errors.Is(err, ErrWithdrawalInvalidTransition) || ruleData(t, err)["current_status"] != model.PayoutStatusApproved {
		t.Fatalf("huỷ yêu cầu đã duyệt: err = %v, muốn invalid transition (approved)", err)
	}
	if n := f.countPayouts(teacher, model.PayoutStatusApproved); n != 1 {
		t.Fatalf("yêu cầu đã duyệt bị đổi trạng thái: approved = %d", n)
	}

	// Danh sách của giảng viên lọc được theo cancelled (enum đã có giá trị mới).
	list, err := f.svc.ListMine(ctx, teacher, model.PayoutStatusCancelled, 1, 20)
	if err != nil || list.TotalCount != 1 || list.Items[0].ID != w.ID {
		t.Fatalf("ListMine(cancelled): %+v err=%v", list, err)
	}
}

// Huỷ đua với admin duyệt / từ chối trên CÙNG 1 yêu cầu: đúng 1 bên thắng, bên kia nhận 409, và
// trạng thái cuối khớp bên thắng. Lặp nhiều vòng để 2 goroutine thực sự chồng nhau. Test phải ĐỎ
// khi Cancel đọc yêu cầu không khoá (cả 2 cùng thấy pending rồi cùng ghi, cả 2 báo thành công).
func TestWithdrawalCancel_RacesWithAdmin(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	admin := uuid.New()
	teacher := f.teacher(true)
	f.order("completed", 0, orderLine{f.course(teacher), 100000000})

	const rounds = 15
	for _, adminOp := range []string{"approve", "reject"} {
		for i := 0; i < rounds; i++ {
			w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(100000))
			if err != nil {
				t.Fatalf("%s vòng %d create: %v", adminOp, i, err)
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			var cancelErr, adminErr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, cancelErr = f.svc.Cancel(ctx, teacher, w.ID)
			}()
			go func() {
				defer wg.Done()
				<-start
				if adminOp == "approve" {
					_, adminErr = f.svc.Approve(ctx, admin, w.ID)
				} else {
					_, adminErr = f.svc.Reject(ctx, admin, w.ID, "QA-race")
				}
			}()
			close(start)
			wg.Wait()

			var final model.InstructorPayout
			f.db.Where("id = ?", w.ID).First(&final)
			switch {
			case cancelErr == nil && errors.Is(adminErr, ErrWithdrawalInvalidTransition):
				if final.Status != model.PayoutStatusCancelled {
					t.Fatalf("%s vòng %d: huỷ thắng nhưng DB = %s", adminOp, i, final.Status)
				}
			case adminErr == nil && errors.Is(cancelErr, ErrWithdrawalInvalidTransition):
				if final.Status == model.PayoutStatusCancelled || final.Status == model.PayoutStatusPending {
					t.Fatalf("%s vòng %d: admin thắng nhưng DB = %s", adminOp, i, final.Status)
				}
			default:
				t.Fatalf("%s vòng %d: phải đúng 1 bên thắng — cancelErr=%v adminErr=%v DB=%s", adminOp, i, cancelErr, adminErr, final.Status)
			}

			// Dọn yêu cầu đã duyệt để vòng sau tạo được (chỉ 1 yêu cầu mở mỗi lúc).
			if final.Status == model.PayoutStatusApproved {
				if _, err := f.svc.MarkCompleted(ctx, admin, w.ID, "QA-FT-race"); err != nil {
					t.Fatalf("mark-completed dọn vòng %d: %v", i, err)
				}
			}
		}
	}
}
