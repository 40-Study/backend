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
	// danh sách dồn thành nhiều dòng "Chrome Browser" y hệt nhau (QA PG-L1). An toàn (review M3):
	//   - KHÔNG BAO GIỜ gộp vào dòng thiết bị hiện tại: hai máy cùng Chrome/OS gửi user_agent y hệt, phiên
	//     của máy thứ hai (có thể là kẻ lạ) phải hiện thành dòng riêng. Dòng hiện tại luôn chỉ là chính nó.
	//   - Dòng gộp báo rõ session_count và liệt kê MỌI device_id (device_ids) để thu hồi theo dòng chạm tới
	//     từng phiên của nhóm.
	//   - Không có user_agent (app di động, device_id ổn định theo lần cài) thì không gộp.
	type group struct {
		rep    dto.DeviceSessionDto
		latest time.Time
	}
	var order []string // khoá nhóm; sắp xếp trước khi xuất để kết quả không phụ thuộc thứ tự map
	groups := map[string]*group{}
	for deviceIDStr, sessionData := range allSessions {
		var session deviceSession
		if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
			continue
		}

		device := dto.DeviceSessionDto{
			DeviceID:     deviceIDStr,
			DeviceName:   session.DeviceName,
			UserAgent:    session.UserAgent,
			LoggedInAt:   session.LoggedInAt,
			IsCurrent:    deviceIDStr == currentDeviceID.String(),
			SessionCount: 1,
			DeviceIDs:    []string{deviceIDStr},
		}
		loggedAt, _ := time.Parse(time.RFC3339, session.LoggedInAt)

		key := deviceIDStr // thiết bị hiện tại / không UA -> mỗi device_id một nhóm riêng
		if session.UserAgent != "" && !device.IsCurrent {
			key = session.DeviceName + "\x00" + session.UserAgent
		}
		g, ok := groups[key]
		if !ok {
			groups[key] = &group{rep: device, latest: loggedAt}
			order = append(order, key)
			continue
		}
		g.rep.SessionCount++
		g.rep.DeviceIDs = append(g.rep.DeviceIDs, deviceIDStr)
		if loggedAt.After(g.latest) { // đại diện = phiên mới nhất của nhóm
			g.latest = loggedAt
			g.rep.DeviceID, g.rep.LoggedInAt = deviceIDStr, session.LoggedInAt
		}
	}

	sort.Strings(order)
	devices := make([]dto.DeviceSessionDto, 0, len(groups))
	for _, key := range order {
		sort.Strings(groups[key].rep.DeviceIDs)
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
