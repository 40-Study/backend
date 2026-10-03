package service

// Lane R2 (QA hồi quy 03/10/2026), Postgres thật:
//   - A-03: xoá nhóm phải đóng hội thoại nhóm. Trước đây hội thoại vẫn nằm trong danh sách của mọi thành viên và
//     gửi tin vẫn được (hội thoại mồ côi). Bỏ DeleteWithConversation (quay về groupRepo.Delete) thì test ĐỎ.
//   - A-06: phụ huynh được mở hội thoại với giảng viên đang dạy lớp/khoá mà con (liên kết đã xác nhận) ghi danh.
//     Bỏ nhánh (b2) trong canCreateDirectConversation thì các ca "cho phép" ĐỎ; nới quá tay thì các ca "từ chối" ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestR2_XoaNhom_DongHoiThoaiNhom(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, member := e.user("owner"), e.user("member")
	g, conv := e.newGroup(owner, "PUBLIC", member)

	if !e.canSend(owner, conv) || !e.canSend(member, conv) {
		t.Fatal("thành viên phải gửi được tin trước khi nhóm bị xoá")
	}
	var before int64
	e.db.Model(&model.Message{}).Where("conversation_id = ?", conv).Count(&before)

	if err := e.groups.DeleteGroup(ctx, owner.ID, g.ID); err != nil {
		t.Fatalf("xoá nhóm: %v", err)
	}

	for name, u := range map[string]model.User{"chủ nhóm": owner, "thành viên": member} {
		list, err := e.convs.ListConversations(ctx, u.ID, 1, 20)
		if err != nil || list.TotalCount != 0 {
			t.Errorf("%s vẫn thấy hội thoại của nhóm đã xoá: err=%v %+v", name, err, list)
		}
		if e.canRead(u, conv) {
			t.Errorf("%s vẫn ĐỌC được tin của nhóm đã xoá", name)
		}
		text := "tin sau khi xoá nhóm"
		_, err = e.convs.SendMessage(ctx, u.ID, conv, dto.SendMessageRequest{Content: &text})
		if !errors.Is(err, ErrNotParticipant) {
			t.Errorf("%s gửi tin vào nhóm đã xoá: muốn ErrNotParticipant, nhận %v", name, err)
		}
		if got, err := e.convs.GetConversation(ctx, u.ID, conv); err == nil || got != nil {
			t.Errorf("%s vẫn mở được hội thoại của nhóm đã xoá", name)
		}
	}

	// Lịch sử tin vẫn còn trong DB (xoá mềm hội thoại, không xoá vật lý tin nhắn).
	var after int64
	e.db.Model(&model.Message{}).Where("conversation_id = ?", conv).Count(&after)
	if after != before || before == 0 {
		t.Errorf("số tin trong hội thoại = %d sau khi xoá nhóm, muốn giữ nguyên %d (>0)", after, before)
	}
}

func TestR2_XoaNhom_ChiDongHoiThoaiCuaNhomBiXoa(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, member := e.user("owner"), e.user("member")
	gDel, convDel := e.newGroup(owner, "PUBLIC", member)
	_, convKeep := e.newGroup(owner, "PUBLIC", member)

	if err := e.groups.DeleteGroup(ctx, owner.ID, gDel.ID); err != nil {
		t.Fatal(err)
	}
	if e.canRead(member, convDel) {
		t.Error("hội thoại của nhóm đã xoá vẫn đọc được")
	}
	if !e.canRead(member, convKeep) || !e.canSend(member, convKeep) {
		t.Error("xoá một nhóm làm hỏng hội thoại của nhóm khác")
	}
	if list, err := e.convs.ListConversations(ctx, member.ID, 1, 20); err != nil || list.TotalCount != 1 {
		t.Errorf("thành viên muốn còn đúng 1 hội thoại (nhóm giữ lại), nhận err=%v %+v", err, list)
	}
}

// A-24: tab Khám phá phải biết người xem đã có yêu cầu chờ / đã là thành viên.
func TestR2_DanhSachNhom_KemVaiVaYeuCauDangCho(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, requester, stranger := e.user("owner"), e.user("requester"), e.user("stranger")
	g, _ := e.newGroup(owner, "PRIVATE")
	if _, err := e.groups.JoinGroup(ctx, requester.ID, g.ID, nil); err != nil {
		t.Fatalf("xin vào nhóm PRIVATE: %v", err)
	}

	find := func(viewer *uuid.UUID) dto.GroupResponse {
		t.Helper()
		list, err := e.groups.ListGroups(ctx, viewer, "", "", 1, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range list.Groups {
			if item.ID == g.ID {
				return item
			}
		}
		t.Fatal("nhóm PRIVATE không có trong danh sách")
		return dto.GroupResponse{}
	}

	if got := find(&requester.ID); got.MyJoinRequest == nil || got.MyRole != nil {
		t.Errorf("người đã xin vào: muốn my_join_request và không có vai, nhận %+v / %v", got.MyJoinRequest, got.MyRole)
	}
	if got := find(&owner.ID); got.MyRole == nil || *got.MyRole != "OWNER" || got.MyJoinRequest != nil {
		t.Errorf("chủ nhóm: muốn vai OWNER và không có yêu cầu, nhận %+v / %v", got.MyJoinRequest, got.MyRole)
	}
	for name, viewer := range map[string]*uuid.UUID{"người lạ": &stranger.ID, "khách": nil} {
		if got := find(viewer); got.MyJoinRequest != nil || got.MyRole != nil {
			t.Errorf("%s không được thấy vai/yêu cầu của người khác, nhận %+v / %v", name, got.MyJoinRequest, got.MyRole)
		}
	}
}

// r2PT: guard hội thoại thật trên schema Postgres, checker phụ huynh -> giảng viên nối đúng như app/services.go
// (wired=false để kiểm trường hợp chưa nối thì đóng).
type r2PT struct {
	t   *testing.T
	db  *gorm.DB
	svc *ConversationService
}

func newR2PT(t *testing.T, wired bool) *r2PT {
	t.Helper()
	db := isolatedAPISchema(t)
	svc := newConversationServiceForTest(db)
	if wired {
		svc.SetParentTeacherChecker(repository.NewParentStudentRepository(db))
	}
	return &r2PT{t: t, db: db, svc: svc}
}

func (f *r2PT) user(tag string) uuid.UUID { return guardUser(f.t, f.db, tag) }

// link tạo liên kết phụ huynh-con active và trả lại dòng quan hệ.
func (f *r2PT) link(p, c uuid.UUID) *model.ParentStudentRelation {
	f.t.Helper()
	guardParentChild(f.t, f.db, p, c, model.ParentStudentStatusActive)
	var rel model.ParentStudentRelation
	if err := f.db.Where("parent_user_id = ? AND student_user_id = ?", p, c).First(&rel).Error; err != nil {
		f.t.Fatal(err)
	}
	return &rel
}

// classOf cho giảng viên dạy một lớp và xếp học viên vào lớp đó với trạng thái studentStatus.
func (f *r2PT) classOf(teacher, student uuid.UUID, studentStatus, classStatus string) {
	f.t.Helper()
	class := model.Class{Name: "Lớp R2 " + uuid.NewString()[:6], Status: classStatus}
	if err := f.db.Create(&class).Error; err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.Create(&model.TeacherClass{TeacherID: teacher, ClassID: class.ID}).Error; err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.Create(&model.StudentClass{StudentID: student, ClassID: class.ID, Status: studentStatus}).Error; err != nil {
		f.t.Fatal(err)
	}
}

func TestR2_PhuHuynhNhanGiangVienCuaCon_ChoPhep(t *testing.T) {
	ctx := context.Background()

	t.Run("con ghi danh khoá của giảng viên (cả hai chiều)", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		f.link(parent, child)
		guardEnroll(t, f.db, child, teacher)
		if _, err := f.svc.CreateDirectConversation(ctx, parent, teacher); err != nil {
			t.Errorf("phụ huynh -> giảng viên của con: muốn cho phép, lỗi %v", err)
		}
		// Cặp mới để kiểm chiều giảng viên -> phụ huynh (cặp trên đã có cuộc nên không đi qua guard nữa).
		parent2, child2, teacher2 := f.user("parent2"), f.user("child2"), f.user("teacher2")
		f.link(parent2, child2)
		guardEnroll(t, f.db, child2, teacher2)
		if _, err := f.svc.CreateDirectConversation(ctx, teacher2, parent2); err != nil {
			t.Errorf("giảng viên -> phụ huynh của học viên mình dạy: muốn cho phép, lỗi %v", err)
		}
	})

	t.Run("con đang học lớp giảng viên được gán dạy", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		f.link(parent, child)
		f.classOf(teacher, child, "active", "active")
		if _, err := f.svc.CreateDirectConversation(ctx, parent, teacher); err != nil {
			t.Errorf("phụ huynh -> giảng viên lớp của con: muốn cho phép, lỗi %v", err)
		}
	})
}

func TestR2_PhuHuynhNhanGiangVien_TuChoiKhiKhongDuDieuKien(t *testing.T) {
	ctx := context.Background()
	denied := func(t *testing.T, f *r2PT, a, b uuid.UUID) {
		t.Helper()
		if _, err := f.svc.CreateDirectConversation(ctx, a, b); !errors.Is(err, ErrConversationNotAllowed) {
			t.Errorf("muốn ErrConversationNotAllowed, nhận %v", err)
		}
	}

	t.Run("giảng viên không dạy con của phụ huynh", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher, other := f.user("parent"), f.user("child"), f.user("teacher"), f.user("other-teacher")
		f.link(parent, child)
		guardEnroll(t, f.db, child, other)
		denied(t, f, parent, teacher)
	})

	t.Run("liên kết phụ huynh-con chưa xác nhận (pending)", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		guardParentChild(t, f.db, parent, child, model.ParentStudentStatusPending)
		guardEnroll(t, f.db, child, teacher)
		denied(t, f, parent, teacher)
	})

	t.Run("phụ huynh bị tắt quyền liên hệ giảng viên (can_contact_teachers=false)", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		rel := f.link(parent, child)
		if err := f.db.Model(rel).Update("can_contact_teachers", false).Error; err != nil {
			t.Fatal(err)
		}
		guardEnroll(t, f.db, child, teacher)
		denied(t, f, parent, teacher)
	})

	t.Run("con đã rời lớp (dropped) hoặc lớp đã lưu trữ", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		f.link(parent, child)
		f.classOf(teacher, child, "dropped", "active")
		f.classOf(teacher, child, "active", "archived")
		denied(t, f, parent, teacher)
	})

	t.Run("ghi danh của con đã bị xoá mềm", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		f.link(parent, child)
		guardEnroll(t, f.db, child, teacher)
		if err := f.db.Where("user_id = ?", child).Delete(&model.Enrollment{}).Error; err != nil {
			t.Fatal(err)
		}
		denied(t, f, parent, teacher)
	})

	t.Run("chưa nối checker thì đóng, không mở cửa", func(t *testing.T) {
		f := newR2PT(t, false)
		parent, child, teacher := f.user("parent"), f.user("child"), f.user("teacher")
		f.link(parent, child)
		guardEnroll(t, f.db, child, teacher)
		denied(t, f, parent, teacher)
	})

	t.Run("giảng viên của học viên KHÔNG phải con mình", func(t *testing.T) {
		f := newR2PT(t, true)
		parent, child, strangerStudent, teacher := f.user("parent"), f.user("child"), f.user("stranger-student"), f.user("teacher")
		f.link(parent, child)
		guardEnroll(t, f.db, strangerStudent, teacher)
		denied(t, f, parent, teacher)
	})
}
