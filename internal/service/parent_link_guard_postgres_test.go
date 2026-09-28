package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Chống spam lớp 1 + 3: không gửi trùng khi đang chờ; bị từ chối thì chờ đủ thời gian mới gửi lại.
func TestParentLink_Postgres_KhongGuiTrungVaCoThoiGianChoSauTuChoi(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	reqID := uuid.MustParse(f.request(parent, child))

	_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email))
	wantLinkCode(t, err, "LINK_REQUEST_PENDING")

	if err := f.svc.Respond(f.ctx, child.ID, reqID, "reject"); err != nil {
		t.Fatalf("con từ chối: %v", err)
	}
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email))
	wantLinkCode(t, err, "LINK_REQUEST_COOLDOWN")

	// Hết thời gian chờ thì gửi lại được.
	realNow := f.svc.now
	f.svc.now = func() time.Time { return realNow().Add(ParentLinkCooldown + time.Hour) }
	if _, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email)); err != nil {
		t.Fatalf("hết thời gian chờ mà vẫn không gửi lại được: %v", err)
	}
}

// Chống spam lớp 2: nhiều request ĐỒNG THỜI tới các học sinh khác nhau vẫn chỉ tạo đúng
// ParentLinkDailyLimit yêu cầu (khoá tư vấn theo phụ huynh tuần tự hoá bước đếm-rồi-ghi).
func TestParentLink_Postgres_GioiHanNgayKhongVuotKhiGuiDongThoi(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	const n = ParentLinkDailyLimit * 2
	children := make([]string, n)
	for i := range children {
		children[i] = f.user("child", "STUDENT").Email
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, limited := 0, 0
	start := make(chan struct{})
	for _, email := range children {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			<-start
			_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
				return
			}
			if le, isLink := err.(*ParentLinkError); isLink && le.Code == "LINK_REQUEST_DAILY_LIMIT" {
				limited++
				return
			}
			t.Errorf("lỗi ngoài dự kiến: %v", err)
		}(email)
	}
	close(start)
	wg.Wait()

	var stored int64
	f.db.Model(&model.ParentLinkRequest{}).Where("parent_user_id = ?", parent.ID).Count(&stored)
	if ok != ParentLinkDailyLimit || limited != n-ParentLinkDailyLimit || stored != ParentLinkDailyLimit {
		t.Fatalf("muốn %d thành công/%d bị chặn/%d dòng, nhận %d/%d/%d", ParentLinkDailyLimit, n-ParentLinkDailyLimit,
			ParentLinkDailyLimit, ok, limited, stored)
	}
}

// DB tự chặn 2 yêu cầu đang chờ cùng cặp, kể cả đường ghi bỏ qua service.
func TestParentLink_Postgres_UniqueIndexChanYeuCauChoTrung(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	f.request(parent, child)
	email := strings.ToLower(child.Email)
	dup := model.ParentLinkRequest{ParentUserID: parent.ID, StudentEmail: email, Relationship: "parent", Status: "pending"}
	if err := f.db.Create(&dup).Error; err == nil {
		t.Fatal("DB cho chèn 2 yêu cầu pending cùng phụ huynh + email")
	}
	bogus := model.ParentLinkRequest{ParentUserID: parent.ID, StudentEmail: email, Relationship: "parent", Status: "bogus"}
	if err := f.db.Create(&bogus).Error; err == nil {
		t.Fatal("CHECK constraint không chặn status lạ của parent_link_requests")
	}
	rel := model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: child.ID, Status: "bogus"}
	if err := f.db.Create(&rel).Error; err == nil {
		t.Fatal("CHECK constraint không chặn status lạ của parent_student_relations")
	}
}

// Huỷ liên kết từ cả 2 phía: mất quyền xem ngay; liên kết lại kích hoạt lại đúng 1 dòng quan hệ.
func TestParentLink_Postgres_HuyLienKetVaLienKetLai(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	accept := func() {
		t.Helper()
		if err := f.svc.Respond(f.ctx, child.ID, uuid.MustParse(f.request(parent, child)), "accept"); err != nil {
			t.Fatalf("con xác nhận: %v", err)
		}
	}

	accept()
	if err := f.svc.UnlinkByParent(f.ctx, parent.ID, child.ID); err != nil {
		t.Fatalf("phụ huynh huỷ liên kết: %v", err)
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("đã huỷ liên kết mà phụ huynh vẫn xem được dữ liệu con")
	}
	wantLinkCode(t, f.svc.UnlinkByParent(f.ctx, parent.ID, child.ID), "LINK_NOT_FOUND")

	accept()
	parents, err := f.svc.ListLinkedParents(f.ctx, child.ID)
	if err != nil || len(parents) != 1 || parents[0].ID != parent.ID.String() {
		t.Fatalf("con phải thấy 1 phụ huynh đang liên kết: %+v, err=%v", parents, err)
	}
	if err := f.svc.UnlinkByStudent(f.ctx, child.ID, parent.ID); err != nil {
		t.Fatalf("con huỷ liên kết: %v", err)
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("con đã huỷ liên kết mà phụ huynh vẫn xem được")
	}
	if rows := f.relationRows(parent.ID, child.ID); len(rows) != 1 || rows[0].Status != model.ParentStudentStatusRevoked {
		t.Fatalf("muốn đúng 1 dòng quan hệ 'revoked', nhận %+v", rows)
	}
}

// Luồng cũ (con mời phụ huynh qua email) sau khi đã huỷ liên kết: mời lại được và chấp nhận thì
// kích hoạt lại dòng cũ, không chèn dòng thứ hai (sẽ vỡ uq_parent_student_relations_pair).
func TestParentInvitation_Postgres_LienKetLaiSauKhiHuy(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	if err := f.svc.Respond(f.ctx, child.ID, uuid.MustParse(f.request(parent, child)), "accept"); err != nil {
		t.Fatalf("con xác nhận: %v", err)
	}
	if err := f.svc.UnlinkByStudent(f.ctx, child.ID, parent.ID); err != nil {
		t.Fatalf("con huỷ liên kết: %v", err)
	}

	// Quyền cũ bị thu hẹp trước khi huỷ; liên kết lại phải đặt lại đủ quyền (MINOR-6).
	if err := f.db.Model(&model.ParentStudentRelation{}).Where("parent_user_id = ? AND student_user_id = ?", parent.ID, child.ID).
		Update("can_view_grades", false).Error; err != nil {
		t.Fatalf("thu hẹp quyền: %v", err)
	}
	// Lời mời MỚI, tạo sau khi huỷ liên kết — hợp lệ.
	inv := model.ParentInvitation{StudentUserID: child.ID, InviteeEmail: parent.Email, InviteeUserID: &parent.ID,
		Relationship: "parent", Status: model.ParentInvitationStatusPending, TokenHash: "qa-r2e-" + uuid.NewString(),
		ExpiresAt: time.Now().Add(24 * time.Hour), CreatedAt: time.Now().Add(time.Second)}
	if err := f.db.Create(&inv).Error; err != nil {
		t.Fatalf("tạo lời mời: %v", err)
	}
	invSvc := NewParentInvitationService(nil, nil, repository.NewParentInvitationRepository(f.db),
		repository.NewParentStudentRepository(f.db), repository.NewUserRepository(f.db), repository.NewUserSystemRoleRepository(f.db))
	if err := invSvc.RespondToInvitation(f.ctx, inv.ID, parent.ID, "accept"); err != nil {
		t.Fatalf("phụ huynh chấp nhận lời mời sau khi đã huỷ liên kết: %v", err)
	}
	if !f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("chấp nhận lời mời mà quan hệ không active lại")
	}
	rows := f.relationRows(parent.ID, child.ID)
	if len(rows) != 1 {
		t.Fatalf("muốn đúng 1 dòng quan hệ, nhận %d", len(rows))
	}
	if !rows[0].CanViewGrades || rows[0].RevokedAt != nil || rows[0].RevokedBy != nil {
		t.Fatalf("kích hoạt lại phải đặt lại quyền và xoá dấu huỷ: %+v", rows[0])
	}
}
