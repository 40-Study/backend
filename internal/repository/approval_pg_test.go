package repository

// Phase 3 duyệt khoá học + duyệt giáo viên — test Postgres THẬT (tự skip khi không kết nối được,
// cùng khuôn user_admin_guards_pg_test.go). Toàn bộ chạy trong 1 transaction rồi ROLLBACK — không
// ghi gì xuống DB thật. Pin phần mà test route (fake repo) không pin được: câu SQL thật, CHECK
// constraint thật đã nới 'rejected', unique index user_system_roles thật khi hồi sinh role.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

func apvPgTx(t *testing.T) *gorm.DB {
	t.Helper()
	db := openTestPostgres(t)
	if !db.Migrator().HasColumn(&model.TeacherProfile{}, "approval_status") {
		t.Skip("DB chua co cot phase 3 (chay API ban nay 1 lan de AutoMigrate + post-migration)")
	}
	tx := db.Begin()
	t.Cleanup(func() {
		tx.Rollback()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return tx
}

func apvUser(t *testing.T, tx *gorm.DB, tag string) model.User {
	t.Helper()
	s := uuid.NewString()
	u := model.User{Email: tag + "-" + s + "@40study.test", PasswordHash: "x", UserName: tag + "-" + s}
	if err := tx.Create(&u).Error; err != nil {
		t.Fatalf("tao user %s: %v", tag, err)
	}
	return u
}

func apvRoleID(t *testing.T, tx *gorm.DB, name string) uuid.UUID {
	t.Helper()
	var r model.SystemRole
	if err := tx.Where("name = ?", name).First(&r).Error; err != nil {
		t.Skipf("DB chua seed role %s (go run ./cmd/seed -mode=base): %v", name, err)
	}
	return r.ID
}

func activeRoleNames(t *testing.T, tx *gorm.DB, userID uuid.UUID) map[string]bool {
	t.Helper()
	var names []string
	if err := tx.Table("user_system_roles").Select("system_roles.name").
		Joins("JOIN system_roles ON system_roles.id = user_system_roles.system_role_id").
		Where("user_system_roles.user_id = ? AND user_system_roles.status = 'active' AND user_system_roles.deleted_at IS NULL", userID).
		Scan(&names).Error; err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

// Duyệt = 1 transaction: TEACHER được gán (kể cả khi user từng có mapping TEACHER bị xoá mềm —
// unique index idx_usr_user_role không partial nên INSERT mới sẽ 23505), TEACHER_APPLICANT bị gỡ.
func TestTeacherApplicationApprove_PG_SwapsRolesAndRevivesSoftDeletedMapping(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	teacherRole, applicantRole := apvRoleID(t, tx, "TEACHER"), apvRoleID(t, tx, "TEACHER_APPLICANT")
	applicant, admin := apvUser(t, tx, "apv-applicant"), apvUser(t, tx, "apv-admin")

	if err := tx.Create(&model.UserSystemRole{UserID: applicant.ID, SystemRoleID: applicantRole, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	old := model.UserSystemRole{UserID: applicant.ID, SystemRoleID: teacherRole, Status: "inactive"}
	if err := tx.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(&old).Error; err != nil { // xoá mềm
		t.Fatal(err)
	}
	if err := tx.Create(&model.TeacherProfile{UserID: applicant.ID, ApprovalStatus: model.TeacherApprovalPending}).Error; err != nil {
		t.Fatal(err)
	}

	repo := NewTeacherApplicationRepository(tx)
	p, err := repo.Approve(ctx, applicant.ID, admin.ID)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if p.ApprovalStatus != model.TeacherApprovalApproved || p.ReviewedBy == nil || *p.ReviewedBy != admin.ID {
		t.Fatalf("ho so sau duyet sai: %+v", p)
	}
	roles := activeRoleNames(t, tx, applicant.ID)
	if !roles["TEACHER"] || roles["TEACHER_APPLICANT"] {
		t.Fatalf("role sau duyet sai: %v", roles)
	}
	if _, err := repo.Approve(ctx, applicant.ID, admin.ID); !errors.Is(err, ErrTeacherApplicationNotPending) {
		t.Fatalf("duyet lan 2 phai loi NotPending, err=%v", err)
	}
}

func TestTeacherApplicationResubmit_PG_LimitAndReject(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	applicant, admin := apvUser(t, tx, "apv-resub"), apvUser(t, tx, "apv-admin2")
	if err := tx.Create(&model.TeacherProfile{UserID: applicant.ID, ApprovalStatus: model.TeacherApprovalPending}).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTeacherApplicationRepository(tx)
	for i := 1; i <= model.MaxTeacherResubmissions; i++ {
		if _, err := repo.Reject(ctx, applicant.ID, admin.ID, "Chua du minh chung"); err != nil {
			t.Fatalf("Reject %d: %v", i, err)
		}
		p, err := repo.Resubmit(ctx, applicant.ID)
		if err != nil || p.ResubmissionCount != i || p.ApprovalStatus != model.TeacherApprovalPending {
			t.Fatalf("Resubmit %d: p=%+v err=%v", i, p, err)
		}
	}
	if _, err := repo.Reject(ctx, applicant.ID, admin.ID, "Van thieu"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Resubmit(ctx, applicant.ID); !errors.Is(err, ErrTeacherResubmissionLimit) {
		t.Fatalf("lan nop lai thu 4 phai bi chan, err=%v", err)
	}
	var stored model.TeacherProfile
	tx.Where("user_id = ?", applicant.ID).First(&stored)
	if stored.ApprovalStatus != model.TeacherApprovalRejected || stored.ResubmissionCount != model.MaxTeacherResubmissions {
		t.Fatalf("ho so sau lan bi chan: %+v", stored)
	}
}

// Review PR #73 (MAJOR #1): GetAll phục vụ route công khai — SQL thật chỉ trả hồ sơ approved.
func TestTeacherProfileGetAll_PG_OnlyApproved(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	tag := "apv-public-" + uuid.NewString()[:8]
	ids := map[string]uuid.UUID{}
	for _, st := range model.TeacherApprovalStatuses {
		u := apvUser(t, tx, tag)
		spec := tag
		p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: st, Specialization: &spec}
		if err := tx.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
		ids[st] = p.ID
	}
	rows, total, err := NewTeacherProfileRepository(tx).GetAll(ctx, 1, 50, tag, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != ids[model.TeacherApprovalApproved] {
		t.Fatalf("GetAll cong khai phai chi co 1 ho so approved, got total=%d rows=%v", total, rows)
	}
}

// CHECK chk_courses_status thật phải chấp nhận 'rejected' (post-migration đã nới) và luồng
// submit -> reject -> submit -> approve chạy được trên SQL thật.
func TestCourseReview_PG_FullCycleAgainstRealConstraint(t *testing.T) {
	tx := apvPgTx(t)
	ctx := context.Background()
	teacher, admin, other := apvUser(t, tx, "apv-teacher"), apvUser(t, tx, "apv-admin3"), apvUser(t, tx, "apv-other")
	course := model.Course{InstructorID: teacher.ID, Title: "PG approval", Slug: "pg-approval-" + uuid.NewString(), Status: model.CourseStatusDraft}
	if err := tx.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewCourseReviewRepository(tx)
	if _, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &other.ID, nil, nil); !errors.Is(err, ErrCourseReviewNotOwner) {
		t.Fatalf("nguoi khac nop phai bi chan, err=%v", err)
	}
	if c, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &teacher.ID, nil, nil); err != nil || c.SubmittedAt == nil {
		t.Fatalf("submit: %v %+v", err, c)
	}
	reason := "Noi dung chua du chi tiet"
	c, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionReject, nil, &admin.ID, &reason)
	if err != nil || c.Status != model.CourseStatusRejected || c.RejectionReason == nil || *c.RejectionReason != reason {
		t.Fatalf("reject tren DB that: err=%v c=%+v", err, c)
	}
	if _, err := repo.ApplyReviewAction(ctx, course.ID, CourseActionSubmit, &teacher.ID, nil, nil); err != nil {
		t.Fatalf("nop lai: %v", err)
	}
	c, err = repo.ApplyReviewAction(ctx, course.ID, CourseActionApprove, nil, &admin.ID, nil)
	if err != nil || c.Status != model.CourseStatusPublished || c.PublishedAt == nil || c.RejectionReason != nil {
		t.Fatalf("approve: err=%v c=%+v", err, c)
	}
	list, total, err := repo.ListForReview(ctx, AdminCourseReviewFilter{Status: model.CourseStatusPublished, Keyword: "PG approval", Page: 1, PageSize: 20})
	if err != nil || total < 1 || len(list) < 1 || list[0].Instructor.Email != teacher.Email {
		t.Fatalf("ListForReview: err=%v total=%d list=%v", err, total, list)
	}
}
