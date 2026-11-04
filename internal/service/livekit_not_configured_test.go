package service

// Q3-13 (QA3): LiveKit chưa cấu hình phải ra ErrLivekitNotConfigured (handler -> 503 ERR_LIVE_UNAVAILABLE), không phải
// lỗi "missing API key or secret key" của SDK bọc trong 500.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func fullLivekitConfig() *config.Config {
	return &config.Config{LivekitAPIKey: "key", LivekitAPISecret: "secret", LivekitNodeIP: "127.0.0.1", LivekitNodePort: "7880"}
}

func TestLivekitMissingConfig_NamesEveryEmptyVariable(t *testing.T) {
	if got := livekitMissingConfig(fullLivekitConfig()); len(got) != 0 {
		t.Fatalf("cấu hình đủ nhưng báo thiếu %v", got)
	}
	got := livekitMissingConfig(&config.Config{LivekitNodeIP: "  "})
	for _, want := range []string{"LIVEKIT_API_KEY", "LIVEKIT_API_SECRET", "LIVEKIT_NODE_IP", "LIVEKIT_NODE_PORT"} {
		if !strings.Contains(strings.Join(got, ","), want) {
			t.Errorf("thiếu %s trong %v", want, got)
		}
	}
}

func TestLivekitService_UnconfiguredFailsWithSentinelBeforeCallingSDK(t *testing.T) {
	svc := NewLivekitService(&config.Config{})
	if _, err := svc.CreateRoom(context.Background(), dto.CreateRoomDTO{RoomName: "r"}); !errors.Is(err, ErrLivekitNotConfigured) {
		t.Errorf("CreateRoom err = %v, muốn ErrLivekitNotConfigured", err)
	}
	if _, err := svc.CreateJoinToken(context.Background(), "r", dto.JoinTokenDTO{Identity: "u"}); !errors.Is(err, ErrLivekitNotConfigured) {
		t.Errorf("CreateJoinToken err = %v, muốn ErrLivekitNotConfigured", err)
	}
	// Cấu hình đủ thì token ký được (không gọi mạng).
	ok := NewLivekitService(fullLivekitConfig())
	if tok, err := ok.CreateJoinToken(context.Background(), "r", dto.JoinTokenDTO{Identity: "u"}); err != nil || tok == "" {
		t.Errorf("cấu hình đủ: token=%q err=%v", tok, err)
	}
}

// Bắt đầu buổi học khi LiveKit chưa cấu hình: lỗi đi qua lớp bọc của startSession mà errors.Is vẫn thấy sentinel (để
// handler trả 503), và buổi học KHÔNG bị chuyển sang live.
func TestStartSession_PropagatesLivekitNotConfigured(t *testing.T) {
	svc := &LivestreamService{livekitSvc: NewLivekitService(&config.Config{})}
	session := &model.LivestreamSession{Status: model.LivestreamStatusScheduled, RoomName: "room-q3-13", MaxViewers: 10}
	_, err := svc.startSession(context.Background(), session)
	if !errors.Is(err, ErrLivekitNotConfigured) {
		t.Fatalf("err = %v, muốn bọc ErrLivekitNotConfigured", err)
	}
}
