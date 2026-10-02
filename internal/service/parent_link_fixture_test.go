package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Fixture chung cho test Postgres THẬT của luồng liên kết phụ huynh-học sinh (QA vòng 2 lane E).
// Dữ liệu COMMIT trong schema tạm (pgtest.IsolatedSchema) bị DROP khi test xong.
type parentLinkFixture struct {
	t     *testing.T
	ctx   context.Context
	db    *gorm.DB
	svc   *ParentLinkService
	dash  *ParentDashboardService
	roles map[string]uuid.UUID
}

func newParentLinkFixture(t *testing.T) *parentLinkFixture {
	t.Helper()
	db := isolatedAPISchema(t)
	f := &parentLinkFixture{
		t: t, ctx: context.Background(), db: db,
		svc: NewParentLinkService(db),
		dash: NewParentDashboardService(repository.NewParentStudentRepository(db), repository.NewUserRepository(db),
			repository.NewEnrollmentRepository(db), nil, nil, nil, nil),
		roles: map[string]uuid.UUID{},
	}
	for _, name := range []string{"PARENT", "STUDENT", "TEACHER"} {
		r := model.SystemRole{Name: name}
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("tạo system role %s: %v", name, err)
		}
		f.roles[name] = r.ID
	}
	return f
}

// user tạo tài khoản kèm các vai hệ thống active; trả về user (email viết HOA một phần để kiểm tra
// tìm email không phân biệt hoa thường).
func (f *parentLinkFixture) user(kind string, roles ...string) model.User {
	f.t.Helper()
	s := uuid.NewString()[:8]
	u := model.User{Email: "QA-r2e-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-r2e-" + kind + "-" + s}
	if err := f.db.Create(&u).Error; err != nil {
		f.t.Fatalf("tạo user: %v", err)
	}
	for _, r := range roles {
		usr := model.UserSystemRole{UserID: u.ID, SystemRoleID: f.roles[r], Status: "active"}
		if err := f.db.Create(&usr).Error; err != nil {
			f.t.Fatalf("gán vai %s: %v", r, err)
		}
	}
	return u
}

// courseWithEnrollment tạo khoá do `teacher` dạy và ghi danh `student` vào đó.
func (f *parentLinkFixture) courseWithEnrollment(teacher, student model.User) model.Course {
	f.t.Helper()
	s := uuid.NewString()[:8]
	c := model.Course{InstructorID: teacher.ID, Title: "QA-r2e course " + s, Slug: "qa-r2e-" + s, Price: decimal.Zero, Status: "published"}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo course: %v", err)
	}
	if err := f.db.Create(&model.Enrollment{UserID: student.ID, CourseID: c.ID}).Error; err != nil {
		f.t.Fatalf("tạo enrollment: %v", err)
	}
	return c
}

func (f *parentLinkFixture) request(parent, student model.User) string {
	f.t.Helper()
	out, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(student.Email))
	if err != nil {
		f.t.Fatalf("gửi yêu cầu liên kết: %v", err)
	}
	return out.ID
}

func (f *parentLinkFixture) relationRows(parentID, studentID uuid.UUID) []model.ParentStudentRelation {
	f.t.Helper()
	var rows []model.ParentStudentRelation
	if err := f.db.Where("parent_user_id = ? AND student_user_id = ?", parentID, studentID).Find(&rows).Error; err != nil {
		f.t.Fatalf("đọc quan hệ: %v", err)
	}
	return rows
}

// canSeeChild — phụ huynh có đọc được dữ liệu con qua API dashboard không (tổng quan + khoá học).
func (f *parentLinkFixture) canSeeChild(parentID, childID uuid.UUID) bool {
	f.t.Helper()
	_, errOverview := f.dash.GetChildOverview(f.ctx, parentID, childID)
	_, errCourses := f.dash.GetChildCourses(f.ctx, parentID, childID, 1, 20)
	if (errOverview == nil) != (errCourses == nil) {
		f.t.Fatalf("overview và courses cho kết quả quyền khác nhau: %v / %v", errOverview, errCourses)
	}
	return errOverview == nil
}

func linkReq(email string) dto.CreateParentLinkRequestDto {
	return dto.CreateParentLinkRequestDto{StudentEmail: email, Relationship: "parent"}
}

// wantLinkCode khẳng định err là ParentLinkError đúng mã.
func wantLinkCode(t *testing.T, err error, code string) {
	t.Helper()
	var le *ParentLinkError
	if !errors.As(err, &le) {
		t.Fatalf("muốn lỗi nghiệp vụ %s, nhận %v", code, err)
	}
	if le.Code != code {
		t.Fatalf("muốn mã %s, nhận %s (%s)", code, le.Code, le.Message)
	}
}
