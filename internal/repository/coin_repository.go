package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type CoinWalletRepositoryInterface interface {
	Create(ctx context.Context, wallet *model.UserCoinWallet) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*model.UserCoinWallet, error)
	GetOrCreate(ctx context.Context, userID uuid.UUID) (*model.UserCoinWallet, error)
	AddBalance(ctx context.Context, walletID uuid.UUID, amount int64) error
	SubtractBalance(ctx context.Context, walletID uuid.UUID, amount int64) error
}

type CoinTransactionRepositoryInterface interface {
	Create(ctx context.Context, tx *model.CoinTransaction) error
	ListByUserID(ctx context.Context, userID uuid.UUID, txType string, page, pageSize int) ([]model.CoinTransaction, int64, error)
}

type CoinPackageRepositoryInterface interface {
	Create(ctx context.Context, pkg *model.CoinPackage) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.CoinPackage, error)
	Update(ctx context.Context, pkg *model.CoinPackage) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListActive(ctx context.Context) ([]model.CoinPackage, error)
}

type CoinPurchaseRepositoryInterface interface {
	Create(ctx context.Context, purchase *model.CoinPurchase) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.CoinPurchase, error)
	Update(ctx context.Context, purchase *model.CoinPurchase) error
	ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.CoinPurchase, int64, error)
}

// ============================================================================
// COIN WALLET REPOSITORY
// ============================================================================

type CoinWalletRepository struct {
	db *gorm.DB
}

func NewCoinWalletRepository(db *gorm.DB) *CoinWalletRepository {
	return &CoinWalletRepository{db: db}
}

func (r *CoinWalletRepository) Create(ctx context.Context, wallet *model.UserCoinWallet) error {
	return r.db.WithContext(ctx).Create(wallet).Error
}

func (r *CoinWalletRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.UserCoinWallet, error) {
	var wallet model.UserCoinWallet
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &wallet, nil
}

func (r *CoinWalletRepository) GetOrCreate(ctx context.Context, userID uuid.UUID) (*model.UserCoinWallet, error) {
	wallet, err := r.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if wallet != nil {
		return wallet, nil
	}

	newWallet := &model.UserCoinWallet{
		UserID:  userID,
		Balance: 0,
	}
	if err := r.Create(ctx, newWallet); err != nil {
		return nil, err
	}
	return newWallet, nil
}

func (r *CoinWalletRepository) AddBalance(ctx context.Context, walletID uuid.UUID, amount int64) error {
	return r.db.WithContext(ctx).Model(&model.UserCoinWallet{}).
		Where("id = ?", walletID).
		Updates(map[string]interface{}{
			"balance":      gorm.Expr("balance + ?", amount),
			"total_earned": gorm.Expr("total_earned + ?", amount),
		}).Error
}

func (r *CoinWalletRepository) SubtractBalance(ctx context.Context, walletID uuid.UUID, amount int64) error {
	result := r.db.WithContext(ctx).Model(&model.UserCoinWallet{}).
		Where("id = ? AND balance >= ?", walletID, amount).
		Updates(map[string]interface{}{
			"balance":     gorm.Expr("balance - ?", amount),
			"total_spent": gorm.Expr("total_spent + ?", amount),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("insufficient balance")
	}
	return nil
}

// ============================================================================
// COIN TRANSACTION REPOSITORY
// ============================================================================

type CoinTransactionRepository struct {
	db *gorm.DB
}

func NewCoinTransactionRepository(db *gorm.DB) *CoinTransactionRepository {
	return &CoinTransactionRepository{db: db}
}

func (r *CoinTransactionRepository) Create(ctx context.Context, tx *model.CoinTransaction) error {
	return r.db.WithContext(ctx).Create(tx).Error
}

func (r *CoinTransactionRepository) ListByUserID(ctx context.Context, userID uuid.UUID, txType string, page, pageSize int) ([]model.CoinTransaction, int64, error) {
	var transactions []model.CoinTransaction
	var total int64

	query := r.db.WithContext(ctx).Model(&model.CoinTransaction{}).Where("user_id = ?", userID)

	if txType != "" {
		query = query.Where("type = ?", txType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("created_at DESC").Find(&transactions).Error
	return transactions, total, err
}

// ============================================================================
// COIN PACKAGE REPOSITORY
// ============================================================================

type CoinPackageRepository struct {
	db *gorm.DB
}

func NewCoinPackageRepository(db *gorm.DB) *CoinPackageRepository {
	return &CoinPackageRepository{db: db}
}

func (r *CoinPackageRepository) Create(ctx context.Context, pkg *model.CoinPackage) error {
	return r.db.WithContext(ctx).Create(pkg).Error
}

func (r *CoinPackageRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.CoinPackage, error) {
	var pkg model.CoinPackage
	err := r.db.WithContext(ctx).First(&pkg, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &pkg, nil
}

func (r *CoinPackageRepository) Update(ctx context.Context, pkg *model.CoinPackage) error {
	return r.db.WithContext(ctx).Save(pkg).Error
}

func (r *CoinPackageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.CoinPackage{}, "id = ?", id).Error
}

func (r *CoinPackageRepository) ListActive(ctx context.Context) ([]model.CoinPackage, error) {
	var packages []model.CoinPackage
	err := r.db.WithContext(ctx).
		Where("is_active = true").
		Order("sort_order ASC, price ASC").
		Find(&packages).Error
	return packages, err
}

// ============================================================================
// COIN PURCHASE REPOSITORY
// ============================================================================

type CoinPurchaseRepository struct {
	db *gorm.DB
}

func NewCoinPurchaseRepository(db *gorm.DB) *CoinPurchaseRepository {
	return &CoinPurchaseRepository{db: db}
}

func (r *CoinPurchaseRepository) Create(ctx context.Context, purchase *model.CoinPurchase) error {
	return r.db.WithContext(ctx).Create(purchase).Error
}

func (r *CoinPurchaseRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.CoinPurchase, error) {
	var purchase model.CoinPurchase
	err := r.db.WithContext(ctx).
		Preload("Package").
		First(&purchase, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &purchase, nil
}

func (r *CoinPurchaseRepository) Update(ctx context.Context, purchase *model.CoinPurchase) error {
	return r.db.WithContext(ctx).Save(purchase).Error
}

func (r *CoinPurchaseRepository) ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.CoinPurchase, int64, error) {
	var purchases []model.CoinPurchase
	var total int64

	query := r.db.WithContext(ctx).Model(&model.CoinPurchase{}).
		Where("user_id = ?", userID).
		Preload("Package")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("created_at DESC").Find(&purchases).Error
	return purchases, total, err
}
