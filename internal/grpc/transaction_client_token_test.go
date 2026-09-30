package grpc

// S6: backend Go gửi token dùng chung tới service giao dịch Python qua metadata gRPC. Luồng mua xu gọi
// CheckTransaction qua client này nên test đi hết đường thật: client THẬT -> server gRPC thật trên cổng tạm.
// Bỏ interceptor (hoặc đổi khoá metadata) thì test "gửi token" ĐỎ; token rỗng phải KHÔNG gắn gì để tương thích
// service Python chưa bật xác thực.

import (
	"context"
	"net"
	"testing"
	"time"

	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type tokenSpyServer struct {
	UnimplementedTransactionServiceServer
	seen chan []string
}

func (s *tokenSpyServer) Health(ctx context.Context, _ *HealthRequest) (*HealthResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.seen <- md.Get(TokenMetadataKey)
	return &HealthResponse{Healthy: true}, nil
}

func startTokenSpy(t *testing.T) (addr string, seen chan []string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	spy := &tokenSpyServer{seen: make(chan []string, 4)}
	srv := grpclib.NewServer()
	RegisterTransactionServiceServer(srv, spy)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), spy.seen
}

func healthSeenToken(t *testing.T, token string) []string {
	t.Helper()
	addr, seen := startTokenSpy(t)
	client, err := NewTransactionClient(addr, token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	healthy, err := client.IsHealthy(ctx)
	if err != nil || !healthy {
		t.Fatalf("IsHealthy: healthy=%v err=%v", healthy, err)
	}
	select {
	case got := <-seen:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("server không nhận được lời gọi")
		return nil
	}
}

func TestTransactionClient_GuiTokenTrongMetadata(t *testing.T) {
	got := healthSeenToken(t, "s6-shared-secret")
	if len(got) != 1 || got[0] != "s6-shared-secret" {
		t.Fatalf("metadata %s = %v, muốn [s6-shared-secret]", TokenMetadataKey, got)
	}
}

func TestTransactionClient_TokenRongKhongGanMetadata(t *testing.T) {
	if got := healthSeenToken(t, ""); len(got) != 0 {
		t.Fatalf("token rỗng mà vẫn gửi metadata: %v", got)
	}
}
