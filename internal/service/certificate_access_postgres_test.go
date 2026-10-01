package service

// GET /certificates/:id với Postgres thật: phụ huynh có liên kết ACTIVE xem được chứng chỉ của con
// qua ParentStudentRepository.HasActiveParent; liên kết đã huỷ (revoked) thì không.

import (
	"context"
	"errors"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestGetCertificateByID_Postgres_ParentLinkStatusDecidesAccess(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()
	rc.certSvc.SetParentLinkChecker(repository.NewParentStudentRepository(rc.db))

	stored, err := repository.NewCertificateRepository(rc.db).GetCertificateByNumber(ctx, rc.number)
	if err != nil || stored == nil {
		t.Fatalf("đọc chứng chỉ: %+v err=%v", stored, err)
	}

	parent := rc.user()
	relation := model.ParentStudentRelation{ParentUserID: parent, StudentUserID: rc.student, Status: model.ParentStudentStatusActive}
	if err := rc.db.Create(&relation).Error; err != nil {
		t.Fatalf("tạo liên kết phụ huynh: %v", err)
	}

	if got, err := rc.certSvc.GetCertificateByID(ctx, stored.ID, parent, false); err != nil || got == nil || got.ID != stored.ID {
		t.Fatalf("phụ huynh active: got=%+v err=%v, muốn xem được", got, err)
	}
	if _, err := rc.certSvc.GetCertificateByID(ctx, stored.ID, rc.user(), false); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("người lạ: err=%v, muốn ErrCertificateNotFound", err)
	}

	rc.exec("UPDATE parent_student_relations SET status = ? WHERE id = ?", model.ParentStudentStatusRevoked, relation.ID)
	if _, err := rc.certSvc.GetCertificateByID(ctx, stored.ID, parent, false); !errors.Is(err, ErrCertificateNotFound) {
		t.Fatalf("liên kết đã huỷ: err=%v, muốn ErrCertificateNotFound", err)
	}
}
