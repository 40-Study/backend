package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type ParentInvitationServiceInterface interface {
	InviteParent(ctx context.Context, studentUserID uuid.UUID, req dto.InviteParentRequestDto) (*dto.InviteParentResponseDto, error)
	ValidateInvitationToken(ctx context.Context, token string) (*model.ParentInvitation, error)
	RespondToInvitation(ctx context.Context, invitationID uuid.UUID, parentUserID uuid.UUID, action string) error
	GetPendingInvitations(ctx context.Context, studentUserID uuid.UUID) ([]model.ParentInvitation, error)
	GetSentInvitations(ctx context.Context, studentUserID uuid.UUID) ([]model.ParentInvitation, error)
	RevokeInvitation(ctx context.Context, invitationID uuid.UUID, studentID uuid.UUID) error
	LinkInvitationToNewUser(ctx context.Context, email string, userID uuid.UUID) error
}

type ParentInvitationService struct {
	cfg               *config.Config
	redisClient       *redis.Client
	invitationRepo    repository.ParentInvitationRepositoryInterface
	parentStudentRepo repository.ParentStudentRepositoryInterface
	userRepo          repository.UserRepositoryInterface
}

func NewParentInvitationService(
	cfg *config.Config,
	redisClient *redis.Client,
	invitationRepo repository.ParentInvitationRepositoryInterface,
	parentStudentRepo repository.ParentStudentRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
) *ParentInvitationService {
	return &ParentInvitationService{
		cfg:               cfg,
		redisClient:       redisClient,
		invitationRepo:    invitationRepo,
		parentStudentRepo: parentStudentRepo,
		userRepo:          userRepo,
	}
}

func (s *ParentInvitationService) InviteParent(
	ctx context.Context,
	studentUserID uuid.UUID,
	req dto.InviteParentRequestDto,
) (*dto.InviteParentResponseDto, error) {
	count_key := constants.KeyParentInviteRateLimit(studentUserID)
	countInt := int64(0)
	count, err := s.redisClient.Get(ctx, count_key).Result()
	if err == redis.Nil {
		countInt = 0
	} else if err != nil {
		return nil, err
	} else {
		countInt, err = strconv.ParseInt(count, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	if countInt >= int64(s.cfg.ParentInvitationDailyLimit) {
		return &dto.InviteParentResponseDto{
			Status:       "error",
			Message:      "Bạn đã đạt giới hạn gửi lời mời phụ huynh trong ngày. Vui lòng thử lại sau 24 giờ.",
			ParentExists: false,
		}, nil
	}
	student, err := s.userRepo.FindUserByID(ctx, studentUserID)
	if err != nil {
		return nil, err
	}
	// Check xem học sinh có gửi tới chính mình không
	if student.Email == req.Email {
		return &dto.InviteParentResponseDto{
			Status:       "error",
			Message:      "Bạn không thể tự mời bản thân làm phụ huynh",
			ParentExists: false,
		}, nil
	}

	// Tìm invitation xem đã gửi chưa
	invitation, err := s.invitationRepo.FindPendingByStudentAndEmail(ctx, studentUserID, req.Email)
	if err != nil {
		return &dto.InviteParentResponseDto{
			Status:       "error",
			Message:      "Đã có lời mời đang chờ hoặc đã gửi tới email này. Vui lòng kiểm tra lại.",
			ParentExists: false,
		}, err
	}
	if invitation != nil {
		return &dto.InviteParentResponseDto{
			Status:       "error",
			Message:      "Đã có lời mời đang chờ hoặc đã gửi tới email này. Vui lòng kiểm tra lại.",
			ParentExists: false,
			InvitationID: invitation.ID,
		}, nil
	}
	// kiểm tra phụ huynh đã có TK chưa
	parent, err := s.userRepo.FindUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	// Nếu đã có TK, kiểm tra xem đã có quan hệ phụ huynh - học sinh chưa
	var inviteeUserID *uuid.UUID
	if parent != nil {
		relation, err := s.parentStudentRepo.FindByParentAndStudent(ctx, parent.ID, studentUserID)
		if err != nil {
			return nil, err
		}
		if relation != nil {
			return &dto.InviteParentResponseDto{
				Status:       "error",
				Message:      "Phụ huynh đã có tài khoản và đã được liên kết với học sinh này.",
				ParentExists: true,
			}, nil
		}
		inviteeUserID = &parent.ID
	}
	token, tokenHash, tokenErr := utils.GenerateSecureToken() // 32 bytes hex encode -> 64 chars
	if tokenErr != nil {
		return nil, tokenErr
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	// Tạo mới invitation
	newInvitation := &model.ParentInvitation{
		StudentUserID: studentUserID,
		InviteeEmail:  req.Email,
		InviteeUserID: inviteeUserID,
		Relationship:  req.Relationship,
		Status:        model.ParentInvitationStatusInvited,
		TokenHash:     tokenHash,
		ExpiresAt:     expiresAt,
		Message:       req.Message,
		InvitationID:  nil,
	}
	err = s.invitationRepo.Create(ctx, newInvitation)
	if err != nil {
		return nil, err
	}

	limitCount, err := s.redisClient.Incr(ctx, count_key).Result()
	if err != nil {
		return nil, err
	}
	if limitCount == 1 {
		if err := s.redisClient.Expire(ctx, count_key, 24*time.Hour).Err(); err != nil {
			return nil, err
		}
	}

	//! sau này sẽ đẩy vào queue để xử lý async, hiện tại tạm thời xử lý sync
	go utils.SendInvitationEmail(s.cfg, req.Email, student.UserName, req.Relationship, token)

	return &dto.InviteParentResponseDto{
		Status:       "success",
		Message:      "Đã gửi lời mời phụ huynh thành công.",
		ParentExists: parent != nil,
		InvitationID: newInvitation.ID,
	}, nil
}

func (s *ParentInvitationService) ValidateInvitationToken(ctx context.Context, token string) (*model.ParentInvitation, error) {
	tokenHash := utils.HashToken(token)
	invitation, err := s.invitationRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	return invitation, nil
}

// RespondToInvitation xử lý khi phụ huynh click accept hoặc reject trong email
func (s *ParentInvitationService) RespondToInvitation(
	ctx context.Context,
	invitationID uuid.UUID,
	parentUserID uuid.UUID,
	action string, // "accept" hoặc "reject"
) error {
	invitation, err := s.invitationRepo.FindByID(ctx, invitationID)
	if err != nil {
		return err
	}
	if invitation == nil {
		return errors.New("lời mời không tồn tại")
	}
	// check xem lời mời có đang ỏw trạng thái pending hoặc invited hay không
	if invitation.Status != model.ParentInvitationStatusPending && invitation.Status != model.ParentInvitationStatusInvited {
		return errors.New("lời mời đã được phản hồi hoặc không còn hiệu lực")
	}
	// check xem lời mời có hết hạn hay không
	if invitation.ExpiresAt.Before(time.Now()) {
		invitation.Status = model.ParentInvitationStatusExpired
		now := time.Now()
		err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, model.ParentInvitationStatusExpired, &now)
		if err != nil {
			return err
		}
		invitation.RespondedAt = &now
		return errors.New("lời mời đã hết hạn")
	}
	// check xem lời mời này có dành cho user này không
	if invitation.InviteeUserID != nil && *invitation.InviteeUserID != parentUserID {
		return errors.New("lời mời này không dành cho bạn")
	}

	// Nếu accept thì tạo quan hệ phụ huynh - học sinh
	// Nếu reject thì chỉ update trạng thái lời mời
	// case reject
	if action == model.ParentInvitationStatusRejected {
		now := time.Now()
		if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, model.ParentInvitationStatusRejected, &now); err != nil {
			return err
		}
		// gửi notification cho học sinh và gửi email để sau làm
		return nil
	}

	if action == model.ParentInvitationStatusRevoked {
		now := time.Now()
		if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, model.ParentInvitationStatusRevoked, &now); err != nil {
			return err
		}
		return nil
	}
	// case accept
	if action != model.ParentInvitationStatusAccepted {
		return errors.New("hành động không hợp lệ")
	}
	// tạo quan hệ học sinh phụ huynh
	now := time.Now()
	relation := &model.ParentStudentRelation{
		ParentUserID:       parentUserID,
		StudentUserID:      invitation.StudentUserID,
		Relationship:       invitation.Relationship,
		CanViewProgress:    true,
		CanViewGrades:      true,
		CanViewAttendance:  true,
		CanContactTeachers: true,
		CanMakePayments:    true,
		CanManageAccount:   false,
		ConfirmedAt:        &now,
		ConfirmedBy:        &invitation.Relationship,
	}
	if err := s.parentStudentRepo.CreateRelation(ctx, relation); err != nil {
		return err
	}
	// update trạng thái lời mời thành accepted
	if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, model.ParentInvitationStatusAccepted, &now); err != nil {
		return err
	}

	// gửi notification cho học sinh và gửi email để sau làm

	return nil
}

// tìm lời mời đang chờ theo ID học sinh
func (s *ParentInvitationService) GetPendingInvitations(ctx context.Context, studentUserID uuid.UUID) ([]model.ParentInvitation, error) {
	user, err := s.userRepo.FindUserByID(ctx, studentUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("học sinh không tồn tại")
	}
	invitations, err := s.invitationRepo.FindPendingByInviteeUserID(ctx, studentUserID)
	if err != nil {
		return nil, err
	}
	return invitations, nil
}

// tìm lời mời đã gửi theo ID học sinh
func (s *ParentInvitationService) GetSentInvitations(ctx context.Context, studentUserID uuid.UUID) ([]model.ParentInvitation, error) {
	invitations, err := s.invitationRepo.FindByStudentUserID(ctx, studentUserID)
	if err != nil {
		return nil, err
	}
	return invitations, nil
}

func (s *ParentInvitationService) RevokeInvitation(ctx context.Context, invitationID uuid.UUID, studentID uuid.UUID) error {
	invitation, err := s.invitationRepo.FindByID(ctx, invitationID)
	if err != nil {
		return err
	}
	if invitation == nil {
		return errors.New("lời mời không tồn tại")
	}
	if invitation.StudentUserID != studentID {
		return errors.New("bạn không có quyền thu hồi lời mời này")
	}

	// pending hoặc invited mới được thu hồi
	if invitation.Status != model.ParentInvitationStatusPending && invitation.Status != model.ParentInvitationStatusInvited {
		return errors.New("lời mời đã được phản hồi hoặc không còn hiệu lực, không thể thu hồi")
	}
	now := time.Now()
	// update trạng thái lời mời thành revoked
	if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, model.ParentInvitationStatusRevoked, &now); err != nil {
		return err
	}
	return nil
}

// Auth Service sau khi phụ huynh mới tạo TK (Register hoặc OAuth) sẽ gọi API này để link lời mời với TK mới tạo
func (s *ParentInvitationService) LinkInvitationToNewUser(ctx context.Context, email string, userID uuid.UUID) error {
	invitations, err := s.invitationRepo.FindInvitedByEmail(ctx, email)
	if err != nil {
		return err
	}
	if len(invitations) == 0 {
		return nil // Không có lời mời nào - bình thường, không phải lỗi
	}
	now := time.Now()
	for _, invitation := range invitations {
		if invitation.Status == model.ParentInvitationStatusInvited {
			if err := s.invitationRepo.UpdateInviteeUserID(ctx, invitation.ID, userID); err != nil {
				return err
			}
			if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, model.ParentInvitationStatusPending, &now); err != nil {
				return err
			}
		}
	}
	return nil
}
