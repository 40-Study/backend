package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type CoinServiceInterface interface {
	// Wallet
	GetWallet(ctx context.Context, userID uuid.UUID) (*dto.CoinWalletResponse, error)
	GetTransactions(ctx context.Context, userID uuid.UUID, txType string, page, pageSize int) (*dto.CoinTransactionListResponse, error)

	// Packages
	ListPackages(ctx context.Context) ([]dto.CoinPackageResponse, error)
	GetPackage(ctx context.Context, id uuid.UUID) (*dto.CoinPackageResponse, error)

	// Purchases
	CreatePurchase(ctx context.Context, userID uuid.UUID, req dto.CreateCoinPurchaseRequest) (*dto.CoinPurchaseResponse, error)
	VerifyPurchase(ctx context.Context, userID, purchaseID uuid.UUID) (*dto.CoinPurchaseResponse, error)
	ListPurchases(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CoinPurchaseListResponse, error)
	GetPurchase(ctx context.Context, userID, purchaseID uuid.UUID) (*dto.CoinPurchaseResponse, error)

	// Gift
	SendGift(ctx context.Context, senderID uuid.UUID, req dto.SendCoinGiftRequest) (*dto.CoinGiftResponse, error)

	// Admin
	CreatePackage(ctx context.Context, req dto.CreateCoinPackageRequest) (*dto.CoinPackageResponse, error)
	UpdatePackage(ctx context.Context, id uuid.UUID, req dto.UpdateCoinPackageRequest) (*dto.CoinPackageResponse, error)
	DeletePackage(ctx context.Context, id uuid.UUID) error
	AdminAdjust(ctx context.Context, req dto.AdminAdjustCoinRequest) (*dto.CoinWalletResponse, error)
}

type CoinService struct {
	walletRepo  *repository.CoinWalletRepository
	txRepo      *repository.CoinTransactionRepository
	packageRepo *repository.CoinPackageRepository
	purchaseRepo *repository.CoinPurchaseRepository
	// transactionService xác minh giao dịch chuyển khoản thật qua gRPC (dùng chung
	// cơ chế với đơn hàng khoá học, xem payment_service.go CheckAndProcessPayment).
	// C4: VerifyPurchase chỉ cộng xu khi có giao dịch NGÂN HÀNG THẬT khớp mã + số tiền,
	// không tin tưởng user tự bấm "verify". nil nếu gRPC service không khởi tạo được.
	transactionService TransactionServiceInterface
}

func NewCoinService(
	walletRepo *repository.CoinWalletRepository,
	txRepo *repository.CoinTransactionRepository,
	packageRepo *repository.CoinPackageRepository,
	purchaseRepo *repository.CoinPurchaseRepository,
	transactionService TransactionServiceInterface,
) *CoinService {
	return &CoinService{
		walletRepo:   walletRepo,
		txRepo:       txRepo,
		transactionService: transactionService,
		packageRepo:  packageRepo,
		purchaseRepo: purchaseRepo,
	}
}

// ============================================================================
// WALLET
// ============================================================================

func (s *CoinService) GetWallet(ctx context.Context, userID uuid.UUID) (*dto.CoinWalletResponse, error) {
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &dto.CoinWalletResponse{
		ID:          wallet.ID,
		UserID:      wallet.UserID,
		Balance:     wallet.Balance,
		TotalEarned: wallet.TotalEarned,
		TotalSpent:  wallet.TotalSpent,
		CreatedAt:   wallet.CreatedAt,
		UpdatedAt:   wallet.UpdatedAt,
	}, nil
}

func (s *CoinService) GetTransactions(ctx context.Context, userID uuid.UUID, txType string, page, pageSize int) (*dto.CoinTransactionListResponse, error) {
	transactions, total, err := s.txRepo.ListByUserID(ctx, userID, txType, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.CoinTransactionResponse, len(transactions))
	for i, t := range transactions {
		responses[i] = dto.CoinTransactionResponse{
			ID:            t.ID,
			Type:          string(t.Type),
			Amount:        t.Amount,
			BalanceAfter:  t.BalanceAfter,
			ReferenceType: t.ReferenceType,
			ReferenceID:   t.ReferenceID,
			Description:   t.Description,
			CreatedAt:     t.CreatedAt,
		}
	}

	return &dto.CoinTransactionListResponse{
		Transactions: responses,
		TotalCount:   total,
		Page:         page,
		Limit:        pageSize,
	}, nil
}

// ============================================================================
// PACKAGES
// ============================================================================

func (s *CoinService) ListPackages(ctx context.Context) ([]dto.CoinPackageResponse, error) {
	packages, err := s.packageRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.CoinPackageResponse, len(packages))
	for i, p := range packages {
		responses[i] = toCoinPackageResponse(p)
	}
	return responses, nil
}

func (s *CoinService) GetPackage(ctx context.Context, id uuid.UUID) (*dto.CoinPackageResponse, error) {
	pkg, err := s.packageRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, errors.New("package not found")
	}

	resp := toCoinPackageResponse(*pkg)
	return &resp, nil
}

// ============================================================================
// PURCHASES
// ============================================================================

func (s *CoinService) CreatePurchase(ctx context.Context, userID uuid.UUID, req dto.CreateCoinPurchaseRequest) (*dto.CoinPurchaseResponse, error) {
	pkg, err := s.packageRepo.GetByID(ctx, req.PackageID)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, errors.New("package not found")
	}
	if !pkg.IsActive {
		return nil, errors.New("package is not available")
	}

	paymentMethod := req.PaymentMethod
	// C4: sinh mã thanh toán để user ghi vào nội dung chuyển khoản; VerifyPurchase
	// sẽ đối chiếu mã này với giao dịch ngân hàng thật (không tự cộng xu vô điều kiện).
	paymentCode := generateCoinPaymentCode()
	purchase := &model.CoinPurchase{
		UserID:           userID,
		PackageID:        &req.PackageID,
		CoinAmount:       pkg.CoinAmount,
		BonusAmount:      pkg.BonusAmount,
		Price:            pkg.Price,
		Currency:         pkg.Currency,
		Status:           model.CoinPurchasePending,
		PaymentMethod:    &paymentMethod,
		PaymentReference: &paymentCode,
	}

	if err := s.purchaseRepo.Create(ctx, purchase); err != nil {
		return nil, err
	}

	return toCoinPurchaseResponse(purchase), nil
}

// VerifyPurchase kiểm tra giao dịch ngân hàng thật qua gRPC transaction service
// (cùng cơ chế order dùng để xác nhận thanh toán, xem payment_service.go
// CheckAndProcessPayment) rồi mới cộng xu — KHÔNG tin tưởng lời gọi của user.
//
// C4(c): trước khi sửa, hàm này cộng xu ngay khi user tự gọi API, không xác minh
// thanh toán gì cả → bất kỳ user nào cũng tự cộng xu miễn phí không giới hạn.
func (s *CoinService) VerifyPurchase(ctx context.Context, userID, purchaseID uuid.UUID) (*dto.CoinPurchaseResponse, error) {
	purchase, err := s.purchaseRepo.GetByID(ctx, purchaseID)
	if err != nil {
		return nil, err
	}
	if purchase == nil {
		return nil, errors.New("purchase not found")
	}
	if purchase.UserID != userID {
		return nil, errors.New("purchase does not belong to you")
	}
	if purchase.Status == model.CoinPurchaseCompleted {
		// Idempotent: gọi verify nhiều lần sau khi đã xác nhận không cộng xu thêm lần nữa.
		return toCoinPurchaseResponse(purchase), nil
	}
	if purchase.Status != model.CoinPurchasePending {
		return nil, errors.New("purchase is not pending")
	}
	if s.transactionService == nil {
		return nil, errors.New("payment verification service unavailable")
	}
	if purchase.PaymentReference == nil || *purchase.PaymentReference == "" {
		return nil, errors.New("purchase has no payment code")
	}

	toTime := time.Now()
	fromTime := toTime.Add(-24 * time.Hour)
	result, err := s.transactionService.CheckTransaction(ctx, *purchase.PaymentReference, fromTime, toTime)
	if err != nil {
		return nil, fmt.Errorf("failed to check transaction: %w", err)
	}
	if !result.Found {
		// Chưa thấy giao dịch chuyển khoản khớp mã — trả về trạng thái pending hiện
		// tại (không phải lỗi) để client có thể poll lại, KHÔNG cộng xu.
		return toCoinPurchaseResponse(purchase), nil
	}

	paidAmount, err := decimal.NewFromString(result.Amount)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction amount from bank: %w", err)
	}
	if !paidAmount.Equal(purchase.Price) {
		return nil, errors.New("payment amount mismatch")
	}

	totalCoins := purchase.CoinAmount + purchase.BonusAmount

	// C3/C4: cộng xu + ghi ledger + đánh dấu purchase completed atomic trong 1
	// transaction, có khoá dòng ví để tránh race condition.
	err = s.walletRepo.WithTransaction(func(tx *gorm.DB) error {
		wallet, err := lockOrCreateWalletTx(tx, userID)
		if err != nil {
			return err
		}

		if err := tx.Model(&model.UserCoinWallet{}).Where("id = ?", wallet.ID).
			Updates(map[string]interface{}{
				"balance":      gorm.Expr("balance + ?", totalCoins),
				"total_earned": gorm.Expr("total_earned + ?", totalCoins),
			}).Error; err != nil {
			return err
		}
		if err := tx.First(wallet, "id = ?", wallet.ID).Error; err != nil {
			return err
		}

		desc := fmt.Sprintf("Purchased %d coins (bank txn %s)", totalCoins, result.TransactionID)
		ledgerTx := &model.CoinTransaction{
			WalletID:      wallet.ID,
			UserID:        userID,
			Type:          model.CoinTxEarnPurchase,
			Amount:        totalCoins,
			BalanceAfter:  wallet.Balance,
			ReferenceType: strPtr("coin_purchase"),
			ReferenceID:   &purchaseID,
			Description:   &desc,
		}
		if err := tx.Create(ledgerTx).Error; err != nil {
			return err
		}

		now := time.Now()
		purchase.Status = model.CoinPurchaseCompleted
		purchase.TransactionID = &ledgerTx.ID
		purchase.CompletedAt = &now
		if err := tx.Save(purchase).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return toCoinPurchaseResponse(purchase), nil
}

// generateCoinPaymentCode sinh mã thanh toán duy nhất cho 1 lượt mua xu, dùng làm
// nội dung chuyển khoản để đối chiếu với giao dịch ngân hàng thật (mirror cách
// payment_service.go generatePaymentCode() làm cho đơn hàng khoá học).
func generateCoinPaymentCode() string {
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	return fmt.Sprintf("COIN%s%s", time.Now().Format("060102150405"), hex.EncodeToString(randomBytes)[:8])
}

func (s *CoinService) ListPurchases(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.CoinPurchaseListResponse, error) {
	purchases, total, err := s.purchaseRepo.ListByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.CoinPurchaseResponse, len(purchases))
	for i, p := range purchases {
		responses[i] = *toCoinPurchaseResponse(&p)
	}

	return &dto.CoinPurchaseListResponse{
		Purchases:  responses,
		TotalCount: total,
		Page:       page,
		Limit:      pageSize,
	}, nil
}

func (s *CoinService) GetPurchase(ctx context.Context, userID, purchaseID uuid.UUID) (*dto.CoinPurchaseResponse, error) {
	purchase, err := s.purchaseRepo.GetByID(ctx, purchaseID)
	if err != nil {
		return nil, err
	}
	if purchase == nil {
		return nil, errors.New("purchase not found")
	}
	if purchase.UserID != userID {
		return nil, errors.New("purchase does not belong to you")
	}

	return toCoinPurchaseResponse(purchase), nil
}

// ============================================================================
// GIFT
// ============================================================================

func (s *CoinService) SendGift(ctx context.Context, senderID uuid.UUID, req dto.SendCoinGiftRequest) (*dto.CoinGiftResponse, error) {
	if senderID == req.ReceiverID {
		return nil, errors.New("cannot send gift to yourself")
	}
	if req.Amount <= 0 {
		return nil, errors.New("amount xu tặng phải > 0")
	}

	var senderBalanceAfter, receiverBalanceAfter int64

	// C3: toàn bộ chuyển xu (trừ ví gửi, cộng ví nhận, ghi 2 dòng ledger) chạy
	// trong 1 DB transaction + khoá dòng (SELECT ... FOR UPDATE) trên cả 2 ví,
	// tránh mất xu nếu 1 bước fail giữa chừng và tránh race condition khi nhiều
	// giao dịch cùng sửa 1 ví đồng thời (không còn `_ = ...Create(...)` nuốt lỗi).
	err := s.walletRepo.WithTransaction(func(tx *gorm.DB) error {
		// Khoá 2 ví theo thứ tự UUID cố định để tránh deadlock khi 2 lượt tặng
		// xu ngược chiều (A→B và B→A) chạy song song và khoá theo thứ tự khác nhau.
		firstID, secondID := senderID, req.ReceiverID
		if secondID.String() < firstID.String() {
			firstID, secondID = secondID, firstID
		}
		first, err := lockOrCreateWalletTx(tx, firstID)
		if err != nil {
			return err
		}
		second, err := lockOrCreateWalletTx(tx, secondID)
		if err != nil {
			return err
		}

		var senderWallet, receiverWallet *model.UserCoinWallet
		if firstID == senderID {
			senderWallet, receiverWallet = first, second
		} else {
			senderWallet, receiverWallet = second, first
		}

		if senderWallet.Balance < req.Amount {
			return errors.New("insufficient balance")
		}

		if err := tx.Model(&model.UserCoinWallet{}).Where("id = ?", senderWallet.ID).
			Updates(map[string]interface{}{
				"balance":     gorm.Expr("balance - ?", req.Amount),
				"total_spent": gorm.Expr("total_spent + ?", req.Amount),
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.UserCoinWallet{}).Where("id = ?", receiverWallet.ID).
			Updates(map[string]interface{}{
				"balance":      gorm.Expr("balance + ?", req.Amount),
				"total_earned": gorm.Expr("total_earned + ?", req.Amount),
			}).Error; err != nil {
			return err
		}

		// Đọc lại số dư sau cập nhật để ghi ledger balance_after chính xác.
		if err := tx.First(senderWallet, "id = ?", senderWallet.ID).Error; err != nil {
			return err
		}
		if err := tx.First(receiverWallet, "id = ?", receiverWallet.ID).Error; err != nil {
			return err
		}

		sendDesc := fmt.Sprintf("Sent %d coins as gift", req.Amount)
		recvDesc := fmt.Sprintf("Received %d coins as gift", req.Amount)

		senderTx := &model.CoinTransaction{
			WalletID:      senderWallet.ID,
			UserID:        senderID,
			Type:          model.CoinTxSpendGift,
			Amount:        -req.Amount,
			BalanceAfter:  senderWallet.Balance,
			ReferenceType: strPtr("gift"),
			ReferenceID:   &req.ReceiverID,
			Description:   &sendDesc,
		}
		if err := tx.Create(senderTx).Error; err != nil {
			return err
		}

		receiverTx := &model.CoinTransaction{
			WalletID:      receiverWallet.ID,
			UserID:        req.ReceiverID,
			Type:          model.CoinTxReceiveGift,
			Amount:        req.Amount,
			BalanceAfter:  receiverWallet.Balance,
			ReferenceType: strPtr("gift"),
			ReferenceID:   &senderID,
			Description:   &recvDesc,
		}
		if err := tx.Create(receiverTx).Error; err != nil {
			return err
		}

		senderBalanceAfter = senderWallet.Balance
		receiverBalanceAfter = receiverWallet.Balance
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &dto.CoinGiftResponse{
		SenderID:        senderID,
		ReceiverID:      req.ReceiverID,
		Amount:          req.Amount,
		Message:         req.Message,
		SenderBalance:   senderBalanceAfter,
		ReceiverBalance: receiverBalanceAfter,
	}, nil
}

// lockOrCreateWalletTx tìm ví xu theo user_id trong 1 transaction đang chạy (tx),
// khoá dòng bằng SELECT ... FOR UPDATE để tránh race condition khi 2 giao dịch
// cùng sửa 1 ví song song; tạo ví mới (balance = 0) nếu user chưa có ví.
func lockOrCreateWalletTx(tx *gorm.DB, userID uuid.UUID) (*model.UserCoinWallet, error) {
	var wallet model.UserCoinWallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).First(&wallet).Error
	if err == nil {
		return &wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	wallet = model.UserCoinWallet{UserID: userID}
	if err := tx.Create(&wallet).Error; err != nil {
		return nil, err
	}
	return &wallet, nil
}

// ============================================================================
// ADMIN
// ============================================================================

func (s *CoinService) CreatePackage(ctx context.Context, req dto.CreateCoinPackageRequest) (*dto.CoinPackageResponse, error) {
	currency := req.Currency
	if currency == "" {
		currency = "VND"
	}

	pkg := &model.CoinPackage{
		Name:            req.Name,
		Description:     req.Description,
		CoinAmount:      req.CoinAmount,
		BonusAmount:     req.BonusAmount,
		Price:           decimal.NewFromFloat(req.Price),
		Currency:        currency,
		DiscountPercent: req.DiscountPercent,
		IsFeatured:      req.IsFeatured,
		IsActive:        true,
		SortOrder:       req.SortOrder,
	}

	if err := s.packageRepo.Create(ctx, pkg); err != nil {
		return nil, err
	}

	resp := toCoinPackageResponse(*pkg)
	return &resp, nil
}

func (s *CoinService) UpdatePackage(ctx context.Context, id uuid.UUID, req dto.UpdateCoinPackageRequest) (*dto.CoinPackageResponse, error) {
	pkg, err := s.packageRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, errors.New("package not found")
	}

	if req.Name != nil {
		pkg.Name = *req.Name
	}
	if req.Description != nil {
		pkg.Description = req.Description
	}
	if req.CoinAmount != nil {
		pkg.CoinAmount = *req.CoinAmount
	}
	if req.BonusAmount != nil {
		pkg.BonusAmount = *req.BonusAmount
	}
	if req.Price != nil {
		pkg.Price = decimal.NewFromFloat(*req.Price)
	}
	if req.DiscountPercent != nil {
		pkg.DiscountPercent = *req.DiscountPercent
	}
	if req.IsFeatured != nil {
		pkg.IsFeatured = *req.IsFeatured
	}
	if req.IsActive != nil {
		pkg.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		pkg.SortOrder = *req.SortOrder
	}

	if err := s.packageRepo.Update(ctx, pkg); err != nil {
		return nil, err
	}

	resp := toCoinPackageResponse(*pkg)
	return &resp, nil
}

func (s *CoinService) DeletePackage(ctx context.Context, id uuid.UUID) error {
	pkg, err := s.packageRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if pkg == nil {
		return errors.New("package not found")
	}
	return s.packageRepo.Delete(ctx, id)
}

func (s *CoinService) AdminAdjust(ctx context.Context, req dto.AdminAdjustCoinRequest) (*dto.CoinWalletResponse, error) {
	wallet, err := s.walletRepo.GetOrCreate(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	if req.Amount > 0 {
		if err := s.walletRepo.AddBalance(ctx, wallet.ID, req.Amount); err != nil {
			return nil, err
		}
	} else if req.Amount < 0 {
		if err := s.walletRepo.SubtractBalance(ctx, wallet.ID, -req.Amount); err != nil {
			return nil, err
		}
	}

	wallet, err = s.walletRepo.GetByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	tx := &model.CoinTransaction{
		WalletID:     wallet.ID,
		UserID:       req.UserID,
		Type:         model.CoinTxAdminAdjust,
		Amount:       req.Amount,
		BalanceAfter: wallet.Balance,
		Description:  &req.Description,
	}
	_ = s.txRepo.Create(ctx, tx)

	return &dto.CoinWalletResponse{
		ID:          wallet.ID,
		UserID:      wallet.UserID,
		Balance:     wallet.Balance,
		TotalEarned: wallet.TotalEarned,
		TotalSpent:  wallet.TotalSpent,
		CreatedAt:   wallet.CreatedAt,
		UpdatedAt:   wallet.UpdatedAt,
	}, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func toCoinPackageResponse(p model.CoinPackage) dto.CoinPackageResponse {
	return dto.CoinPackageResponse{
		ID:              p.ID,
		Name:            p.Name,
		Description:     p.Description,
		CoinAmount:      p.CoinAmount,
		BonusAmount:     p.BonusAmount,
		TotalCoins:      p.CoinAmount + p.BonusAmount,
		Price:           p.Price,
		Currency:        p.Currency,
		DiscountPercent: p.DiscountPercent,
		IsFeatured:      p.IsFeatured,
		SortOrder:       p.SortOrder,
	}
}

func toCoinPurchaseResponse(p *model.CoinPurchase) *dto.CoinPurchaseResponse {
	return &dto.CoinPurchaseResponse{
		ID:               p.ID,
		UserID:           p.UserID,
		PackageID:        p.PackageID,
		CoinAmount:       p.CoinAmount,
		BonusAmount:      p.BonusAmount,
		Price:            p.Price,
		Currency:         p.Currency,
		Status:           string(p.Status),
		PaymentMethod:    p.PaymentMethod,
		PaymentReference: p.PaymentReference,
		CompletedAt:      p.CompletedAt,
		CreatedAt:        p.CreatedAt,
	}
}

func strPtr(s string) *string {
	return &s
}
