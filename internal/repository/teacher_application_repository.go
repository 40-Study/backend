package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// Phase 3 duyệt giáo viên đăng ký (2026-09-28).
var (
	ErrTeacherApplicationNotFound    = errors.New("teacher application not found")
	ErrTeacherApplicationNotPending  = errors.New("teacher application is not pending")
	ErrTeacherApplicationNotRejected = errors.New("teacher application is not rejected")
	ErrTeacherResubmissionLimit      = errors.New("teacher application resubmission limit reached")
	ErrSystemRoleMissing             = errors.New("required system role is not seeded")
)

const (
	systemRoleTeacher          = "TEACHER"
	systemRoleTeacherApplicant = "TEACHER_APPLICANT"
)

// EvaluateTeacherReview — hàm THUẦN: admin chỉ duyệt/từ chối được hồ sơ đang pending.
func EvaluateTeacherReview(current string) error {
	if current != model.TeacherApprovalPending {
		return ErrTeacherApplicationNotPending
	}
	return nil
}

// EvaluateTeacherResubmission — hàm THUẦN cho quyết định #5: chỉ hồ sơ bị từ chối mới nộp lại
// được, và số lần NỘP LẠI đã dùng phải < MaxTeacherResubmissions (lần nộp lại thứ 4 bị chặn).
func EvaluateTeacherResubmission(current string, resubmissionCount int) error {
	if current != model.TeacherApprovalRejected {
		return ErrTeacherApplicationNotRejected
	}
	if resubmissionCount >= model.MaxTeacherResubmissions {
		return ErrTeacherResubmissionLimit
	}
	return nil
}

// TeacherApplicationRow — 1 dòng hàng chờ (teacher_profiles JOIN users).
type TeacherApplicationRow struct {
	model.TeacherProfile
	Email    string  `gorm:"column:email"`
	FullName *string `gorm:"column:full_name"`
}

type TeacherApplicationFilter struct {
	Status  string
	Keyword string
	Page    int
	Limit   int
}

type TeacherApplicationRepositoryInterface interface {
	List(ctx context.Context, filter TeacherApplicationFilter) ([]TeacherApplicationRow, int64, error)
	// Approve: 1 transaction — khoá hồ sơ, set approved, GÁN TEACHER rồi mới GỠ
	// TEACHER_APPLICANT (thứ tự này để user không bao giờ ở trạng thái 0 vai trò).
	Approve(ctx context.Context, userID, reviewerID uuid.UUID) (*model.TeacherProfile, error)
	Reject(ctx context.Context, userID, reviewerID uuid.UUID, reason string) (*model.TeacherProfile, error)
	Resubmit(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error)
}

type TeacherApplicationRepository struct {
	db *gorm.DB
}

func NewTeacherApplicationRepository(db *gorm.DB) *TeacherApplicationRepository {
	return &TeacherApplicationRepository{db: db}
}

func (r *TeacherApplicationRepository) List(ctx context.Context, filter TeacherApplicationFilter) ([]TeacherApplicationRow, int64, error) {
	var rows []TeacherApplicationRow
	var total int64

	query := r.db.WithContext(ctx).Model(&model.TeacherProfile{}).
		Joins("JOIN users ON users.id = teacher_profiles.user_id AND users.deleted_at IS NULL").
		Where("teacher_profiles.approval_status = ?", filter.Status)
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		query = utils.ApplyKeywordSearch(query, kw, "users.email", "users.full_name", "teacher_profiles.specialization")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := utils.ApplyPagination(query, filter.Page, filter.Limit).
		Select("teacher_profiles.*, users.email AS email, users.full_name AS full_name").
		Order("teacher_profiles.updated_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// lockProfileByUserID khoá (FOR UPDATE) hồ sơ của user trong tx — mọi thao tác duyệt/từ
// chối/nộp lại đi qua đây nên 2 thao tác đồng thời trên cùng hồ sơ tự xếp hàng.
func lockProfileByUserID(tx *gorm.DB, userID uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTeacherApplicationNotFound
		}
		return nil, err
	}
	return &profile, nil
}

func findSystemRoleID(tx *gorm.DB, name string) (uuid.UUID, error) {
	var role model.SystemRole
	if err := tx.Where("name = ?", name).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, ErrSystemRoleMissing
		}
		return uuid.Nil, err
	}
	return role.ID, nil
}

func (r *TeacherApplicationRepository) Approve(ctx context.Context, userID, reviewerID uuid.UUID) (*model.TeacherProfile, error) {
	var out *model.TeacherProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		profile, err := lockProfileByUserID(tx, userID)
		if err != nil {
			return err
		}
		if err := EvaluateTeacherReview(profile.ApprovalStatus); err != nil {
			return err
		}
		if err := swapApplicantToTeacherTx(tx, userID, reviewerID); err != nil {
			return err
		}

		now := time.Now()
		profile.ApprovalStatus = model.TeacherApprovalApproved
		profile.ReviewedBy = &reviewerID
		profile.ReviewedAt = &now
		profile.RejectionReason = nil
		if err := tx.Save(profile).Error; err != nil {
			return err
		}
		out = profile
		return nil
	})
	return out, err
}

// swapApplicantToTeacherTx — phần đổi vai trò của Approve: GÁN TEACHER trước rồi mới GỠ
// TEACHER_APPLICANT (user không bao giờ ở trạng thái 0 vai trò). Tách hàm riêng để pin bằng test
// DryRun không cần Postgres (approval_role_swap_dryrun_test.go).
func swapApplicantToTeacherTx(tx *gorm.DB, userID, reviewerID uuid.UUID) error {
	teacherRoleID, err := findSystemRoleID(tx, systemRoleTeacher)
	if err != nil {
		return err
	}
	if err := grantSystemRoleTx(tx, userID, teacherRoleID, reviewerID); err != nil {
		return err
	}
	// TEACHER_APPLICANT có thể chưa được seed trên DB cũ, hoặc user nộp hồ sơ mà không giữ
	// role này (vd học viên tự tạo hồ sơ) — khi đó không có gì để gỡ, không phải lỗi.
	applicantRoleID, err := findSystemRoleID(tx, systemRoleTeacherApplicant)
	if errors.Is(err, ErrSystemRoleMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	return revokeSystemRoleTx(tx, userID, applicantRoleID, reviewerID)
}

// grantSystemRoleTx gán role (hoặc kích hoạt lại mapping cũ). Unique index idx_usr_user_role
// (user_id, system_role_id) KHÔNG partial nên phải tìm cả dòng đã xoá mềm/inactive rồi hồi sinh,
// không INSERT mới (sẽ 23505).
func grantSystemRoleTx(tx *gorm.DB, userID, roleID, grantedBy uuid.UUID) error {
	var existing model.UserSystemRole
	err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND system_role_id = ?", userID, roleID).First(&existing).Error
	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&model.UserSystemRole{
			UserID: userID, SystemRoleID: roleID, GrantedAt: now, GrantedBy: &grantedBy,
			Status: model.UserSystemRoleStatusActive,
		}).Error
	}
	if err != nil {
		return err
	}
	return tx.Unscoped().Model(&model.UserSystemRole{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
		"status": model.UserSystemRoleStatusActive, "granted_at": now, "granted_by": grantedBy,
		"revoked_at": nil, "revoked_by": nil, "deleted_at": nil,
	}).Error
}

func revokeSystemRoleTx(tx *gorm.DB, userID, roleID, revokedBy uuid.UUID) error {
	return tx.Model(&model.UserSystemRole{}).
		Where("user_id = ? AND system_role_id = ? AND status = ?", userID, roleID, model.UserSystemRoleStatusActive).
		Updates(map[string]interface{}{
			"status": model.UserSystemRoleStatusInactive, "revoked_by": revokedBy, "revoked_at": time.Now(),
		}).Error
}

func (r *TeacherApplicationRepository) Reject(ctx context.Context, userID, reviewerID uuid.UUID, reason string) (*model.TeacherProfile, error) {
	var out *model.TeacherProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		profile, err := lockProfileByUserID(tx, userID)
		if err != nil {
			return err
		}
		if err := EvaluateTeacherReview(profile.ApprovalStatus); err != nil {
			return err
		}
		now := time.Now()
		profile.ApprovalStatus = model.TeacherApprovalRejected
		profile.RejectionReason = &reason
		profile.ReviewedBy = &reviewerID
		profile.ReviewedAt = &now
		if err := tx.Save(profile).Error; err != nil {
			return err
		}
		out = profile
		return nil
	})
	return out, err
}

func (r *TeacherApplicationRepository) Resubmit(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	var out *model.TeacherProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		profile, err := lockProfileByUserID(tx, userID)
		if err != nil {
			return err
		}
		if err := EvaluateTeacherResubmission(profile.ApprovalStatus, profile.ResubmissionCount); err != nil {
			return err
		}
		// Giữ rejection_reason/reviewed_* của lần từ chối trước làm lịch sử cho admin đọc khi xét
		// lại; approve sẽ xoá rejection_reason, reject sẽ ghi đè.
		profile.ApprovalStatus = model.TeacherApprovalPending
		profile.ResubmissionCount++
		if err := tx.Save(profile).Error; err != nil {
			return err
		}
		out = profile
		return nil
	})
	return out, err
}

func (r *TeacherProfileRepository) GetByUserIDIncludingDeleted(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	err := r.db.WithContext(ctx).Unscoped().Where("user_id = ?", userID).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// Restore bỏ deleted_at và ghi nội dung mới; resubmission_count/rejection_reason giữ nguyên.
func (r *TeacherProfileRepository) Restore(ctx context.Context, profile *model.TeacherProfile) error {
	profile.DeletedAt = gorm.DeletedAt{}
	return r.db.WithContext(ctx).Unscoped().Save(profile).Error
}

// HasActiveSystemRole (TeacherProfileRepository) — dùng khi tạo hồ sơ giáo viên: người ĐÃ giữ
// role TEACHER (giáo viên cũ tạo hồ sơ muộn) thì hồ sơ coi như đã duyệt, không lọt hàng chờ.
func (r *TeacherProfileRepository) HasActiveSystemRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserSystemRole{}).
		Joins("JOIN system_roles ON system_roles.id = user_system_roles.system_role_id").
		Where("user_system_roles.user_id = ? AND user_system_roles.status = ? AND system_roles.name = ?",
			userID, model.UserSystemRoleStatusActive, roleName).
		Count(&count).Error
	return count > 0, err
}
