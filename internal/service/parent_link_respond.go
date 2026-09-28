package service

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
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
		if req == nil {
			return errLinkRequestNotFound
		}
		if ok, err := s.isAddressee(ctx, r, req, studentID); err != nil {
			return err
		} else if !ok {
			return errLinkRequestNotFound
		}
		if req.Status != model.ParentLinkRequestStatusPending {
			return errLinkNotPending
		}
		now := s.now()
		if action == "reject" {
			return r.Respond(ctx, req.ID, studentID, model.ParentLinkRequestStatusRejected, now)
		}

		// Review PR #81 MINOR-9: người gửi có thể đã bị gỡ vai PARENT sau khi gửi.
		if isParent, err := r.HasActiveSystemRole(ctx, req.ParentUserID, roleParent); err != nil {
			return err
		} else if !isParent {
			return errLinkParentNoLonger
		}
		// MINOR-4: khoá theo cặp, cùng khoá với ParentInvitationService.RespondToInvitation, để hai
		// luồng không cùng chèn dòng quan hệ cho một cặp.
		if err := r.LockKey(ctx, pairLockKey(req.ParentUserID, studentID)); err != nil {
			return err
		}
		if reverse, err := r.HasActiveRelation(ctx, studentID, req.ParentUserID); err != nil {
			return err
		} else if reverse {
			return errLinkCircular
		}
		// Kích hoạt LẠI dòng quan hệ cũ nếu có (từng liên kết rồi huỷ), không chèn dòng thứ hai cho
		// cùng cặp — xem uq_parent_student_relations_pair.
		rel, err := r.FindRelationForUpdate(ctx, req.ParentUserID, studentID)
		if err != nil {
			return err
		}
		if rel == nil {
			rel = &model.ParentStudentRelation{ParentUserID: req.ParentUserID, StudentUserID: studentID}
		}
		by := confirmedByStudent
		rel.Relationship = req.Relationship
		rel.Status = model.ParentStudentStatusActive
		rel.CanViewProgress, rel.CanViewGrades, rel.CanViewAttendance = true, true, true
		rel.CanContactTeachers, rel.CanMakePayments, rel.CanManageAccount = true, true, false
		rel.ConfirmedAt, rel.ConfirmedBy = &now, &by
		rel.RevokedAt, rel.RevokedBy = nil, nil
		if err := r.SaveRelation(ctx, rel); err != nil {
			return err
		}
		return r.Respond(ctx, req.ID, studentID, model.ParentLinkRequestStatusAccepted, now)
	})
}

// isAddressee: yêu cầu gắn id học sinh thì so id; yêu cầu gửi theo email lúc email chưa thuộc
// học sinh nào thì người trả lời phải có đúng email đó VÀ đang là học sinh.
func (s *ParentLinkService) isAddressee(ctx context.Context, r *repository.ParentLinkRequestRepository, req *model.ParentLinkRequest, studentID uuid.UUID) (bool, error) {
	if req.StudentUserID != nil {
		return *req.StudentUserID == studentID, nil
	}
	me, err := r.FindUserByID(ctx, studentID)
	if err != nil || me == nil || normalizeLinkEmail(me.Email) != req.StudentEmail {
		return false, err
	}
	return r.HasActiveSystemRole(ctx, studentID, roleStudent)
}

// pairLockKey — khoá tư vấn cho một cặp phụ huynh–con, dùng chung giữa luồng mới và luồng lời mời cũ.
func pairLockKey(parentID, studentID uuid.UUID) string {
	return "parent-student-pair:" + parentID.String() + ":" + studentID.String()
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
	return s.revoke(ctx, parentID, childID, model.RelationRevokedByParent)
}

// UnlinkByStudent — học sinh chủ động huỷ liên kết với một phụ huynh. Kéo theo thời gian chờ
// ParentLinkCooldown trước khi phụ huynh đó gửi lại được yêu cầu.
func (s *ParentLinkService) UnlinkByStudent(ctx context.Context, studentID, parentID uuid.UUID) error {
	return s.revoke(ctx, parentID, studentID, model.RelationRevokedByStudent)
}

// revoke — review PR #81, MAJOR-2: huỷ liên kết phải dọn cả MỌI lời mời (luồng cũ) và yêu cầu
// (luồng mới) còn chờ của cặp, nếu không một lời mời cũ chưa trả lời sẽ cho phụ huynh tự liên kết
// lại mà con không đồng ý lần nữa.
func (s *ParentLinkService) revoke(ctx context.Context, parentID, studentID uuid.UUID, by string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := s.repo(tx)
		if err := r.LockKey(ctx, pairLockKey(parentID, studentID)); err != nil {
			return err
		}
		rel, err := r.FindRelationForUpdate(ctx, parentID, studentID)
		if err != nil {
			return err
		}
		if rel == nil || rel.Status != model.ParentStudentStatusActive {
			return errLinkNotFound
		}
		now := s.now()
		if err := r.RevokeRelation(ctx, rel.ID, by, now); err != nil {
			return err
		}
		parent, err := r.FindUserByID(ctx, parentID)
		if err != nil {
			return err
		}
		student, err := r.FindUserByID(ctx, studentID)
		if err != nil {
			return err
		}
		parentEmail, studentEmail := "", ""
		if parent != nil {
			parentEmail = parent.Email
		}
		if student != nil {
			studentEmail = student.Email
		}
		if err := r.CancelPendingRequestsForPair(ctx, parentID, studentID, studentEmail, now); err != nil {
			return err
		}
		return r.RevokeInvitationsForPair(ctx, parentID, parentEmail, studentID, now)
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

// toParentLinkRequestDto — khi yêu cầu còn chờ, phía phụ huynh chỉ thấy email đã nhập, không
// thấy tên/avatar học sinh: nếu không, danh sách "đã gửi" chính là công cụ dò tài khoản (MAJOR-1).
// Tên học sinh chỉ hiện sau khi con đã trả lời.
func toParentLinkRequestDto(m *model.ParentLinkRequest) dto.ParentLinkRequestDto {
	out := dto.ParentLinkRequestDto{
		ID:           m.ID.String(),
		Status:       m.Status,
		Relationship: m.Relationship,
		Message:      m.Message,
		StudentEmail: m.StudentEmail,
		CreatedAt:    utils.FormatTimestamp(m.CreatedAt),
		RespondedAt:  utils.FormatTimestampPtr(m.RespondedAt),
	}
	if m.Parent != nil {
		p := toLinkUserDto(m.Parent)
		out.Parent = &p
	}
	if m.Student != nil && m.RespondedAt != nil &&
		(m.Status == model.ParentLinkRequestStatusAccepted || m.Status == model.ParentLinkRequestStatusRejected) {
		st := toLinkUserDto(m.Student)
		out.Student = &st
	}
	return out
}
