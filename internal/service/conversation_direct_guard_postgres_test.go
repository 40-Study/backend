package service

// Test Postgres THẬT cho guard "giới hạn nhắn tin theo quan hệ" (Lane G, QA 260927,
// POST /conversations/direct) — canCreateDirectConversation trong conversation_service.go.
// dùng pgtest.IsolatedSchema + migrateLikeAPIBoot (đã định nghĩa ở withdrawal_service_postgres_test.go,
// cùng package service) giống các test Postgres khác trong package này.

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/testutil/pgtest"
)

func newConversationServiceForTest(db *gorm.DB) *ConversationService {
	notifier := socket.NewNotifier(socket.NewHub())
	return NewConversationService(
		repository.NewConversationRepository(db),
		repository.NewConversationParticipantRepository(db),
		repository.NewMessageRepository(db),
		repository.NewMessageReactionRepository(db),
		notifier,
		repository.NewEnrollmentRepository(db),
		repository.NewParentStudentRepository(db),
		repository.NewUserSystemRoleRepository(db),
	)
}

func guardUser(t *testing.T, db *gorm.DB, tag string) uuid.UUID {
	t.Helper()
	id := uuid.NewString()
	u := model.User{
		Email:        "qa-guard-" + tag + "-" + id + "@40study.test",
		PasswordHash: "x",
		UserName:     "QA-guard-" + tag + id[:8],
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("tạo user %s: %v", tag, err)
	}
	return u.ID
}

func guardEnroll(t *testing.T, db *gorm.DB, studentID, teacherID uuid.UUID) {
	t.Helper()
	course := model.Course{
		InstructorID: teacherID,
		Title:        "Khoá QA guard " + uuid.NewString()[:8],
		Slug:         "qa-guard-" + uuid.NewString(),
		Price:        decimal.NewFromInt(1),
	}
	if err := db.Create(&course).Error; err != nil {
		t.Fatalf("tạo khoá: %v", err)
	}
	enrollment := model.Enrollment{UserID: studentID, CourseID: course.ID}
	if err := db.Create(&enrollment).Error; err != nil {
		t.Fatalf("ghi danh: %v", err)
	}
}

func guardParentChild(t *testing.T, db *gorm.DB, parentID, studentID uuid.UUID, status string) {
	t.Helper()
	rel := model.ParentStudentRelation{ParentUserID: parentID, StudentUserID: studentID, Status: status}
	if err := db.Create(&rel).Error; err != nil {
		t.Fatalf("tạo quan hệ phụ huynh-con: %v", err)
	}
}

func guardMakeAdmin(t *testing.T, db *gorm.DB, userID uuid.UUID) {
	t.Helper()
	role := model.SystemRole{Name: "SYSTEM_ADMIN", Status: "active"}
	if err := db.Where("name = ?", role.Name).FirstOrCreate(&role).Error; err != nil {
		t.Fatalf("tạo role SYSTEM_ADMIN: %v", err)
	}
	usr := model.UserSystemRole{UserID: userID, SystemRoleID: role.ID, Status: model.UserSystemRoleStatusActive}
	if err := db.Create(&usr).Error; err != nil {
		t.Fatalf("gán SYSTEM_ADMIN: %v", err)
	}
}

// TestCreateDirectConversation_QuanHeHopLe_ChoTao — mỗi quan hệ hợp lệ (a/b/d — (c) bạn bè chưa
// có bảng dữ liệu, xem docstring canCreateDirectConversation) phải cho tạo được cuộc trò chuyện.
// Xoá/vô hiệu canCreateDirectConversation (hoặc bỏ lời gọi nó khỏi CreateDirectConversation) sẽ
// không làm case này đỏ (guard tháo ra thì mọi cặp đều cho qua) — case NgườiLạ bên dưới mới là
// case chứng minh guard có tác dụng thật.
func TestCreateDirectConversation_QuanHeHopLe_ChoTao(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newConversationServiceForTest(db)
	ctx := t.Context()

	t.Run("học viên tạo với giảng viên của khoá đang học", func(t *testing.T) {
		student, teacher := guardUser(t, db, "student1"), guardUser(t, db, "teacher1")
		guardEnroll(t, db, student, teacher)
		if _, err := svc.CreateDirectConversation(ctx, student, teacher); err != nil {
			t.Errorf("học viên -> giảng viên: muốn cho phép, lỗi %v", err)
		}
	})

	t.Run("giảng viên tạo với học viên của khoá mình dạy (chiều ngược lại)", func(t *testing.T) {
		student, teacher := guardUser(t, db, "student2"), guardUser(t, db, "teacher2")
		guardEnroll(t, db, student, teacher)
		if _, err := svc.CreateDirectConversation(ctx, teacher, student); err != nil {
			t.Errorf("giảng viên -> học viên: muốn cho phép, lỗi %v", err)
		}
	})

	t.Run("phụ huynh-con đã xác nhận (active)", func(t *testing.T) {
		parent, child := guardUser(t, db, "parent1"), guardUser(t, db, "child1")
		guardParentChild(t, db, parent, child, model.ParentStudentStatusActive)
		if _, err := svc.CreateDirectConversation(ctx, parent, child); err != nil {
			t.Errorf("phụ huynh -> con (active): muốn cho phép, lỗi %v", err)
		}
	})

	t.Run("admin tạo với người lạ bất kỳ", func(t *testing.T) {
		admin, stranger := guardUser(t, db, "admin1"), guardUser(t, db, "stranger1")
		guardMakeAdmin(t, db, admin)
		if _, err := svc.CreateDirectConversation(ctx, admin, stranger); err != nil {
			t.Errorf("admin -> người lạ: muốn cho phép, lỗi %v", err)
		}
	})
}

// TestCreateDirectConversation_NguoiLa_Tra403 — case CHỨNG MINH guard có tác dụng: 2 người dùng
// không có bất kỳ quan hệ nào phải bị chặn với đúng lỗi ErrConversationNotAllowed (handler map
// sang 403 + code CONVERSATION_NOT_ALLOWED). Bỏ lời gọi canCreateDirectConversation khỏi
// CreateDirectConversation sẽ làm test này ĐỎ (đã tự kiểm 1 lần, xem báo cáo bàn giao).
func TestCreateDirectConversation_NguoiLa_Tra403(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newConversationServiceForTest(db)
	ctx := t.Context()

	a, b := guardUser(t, db, "stranger-a"), guardUser(t, db, "stranger-b")
	_, err := svc.CreateDirectConversation(ctx, a, b)
	if err == nil {
		t.Fatal("2 người lạ: muốn bị chặn, nhưng tạo thành công")
	}
	if !errors.Is(err, ErrConversationNotAllowed) {
		t.Errorf("2 người lạ: muốn lỗi ErrConversationNotAllowed, nhận %v", err)
	}
}

// TestCreateDirectConversation_LienKetPhuHuynhChuaXacNhan_Tra403 — liên kết phụ huynh-con đang
// "pending" (chưa xác nhận) KHÔNG được tính là quan hệ hợp lệ.
func TestCreateDirectConversation_LienKetPhuHuynhChuaXacNhan_Tra403(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newConversationServiceForTest(db)
	ctx := t.Context()

	parent, child := guardUser(t, db, "parent-pending"), guardUser(t, db, "child-pending")
	guardParentChild(t, db, parent, child, model.ParentStudentStatusPending)

	_, err := svc.CreateDirectConversation(ctx, parent, child)
	if !errors.Is(err, ErrConversationNotAllowed) {
		t.Errorf("liên kết đang pending: muốn ErrConversationNotAllowed, nhận %v", err)
	}
}

// TestCreateDirectConversation_CuocTroChuyenDaCo_KhongBiChan — guard chỉ áp dụng khi TẠO MỚI;
// cuộc trò chuyện đã tồn tại (kể cả giữa 2 người lạ — dữ liệu cũ, hoặc quan hệ đã hết) vẫn trả về
// bình thường, không lỗi.
func TestCreateDirectConversation_CuocTroChuyenDaCo_KhongBiChan(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newConversationServiceForTest(db)
	ctx := t.Context()

	admin, stranger := guardUser(t, db, "admin2"), guardUser(t, db, "stranger2")
	guardMakeAdmin(t, db, admin)
	if _, err := svc.CreateDirectConversation(ctx, admin, stranger); err != nil {
		t.Fatalf("tạo lần đầu (qua quan hệ admin): %v", err)
	}

	// Gọi lại lần 2 giữa ĐÚNG cặp này — GetDirectBetweenUsers phải tìm thấy cuộc trò chuyện đã có
	// và trả về ngay, không đi qua guard lần nữa (không lỗi dù giả sử quan hệ có mất đi).
	if _, err := svc.CreateDirectConversation(ctx, admin, stranger); err != nil {
		t.Errorf("gọi lại lần 2 (cuộc trò chuyện đã có): muốn không lỗi, nhận %v", err)
	}
}
