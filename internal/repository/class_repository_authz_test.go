package repository

// D6/R2-7 (issue #58 review vòng 3): "thành viên lớp" (dùng ở resolveJoinRole/EnsureSessionMember
// để quyết định ai được vào phiên livestream/gửi-đọc chat) trước đây không lọc theo
// StudentClass.Status — học sinh đã 'dropped'/'completed' vẫn được tính là thành viên. Pin lại
// đúng câu SQL THẬT (gọi thẳng method production, không hand-roll lại — tránh "green that proves
// nothing" như ghi chú M2-05 ở enrollment_repository_test.go) phải chứa điều kiện lọc status.
//
// Dùng gorm.io/gorm/utils/tests.DummyDialector ở chế độ DryRun (đã có sẵn trong module gorm đang
// dùng, không thêm dependency mới) — không cần Postgres thật, giống pattern
// coupon_repository_test.go/enrollment_repository_test.go.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

func dryRunClassRepo(t *testing.T) *ClassRepository {
	t.Helper()
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	return &ClassRepository{db: db.Session(&gorm.Session{DryRun: true})}
}

func TestStudentClassExists_LocTheoStatusActive(t *testing.T) {
	repo := dryRunClassRepo(t)
	classID, studentID := uuid.New(), uuid.New()

	// Goi thang ham BUILD cau query production (buildStudentClassExistsQuery) — cung ham ma
	// StudentClassExists uy quyen toi — de doc Statement.SQL tu chinh *gorm.DB tra ve, thay vi
	// hand-roll lai cau query trong test.
	var count int64
	tx := buildStudentClassExistsQuery(repo.db, classID, studentID, &count)

	sql := tx.Statement.SQL.String()
	if !strings.Contains(sql, "student_classes") {
		t.Fatalf("khong nhan duoc cau truy van tren student_classes, got: %s", sql)
	}
	if !strings.Contains(sql, StudentClassActiveCondition) {
		t.Errorf("thieu dieu kien loc status active (D6) trong WHERE — hoc sinh da nghi lop van duoc tinh la thanh vien. SQL: %s", sql)
	}
}

func TestIsUserRelatedToClass_LocHocSinhTheoStatusActive(t *testing.T) {
	repo := dryRunClassRepo(t)
	classID, userID := uuid.New(), uuid.New()

	var exists bool
	tx := buildIsUserRelatedToClassQuery(repo.db, classID, userID, &exists)

	sql := tx.Statement.SQL.String()
	if !strings.Contains(sql, "student_classes") {
		t.Fatalf("khong nhan duoc cau truy van tren student_classes, got: %s", sql)
	}
	if !strings.Contains(sql, StudentClassActiveCondition) {
		t.Errorf("thieu dieu kien loc status active (D6) trong nhanh student_classes cua UNION — hoc sinh da nghi lop van duoc tinh la thanh vien phien. SQL: %s", sql)
	}
}
