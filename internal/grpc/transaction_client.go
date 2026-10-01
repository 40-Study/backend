package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// TokenMetadataKey là khoá metadata gRPC mang secret dùng chung với service Python (grpc_server.py).
const TokenMetadataKey = "x-transaction-token"

// TransactionClient is a client for the TransactionService gRPC service
type TransactionClient struct {
	conn   *grpc.ClientConn
	client TransactionServiceClient
	addr   string
}

// NewTransactionClient creates a new transaction gRPC client.
//
// token (S6): secret dùng chung với service giao dịch, gắn vào metadata của MỌI lời gọi. token rỗng thì
// không gắn gì — giữ nguyên hành vi cũ để bật xác thực theo từng bước mà không làm hỏng luồng thanh toán
// (đặt token ở backend Go trước, rồi mới bật ở service Python).
func NewTransactionClient(addr string, token string) (*TransactionClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token != "" {
		opts = append(opts, grpc.WithUnaryInterceptor(tokenInterceptor(token)))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to transaction service: %w", err)
	}

	return &TransactionClient{
		conn:   conn,
		client: NewTransactionServiceClient(conn),
		addr:   addr,
	}, nil
}

// tokenInterceptor gắn token vào metadata gửi đi của mọi RPC unary.
func tokenInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, TokenMetadataKey, token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// Close closes the gRPC connection
func (c *TransactionClient) Close() error {
	return c.conn.Close()
}

// BankTransaction — một giao dịch ghi có khớp mã thanh toán.
type BankTransaction struct {
	TransactionID   string
	Amount          string
	Currency        string
	Description     string
	TransactionDate string
}

// CheckTransactionResult represents the result of checking a transaction.
//
// TransactionID/Amount/Currency/Description/TransactionDate là giao dịch khớp ĐẦU TIÊN (hợp đồng cũ,
// coin_service vẫn dùng). Transactions là MỌI giao dịch khớp (L1): khách có thể chuyển nhiều lần vào
// cùng một mã, và lần thứ hai trở đi trước đây không bao giờ tới được Go nên luồng cờ hoàn tiền muộn
// không bật. Đọc qua AllTransactions để tương thích service Python cũ chưa trả danh sách.
type CheckTransactionResult struct {
	Found           bool
	TransactionID   string
	Amount          string
	Currency        string
	Description     string
	TransactionDate string
	Status          string
	ErrorMessage    string
	Transactions    []BankTransaction
}

// AllTransactions trả mọi giao dịch khớp theo thứ tự sao kê. Service Python cũ không gửi danh sách:
// khi đó (Found=true) dựng một phần tử từ các field đầu tiên, nên caller không phải rẽ nhánh theo phiên
// bản server. Found=false thì rỗng.
func (r *CheckTransactionResult) AllTransactions() []BankTransaction {
	if r == nil || !r.Found {
		return nil
	}
	if len(r.Transactions) > 0 {
		return r.Transactions
	}
	return []BankTransaction{{
		TransactionID:   r.TransactionID,
		Amount:          r.Amount,
		Currency:        r.Currency,
		Description:     r.Description,
		TransactionDate: r.TransactionDate,
	}}
}

// CheckTransaction checks if a transaction exists with the given payment code
func (c *TransactionClient) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*CheckTransactionResult, error) {
	req := &CheckTransactionRequest{
		PaymentCode:   paymentCode,
		FromTimestamp: fromTime.Unix(),
		ToTimestamp:   toTime.Unix(),
	}

	resp, err := c.client.CheckTransaction(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to check transaction: %w", err)
	}

	result := &CheckTransactionResult{
		Found:           resp.Found,
		TransactionID:   resp.TransactionId,
		Amount:          resp.Amount,
		Currency:        resp.Currency,
		Description:     resp.Description,
		TransactionDate: resp.TransactionDate,
		Status:          resp.Status,
		ErrorMessage:    resp.ErrorMessage,
	}
	for _, t := range resp.Transactions {
		if t == nil {
			continue
		}
		result.Transactions = append(result.Transactions, BankTransaction{
			TransactionID:   t.TransactionId,
			Amount:          t.Amount,
			Currency:        t.Currency,
			Description:     t.Description,
			TransactionDate: t.TransactionDate,
		})
	}
	return result, nil
}

// IsHealthy checks if the transaction service is healthy
func (c *TransactionClient) IsHealthy(ctx context.Context) (bool, error) {
	resp, err := c.client.Health(ctx, &HealthRequest{})
	if err != nil {
		return false, fmt.Errorf("failed to check health: %w", err)
	}
	return resp.Healthy, nil
}
