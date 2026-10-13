package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
)


func (s *AuthService) GetAllDevices(ctx context.Context, userID, currentDeviceID uuid.UUID) ([]dto.DeviceSessionDto, error) {
	
	// ===== 1. Get all sessions from Redis =====
	sessionKey := constants.KeySession(userID.String())
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

	// Gộp theo dấu vân tay (device_name + user_agent): session lưu theo device_id do CLIENT sinh nên mỗi
	// trình duyệt/profile mới (localStorage trống) là một device_id mới, và không có TTL theo từng dòng —
	// danh sách dồn thành nhiều dòng "Chrome Browser" y hệt nhau (QA PG-L1). Không có user_agent (app di
	// động, device_id ổn định theo lần cài) thì KHÔNG gộp: hai máy cùng kiểu máy là hai thiết bị thật.
	type group struct {
		rep    dto.DeviceSessionDto
		latest time.Time
	}
	var order []string // giữ thứ tự xuất hiện của khoá để kết quả không phụ thuộc thứ tự map
	groups := map[string]*group{}
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
			IsCurrent:  deviceIDStr == currentDeviceID.String(),
		}
		loggedAt, _ := time.Parse(time.RFC3339, session.LoggedInAt)

		key := deviceIDStr // không UA -> mỗi device_id một nhóm riêng
		if session.UserAgent != "" {
			key = session.DeviceName + "\x00" + session.UserAgent
		}
		g, ok := groups[key]
		if !ok {
			groups[key] = &group{rep: device, latest: loggedAt}
			order = append(order, key)
			continue
		}
		// Đại diện nhóm: thiết bị hiện tại nếu có, ngược lại phiên mới nhất; giờ hiển thị luôn là mới nhất nhóm.
		newestAt := g.latest
		if loggedAt.After(newestAt) {
			newestAt = loggedAt
		}
		switch {
		case device.IsCurrent:
			g.rep = device
		case !g.rep.IsCurrent && loggedAt.After(g.latest):
			g.rep = device
		}
		g.latest = newestAt
		if !newestAt.IsZero() {
			g.rep.LoggedInAt = newestAt.Format(time.RFC3339)
		}
	}

	sort.Strings(order)
	devices := make([]dto.DeviceSessionDto, 0, len(groups))
	for _, key := range order {
		devices = append(devices, groups[key].rep)
	}
	// Thiết bị hiện tại trước, rồi mới đăng nhập nhất trước.
	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].IsCurrent != devices[j].IsCurrent {
			return devices[i].IsCurrent
		}
		ti, _ := time.Parse(time.RFC3339, devices[i].LoggedInAt)
		tj, _ := time.Parse(time.RFC3339, devices[j].LoggedInAt)
		return ti.After(tj)
	})

	return devices, nil
}
