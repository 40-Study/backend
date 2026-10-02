package service

import "github.com/google/uuid"

// Cài đặt riêng tư "Hiển thị trên bảng xếp hạng" (user_preferences.leaderboard_display).
const (
	LeaderboardDisplayName      = "name"      // tên thật (mặc định, và mọi giá trị không nhận ra)
	LeaderboardDisplayUsername  = "username"  // chỉ tên đăng nhập, không hiện họ tên
	LeaderboardDisplayAnonymous = "anonymous" // ẩn danh hoàn toàn

	// AnonymousLearnerLabel là tên hiển thị thay cho người chọn ẩn danh.
	AnonymousLearnerLabel = "Học viên ẩn danh"
)

// LeaderboardViewer là người đang xem bảng xếp hạng; nil = khách chưa đăng nhập. Chính chủ và admin luôn thấy
// thông tin thật của mục tương ứng, bất kể cài đặt (cùng luật với GetPublicProfile ở S6).
type LeaderboardViewer struct {
	UserID  uuid.UUID
	IsAdmin bool
}

// leaderboardPerson là phần danh tính của một dòng xếp hạng mà người xem được phép thấy.
type leaderboardPerson struct {
	UserID      *uuid.UUID // nil khi ẩn danh: id dẫn tới hồ sơ (/users/:id/public-profile) nên không được lộ
	UserName    string
	FullName    *string
	AvatarURL   *string
	DisplayName string
	IsMe        bool
}

// presentLeaderboardUser áp cài đặt `display` của chủ dòng (ownerID) cho người xem. Là hàm thuần để mọi bảng xếp
// hạng (điểm thưởng, cuộc thi) dùng chung MỘT luật, thay vì mỗi nơi tự lọc.
func presentLeaderboardUser(display string, ownerID uuid.UUID, fullName *string, userName string, avatar *string, viewer *LeaderboardViewer) leaderboardPerson {
	isMe := viewer != nil && viewer.UserID == ownerID
	revealAll := isMe || (viewer != nil && viewer.IsAdmin)

	showFullName := true
	switch {
	case revealAll:
	case display == LeaderboardDisplayAnonymous:
		return leaderboardPerson{DisplayName: AnonymousLearnerLabel}
	case display == LeaderboardDisplayUsername:
		showFullName = false
	}

	id := ownerID
	p := leaderboardPerson{UserID: &id, UserName: userName, AvatarURL: avatar, DisplayName: userName, IsMe: isMe}
	if showFullName {
		p.FullName = fullName
		if fullName != nil && *fullName != "" {
			p.DisplayName = *fullName
		}
	}
	return p
}
