package grpc

// L1: CheckTransaction trả MỌI giao dịch khớp qua field 9 (repeated MatchedTransaction). Đi hết đường
// thật: client THẬT -> server gRPC thật trên cổng tạm, nên field/tag sai (hoặc bỏ qua danh sách) làm
// test đỏ. Server cũ (không gửi field 9) vẫn phải cho đúng một giao dịch qua AllTransactions.

import (
	"context"
	"net"
	"testing"
	"time"

	grpclib "google.golang.org/grpc"
)

type checkStubServer struct {
	UnimplementedTransactionServiceServer
	resp *CheckTransactionResponse
}

func (s *checkStubServer) CheckTransaction(context.Context, *CheckTransactionRequest) (*CheckTransactionResponse, error) {
	return s.resp, nil
}

func checkVia(t *testing.T, resp *CheckTransactionResponse) *CheckTransactionResult {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpclib.NewServer()
	RegisterTransactionServiceServer(srv, &checkStubServer{resp: resp})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	client, err := NewTransactionClient(lis.Addr().String(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := client.CheckTransaction(ctx, "40STUDY ORD1", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("CheckTransaction: %v", err)
	}
	return got
}

func TestCheckTransaction_ReturnsEveryMatchedTransaction(t *testing.T) {
	got := checkVia(t, &CheckTransactionResponse{
		Found: true, Status: "success", TransactionId: "A", Amount: "100", Currency: "VND", Description: "d1", TransactionDate: "01/10/2026 10:00:00",
		Transactions: []*MatchedTransaction{
			{TransactionId: "A", Amount: "100", Currency: "VND", Description: "d1", TransactionDate: "01/10/2026 10:00:00"},
			{TransactionId: "B", Amount: "200", Currency: "VND", Description: "d2", TransactionDate: "02/10/2026 11:00:00"},
		},
	})
	if got.TransactionID != "A" || got.Amount != "100" {
		t.Fatalf("field đơn lẻ = %s/%s, muốn giao dịch đầu A/100", got.TransactionID, got.Amount)
	}
	all := got.AllTransactions()
	if len(all) != 2 || all[0].TransactionID != "A" || all[1].TransactionID != "B" || all[1].Amount != "200" || all[1].TransactionDate != "02/10/2026 11:00:00" {
		t.Fatalf("AllTransactions = %+v, muốn [A, B] đúng thứ tự và đủ field", all)
	}
}

func TestCheckTransaction_OldServerWithoutListYieldsSingleTransaction(t *testing.T) {
	got := checkVia(t, &CheckTransactionResponse{Found: true, Status: "success", TransactionId: "ONLY", Amount: "300", TransactionDate: "03/10/2026 12:00:00"})
	all := got.AllTransactions()
	if len(all) != 1 || all[0].TransactionID != "ONLY" || all[0].Amount != "300" {
		t.Fatalf("AllTransactions = %+v, muốn đúng 1 giao dịch dựng từ field đơn lẻ", all)
	}
}

func TestAllTransactions_NotFoundIsEmpty(t *testing.T) {
	got := checkVia(t, &CheckTransactionResponse{Found: false, Status: "not_found"})
	if all := got.AllTransactions(); len(all) != 0 {
		t.Fatalf("Found=false mà AllTransactions = %+v", all)
	}
	var nilResult *CheckTransactionResult
	if len(nilResult.AllTransactions()) != 0 {
		t.Fatalf("nil result phải rỗng")
	}
}
