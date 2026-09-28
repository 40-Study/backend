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

// LockParent tuần tự hoá các lần gửi yêu cầu của CÙNG một phụ huynh trong transaction hiện tại
// (khoá tư vấn tự nhả khi COMMIT/ROLLBACK). Không có khoá này, N request đồng thời cùng đếm
// "đã gửi 4 lần" rồi cùng chèn, vượt giới hạn ngày.
func (r *ParentLinkRequestRepository) LockParent(ctx context.Context, parentID uuid.UUID) error {
	return r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "parent-link:"+parentID.String()).Error
}

// FindUserByEmailCI tìm user chưa xoá theo email, không phân biệt hoa thường (bảng users lưu
// email nguyên dạng người dùng nhập lúc đăng ký).
func (r *ParentLinkRequestRepository) FindUserByEmailCI(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("LOWER(email) = LOWER(?)", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
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

// CountCreatedSince đếm số yêu cầu phụ huynh đã gửi từ `since` (mọi trạng thái, kể cả đã huỷ —
// huỷ rồi gửi lại vẫn là một lần gửi, không để vòng gửi/huỷ né giới hạn).
func (r *ParentLinkRequestRepository) CountCreatedSince(ctx context.Context, parentID uuid.UUID, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ParentLinkRequest{}).
		Where("parent_user_id = ? AND created_at >= ?", parentID, since).Count(&n).Error
	return n, err
}

// FindPending trả yêu cầu đang chờ của cặp phụ huynh-học sinh (tối đa 1 nhờ unique index).
func (r *ParentLinkRequestRepository) FindPending(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentLinkRequest, error) {
	var req model.ParentLinkRequest
	err := r.db.WithContext(ctx).
		Where("parent_user_id = ? AND student_user_id = ? AND status = ?", parentID, studentID, model.ParentLinkRequestStatusPending).
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

// ListPendingForStudent — yêu cầu đang chờ học sinh trả lời.
func (r *ParentLinkRequestRepository) ListPendingForStudent(ctx context.Context, studentID uuid.UUID) ([]model.ParentLinkRequest, error) {
	var out []model.ParentLinkRequest
	err := r.db.WithContext(ctx).Preload("Parent").
		Where("student_user_id = ? AND status = ?", studentID, model.ParentLinkRequestStatusPending).
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

// SaveRelation chèn dòng mới (ID rỗng) hoặc ghi đè toàn bộ dòng đã có.
func (r *ParentLinkRequestRepository) SaveRelation(ctx context.Context, rel *model.ParentStudentRelation) error {
	if rel.ID == uuid.Nil {
		return r.db.WithContext(ctx).Create(rel).Error
	}
	return r.db.WithContext(ctx).Save(rel).Error
}

// SetRelationStatus đổi trạng thái quan hệ.
func (r *ParentLinkRequestRepository) SetRelationStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.db.WithContext(ctx).Model(&model.ParentStudentRelation{}).Where("id = ?", id).
		Update("status", status).Error
}

// ListActiveParents — phụ huynh đang liên kết với học sinh.
func (r *ParentLinkRequestRepository) ListActiveParents(ctx context.Context, studentID uuid.UUID) ([]model.ParentStudentRelation, error) {
	var out []model.ParentStudentRelation
	err := r.db.WithContext(ctx).Preload("Parent").
		Where("student_user_id = ? AND status = ?", studentID, model.ParentStudentStatusActive).
		Order("created_at ASC").Find(&out).Error
	return out, err
}
