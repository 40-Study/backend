package service

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

// annotateTimetableOrganizations gắn tổ chức sở hữu lớp vào từng mục thời khoá biểu, để client phân biệt lớp
// cá nhân (dạy tự do) với lớp của tổ chức. Chạy SAU khi đọc cache nên mục cache cũ (trước khi có trường này)
// vẫn được gắn đúng. Một truy vấn cho cả danh sách lớp.
func annotateTimetableOrganizations(ctx context.Context, repo repository.ScheduleRepositoryInterface, entries []dto.TimetableEntryDTO) error {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]struct{}, len(entries))
	classIDs := make([]uuid.UUID, 0, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.ClassID]; !ok {
			seen[e.ClassID] = struct{}{}
			classIDs = append(classIDs, e.ClassID)
		}
	}
	orgs, err := repo.GetClassOrganizations(ctx, classIDs)
	if err != nil {
		return err
	}
	for i := range entries {
		org := orgs[entries[i].ClassID]
		entries[i].OrganizationID = org.ID
		entries[i].OrganizationName = org.Name
	}
	return nil
}
