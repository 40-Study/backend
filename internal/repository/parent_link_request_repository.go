package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// ParentLinkRequestRepository — truy cập dữ liệu cho luồng phụ huynh gửi yêu cầu liên kết con.
// Mọi hàm chạy trên `db` được truyền vào lúc tạo, nên service tạo repo trên transaction (tx) khi
// cần nhiều bước nguyên tử (khoá, kiểm, ghi).
type ParentLinkRequestRepository struct {
	db *gorm.DB
}

func NewParentLinkRequestRepository(db *gorm.DB) *ParentLinkRequestRepository {
	return &ParentLinkRequestRepository{db: db}
}

// LockKey tuần tự hoá các thao tác cùng khoá trong transaction hiện tại (khoá tư vấn tự nhả khi
// COMMIT/ROLLBACK). Dùng theo phụ huynh khi gửi yêu cầu (đếm-rồi-ghi hạn mức) và theo cặp khi tạo
// quan hệ (hai luồng không cùng chèn dòng quan hệ cho một cặp — review PR #81, MINOR-4).
func (r *ParentLinkRequestRepository) LockKey(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", key).Error
}

// FindUserByID trả user chưa xoá (nil nếu không có).
func (r *ParentLinkRequestRepository) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

// FindStudentIDByEmailCI trả id của tài khoản HỌC SINH (đang giữ vai STUDENT active) có email này,
// không phân biệt hoa thường; nil nếu email không tồn tại HOẶC không phải học sinh. Một truy vấn
// duy nhất cho cả hai trường hợp (review PR #81, MINOR-3: 1 và 2 truy vấn cho thời gian phản hồi
// khác nhau, lộ loại tài khoản).
func (r *ParentLinkRequestRepository) FindStudentIDByEmailCI(ctx context.Context, email string) (*uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`
		SELECT u.id FROM users u
		WHERE LOWER(u.email) = LOWER(?) AND u.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM user_system_roles usr
			JOIN system_roles sr ON sr.id = usr.system_role_id
			WHERE usr.user_id = u.id AND usr.status = 'active' AND usr.deleted_at IS NULL
			  AND sr.deleted_at IS NULL AND sr.name = 'STUDENT'
		  )
		LIMIT 1`, email).Scan(&ids).Error
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	return &ids[0], nil
}

// HasActiveSystemRole trả true nếu user đang giữ vai hệ thống `roleName` (status active).
func (r *ParentLinkRequestRepository) HasActiveSystemRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM user_system_roles usr
			JOIN system_roles sr ON sr.id = usr.system_role_id
			WHERE usr.user_id = ? AND usr.status = 'active' AND usr.deleted_at IS NULL
			  AND sr.deleted_at IS NULL AND sr.name = ?
		)`, userID, roleName).Scan(&exists).Error
	return exists, err
}

// CountAttemptsSince đếm MỌI lần gửi của phụ huynh từ `since`, kể cả lần thất bại.
func (r *ParentLinkRequestRepository) CountAttemptsSince(ctx context.Context, parentID uuid.UUID, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ParentLinkAttempt{}).
		Where("parent_user_id = ? AND created_at >= ?", parentID, since).Count(&n).Error
	return n, err
}

func (r *ParentLinkRequestRepository) CreateAttempt(ctx context.Context, parentID uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).Create(&model.ParentLinkAttempt{ParentUserID: parentID, CreatedAt: at}).Error
}

// FindPending trả yêu cầu đang chờ của phụ huynh tới email này (tối đa 1 nhờ unique index).
func (r *ParentLinkRequestRepository) FindPending(ctx context.Context, parentID uuid.UUID, email string) (*model.ParentLinkRequest, error) {
	var req model.ParentLinkRequest
	err := r.db.WithContext(ctx).
		Where("parent_user_id = ? AND student_email = ? AND status = ?", parentID, email, model.ParentLinkRequestStatusPending).
		First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &req, err
}

// LatestRejectedAt trả thời điểm học sinh từ chối gần nhất yêu cầu của phụ huynh này (nil nếu chưa).
func (r *ParentLinkRequestRepository) LatestRejectedAt(ctx context.Context, parentID, studentID uuid.UUID) (*time.Time, error) {
	var req model.ParentLinkRequest
	err := r.db.WithContext(ctx).
		Where("parent_user_id = ? AND student_user_id = ? AND status = ? AND responded_at IS NOT NULL",
			parentID, studentID, model.ParentLinkRequestStatusRejected).
		Order("responded_at DESC").First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return req.RespondedAt, nil
}

func (r *ParentLinkRequestRepository) Create(ctx context.Context, req *model.ParentLinkRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

// FindByIDForUpdate khoá dòng yêu cầu để 2 lần bấm xác nhận/từ chối/huỷ đồng thời không cùng xử lý.
func (r *ParentLinkRequestRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*model.ParentLinkRequest, error) {
	var req model.ParentLinkRequest
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&req, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &req, err
}

// Respond ghi trạng thái trả lời và gắn học sinh đã trả lời (yêu cầu gửi theo email có thể chưa có id).
func (r *ParentLinkRequestRepository) Respond(ctx context.Context, id, studentID uuid.UUID, status string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ParentLinkRequest{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": status, "responded_at": at, "student_user_id": studentID}).Error
}

func (r *ParentLinkRequestRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, respondedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ParentLinkRequest{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": status, "responded_at": respondedAt}).Error
}

// ListByParent — yêu cầu phụ huynh đã gửi, mới nhất trước.
func (r *ParentLinkRequestRepository) ListByParent(ctx context.Context, parentID uuid.UUID, limit int) ([]model.ParentLinkRequest, error) {
	var out []model.ParentLinkRequest
	err := r.db.WithContext(ctx).Preload("Student").Where("parent_user_id = ?", parentID).
		Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

// ListPendingForStudent — yêu cầu đang chờ học sinh trả lời: gửi tới id của học sinh, hoặc gửi tới
// email của học sinh khi lúc đó email chưa gắn với tài khoản học sinh nào.
func (r *ParentLinkRequestRepository) ListPendingForStudent(ctx context.Context, studentID uuid.UUID, email string) ([]model.ParentLinkRequest, error) {
	var out []model.ParentLinkRequest
	err := r.db.WithContext(ctx).Preload("Parent").
		Where("status = ? AND (student_user_id = ? OR (student_user_id IS NULL AND student_email = LOWER(?)))",
			model.ParentLinkRequestStatusPending, studentID, email).
		Order("created_at DESC").Find(&out).Error
	return out, err
}

// FindRelationForUpdate trả dòng quan hệ của cặp (mọi trạng thái), khoá để cập nhật.
func (r *ParentLinkRequestRepository) FindRelationForUpdate(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentStudentRelation, error) {
	var rel model.ParentStudentRelation
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("parent_user_id = ? AND student_user_id = ?", parentID, studentID).
		Order("created_at ASC").First(&rel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rel, err
}

// HasActiveRelation — cặp (parentID là phụ huynh của studentID) đang active.
func (r *ParentLinkRequestRepository) HasActiveRelation(ctx context.Context, parentID, studentID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ParentStudentRelation{}).
		Where("parent_user_id = ? AND student_user_id = ? AND status = ?", parentID, studentID, model.ParentStudentStatusActive).
		Count(&n).Error
	return n > 0, err
}

// SaveRelation chèn dòng mới (ID rỗng) hoặc ghi đè toàn bộ dòng đã có.
func (r *ParentLinkRequestRepository) SaveRelation(ctx context.Context, rel *model.ParentStudentRelation) error {
	if rel.ID == uuid.Nil {
		return r.db.WithContext(ctx).Create(rel).Error
	}
	return r.db.WithContext(ctx).Save(rel).Error
}

// RevokeRelation đặt quan hệ về 'revoked', ghi ai huỷ và lúc nào.
func (r *ParentLinkRequestRepository) RevokeRelation(ctx context.Context, id uuid.UUID, by string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ParentStudentRelation{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": model.ParentStudentStatusRevoked, "revoked_by": by, "revoked_at": at}).Error
}

// CancelPendingRequestsForPair huỷ mọi yêu cầu (luồng mới) còn chờ của cặp, kể cả yêu cầu gửi theo
// email chưa gắn id.
func (r *ParentLinkRequestRepository) CancelPendingRequestsForPair(ctx context.Context, parentID, studentID uuid.UUID, studentEmail string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ParentLinkRequest{}).
		Where("parent_user_id = ? AND status = ? AND (student_user_id = ? OR (student_user_id IS NULL AND student_email = LOWER(?)))",
			parentID, model.ParentLinkRequestStatusPending, studentID, studentEmail).
		Updates(map[string]interface{}{"status": model.ParentLinkRequestStatusCancelled, "responded_at": at}).Error
}

// RevokeInvitationsForPair thu hồi mọi lời mời (luồng cũ: con mời phụ huynh) còn hiệu lực của cặp,
// khớp theo tài khoản phụ huynh hoặc email của phụ huynh (lời mời gửi khi phụ huynh chưa có tài khoản).
func (r *ParentLinkRequestRepository) RevokeInvitationsForPair(ctx context.Context, parentID uuid.UUID, parentEmail string, studentID uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ParentInvitation{}).
		Where("student_user_id = ? AND status IN ? AND (invitee_user_id = ? OR LOWER(invitee_email) = LOWER(?))",
			studentID, []string{model.ParentInvitationStatusInvited, model.ParentInvitationStatusPending}, parentID, parentEmail).
		Updates(map[string]interface{}{"status": model.ParentInvitationStatusRevoked, "responded_at": at}).Error
}

// ListActiveParents — phụ huynh đang liên kết với học sinh.
func (r *ParentLinkRequestRepository) ListActiveParents(ctx context.Context, studentID uuid.UUID) ([]model.ParentStudentRelation, error) {
	var out []model.ParentStudentRelation
	err := r.db.WithContext(ctx).Preload("Parent").
		Where("student_user_id = ? AND status = ?", studentID, model.ParentStudentStatusActive).
		Order("created_at ASC").Find(&out).Error
	return out, err
}
