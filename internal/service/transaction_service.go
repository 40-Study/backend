package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"study.com/v1/internal/grpc"
)

// TransactionServiceInterface defines the interface for transaction service
type TransactionServiceInterface interface {
	CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error)
	GetAllTransactions(ctx context.Context, fromTime, toTime time.Time)
	IsHealthy(ctx context.Context) (bool, error)
}

// TransactionService calls the MBBank HTTP API
type TransactionService struct {
	baseURL    string
	httpClient *http.Client
}

// NewTransactionService creates a new transaction service with HTTP client
func NewTransactionService(host, port string) (*TransactionService, error) {
	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	return &TransactionService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// checkPinResponse matches the Python API response
type checkPinResponse struct {
	Success      bool                   `json:"success"`
	Found        bool                   `json:"found"`
	Pin          string                 `json:"pin"`
	MatchCount   int                    `json:"match_count"`
	Message      string                 `json:"message"`
	Transactions []transactionResponse  `json:"transactions"`
}

type transactionResponse struct {
	PostingDate      string `json:"posting_date"`
	TransactionDate  string `json:"transaction_date"`
	AccountNo        string `json:"account_no"`
	CreditAmount     string `json:"credit_amount"`
	DebitAmount      string `json:"debit_amount"`
	Currency         string `json:"currency"`
	Description      string `json:"description"`
	AddDescription   string `json:"add_description"`
	AvailableBalance string `json:"available_balance"`
	RefNo            string `json:"ref_no"`
	BenAccountName   string `json:"ben_account_name"`
	BankName         string `json:"bank_name"`
	TransactionType  string `json:"transaction_type"`
}

// formatDateForAPI converts time to hh-mm-ss-dd-mm-yyyy format
func formatDateForAPI(t time.Time) string {
	return t.Format("15-04-05-02-01-2006")
}

// CheckTransaction checks if a transaction exists with the given payment code via HTTP
func (s *TransactionService) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error) {
	endpoint := fmt.Sprintf("%s/transactions/check-pin", s.baseURL)

	params := url.Values{}
	params.Set("pin", paymentCode)
	params.Set("from_date", formatDateForAPI(fromTime))
	params.Set("to_date", formatDateForAPI(toTime))

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	log.Printf("🔍 Calling MB Bank API: pin=%s, from=%s, to=%s", paymentCode, formatDateForAPI(fromTime), formatDateForAPI(toTime))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call transaction API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transaction API returned status %d", resp.StatusCode)
	}

	var result checkPinResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("📋 MB Bank API Response: found=%v, match_count=%d, message=%s", result.Found, result.MatchCount, result.Message)
	for i, tx := range result.Transactions {
		log.Printf("   [%d] Amount: %s, Desc: %s, AddDesc: %s, Date: %s", i+1, tx.CreditAmount, tx.Description, tx.AddDescription, tx.TransactionDate)
	}

	if !result.Found || len(result.Transactions) == 0 {
		return &grpc.CheckTransactionResult{
			Found:        false,
			Status:       "not_found",
			ErrorMessage: "No transaction found with this payment code",
		}, nil
	}

	trans := result.Transactions[0]
	return &grpc.CheckTransactionResult{
		Found:           true,
		TransactionID:   trans.RefNo,
		Amount:          trans.CreditAmount,
		Currency:        "VND",
		Description:     trans.Description,
		TransactionDate: trans.TransactionDate,
		Status:          "success",
	}, nil
}

// IsHealthy checks if the transaction service is healthy
func (s *TransactionService) IsHealthy(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/docs", nil)
	if err != nil {
		return false, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Close is a no-op for HTTP client
func (s *TransactionService) Close() error {
	return nil
}

// GetAllTransactions - Debug: get all transactions in time range
func (s *TransactionService) GetAllTransactions(ctx context.Context, fromTime, toTime time.Time) {
	endpoint := fmt.Sprintf("%s/transactions", s.baseURL)

	params := url.Values{}
	params.Set("from_date", formatDateForAPI(fromTime))
	params.Set("to_date", formatDateForAPI(toTime))

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		log.Printf("❌ Failed to create request: %v", err)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("❌ Failed to call API: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Success          bool                  `json:"success"`
		TransactionCount int                   `json:"transaction_count"`
		Transactions     []transactionResponse `json:"transactions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("❌ Failed to decode: %v", err)
		return
	}

	log.Printf("📋 ALL TRANSACTIONS in range (%s -> %s): count=%d", formatDateForAPI(fromTime), formatDateForAPI(toTime), result.TransactionCount)
	for i, tx := range result.Transactions {
		log.Printf("   [%d] Credit: %s | Desc: %s | AddDesc: %s", i+1, tx.CreditAmount, tx.Description, tx.AddDescription)
	}
}
