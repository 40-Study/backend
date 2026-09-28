package service

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// confirmedByStudent — giá trị cột parent_student_relations.confirmed_by khi con tự xác nhận.
const confirmedByStudent = "student"

// Respond — học sinh xác nhận hoặc từ chối yêu cầu của phụ huynh. Chỉ chính học sinh được gửi
// yêu cầu mới trả lời được; người khác nhận 404 như thể yêu cầu không tồn tại (không để dò ID).
func (s *ParentLinkService) Respond(ctx context.Context, studentID, requestID uuid.UUID, action string) error {
	if action != "accept" && action != "reject" {
		return errLinkInvalidAction
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := s.repo(tx)
		req, err := r.FindByIDForUpdate(ctx, requestID)
		if err != nil {
			return err
		}
		if req == nil || req.StudentUserID != studentID {
			return errLinkRequestNotFound
		}
		if req.Status != model.ParentLinkRequestStatusPending {
			return errLinkNotPending
		}
		now := s.now()
		if action == "reject" {
			return r.UpdateStatus(ctx, req.ID, model.ParentLinkRequestStatusRejected, now)
		}

		// Xác nhận: kích hoạt LẠI dòng quan hệ cũ nếu có (từng liên kết rồi huỷ), không chèn dòng
		// thứ hai cho cùng cặp — xem uq_parent_student_relations_pair.
		rel, err := r.FindRelationForUpdate(ctx, req.ParentUserID, req.StudentUserID)
		if err != nil {
			return err
		}
		if rel == nil {
			rel = &model.ParentStudentRelation{ParentUserID: req.ParentUserID, StudentUserID: req.StudentUserID}
		}
		by := confirmedByStudent
		rel.Relationship = req.Relationship
		rel.Status = model.ParentStudentStatusActive
		rel.CanViewProgress, rel.CanViewGrades, rel.CanViewAttendance = true, true, true
		rel.CanContactTeachers, rel.CanMakePayments, rel.CanManageAccount = true, true, false
		rel.ConfirmedAt, rel.ConfirmedBy = &now, &by
		if err := r.SaveRelation(ctx, rel); err != nil {
			return err
		}
		return r.UpdateStatus(ctx, req.ID, model.ParentLinkRequestStatusAccepted, now)
	})
}

func (s *ParentLinkService) ListLinkedParents(ctx context.Context, studentID uuid.UUID) ([]dto.LinkedParentDto, error) {
	rels, err := s.repo(s.db).ListActiveParents(ctx, studentID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.LinkedParentDto, 0, len(rels))
	for _, rel := range rels {
		if rel.Parent == nil {
			continue
		}
		out = append(out, dto.LinkedParentDto{
			LinkUserDto:  toLinkUserDto(rel.Parent),
			Relationship: rel.Relationship,
			LinkedAt:     utils.FormatTimestampPtr(rel.ConfirmedAt),
		})
	}
	return out, nil
}

// UnlinkByParent — phụ huynh huỷ liên kết với con. Sau lệnh này verifyParentChildRelation của
// dashboard từ chối ngay vì quan hệ không còn active.
func (s *ParentLinkService) UnlinkByParent(ctx context.Context, parentID, childID uuid.UUID) error {
	return s.revoke(ctx, parentID, childID)
}

// UnlinkByStudent — học sinh chủ động huỷ liên kết với một phụ huynh.
func (s *ParentLinkService) UnlinkByStudent(ctx context.Context, studentID, parentID uuid.UUID) error {
	return s.revoke(ctx, parentID, studentID)
}

func (s *ParentLinkService) revoke(ctx context.Context, parentID, studentID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := s.repo(tx)
		rel, err := r.FindRelationForUpdate(ctx, parentID, studentID)
		if err != nil {
			return err
		}
		if rel == nil || rel.Status != model.ParentStudentStatusActive {
			return errLinkNotFound
		}
		return r.SetRelationStatus(ctx, rel.ID, model.ParentStudentStatusRevoked)
	})
}

func toLinkUserDto(u *model.User) dto.LinkUserDto {
	return dto.LinkUserDto{
		ID:        u.ID.String(),
		Username:  u.UserName,
		FullName:  u.FullName,
		AvatarURL: u.AvatarURL,
		Email:     u.Email,
	}
}

func toParentLinkRequestDto(m *model.ParentLinkRequest) dto.ParentLinkRequestDto {
	out := dto.ParentLinkRequestDto{
		ID:           m.ID.String(),
		Status:       m.Status,
		Relationship: m.Relationship,
		Message:      m.Message,
		CreatedAt:    utils.FormatTimestamp(m.CreatedAt),
		RespondedAt:  utils.FormatTimestampPtr(m.RespondedAt),
	}
	if m.Parent != nil {
		p := toLinkUserDto(m.Parent)
		out.Parent = &p
	}
	if m.Student != nil {
		st := toLinkUserDto(m.Student)
		out.Student = &st
	}
	return out
}
