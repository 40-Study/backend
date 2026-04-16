package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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
}

func NewCoinService(
	walletRepo *repository.CoinWalletRepository,
	txRepo *repository.CoinTransactionRepository,
	packageRepo *repository.CoinPackageRepository,
	purchaseRepo *repository.CoinPurchaseRepository,
) *CoinService {
	return &CoinService{
		walletRepo:   walletRepo,
		txRepo:       txRepo,
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
	purchase := &model.CoinPurchase{
		UserID:        userID,
		PackageID:     &req.PackageID,
		CoinAmount:    pkg.CoinAmount,
		BonusAmount:   pkg.BonusAmount,
		Price:         pkg.Price,
		Currency:      pkg.Currency,
		Status:        model.CoinPurchasePending,
		PaymentMethod: &paymentMethod,
	}

	if err := s.purchaseRepo.Create(ctx, purchase); err != nil {
		return nil, err
	}

	return toCoinPurchaseResponse(purchase), nil
}

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
	if purchase.Status != model.CoinPurchasePending {
		return nil, errors.New("purchase is not pending")
	}

	// Mark as completed
	purchase.Status = model.CoinPurchaseCompleted

	// Get or create wallet
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}

	totalCoins := purchase.CoinAmount + purchase.BonusAmount
	if err := s.walletRepo.AddBalance(ctx, wallet.ID, totalCoins); err != nil {
		return nil, err
	}

	// Refresh wallet for balance after
	wallet, err = s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	desc := fmt.Sprintf("Purchased %d coins", totalCoins)
	tx := &model.CoinTransaction{
		WalletID:      wallet.ID,
		UserID:        userID,
		Type:          model.CoinTxEarnPurchase,
		Amount:        totalCoins,
		BalanceAfter:  wallet.Balance,
		ReferenceType: strPtr("coin_purchase"),
		ReferenceID:   &purchaseID,
		Description:   &desc,
	}
	if err := s.txRepo.Create(ctx, tx); err != nil {
		return nil, err
	}

	purchase.TransactionID = &tx.ID
	if err := s.purchaseRepo.Update(ctx, purchase); err != nil {
		return nil, err
	}

	return toCoinPurchaseResponse(purchase), nil
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

	senderWallet, err := s.walletRepo.GetOrCreate(ctx, senderID)
	if err != nil {
		return nil, err
	}

	if senderWallet.Balance < req.Amount {
		return nil, errors.New("insufficient balance")
	}

	receiverWallet, err := s.walletRepo.GetOrCreate(ctx, req.ReceiverID)
	if err != nil {
		return nil, err
	}

	// Deduct from sender
	if err := s.walletRepo.SubtractBalance(ctx, senderWallet.ID, req.Amount); err != nil {
		return nil, err
	}

	// Add to receiver
	if err := s.walletRepo.AddBalance(ctx, receiverWallet.ID, req.Amount); err != nil {
		return nil, err
	}

	// Refresh wallets
	senderWallet, _ = s.walletRepo.GetByUserID(ctx, senderID)
	receiverWallet, _ = s.walletRepo.GetByUserID(ctx, req.ReceiverID)

	// Create transaction records
	sendDesc := fmt.Sprintf("Sent %d coins as gift", req.Amount)
	recvDesc := fmt.Sprintf("Received %d coins as gift", req.Amount)

	senderTx := &model.CoinTransaction{
		WalletID:     senderWallet.ID,
		UserID:       senderID,
		Type:         model.CoinTxSpendGift,
		Amount:       -req.Amount,
		BalanceAfter: senderWallet.Balance,
		ReferenceType: strPtr("gift"),
		ReferenceID:   &req.ReceiverID,
		Description:   &sendDesc,
	}
	_ = s.txRepo.Create(ctx, senderTx)

	receiverTx := &model.CoinTransaction{
		WalletID:     receiverWallet.ID,
		UserID:       req.ReceiverID,
		Type:         model.CoinTxReceiveGift,
		Amount:       req.Amount,
		BalanceAfter: receiverWallet.Balance,
		ReferenceType: strPtr("gift"),
		ReferenceID:   &senderID,
		Description:   &recvDesc,
	}
	_ = s.txRepo.Create(ctx, receiverTx)

	return &dto.CoinGiftResponse{
		SenderID:        senderID,
		ReceiverID:      req.ReceiverID,
		Amount:          req.Amount,
		Message:         req.Message,
		SenderBalance:   senderWallet.Balance,
		ReceiverBalance: receiverWallet.Balance,
	}, nil
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
		Price:           req.Price,
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
		pkg.Price = *req.Price
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
