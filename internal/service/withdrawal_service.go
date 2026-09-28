package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Lỗi nghiệp vụ rút tiền (Phase 4). Handler map từng lỗi sang HTTP status + mã `error` của contract.
var (
	ErrWithdrawalInvalidAmount          = errors.New("amount must be greater than 0")
	ErrWithdrawalBelowMinimum           = errors.New("amount is below the minimum withdrawal")
	ErrWithdrawalBankInfoRequired       = errors.New("bank info required before withdrawal")
	ErrWithdrawalNegativeBalance        = errors.New("balance is negative, withdrawals are blocked until new earnings cover it")
	ErrWithdrawalInsufficientBalance    = errors.New("amount exceeds available balance")
	ErrWithdrawalAlreadyOpen            = errors.New("you already have a withdrawal request in progress")
	ErrWithdrawalTeacherProfileRequired = errors.New("teacher profile required")
	ErrWithdrawalNotFound               = errors.New("withdrawal not found")
	ErrWithdrawalInvalidTransition      = errors.New("invalid status transition")
	ErrWithdrawalReasonRequired         = errors.New("reason is required")
	ErrWithdrawalTransactionIDRequired  = errors.New("transaction_id is required")
	// ErrWithdrawalPayoutNegativeBalance — admin duyệt/đánh dấu đã chuyển khi số dư GV đang âm
	// (vd đơn bị hoàn sau khi GV gửi yêu cầu). Khác ErrWithdrawalNegativeBalance (GV tạo yêu cầu,
	// 400): đây là xung đột trạng thái nên trả 409.
	ErrWithdrawalPayoutNegativeBalance = errors.New("teacher balance is negative, this withdrawal cannot be approved or paid out")
)

// WithdrawalRuleError gói 1 lỗi nghiệp vụ kèm dữ liệu cho client (số dư, mức tối thiểu, id yêu
// cầu đang mở...). errors.Is(err, ErrWithdrawalX) vẫn hoạt động nhờ Unwrap.
type WithdrawalRuleError struct {
	Kind error
	Data map[string]interface{}
}

func (e *WithdrawalRuleError) Error() string { return e.Kind.Error() }
func (e *WithdrawalRuleError) Unwrap() error { return e.Kind }

func ruleErr(kind error, data map[string]interface{}) error {
	return &WithdrawalRuleError{Kind: kind, Data: data}
}

// DefaultMinWithdrawalAmount — quyết định chủ dự án #7: tối thiểu 100.000đ. Giá trị thật lấy từ
// config WITHDRAWAL_MIN_AMOUNT; hằng này chỉ là mặc định của config.
var DefaultMinWithdrawalAmount = decimal.NewFromInt(100000)

type WithdrawalServiceInterface interface {
	Create(ctx context.Context, teacherID uuid.UUID, amount decimal.Decimal) (*dto.WithdrawalItem, error)
	ListMine(ctx context.Context, teacherID uuid.UUID, status string, page, limit int) (*dto.WithdrawalListResponse, error)
	AdminList(ctx context.Context, teacherID *uuid.UUID, status string, page, limit int) (*dto.AdminWithdrawalListResponse, error)
	NegativeBalances(ctx context.Context) (*dto.NegativeBalanceListResponse, error)
	Approve(ctx context.Context, actorID, id uuid.UUID) (*dto.WithdrawalStatusResponse, error)
	Reject(ctx context.Context, actorID, id uuid.UUID, reason string) (*dto.WithdrawalStatusResponse, error)
	MarkCompleted(ctx context.Context, actorID, id uuid.UUID, transactionID string) (*dto.WithdrawalStatusResponse, error)
}

type WithdrawalService struct {
	repo       *repository.WithdrawalRepository
	walletRepo *repository.WalletRepository
	minAmount  decimal.Decimal
}

func NewWithdrawalService(repo *repository.WithdrawalRepository, walletRepo *repository.WalletRepository, minAmount decimal.Decimal) *WithdrawalService {
	return &WithdrawalService{repo: repo, walletRepo: walletRepo, minAmount: minAmount}
}

// ─── Hàm thuần: công thức + quy tắc (unit test không cần DB) ────────────────

// availableBalance = phần GV của đơn completed − mọi yêu cầu rút pending/approved/completed.
// Có thể âm khi đơn bị hoàn tiền sau khi giảng viên đã rút (quyết định #8).
func availableBalance(earnings decimal.Decimal, sums repository.TeacherPayoutSums) decimal.Decimal {
	return earnings.Sub(sums.Reserved())
}

// validateWithdrawalAmount — kiểm tra số tiền trước khi mở transaction (không cần khoá).
func validateWithdrawalAmount(amount, minAmount decimal.Decimal) error {
	if !amount.IsPositive() {
		return ruleErr(ErrWithdrawalInvalidAmount, nil)
	}
	if amount.LessThan(minAmount) {
		return ruleErr(ErrWithdrawalBelowMinimum, map[string]interface{}{"min_amount": minAmount})
	}
	return nil
}

func hasBankInfo(p *model.TeacherProfile) bool {
	nonEmpty := func(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }
	return nonEmpty(p.BankName) && nonEmpty(p.BankAccountNumber) && nonEmpty(p.BankAccountName)
}

// checkWithdrawalEligibility — chạy TRONG transaction sau khi đã khoá teacher_profiles, trên số
// liệu đọc sau khoá. openID != nil nghĩa là đang có yêu cầu pending/approved (quyết định #7).
func checkWithdrawalEligibility(profile *model.TeacherProfile, openID *uuid.UUID, balance, amount decimal.Decimal) error {
	if !hasBankInfo(profile) {
		return ruleErr(ErrWithdrawalBankInfoRequired, nil)
	}
	if openID != nil {
		return ruleErr(ErrWithdrawalAlreadyOpen, map[string]interface{}{"id": *openID})
	}
	if balance.IsNegative() {
		return ruleErr(ErrWithdrawalNegativeBalance, map[string]interface{}{"available_balance": balance})
	}
	if amount.GreaterThan(balance) {
		return ruleErr(ErrWithdrawalInsufficientBalance, map[string]interface{}{"available_balance": balance})
	}
	return nil
}

// ─── Giáo viên ─────────────────────────────────────────────────────────────

// Create — POST /api/wallet/teacher/withdrawals.
//
// Chống race (2 request rút cùng lúc của cùng giảng viên): mọi request phải khoá CÙNG 1 dòng
// teacher_profiles trước (LockTeacherProfile), rồi mới đọc yêu cầu đang mở + tính số dư BÊN TRONG
// transaction đó. Request thứ 2 chờ khoá, đọc lại sau khi request 1 commit, thấy yêu cầu vừa tạo →
// 409. Bỏ khoá thì cả 2 cùng đọc "chưa có yêu cầu" và cùng INSERT — xem test race Postgres thật.
func (s *WithdrawalService) Create(ctx context.Context, teacherID uuid.UUID, amount decimal.Decimal) (*dto.WithdrawalItem, error) {
	if err := validateWithdrawalAmount(amount, s.minAmount); err != nil {
		return nil, err
	}

	var created *model.InstructorPayout
	err := s.repo.Transaction(ctx, func(txRepo *repository.WithdrawalRepository, txWallet *repository.WalletRepository) error {
		profile, err := txRepo.LockTeacherProfile(ctx, teacherID)
		if errors.Is(err, repository.ErrWithdrawalRecordNotFound) {
			return ruleErr(ErrWithdrawalTeacherProfileRequired, nil)
		}
		if err != nil {
			return err
		}

		open, err := txRepo.FindOpenByTeacher(ctx, teacherID)
		if err != nil {
			return err
		}
		var openID *uuid.UUID
		if open != nil {
			openID = &open.ID
		}

		earnings, err := txWallet.GetTeacherEarnings(teacherID)
		if err != nil {
			return err
		}
		sums, err := txWallet.GetTeacherPayoutSums(teacherID)
		if err != nil {
			return err
		}

		if err := checkWithdrawalEligibility(profile, openID, availableBalance(earnings.TotalEarnings, sums), amount); err != nil {
			return err
		}

		// Snapshot thông tin ngân hàng vào chính yêu cầu: đổi bank info sau này không làm sai
		// lệch yêu cầu đã gửi.
		method := "bank_transfer"
		created = &model.InstructorPayout{
			InstructorID:      teacherID,
			Amount:            amount,
			Currency:          "VND",
			Status:            model.PayoutStatusPending,
			PaymentMethod:     &method,
			BankName:          profile.BankName,
			BankAccountNumber: profile.BankAccountNumber,
			BankAccountName:   profile.BankAccountName,
		}
		return txRepo.Create(ctx, created)
	})
	if err != nil {
		return nil, err
	}
	item := toWithdrawalItem(created)
	return &item, nil
}

// ListMine — chỉ yêu cầu của CHÍNH giáo viên (teacherID lấy từ token, không nhận từ client).
func (s *WithdrawalService) ListMine(ctx context.Context, teacherID uuid.UUID, status string, page, limit int) (*dto.WithdrawalListResponse, error) {
	rows, total, err := s.repo.List(ctx, repository.WithdrawalListFilter{TeacherID: &teacherID, Status: status, Page: page, Limit: limit})
	if err != nil {
		return nil, err
	}
	items := make([]dto.WithdrawalItem, 0, len(rows))
	for i := range rows {
		items = append(items, toWithdrawalItem(&rows[i]))
	}
	return &dto.WithdrawalListResponse{Items: items, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages(total, limit)}, nil
}

// ─── Admin ─────────────────────────────────────────────────────────────────

func (s *WithdrawalService) AdminList(ctx context.Context, teacherID *uuid.UUID, status string, page, limit int) (*dto.AdminWithdrawalListResponse, error) {
	rows, total, err := s.repo.List(ctx, repository.WithdrawalListFilter{TeacherID: teacherID, Status: status, Page: page, Limit: limit})
	if err != nil {
		return nil, err
	}

	ids := uniqueInstructorIDs(rows)
	balances, err := s.balancesFor(ids)
	if err != nil {
		return nil, err
	}

	items := make([]dto.AdminWithdrawalItem, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		items = append(items, dto.AdminWithdrawalItem{
			WithdrawalItem:          toWithdrawalItem(r),
			TeacherID:               r.InstructorID,
			TeacherName:             displayName(&r.Instructor),
			TeacherEmail:            r.Instructor.Email,
			TeacherAvailableBalance: balances[r.InstructorID],
		})
	}
	return &dto.AdminWithdrawalListResponse{Items: items, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages(total, limit)}, nil
}

// NegativeBalances — giảng viên có số dư âm (quyết định #8: admin thấy cảnh báo).
func (s *WithdrawalService) NegativeBalances(ctx context.Context) (*dto.NegativeBalanceListResponse, error) {
	ids, err := s.walletRepo.GetTeacherIDsWithReservedPayouts()
	if err != nil {
		return nil, err
	}
	balances, err := s.balancesFor(ids)
	if err != nil {
		return nil, err
	}

	var negativeIDs []uuid.UUID
	for _, id := range ids {
		if balances[id].IsNegative() {
			negativeIDs = append(negativeIDs, id)
		}
	}
	users, err := s.repo.FindUsersByIDs(ctx, negativeIDs)
	if err != nil {
		return nil, err
	}

	out := make([]dto.NegativeBalanceTeacher, 0, len(users))
	for i := range users {
		u := &users[i]
		out = append(out, dto.NegativeBalanceTeacher{
			TeacherID: u.ID, TeacherName: displayName(u), TeacherEmail: u.Email, AvailableBalance: balances[u.ID],
		})
	}
	return &dto.NegativeBalanceListResponse{Items: out}, nil
}

// balancesFor tính số dư khả dụng cho nhiều giảng viên bằng 2 câu gom nhóm (cùng công thức).
func (s *WithdrawalService) balancesFor(ids []uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	earnings, err := s.walletRepo.GetTeacherEarningsByIDs(ids)
	if err != nil {
		return nil, err
	}
	reserved, err := s.walletRepo.GetTeacherReservedByIDs(ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]decimal.Decimal, len(ids))
	for _, id := range ids {
		out[id] = earnings[id].Sub(reserved[id])
	}
	return out, nil
}

func (s *WithdrawalService) Approve(ctx context.Context, actorID, id uuid.UUID) (*dto.WithdrawalStatusResponse, error) {
	return s.transition(ctx, actorID, id, model.PayoutStatusPending, model.PayoutStatusApproved, nil)
}

func (s *WithdrawalService) Reject(ctx context.Context, actorID, id uuid.UUID, reason string) (*dto.WithdrawalStatusResponse, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrWithdrawalReasonRequired
	}
	now := time.Now()
	return s.transition(ctx, actorID, id, model.PayoutStatusPending, model.PayoutStatusRejected,
		map[string]interface{}{"rejection_reason": reason, "processed_at": now})
}

func (s *WithdrawalService) MarkCompleted(ctx context.Context, actorID, id uuid.UUID, transactionID string) (*dto.WithdrawalStatusResponse, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return nil, ErrWithdrawalTransactionIDRequired
	}
	now := time.Now()
	return s.transition(ctx, actorID, id, model.PayoutStatusApproved, model.PayoutStatusCompleted,
		map[string]interface{}{"transaction_id": transactionID, "processed_at": now})
}

// requiresNonNegativeBalance — bước chuyển nào đưa tiền ra khỏi nền tảng thì phải chặn khi số dư
// âm (quyết định #8). Từ chối (rejected) luôn được phép: nó chỉ trả tiền về số dư.
func requiresNonNegativeBalance(to string) bool {
	return to == model.PayoutStatusApproved || to == model.PayoutStatusCompleted
}

// checkPayoutBalance — chạy trong transaction, sau khi đã khoá hồ sơ GV. balance là số dư khả dụng
// HIỆN TẠI (đã trừ chính yêu cầu này, vì nó đang pending/approved) — đúng con số admin thấy trên
// danh sách. Âm nghĩa là doanh thu còn lại (sau hoàn tiền) không đủ trả yêu cầu này.
func checkPayoutBalance(balance, amount decimal.Decimal) error {
	if balance.IsNegative() {
		return ruleErr(ErrWithdrawalPayoutNegativeBalance, map[string]interface{}{"available_balance": balance, "amount": amount})
	}
	return nil
}

// transition — khoá dòng yêu cầu, chỉ cho đổi from→to đúng luồng
// pending→approved→completed / pending→rejected; mọi transition khác trả 409. Ghi admin thao tác
// vào notes làm dấu vết đối soát (bảng không có cột actor riêng).
//
// Duyệt/đánh dấu đã chuyển (review Phase 4, B-1): khoá teacher_profiles của GV TRƯỚC (cùng khoá mà
// Create và hoàn tiền dùng), rồi mới khoá yêu cầu và tính lại số dư. Nhờ vậy 1 lần hoàn tiền chạy
// song song hoặc đã commit trước (số dư giảm) hoặc phải chờ bước này xong; không có lúc admin duyệt
// dựa trên số dư đã cũ.
func (s *WithdrawalService) transition(ctx context.Context, actorID, id uuid.UUID, from, to string, extra map[string]interface{}) (*dto.WithdrawalStatusResponse, error) {
	err := s.repo.Transaction(ctx, func(txRepo *repository.WithdrawalRepository, txWallet *repository.WalletRepository) error {
		checkBalance := requiresNonNegativeBalance(to)
		if checkBalance {
			teacherID, err := txRepo.FindInstructorID(ctx, id)
			if errors.Is(err, repository.ErrWithdrawalRecordNotFound) {
				return ErrWithdrawalNotFound
			}
			if err != nil {
				return err
			}
			if _, err := txRepo.LockTeacherProfile(ctx, teacherID); err != nil && !errors.Is(err, repository.ErrWithdrawalRecordNotFound) {
				return err
			}
		}

		p, err := txRepo.LockByID(ctx, id)
		if errors.Is(err, repository.ErrWithdrawalRecordNotFound) {
			return ErrWithdrawalNotFound
		}
		if err != nil {
			return err
		}
		if p.Status != from {
			return ruleErr(ErrWithdrawalInvalidTransition, map[string]interface{}{"current_status": p.Status})
		}

		if checkBalance {
			earnings, err := txWallet.GetTeacherEarnings(p.InstructorID)
			if err != nil {
				return err
			}
			sums, err := txWallet.GetTeacherPayoutSums(p.InstructorID)
			if err != nil {
				return err
			}
			if err := checkPayoutBalance(availableBalance(earnings.TotalEarnings, sums), p.Amount); err != nil {
				return err
			}
		}

		fields := map[string]interface{}{"status": to, "notes": appendAuditNote(p.Notes, actorID, to)}
		for k, v := range extra {
			fields[k] = v
		}
		return txRepo.UpdateFields(ctx, id, fields)
	})
	if err != nil {
		return nil, err
	}
	return &dto.WithdrawalStatusResponse{ID: id, Status: to}, nil
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func appendAuditNote(existing *string, actorID uuid.UUID, to string) string {
	line := time.Now().UTC().Format(time.RFC3339) + " " + to + " by " + actorID.String()
	if existing == nil || *existing == "" {
		return line
	}
	return *existing + "\n" + line
}

func toWithdrawalItem(p *model.InstructorPayout) dto.WithdrawalItem {
	return dto.WithdrawalItem{
		ID: p.ID, Amount: p.Amount, Currency: p.Currency, Status: p.Status,
		RejectionReason: p.RejectionReason, TransactionID: p.TransactionID,
		BankName: p.BankName, BankAccountNumber: p.BankAccountNumber, BankAccountName: p.BankAccountName,
		CreatedAt: p.CreatedAt, ProcessedAt: p.ProcessedAt,
	}
}

func uniqueInstructorIDs(rows []model.InstructorPayout) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if _, ok := seen[r.InstructorID]; !ok {
			seen[r.InstructorID] = struct{}{}
			ids = append(ids, r.InstructorID)
		}
	}
	return ids
}

func displayName(u *model.User) string {
	if u.FullName != nil && strings.TrimSpace(*u.FullName) != "" {
		return *u.FullName
	}
	return u.UserName
}

func totalPages(total int64, limit int) int {
	if limit <= 0 {
		return 0
	}
	return int((total + int64(limit) - 1) / int64(limit))
}
