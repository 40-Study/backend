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

// CheckTransactionResult represents the result of checking a transaction
type CheckTransactionResult struct {
	Found           bool
	TransactionID   string
	Amount          string
	Currency        string
	Description     string
	TransactionDate string
	Status          string
	ErrorMessage    string
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

	return &CheckTransactionResult{
		Found:           resp.Found,
		TransactionID:   resp.TransactionId,
		Amount:          resp.Amount,
		Currency:        resp.Currency,
		Description:     resp.Description,
		TransactionDate: resp.TransactionDate,
		Status:          resp.Status,
		ErrorMessage:    resp.ErrorMessage,
	}, nil
}

// IsHealthy checks if the transaction service is healthy
func (c *TransactionClient) IsHealthy(ctx context.Context) (bool, error) {
	resp, err := c.client.Health(ctx, &HealthRequest{})
	if err != nil {
		return false, fmt.Errorf("failed to check health: %w", err)
	}
	return resp.Healthy, nil
}
