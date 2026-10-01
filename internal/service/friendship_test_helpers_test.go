package service

// Helper dùng chung cho test Bạn bè (Postgres thật, schema tạm riêng qua pgtest.IsolatedSchema).

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

var errTest = errors.New("notifier down (test)")

// fakeFriendNotifier ghi lại thông báo thay vì gửi; err != nil để giả lập lỗi hạ tầng.
type fakeFriendNotifier struct {
	mu   sync.Mutex
	sent []dto.CreateNotificationDTO
	err  error
}

func (f *fakeFriendNotifier) SendNotification(req dto.CreateNotificationDTO) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, req)
	return f.err
}

func (f *fakeFriendNotifier) all() []dto.CreateNotificationDTO {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dto.CreateNotificationDTO(nil), f.sent...)
}

type friendFx struct {
	t     *testing.T
	db    *gorm.DB
	svc   *FriendshipService
	notif *fakeFriendNotifier
	seq   int
}

func newFriendFx(t *testing.T) *friendFx {
	t.Helper()
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := NewFriendshipService(repository.NewFriendshipRepository(db), repository.NewUserBlockRepository(db))
	notif := &fakeFriendNotifier{}
	svc.SetNotifier(notif)
	return &friendFx{t: t, db: db, svc: svc, notif: notif}
}

func (fx *friendFx) role(name string) uuid.UUID {
	fx.t.Helper()
	r := model.SystemRole{Name: name, Status: "active"}
	if err := fx.db.Where("name = ?", name).FirstOrCreate(&r).Error; err != nil {
		fx.t.Fatalf("tạo role %s: %v", name, err)
	}
	return r.ID
}

// user tạo tài khoản đang hoạt động mang các vai trò hệ thống (active) đã cho.
func (fx *friendFx) user(tag string, roles ...string) uuid.UUID {
	fx.t.Helper()
	fx.seq++
	id := uuid.NewString()
	full := "Ho Ten " + tag
	u := model.User{
		Email:        "fr-" + tag + "-" + id + "@40study.test",
		PasswordHash: "x",
		UserName:     tag + "-" + id[:6],
		FullName:     &full,
		IsActive:     true,
	}
	if err := fx.db.Create(&u).Error; err != nil {
		fx.t.Fatalf("tạo user %s: %v", tag, err)
	}
	for _, r := range roles {
		usr := model.UserSystemRole{UserID: u.ID, SystemRoleID: fx.role(r), Status: model.UserSystemRoleStatusActive}
		if err := fx.db.Create(&usr).Error; err != nil {
			fx.t.Fatalf("gán role %s: %v", r, err)
		}
	}
	return u.ID
}

func (fx *friendFx) student(tag string) uuid.UUID { return fx.user(tag, "STUDENT") }

// namedStudent tạo học viên với user_name/full_name cụ thể (cho test tìm kiếm).
func (fx *friendFx) namedStudent(userName, fullName string) uuid.UUID {
	fx.t.Helper()
	id := fx.student("n")
	if err := fx.db.Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{"user_name": userName, "full_name": fullName}).Error; err != nil {
		fx.t.Fatalf("đặt tên: %v", err)
	}
	return id
}

func (fx *friendFx) setVisibility(id uuid.UUID, vis string) {
	fx.t.Helper()
	if err := fx.db.Create(&model.UserPreference{UserID: id, ProfileVisibility: vis}).Error; err != nil {
		fx.t.Fatalf("đặt profile_visibility: %v", err)
	}
}

func (fx *friendFx) deactivate(id uuid.UUID) {
	fx.t.Helper()
	if err := fx.db.Model(&model.User{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		fx.t.Fatalf("khoá user: %v", err)
	}
}

// befriend tạo thẳng một tình bạn ACCEPTED (bỏ qua luồng gửi/chấp nhận, không tính hạn mức).
func (fx *friendFx) befriend(a, b uuid.UUID) {
	fx.t.Helper()
	now := time.Now()
	f := model.Friendship{RequesterID: a, AddresseeID: b, Status: model.FriendshipStatusAccepted, RequestedAt: now, RespondedAt: &now}
	if err := fx.db.Create(&f).Error; err != nil {
		fx.t.Fatalf("tạo bạn bè: %v", err)
	}
}

// rowOf đọc dòng friendships của cặp (nil nếu không có).
func (fx *friendFx) rowOf(a, b uuid.UUID) *model.Friendship {
	fx.t.Helper()
	var f model.Friendship
	err := fx.db.Where("(requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?)", a, b, b, a).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		fx.t.Fatalf("đọc dòng bạn bè: %v", err)
	}
	return &f
}

func (fx *friendFx) countRows(a, b uuid.UUID) int64 {
	fx.t.Helper()
	var n int64
	fx.db.Model(&model.Friendship{}).
		Where("(requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?)", a, b, b, a).Count(&n)
	return n
}

// send gửi lời mời và FAIL nếu lỗi; trả id lời mời.
func (fx *friendFx) send(from, to uuid.UUID) uuid.UUID {
	fx.t.Helper()
	out, err := fx.svc.SendRequest(fx.t.Context(), from, to)
	if err != nil {
		fx.t.Fatalf("SendRequest: %v", err)
	}
	return out.Result.ID
}

// age lùi thời điểm của dòng cặp (a,b) để thử cooldown/hạn mức 24h mà không phải chờ thật.
func (fx *friendFx) age(a, b uuid.UUID, by time.Duration) {
	fx.t.Helper()
	interval := fmt.Sprintf("%d seconds", int64(by.Seconds()))
	err := fx.db.Model(&model.Friendship{}).
		Where("(requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?)", a, b, b, a).
		Updates(map[string]any{
			"requested_at": gorm.Expr("requested_at - ?::interval", interval),
			"responded_at": gorm.Expr("responded_at - ?::interval", interval),
		}).Error
	if err != nil {
		fx.t.Fatalf("lùi thời gian: %v", err)
	}
}

func wantErr(t *testing.T, got, want error, what string) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Errorf("%s: muốn %v, nhận %v", what, want, got)
	}
}
