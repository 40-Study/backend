package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
)


func (s *AuthService) GetAllDevices(ctx context.Context, userID, currentDeviceID uuid.UUID) ([]dto.DeviceSessionDto, error) {
	
	// ===== 1. Get all sessions from Redis =====
	sessionKey := fmt.Sprintf("session:%s", userID)
	allSessions, err := s.redisClient.HGetAll(ctx, sessionKey).Result()
	
	if err == redis.Nil {
		return []dto.DeviceSessionDto{}, nil
	}
	if err != nil {
		return nil, err
	}

	// ===== 2. Parse sessions and build device list =====
	type deviceSession struct {
		DeviceID   uuid.UUID `json:"device_id"`
		DeviceName string    `json:"device_name"`
		UserAgent  string    `json:"user_agent"`
		LoggedInAt string    `json:"logged_in_at"`
	}

	var devices []dto.DeviceSessionDto
	for deviceIDStr, sessionData := range allSessions {
		var session deviceSession
		if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
			continue
		}

		device := dto.DeviceSessionDto{
			DeviceID:   deviceIDStr,
			DeviceName: session.DeviceName,
			UserAgent:  session.UserAgent,
			LoggedInAt: session.LoggedInAt,
		}

		// Mark current device
		if deviceIDStr == currentDeviceID.String() {
			device.IsCurrent = true
		}

		devices = append(devices, device)
	}

	return devices, nil
}
