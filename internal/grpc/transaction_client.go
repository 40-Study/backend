package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TransactionClient is a client for the TransactionService gRPC service
type TransactionClient struct {
	conn   *grpc.ClientConn
	client TransactionServiceClient
	addr   string
}

// NewTransactionClient creates a new transaction gRPC client
func NewTransactionClient(addr string) (*TransactionClient, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to transaction service: %w", err)
	}

	return &TransactionClient{
		conn:   conn,
		client: NewTransactionServiceClient(conn),
		addr:   addr,
	}, nil
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
