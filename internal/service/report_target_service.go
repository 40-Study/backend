package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ADM-N4 - contract: plans/261013-qa2-role-findings/contract-be3.md §1a.

// reportTargetNameMaxRunes: tên hiển thị của review/thảo luận lấy từ nội dung nên cắt gọn.
const reportTargetNameMaxRunes = 80

// ReportTargetResolverInterface: tra tóm tắt đối tượng bị báo cáo cho một lô report (key = report id).
type ReportTargetResolverInterface interface {
	Resolve(ctx context.Context, reports []model.Report) (map[uuid.UUID]*dto.ReportTargetDTO, error)
}

type ReportTargetResolver struct {
	repo repository.ReportTargetRepositoryInterface
}

func NewReportTargetResolver(repo repository.ReportTargetRepositoryInterface) *ReportTargetResolver {
	return &ReportTargetResolver{repo: repo}
}

func (r *ReportTargetResolver) Resolve(ctx context.Context, reports []model.Report) (map[uuid.UUID]*dto.ReportTargetDTO, error) {
	idsByType := map[string][]uuid.UUID{}
	seen := map[string]map[uuid.UUID]bool{}
	for _, rep := range reports {
		if seen[rep.ReportedType] == nil {
			seen[rep.ReportedType] = map[uuid.UUID]bool{}
		}
		if !seen[rep.ReportedType][rep.ReportedID] {
			seen[rep.ReportedType][rep.ReportedID] = true
			idsByType[rep.ReportedType] = append(idsByType[rep.ReportedType], rep.ReportedID)
		}
	}
	found, err := r.repo.FindTargets(ctx, idsByType)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]*dto.ReportTargetDTO, len(reports))
	for _, rep := range reports {
		var row *repository.ReportTargetRow
		if r, ok := found[rep.ReportedType][rep.ReportedID]; ok {
			row = &r
		}
		out[rep.ID] = buildReportTarget(rep.ReportedType, rep.ReportedID, row)
	}
	return out, nil
}

func buildReportTarget(typ string, id uuid.UUID, row *repository.ReportTargetRow) *dto.ReportTargetDTO {
	t := &dto.ReportTargetDTO{Type: typ, ID: id, AvailableActions: []string{}}
	if row == nil {
		return t
	}
	t.Exists = true
	t.Name = reportTargetName(typ, row)
	if row.Status != "" {
		status := row.Status
		t.Status = &status
	}
	t.Slug, t.CourseID, t.CourseSlug, t.OwnerID = row.Slug, row.CourseID, row.CourseSlug, row.OwnerID
	t.AvailableActions = reportTargetActions(typ, row.Status)
	return t
}

func reportTargetName(typ string, row *repository.ReportTargetRow) *string {
	pick := func(vals ...*string) *string {
		for _, v := range vals {
			if v != nil && strings.TrimSpace(*v) != "" {
				return v
			}
		}
		return nil
	}
	switch typ {
	case "course":
		return row.Name
	case "user":
		return pick(row.Name, row.Text)
	case "discussion":
		return truncateReportName(pick(row.Name, row.Text), reportTargetNameMaxRunes)
	case "review":
		return truncateReportName(pick(row.Name), reportTargetNameMaxRunes)
	}
	return nil
}

func truncateReportName(s *string, max int) *string {
	if s == nil {
		return nil
	}
	r := []rune(strings.TrimSpace(*s))
	if len(r) > max {
		r = append(r[:max], '…')
	}
	out := string(r)
	return &out
}

// reportTargetActions: các hành động POST /reports/:id/actions đang áp dụng được cho đối tượng ở trạng thái này.
// User không có hành động ở đây (khoá tài khoản đi qua PUT /users/:id/status), review không có trạng thái ẩn.
func reportTargetActions(typ, status string) []string {
	switch {
	case typ == "course" && status == model.CourseStatusPublished:
		return []string{repository.ReportActionUnpublishCourse}
	case typ == "discussion" && status == "visible":
		return []string{repository.ReportActionHideDiscussion}
	case typ == "discussion" && status == "hidden":
		return []string{repository.ReportActionUnhideDiscussion}
	}
	return []string{}
}
