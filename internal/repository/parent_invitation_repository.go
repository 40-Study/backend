package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ParentInvitationRepository struct {
	db *gorm.DB
}

type ParentInvitationRepositoryInterface interface {
	Create(ctx context.Context, invitation *model.ParentInvitation) error                                                 // tạo lời mời
	FindByID(ctx context.Context, id uuid.UUID) (*model.ParentInvitation, error)                                          // tìm lời mời theo ID
	FindByTokenHash(ctx context.Context, tokenHash string) (*model.ParentInvitation, error)                               // tìm lời mời theo token hash (khi click link trong email)
	FindPendingByStudentAndEmail(ctx context.Context, studentID uuid.UUID, email string) (*model.ParentInvitation, error) // tìm lời mời đang chờ theo học sinh và email
	FindPendingByInviteeUserID(ctx context.Context, userID uuid.UUID) ([]model.ParentInvitation, error)                   // tìm lời mời đang chờ theo ID người được mời
	FindByStudentUserID(ctx context.Context, studentID uuid.UUID) ([]model.ParentInvitation, error)                       // tìm lời mời theo ID học sinh
	FindInvitedByEmail(ctx context.Context, email string) ([]model.ParentInvitation, error)                               // tìm lời mời đã gửi theo email
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, respondedAt *time.Time) error                          // cập nhật trạng thái lời mời
	UpdateInviteeUserID(ctx context.Context, id uuid.UUID, userID uuid.UUID) error                                        // cập nhật ID người được mời
	ExpireOldInvitations(ctx context.Context) (int64, error)
}

func NewParentInvitationRepository(db *gorm.DB) ParentInvitationRepositoryInterface {
	return &ParentInvitationRepository{db: db}
}

func (r *ParentInvitationRepository) Create(ctx context.Context, invitation *model.ParentInvitation) error {
	return r.db.WithContext(ctx).Create(invitation).Error
}

func (r *ParentInvitationRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.ParentInvitation, error) {
	var invitation model.ParentInvitation
	err := r.db.WithContext(ctx).First(&invitation, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &invitation, nil
}

func (r *ParentInvitationRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*model.ParentInvitation, error) {
	var invitation model.ParentInvitation
	err := r.db.WithContext(ctx).Preload("Student").First(&invitation, "token_hash = ?", tokenHash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &invitation, nil
}

func (r *ParentInvitationRepository) FindPendingByStudentAndEmail(ctx context.Context, studentID uuid.UUID, email string) (*model.ParentInvitation, error) {
	var invitation model.ParentInvitation
	// chỉ tìm lời mời là "invited" hoặc "pending"
	// invited : đã gửi lời mời, chưa biết người nhận đã xem hay chưa
	// pending : người nhận đã xem lời mời nhưng chưa phản hồi (chấp nhận hay từ chối)
	err := r.db.WithContext(ctx).Where("student_user_id = ? AND invitee_email = ? AND status IN ?", studentID, email, []string{model.ParentInvitationStatusInvited, model.ParentInvitationStatusPending}).First(&invitation).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Chưa có lời mời nào → bình thường
		}
		return nil, err
	}
	return &invitation, nil
}

func (r *ParentInvitationRepository) FindPendingByInviteeUserID(ctx context.Context, userID uuid.UUID) ([]model.ParentInvitation, error) {
	var invitations []model.ParentInvitation
	err := r.db.WithContext(ctx).Where("invitee_user_id = ? AND status IN ?", userID, []string{model.ParentInvitationStatusInvited, model.ParentInvitationStatusPending}).Find(&invitations).Error
	return invitations, err
}

func (r *ParentInvitationRepository) FindByStudentUserID(ctx context.Context, studentID uuid.UUID) ([]model.ParentInvitation, error) {
	var invitations []model.ParentInvitation
	err := r.db.WithContext(ctx).Where("student_user_id = ?", studentID).Find(&invitations).Error
	return invitations, err
}

func (r *ParentInvitationRepository) FindInvitedByEmail(ctx context.Context, email string) ([]model.ParentInvitation, error) {
	var invitations []model.ParentInvitation
	err := r.db.WithContext(ctx).Where("invitee_email = ? AND status IN ?", email, []string{model.ParentInvitationStatusInvited, model.ParentInvitationStatusPending}).Find(&invitations).Error
	return invitations, err
}

func (r *ParentInvitationRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, respondedAt *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ParentInvitation{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       status,
		"responded_at": respondedAt,
	}).Error
}

func (r *ParentInvitationRepository) UpdateInviteeUserID(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.ParentInvitation{}).Where("id = ?", id).Update("invitee_user_id", userID).Error
}

func (r *ParentInvitationRepository) ExpireOldInvitations(ctx context.Context) (int64, error) {
	// chỉ sửa những lời mời có status là "invited" hoặc "pending" và đã hết hạn (expires_at < now), chuyển sang status "expired"
	result := r.db.WithContext(ctx).Model(&model.ParentInvitation{}).Where("expires_at < ? AND status IN ?", time.Now(), []string{model.ParentInvitationStatusInvited, model.ParentInvitationStatusPending}).Updates(map[string]interface{}{
		"status": model.ParentInvitationStatusExpired,
	})
	return result.RowsAffected, result.Error
}
