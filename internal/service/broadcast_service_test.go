package service

// Contract C3 (phase 4): thông báo hệ thống tới mọi người dùng hoặc theo vai trò. Audience/phân trang keyset chạy trên
// Postgres THẬT (schema tạm riêng của pgtest.IsolatedSchema, nên đếm "all" chính xác dù lane khác dùng chung DB);
// chỉ notifier là bản giả để ghi lại từng lô.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

type fakeBroadcastSender struct {
	calls  []dto.CreateNotificationDTO
	failAt int // gọi thứ failAt (đếm từ 1) trả lỗi; 0 = không lỗi
}

func (f *fakeBroadcastSender) SendNotification(req dto.CreateNotificationDTO) error {
	if f.failAt > 0 && len(f.calls)+1 == f.failAt {
		return errors.New("db down")
	}
	f.calls = append(f.calls, req)
	return nil
}

func (f *fakeBroadcastSender) delivered() map[uuid.UUID]int {
	out := map[uuid.UUID]int{}
	for _, c := range f.calls {
		for _, id := range c.UserIDs {
			out[id]++
		}
	}
	return out
}

func broadcastDB(t *testing.T) *gorm.DB {
	t.Helper()
	return pgtest.IsolatedSchema(t, database.Migrate)
}

func newBroadcastSvc(db *gorm.DB, sender broadcastSender) *BroadcastService {
	return NewBroadcastService(repository.NewBroadcastAudienceRepository(db), sender)
}

func broadcastUsers(t *testing.T, db *gorm.DB, n int) []model.User {
	t.Helper()
	users := make([]model.User, n)
	for i := range users {
		s := uuid.NewString()
		users[i] = model.User{Email: "bc-" + s + "@40study.test", PasswordHash: "x", UserName: "BC" + s[:8]}
	}
	if err := db.CreateInBatches(&users, 200).Error; err != nil {
		t.Fatalf("tạo user: %v", err)
	}
	return users
}

// lockBroadcastUser khoá tài khoản bằng Update (cột is_active có default:true nên Create với false bị bỏ qua).
func lockBroadcastUser(t *testing.T, db *gorm.DB, id uuid.UUID) {
	t.Helper()
	if err := db.Model(&model.User{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		t.Fatalf("khoá user: %v", err)
	}
}

func broadcastRole(t *testing.T, db *gorm.DB, name string) model.SystemRole {
	t.Helper()
	r := model.SystemRole{Name: name, Status: "active"}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("tạo role: %v", err)
	}
	return r
}

func grantBroadcastRole(t *testing.T, db *gorm.DB, userID, roleID uuid.UUID, status string) {
	t.Helper()
	usr := model.UserSystemRole{UserID: userID, SystemRoleID: roleID, Status: model.UserSystemRoleStatusActive}
	if err := db.Create(&usr).Error; err != nil {
		t.Fatalf("gán role: %v", err)
	}
	if status != model.UserSystemRoleStatusActive {
		if err := db.Model(&usr).Update("status", status).Error; err != nil {
			t.Fatalf("đổi trạng thái gán role: %v", err)
		}
	}
}

func broadcastReq(audience string, roles ...string) dto.BroadcastRequestDTO {
	return dto.BroadcastRequestDTO{Title: "Bảo trì", Content: "Hệ thống bảo trì lúc 22h", Audience: audience, Roles: roles}
}

func requireBroadcastCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var be *BroadcastError
	if !errors.As(err, &be) {
		t.Fatalf("muốn *BroadcastError %s, được %v", code, err)
	}
	if be.Status != status || be.Code != code {
		t.Fatalf("muốn %d %s, được %d %s", status, code, be.Status, be.Code)
	}
}

func TestBroadcast_AudienceAll_BoQuaTaiKhoanBiKhoa(t *testing.T) {
	db := broadcastDB(t)
	users := broadcastUsers(t, db, 4)
	lockBroadcastUser(t, db, users[3].ID)
	sender := &fakeBroadcastSender{}
	svc := newBroadcastSvc(db, sender)

	prev, err := svc.Preview(context.Background(), dto.BroadcastPreviewRequestDTO{Audience: "all"})
	if err != nil || prev.RecipientCount != 3 {
		t.Fatalf("preview = %+v, %v; muốn 3 (user bị khoá không tính)", prev, err)
	}
	res, err := svc.Send(context.Background(), broadcastReq("all"))
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got := sender.delivered()
	if res.RecipientCount != 3 || len(got) != 3 {
		t.Fatalf("recipient_count=%d, người nhận=%d; muốn 3", res.RecipientCount, len(got))
	}
	if _, locked := got[users[3].ID]; locked {
		t.Fatal("tài khoản bị khoá vẫn nhận thông báo")
	}
	if res.Audience != "all" || res.NotificationType != "system" || res.Roles == nil || len(res.Roles) != 0 {
		t.Fatalf("kết quả = %+v; muốn audience=all, type=system, roles=[] (không nil)", res)
	}
}

func TestBroadcast_AudienceRoles_ChiVaiTroDuocChonVaGanActive(t *testing.T) {
	db := broadcastDB(t)
	users := broadcastUsers(t, db, 6)
	student, teacher := broadcastRole(t, db, "BC_STUDENT"), broadcastRole(t, db, "BC_TEACHER")
	grantBroadcastRole(t, db, users[0].ID, student.ID, "active")
	grantBroadcastRole(t, db, users[1].ID, student.ID, "active")
	grantBroadcastRole(t, db, users[1].ID, teacher.ID, "active")   // giữ cả hai vai trò: chỉ nhận MỘT lần
	grantBroadcastRole(t, db, users[2].ID, teacher.ID, "active")   // vai trò khác
	grantBroadcastRole(t, db, users[3].ID, student.ID, "inactive") // vai trò đã bị gỡ
	grantBroadcastRole(t, db, users[4].ID, student.ID, "active")
	lockBroadcastUser(t, db, users[4].ID) // đúng vai trò nhưng tài khoản bị khoá
	// users[5] không có vai trò nào

	sender := &fakeBroadcastSender{}
	svc := newBroadcastSvc(db, sender)
	res, err := svc.Send(context.Background(), broadcastReq("roles", "BC_STUDENT", " BC_STUDENT "))
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got := sender.delivered()
	want := map[uuid.UUID]int{users[0].ID: 1, users[1].ID: 1}
	if len(got) != len(want) || got[users[0].ID] != 1 || got[users[1].ID] != 1 {
		t.Fatalf("người nhận = %v; muốn đúng user 0 và 1, mỗi người một lần", got)
	}
	if res.RecipientCount != 2 || len(res.Roles) != 1 || res.Roles[0] != "BC_STUDENT" {
		t.Fatalf("kết quả = %+v; muốn 2 người, roles khử trùng = [BC_STUDENT]", res)
	}

	sender2 := &fakeBroadcastSender{}
	res2, err := newBroadcastSvc(db, sender2).Send(context.Background(), broadcastReq("roles", "BC_STUDENT", "BC_TEACHER"))
	if err != nil || res2.RecipientCount != 3 {
		t.Fatalf("hợp hai vai trò = %+v, %v; muốn 3 người (user 0,1,2)", res2, err)
	}
	prev, err := svc.Preview(context.Background(), dto.BroadcastPreviewRequestDTO{Audience: "roles", Roles: []string{"BC_TEACHER"}})
	if err != nil || prev.RecipientCount != 2 {
		t.Fatalf("preview BC_TEACHER = %+v, %v; muốn 2", prev, err)
	}
}

func TestBroadcast_UnknownRole_400(t *testing.T) {
	db := broadcastDB(t)
	broadcastUsers(t, db, 2)
	broadcastRole(t, db, "BC_REAL")
	sender := &fakeBroadcastSender{}
	svc := newBroadcastSvc(db, sender)

	_, err := svc.Send(context.Background(), broadcastReq("roles", "BC_REAL", "BC_GHOST"))
	requireBroadcastCode(t, err, 400, "UNKNOWN_ROLE")
	if !strings.Contains(err.Error(), "BC_GHOST") || strings.Contains(err.Error(), "BC_REAL") {
		t.Fatalf("thông điệp phải nêu đúng vai trò không tồn tại: %q", err.Error())
	}
	_, err = svc.Preview(context.Background(), dto.BroadcastPreviewRequestDTO{Audience: "roles", Roles: []string{"BC_GHOST"}})
	requireBroadcastCode(t, err, 400, "UNKNOWN_ROLE")
	if len(sender.calls) != 0 {
		t.Fatal("vai trò không tồn tại mà vẫn gửi thông báo")
	}
}

func TestBroadcast_NoRecipients_422(t *testing.T) {
	db := broadcastDB(t)
	users := broadcastUsers(t, db, 2)
	for _, u := range users {
		lockBroadcastUser(t, db, u.ID)
	}
	broadcastRole(t, db, "BC_EMPTY")
	sender := &fakeBroadcastSender{}
	svc := newBroadcastSvc(db, sender)

	_, err := svc.Send(context.Background(), broadcastReq("all"))
	requireBroadcastCode(t, err, 422, "NO_RECIPIENTS")
	_, err = svc.Send(context.Background(), broadcastReq("roles", "BC_EMPTY"))
	requireBroadcastCode(t, err, 422, "NO_RECIPIENTS")
	if len(sender.calls) != 0 {
		t.Fatalf("không có người nhận mà SendNotification vẫn được gọi %d lần", len(sender.calls))
	}
	prev, err := svc.Preview(context.Background(), dto.BroadcastPreviewRequestDTO{Audience: "all"})
	if err != nil || prev.RecipientCount != 0 {
		t.Fatalf("preview = %+v, %v; muốn 0 (preview không lỗi, để UI hiện 0 người)", prev, err)
	}
}

func chunkSizes(calls []dto.CreateNotificationDTO) []int {
	out := make([]int, len(calls))
	for i, c := range calls {
		out[i] = len(c.UserIDs)
	}
	return out
}

func TestBroadcast_ChunkBoundary_MoiNguoiDungMotLan(t *testing.T) {
	db := broadcastDB(t)
	users := broadcastUsers(t, db, broadcastChunkSize+1)

	sender := &fakeBroadcastSender{}
	res, err := newBroadcastSvc(db, sender).Send(context.Background(), broadcastReq("all"))
	if err != nil {
		t.Fatalf("send 501: %v", err)
	}
	if sizes := chunkSizes(sender.calls); fmt.Sprint(sizes) != "[500 1]" {
		t.Fatalf("501 người nhận chia lô %v; muốn [500 1]", sizes)
	}
	got := sender.delivered()
	if len(got) != len(users) || res.RecipientCount != int64(len(users)) {
		t.Fatalf("người nhận khác biệt=%d, recipient_count=%d; muốn %d", len(got), res.RecipientCount, len(users))
	}
	for id, n := range got {
		if n != 1 {
			t.Fatalf("user %s nhận %d lần; muốn đúng 1", id, n)
		}
	}

	// Đúng 500 người: một lô, không có lô thứ hai rỗng.
	lockBroadcastUser(t, db, users[0].ID)
	sender = &fakeBroadcastSender{}
	if _, err := newBroadcastSvc(db, sender).Send(context.Background(), broadcastReq("all")); err != nil {
		t.Fatalf("send 500: %v", err)
	}
	if sizes := chunkSizes(sender.calls); fmt.Sprint(sizes) != "[500]" {
		t.Fatalf("500 người nhận chia lô %v; muốn [500]", sizes)
	}
}

func TestBroadcast_FailureMidway_NeuSoDaGiao(t *testing.T) {
	db := broadcastDB(t)
	broadcastUsers(t, db, broadcastChunkSize+1)
	sender := &fakeBroadcastSender{failAt: 2}

	_, err := newBroadcastSvc(db, sender).Send(context.Background(), broadcastReq("all"))
	var partial *BroadcastPartialError
	if !errors.As(err, &partial) {
		t.Fatalf("muốn *BroadcastPartialError, được %v", err)
	}
	if partial.Delivered != broadcastChunkSize {
		t.Fatalf("delivered = %d; muốn %d (lô đầu đã giao)", partial.Delivered, broadcastChunkSize)
	}

	// Lỗi ngay lô đầu: chưa giao ai thì là lỗi thường, không phải "một phần".
	_, err = newBroadcastSvc(db, &fakeBroadcastSender{failAt: 1}).Send(context.Background(), broadcastReq("all"))
	if err == nil || errors.As(err, &partial) {
		t.Fatalf("lỗi ở lô đầu phải là lỗi thường, được %v", err)
	}
}

func TestBroadcast_NoiDungVaLoai(t *testing.T) {
	db := broadcastDB(t)
	broadcastUsers(t, db, 1)
	sender := &fakeBroadcastSender{}
	svc := newBroadcastSvc(db, sender)

	req := broadcastReq("all")
	req.Title, req.Content, req.NotificationType = "  Khuyến mãi  ", "\n Giảm 50%  \t", "promotion"
	res, err := svc.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	c := sender.calls[0]
	if c.Title != "Khuyến mãi" || c.Content != "Giảm 50%" || c.NotificationType != "promotion" || res.NotificationType != "promotion" {
		t.Fatalf("lô gửi = %+v, kết quả = %+v; muốn tiêu đề/nội dung đã trim và type=promotion", c, res)
	}
	if c.ReferenceType != nil || c.ReferenceID != nil {
		t.Fatal("thông báo hệ thống không có tham chiếu")
	}
}

// Biên kiểm tra đầu vào (không cần DB: repo giả trả 1 người nhận cố định nên lỗi chỉ đến từ validate).
type fakeBroadcastAudience struct{ roles []string }

func (f *fakeBroadcastAudience) CountRecipients(context.Context, string, []string) (int64, error) {
	return 1, nil
}
func (f *fakeBroadcastAudience) StreamRecipientIDs(_ context.Context, _ string, _ []string, after uuid.UUID, _ int) ([]uuid.UUID, error) {
	if after != uuid.Nil {
		return nil, nil
	}
	return []uuid.UUID{uuid.New()}, nil
}
func (f *fakeBroadcastAudience) ExistingRoleNames(_ context.Context, names []string) ([]string, error) {
	return names, nil
}

func TestBroadcast_ValidationBounds(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*dto.BroadcastRequestDTO)
		ok     bool
	}{
		{"tiêu đề đúng 255", func(r *dto.BroadcastRequestDTO) { r.Title = strings.Repeat("ế", 255) }, true},
		{"tiêu đề 256", func(r *dto.BroadcastRequestDTO) { r.Title = strings.Repeat("ế", 256) }, false},
		{"tiêu đề chỉ khoảng trắng", func(r *dto.BroadcastRequestDTO) { r.Title = "   \t" }, false},
		{"nội dung đúng 2000", func(r *dto.BroadcastRequestDTO) { r.Content = strings.Repeat("á", 2000) }, true},
		{"nội dung 2001", func(r *dto.BroadcastRequestDTO) { r.Content = strings.Repeat("á", 2001) }, false},
		{"nội dung chỉ xuống dòng", func(r *dto.BroadcastRequestDTO) { r.Content = "\n\n" }, false},
		{"loại không hợp lệ", func(r *dto.BroadcastRequestDTO) { r.NotificationType = "payment_failed" }, false},
		{"audience lạ", func(r *dto.BroadcastRequestDTO) { r.Audience = "everyone" }, false},
		{"roles mà không chọn vai trò", func(r *dto.BroadcastRequestDTO) { r.Audience, r.Roles = "roles", nil }, false},
		{"roles toàn khoảng trắng tính là trống", func(r *dto.BroadcastRequestDTO) { r.Audience, r.Roles = "roles", []string{" "} }, false},
		{"quá nhiều vai trò", func(r *dto.BroadcastRequestDTO) {
			r.Audience = "roles"
			for i := 0; i <= broadcastMaxRoles; i++ {
				r.Roles = append(r.Roles, fmt.Sprintf("R%d", i))
			}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeBroadcastSender{}
			svc := NewBroadcastService(&fakeBroadcastAudience{}, sender)
			req := broadcastReq("all")
			tc.mutate(&req)
			_, err := svc.Send(context.Background(), req)
			if tc.ok {
				if err != nil {
					t.Fatalf("muốn hợp lệ, được %v", err)
				}
				return
			}
			requireBroadcastCode(t, err, 400, "VALIDATION_FAILED")
			if len(sender.calls) != 0 {
				t.Fatal("đầu vào sai mà vẫn gửi")
			}
		})
	}
}
