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
	// R3-3 (issue #58 review vong 3): khang dinh CHUOI LITERAL "status = 'active'" thay vi so
	// sanh voi hang so StudentClassActiveCondition — neu so voi chinh hang so, mutation M-8b
	// (giu nguyen TEN hang so nhung doi NOI DUNG thanh "(1=1)") se lam ca production lan test
	// cung doi theo, tuc D6 bi vo hieu hoan toan ma test van xanh (khang dinh vong tron).
	if !strings.Contains(sql, "status = 'active'") {
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
	// R3-3: khang dinh chuoi literal, xem ghi chu chi tiet o TestStudentClassExists_LocTheoStatusActive.
	if !strings.Contains(sql, "status = 'active'") {
		t.Errorf("thieu dieu kien loc status active (D6) trong nhanh student_classes cua UNION — hoc sinh da nghi lop van duoc tinh la thanh vien phien. SQL: %s", sql)
	}
}

// TestGetAll_LocHocSinhTheoStatusActive (R3-3, review vong 3): D6 truoc day chi duoc pin o 2/3
// noi (StudentClassExists, IsUserRelatedToClass) — nhanh student_classes trong
// LivestreamRepository.GetAll (F-1, phien nao duoc liet ke cho non-admin) khong co test DryRun
// nao. Goi thang buildGetAllQuery (ham production that ma GetAll uy quyen toi).
func TestGetAll_LocHocSinhTheoStatusActive(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &LivestreamRepository{db: db.Session(&gorm.Session{DryRun: true})}

	userID := uuid.New()
	var total int64
	tx := buildGetAllQuery(repo.db, userID, false, "", nil, nil).Count(&total)

	sql := tx.Statement.SQL.String()
	if !strings.Contains(sql, "student_classes") {
		t.Fatalf("khong nhan duoc cau truy van tren student_classes, got: %s", sql)
	}
	if !strings.Contains(sql, "status = 'active'") {
		t.Errorf("thieu dieu kien loc status active (D6) trong nhanh student_classes cua GetAll — hoc sinh da nghi lop van thay phien cua lop do trong danh sach. SQL: %s", sql)
	}
}
