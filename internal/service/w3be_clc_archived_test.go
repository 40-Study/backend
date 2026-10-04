package service

// W3-BE: gán nội dung bài học vào lớp (/lesson-contents/:id/classes) là thao tác GHI vào lớp, nên lớp đã lưu trữ cũng chỉ
// đọc: gán, gán cả lô, đổi lịch, gỡ đều trả ErrClassArchived và KHÔNG chạm dữ liệu; lớp đang hoạt động đi bình thường.
// Quyền xét trước: người ngoài lớp vẫn nhận ErrClassNotFound dù lớp đã lưu trữ. Bỏ ensureClassWriteOpen ở một hàm thì
// dòng tương ứng ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestW3BE_ClassLessonContent_LopLuuTruChiDoc(t *testing.T) {
	courseID, classID, teacher, stranger := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ctx := context.Background()
	contentID := uuid.New()
	open := "2031-01-01T00:00:00Z"

	build := func(status string, isTeacher bool) (*ClassLessonContentService, *fakeCLCRepoAuthz) {
		svc, classRepo, clcRepo := newCLCServiceForAuthz(courseID)
		classRepo.class = &model.Class{BaseModel: model.BaseModel{ID: classID}, CourseID: &courseID, Status: status}
		classRepo.isTeacher = isTeacher
		return svc, clcRepo
	}
	ops := map[string]func(svc *ClassLessonContentService, actor uuid.UUID) error{
		"gán lớp vào nội dung": func(svc *ClassLessonContentService, actor uuid.UUID) error {
			_, err := svc.AssignClassToContent(ctx, contentID, actor, false, dto.AssignClassToContentDTO{ClassID: classID})
			return err
		},
		"gán lô": func(svc *ClassLessonContentService, actor uuid.UUID) error {
			_, err := svc.BulkAssignClassesToContent(ctx, contentID, actor, false, dto.BulkAssignClassesToContentDTO{ClassIDs: []uuid.UUID{classID}})
			return err
		},
		"đổi lịch": func(svc *ClassLessonContentService, actor uuid.UUID) error {
			_, err := svc.UpdateClassContentSchedule(ctx, contentID, classID, actor, false, dto.UpdateClassContentScheduleDTO{OpenDate: &open})
			return err
		},
		"gỡ lớp": func(svc *ClassLessonContentService, actor uuid.UUID) error {
			return svc.RemoveClassFromContent(ctx, contentID, classID, actor, false)
		},
	}

	for name, run := range ops {
		t.Run(name, func(t *testing.T) {
			archived, repo := build("archived", true)
			if err := run(archived, teacher); !errors.Is(err, ErrClassArchived) {
				t.Errorf("lớp archived, giảng viên lớp: err=%v, muốn ErrClassArchived", err)
			}
			if repo.createCalls != 0 || repo.createBatchCall != 0 {
				t.Errorf("ghi dữ liệu dù lớp archived: create=%d batch=%d", repo.createCalls, repo.createBatchCall)
			}
			// Quyền trước, lưu trữ sau: người ngoài lớp không thấy 409.
			outsiderSvc, _ := build("archived", false)
			if err := run(outsiderSvc, stranger); !errors.Is(err, ErrClassNotFound) {
				t.Errorf("lớp archived, người ngoài: err=%v, muốn ErrClassNotFound", err)
			}
			active, _ := build("active", true)
			// Fake repo không có Update/Delete: panic ở đó nghĩa là đã QUA cổng lưu trữ (chỉ cần không bị ErrClassArchived).
			func() {
				defer func() { _ = recover() }()
				if err := run(active, teacher); errors.Is(err, ErrClassArchived) {
					t.Errorf("lớp active bị chặn nhầm: %v", err)
				}
			}()
		})
	}
}
