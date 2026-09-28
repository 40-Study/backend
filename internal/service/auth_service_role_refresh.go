package service

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

// Phase 3 duyệt giáo viên — quyết định #6 của chủ dự án: duyệt có hiệu lực NGAY, vô hiệu phiên
// cũ (user_version) nhưng web TỰ làm mới token để người dùng thấy khu giáo viên mà không phải
// đăng xuất.
//
// Vì sao không dùng revokeAllSessions: hàm đó xoá luôn refresh token (auth:refresh:{uid}) và
// RefreshToken từ chối mọi token có user_version lệch — tức người dùng BẮT BUỘC đăng nhập lại,
// trái quyết định #6. Ở đây chỉ INCR user_version (mọi access token cũ 401 ngay) và ghi lại giá
// trị version mới vào marker auth:role_changed_version:{uid}. RefreshToken chấp nhận version lệch
// CHỈ KHI marker == user_version hiện tại, tức lần bump GẦN NHẤT chính là đổi vai trò. Nếu sau đó
// có đăng xuất mọi nơi / khoá tài khoản / đổi mật khẩu (revokeAllSessions INCR tiếp) thì marker
// lệch version -> refresh bị từ chối như cũ; refresh token vẫn phải khớp bản lưu trong Redis
// (bước 3 của RefreshToken), nên cơ chế này không mở thêm đường nào cho token đã bị thu hồi.

// MarkRoleChanged bump user_version + ghi marker đổi vai trò. Gọi SAU khi transaction gán/gỡ
// role đã commit.
func (s *AuthService) MarkRoleChanged(ctx context.Context, userID uuid.UUID) error {
	if s.redisClient == nil {
		return errors.New("redis not configured: cannot invalidate sessions after role change")
	}
	newVersion, err := s.redisClient.Incr(ctx, constants.KeyUserVersion(userID.String())).Result()
	if err != nil {
		return err
	}
	return s.redisClient.Set(ctx, constants.KeyRoleChanged(userID.String()), newVersion, s.cfg.JWTRefreshExpiration).Err()
}

// isRoleChangeRefresh: token có version CŨ hơn hiện tại VÀ lần bump gần nhất là đổi vai trò.
func (s *AuthService) isRoleChangeRefresh(ctx context.Context, userID uuid.UUID, tokenVersion, currentVersion int64) (bool, error) {
	if tokenVersion >= currentVersion {
		return false, nil
	}
	markerStr, err := s.redisClient.Get(ctx, constants.KeyRoleChanged(userID.String())).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	marker, convErr := strconv.ParseInt(markerStr, 10, 64)
	if convErr != nil {
		return false, nil
	}
	return marker == currentVersion, nil
}

// resolveActiveRoleAfterRoleChange chọn active_role cho token mới: giữ role cũ nếu user vẫn còn
// giữ; role tổ chức (có active_org) không bị luồng này đụng tới nên giữ nguyên; còn lại (vd
// TEACHER_APPLICANT vừa bị gỡ) chọn role ưu tiên cao nhất đang có (determineEntryContext —
// TEACHER đứng trên STUDENT).
func (s *AuthService) resolveActiveRoleAfterRoleChange(ctx context.Context, claims *utils.Claims) (string, error) {
	if claims.ActiveOrgID != nil {
		return claims.ActiveRole, nil
	}
	roles, err := s.GetMySystemRoles(ctx, claims.UserID)
	if err != nil {
		return "", err
	}
	for _, r := range roles {
		if r.Name == claims.ActiveRole {
			return claims.ActiveRole, nil
		}
	}
	entry := s.determineEntryContext(roles)
	if entry == nil || entry.PrimaryRole == "" {
		return "", errors.New("no active role left - please login again")
	}
	return entry.PrimaryRole, nil
}

// refreshTokensAfterRoleChange sinh cặp token mới (version hiện tại, active_role đã tính lại) —
// phần còn lại của RefreshToken (lưu refresh token vào Redis) dùng chung đường cũ.
func (s *AuthService) refreshTokensAfterRoleChange(ctx context.Context, claims *utils.Claims, currentVersion int64) (*dto.RefreshTokenResponseDto, error) {
	activeRole, err := s.resolveActiveRoleAfterRoleChange(ctx, claims)
	if err != nil {
		return nil, err
	}
	access, refresh, err := utils.GenerateTokens(s.cfg, claims.UserID, claims.DeviceID, activeRole, claims.ActiveOrgID, currentVersion)
	if err != nil {
		return nil, err
	}
	return &dto.RefreshTokenResponseDto{
		AccessToken:  access,
		RefreshToken: refresh,
		RoleChanged:  true,
		ActiveRole:   activeRole,
	}, nil
}
