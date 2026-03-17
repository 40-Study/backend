package service

import (
	"context"
	"fmt"
	"time"

	"study.com/v1/internal/grpc"
)

// TransactionServiceInterface defines the interface for transaction service
type TransactionServiceInterface interface {
	CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error)
	IsHealthy(ctx context.Context) (bool, error)
}

// TransactionService is a wrapper around the gRPC transaction client
type TransactionService struct {
	client *grpc.TransactionClient
}

// NewTransactionService creates a new transaction service
func NewTransactionService(host, port string) (*TransactionService, error) {
	addr := fmt.Sprintf("%s:%s", host, port)
	client, err := grpc.NewTransactionClient(addr)
	if err != nil {
		return nil, err
	}

	return &TransactionService{
		client: client,
	}, nil
}

// CheckTransaction checks if a transaction exists with the given payment code
func (s *TransactionService) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error) {
	return s.client.CheckTransaction(ctx, paymentCode, fromTime, toTime)
}

// IsHealthy checks if the transaction service is healthy
func (s *TransactionService) IsHealthy(ctx context.Context) (bool, error) {
	return s.client.IsHealthy(ctx)
}

// Close closes the transaction service connection
func (s *TransactionService) Close() error {
	return s.client.Close()
}
