package service

// QA vòng 2 (28/09/2026), lane D:
//   - Q5/D3: khoá đang chờ duyệt thì KHÔNG sửa được thông tin, chương, bài, nội dung bài.
//   - D4: khoá nháp/chờ duyệt/bị từ chối chỉ chủ khoá, admin, người đã ghi danh xem được.
//   - Reorder chương/bài phải kiểm chủ sở hữu (trước đây không nhận người gọi).
// Repo là stub trong bộ nhớ: đây là test luật nghiệp vụ, câu SQL đã có test Postgres riêng.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type editLockCourseRepo struct {
	repository.CourseRepositoryInterface
	course  *model.Course
	updated bool
}

func (r *editLockCourseRepo) GetByID(context.Context, uuid.UUID) (*model.Course, error) {
	return r.course, nil
}
func (r *editLockCourseRepo) GetDetailByID(context.Context, uuid.UUID) (*model.Course, error) {
	return r.course, nil
}
func (r *editLockCourseRepo) Update(context.Context, *model.Course) error { r.updated = true; return nil }
func (r *editLockCourseRepo) Delete(context.Context, uuid.UUID) error        { r.updated = true; return nil }
func (r *editLockCourseRepo) RecalculateLessonStats(context.Context, uuid.UUID) error {
	return nil
}

type editLockSectionRepo struct {
	repository.SectionRepositoryInterface
	section *model.Section
	writes  int
}

func (r *editLockSectionRepo) GetByID(context.Context, uuid.UUID) (*model.Section, error) {
	return r.section, nil
}
func (r *editLockSectionRepo) GetAllByCourseID(context.Context, uuid.UUID) ([]model.Section, error) {
	return []model.Section{*r.section}, nil
}
func (r *editLockSectionRepo) GetMaxDisplayOrder(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (r *editLockSectionRepo) Create(context.Context, *model.Section) error { r.writes++; return nil }
func (r *editLockSectionRepo) Update(context.Context, *model.Section) error { r.writes++; return nil }
func (r *editLockSectionRepo) Delete(context.Context, uuid.UUID) error      { r.writes++; return nil }
func (r *editLockSectionRepo) BelongsToCourse(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}
func (r *editLockSectionRepo) Reorder(context.Context, []repository.ReorderItem) error {
	r.writes++
	return nil
}

type editLockLessonRepo struct {
	repository.LessonRepositoryInterface
	lesson *model.Lesson
	writes int
}

func (r *editLockLessonRepo) GetByID(context.Context, uuid.UUID) (*model.Lesson, error) {
	return r.lesson, nil
}
func (r *editLockLessonRepo) GetAllBySectionID(context.Context, uuid.UUID) ([]model.Lesson, error) {
	return []model.Lesson{*r.lesson}, nil
}
func (r *editLockLessonRepo) GetMaxDisplayOrder(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (r *editLockLessonRepo) Create(context.Context, *model.Lesson) error { r.writes++; return nil }
func (r *editLockLessonRepo) Update(context.Context, *model.Lesson) error { r.writes++; return nil }
func (r *editLockLessonRepo) Delete(context.Context, uuid.UUID) error     { r.writes++; return nil }
func (r *editLockLessonRepo) CreateContent(context.Context, *model.LessonContent) error {
	r.writes++
	return nil
}
func (r *editLockLessonRepo) GetContentsByLessonID(context.Context, uuid.UUID) ([]model.LessonContent, error) {
	return nil, nil
}
func (r *editLockLessonRepo) CountByIDsAndSection(_ context.Context, ids []uuid.UUID, _ uuid.UUID) (int64, error) {
	return int64(len(ids)), nil
}
func (r *editLockLessonRepo) Reorder(context.Context, []repository.ReorderItem) error {
	r.writes++
	return nil
}

type editLockEnrollmentRepo struct {
	repository.EnrollmentRepositoryInterface
	enrolledUser uuid.UUID
	lessonOrder  []repository.LessonOrderInfo
}

func (r *editLockEnrollmentRepo) GetByUserAndCourse(_ context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	if r.enrolledUser != uuid.Nil && userID == r.enrolledUser {
		return &model.Enrollment{UserID: userID, CourseID: courseID}, nil
	}
	return nil, nil
}
func (r *editLockEnrollmentRepo) GetLessonOrderInfoByCourseID(context.Context, uuid.UUID) ([]repository.LessonOrderInfo, error) {
	return r.lessonOrder, nil
}
func (r *editLockEnrollmentRepo) GetLessonProgressMapByUserAndCourse(context.Context, uuid.UUID, uuid.UUID) (map[uuid.UUID]*model.LessonProgress, error) {
	return map[uuid.UUID]*model.LessonProgress{}, nil
}
func (r *editLockEnrollmentRepo) GetCourseIDByLessonID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

type editLockFixture struct {
	owner      uuid.UUID
	course     *editLockCourseRepo
	sections   *editLockSectionRepo
	lessons    *editLockLessonRepo
	enrollment *editLockEnrollmentRepo
}

func newEditLockFixture(status string) *editLockFixture {
	owner := uuid.New()
	course := &model.Course{InstructorID: owner, Status: status}
	course.ID = uuid.New()
	section := &model.Section{CourseID: course.ID, Title: "QA-chuong"}
	section.ID = uuid.New()
	lesson := &model.Lesson{ID: uuid.New(), SectionID: section.ID, Title: "QA-bai"}
	section.Lessons = []model.Lesson{*lesson}
	return &editLockFixture{
		owner:      owner,
		course:     &editLockCourseRepo{course: course},
		sections:   &editLockSectionRepo{section: section},
		lessons:    &editLockLessonRepo{lesson: lesson},
		enrollment: &editLockEnrollmentRepo{},
	}
}

func (f *editLockFixture) courseID() uuid.UUID  { return f.course.course.ID }
func (f *editLockFixture) sectionID() uuid.UUID { return f.sections.section.ID }
func (f *editLockFixture) lessonID() uuid.UUID  { return f.lessons.lesson.ID }

func (f *editLockFixture) sectionSvc() *SectionService {
	return NewSectionService(f.sections, f.course, f.enrollment)
}
func (f *editLockFixture) lessonSvc() *LessonService {
	return NewLessonService(f.lessons, f.sections, f.course, f.enrollment)
}

func title(s string) *string { return &s }

// Q5: MỌI thao tác ghi trên khoá đang chờ duyệt trả ErrCourseLockedForReview và không ghi gì —
// kể cả admin (admin duyệt đúng nội dung đã gửi, không sửa hộ).
func TestCourseEditLock_PendingReviewBlocksEveryWrite(t *testing.T) {
	ctx := context.Background()
	for _, asAdmin := range []bool{false, true} {
		f := newEditLockFixture(model.CourseStatusPendingReview)
		actor := f.owner
		if asAdmin {
			actor = uuid.New()
		}
		ops := map[string]func() error{
			// Chủ dự án chốt 28/09: xoá khoá đang chờ duyệt cũng bị khoá.
			"DeleteCourse": func() error {
				return NewCourseService(f.course, nil, nil, nil).DeleteCourse(ctx, f.courseID(), actor, asAdmin)
			},
			"UpdateCourse": func() error {
				_, err := NewCourseService(f.course, nil, nil, nil).UpdateCourse(ctx, f.courseID(), actor, asAdmin, dto.UpdateCourseDTO{Title: title("QA-sua")})
				return err
			},
			"UpdateSection": func() error {
				_, err := f.sectionSvc().UpdateSection(ctx, f.courseID(), f.sectionID(), actor, asAdmin, dto.UpdateSectionDTO{Title: title("QA-sua")})
				return err
			},
			"DeleteSection": func() error { return f.sectionSvc().DeleteSection(ctx, f.courseID(), f.sectionID(), actor, asAdmin) },
			"ReorderSections": func() error {
				return f.sectionSvc().ReorderSections(ctx, f.courseID(), actor, asAdmin, dto.ReorderDTO{})
			},
			"UpdateLesson": func() error {
				_, err := f.lessonSvc().UpdateLesson(ctx, f.lessonID(), actor, asAdmin, dto.UpdateLessonDTO{Title: title("QA-sua")})
				return err
			},
			"DeleteLesson": func() error { return f.lessonSvc().DeleteLesson(ctx, f.lessonID(), actor, asAdmin) },
			"ReorderLessons": func() error {
				return f.lessonSvc().ReorderLessons(ctx, f.sectionID(), actor, asAdmin, dto.ReorderDTO{})
			},
			// Nội dung bài (video/bài tập) đi qua requireLessonCourseOwnerOrAdmin dùng chung.
			"CreateContent": func() error {
				_, err := NewLessonContentService(f.lessons, f.sections, f.course, f.enrollment, nil).
					CreateContent(ctx, f.lessonID(), actor, asAdmin, dto.CreateLessonContentDTO{Type: "exercise", Title: title("QA-noi-dung")})
				return err
			},
		}
		if !asAdmin {
			// Tạo chương/bài không có nhánh admin (C-12 vòng 2) — chỉ chủ khoá.
			ops["CreateSection"] = func() error {
				_, err := f.sectionSvc().CreateSection(ctx, f.courseID(), actor, dto.CreateSectionDTO{Title: "QA-chuong-moi"})
				return err
			}
			ops["CreateLesson"] = func() error {
				_, err := f.lessonSvc().CreateLesson(ctx, f.sectionID(), actor, dto.CreateLessonDTO{Title: "QA-bai-moi"})
				return err
			}
		}
		for name, op := range ops {
			if err := op(); !errors.Is(err, ErrCourseLockedForReview) {
				t.Errorf("admin=%v %s khi pending_review: err=%v, muốn ErrCourseLockedForReview", asAdmin, name, err)
			}
		}
		if f.course.updated || f.sections.writes != 0 || f.lessons.writes != 0 {
			t.Errorf("admin=%v: bị chặn nhưng vẫn ghi (course=%v section=%d lesson=%d)", asAdmin, f.course.updated, f.sections.writes, f.lessons.writes)
		}
	}
}

// Khoá nháp (và bị từ chối) vẫn sửa được bình thường — guard chỉ nhắm pending_review.
func TestCourseEditLock_DraftAndRejectedStayEditable(t *testing.T) {
	ctx := context.Background()
	for _, status := range []string{model.CourseStatusDraft, model.CourseStatusRejected, model.CourseStatusPublished} {
		f := newEditLockFixture(status)
		if _, err := f.sectionSvc().CreateSection(ctx, f.courseID(), f.owner, dto.CreateSectionDTO{Title: "QA-chuong"}); err != nil {
			t.Errorf("%s: CreateSection: %v", status, err)
		}
		if _, err := f.lessonSvc().CreateLesson(ctx, f.sectionID(), f.owner, dto.CreateLessonDTO{Title: "QA-bai"}); err != nil {
			t.Errorf("%s: CreateLesson: %v", status, err)
		}
		// Đổi mô tả (không đổi title nên không cần SlugExists).
		if _, err := NewCourseService(f.course, nil, nil, nil).UpdateCourse(ctx, f.courseID(), f.owner, false, dto.UpdateCourseDTO{Description: title("QA-mo-ta")}); err != nil || !f.course.updated {
			t.Errorf("%s: UpdateCourse: err=%v updated=%v", status, err, f.course.updated)
		}
	}
}

// Reorder trước đây không kiểm chủ sở hữu: giảng viên khác sắp lại được chương/bài của khoá người khác.
func TestCourseEditLock_ReorderRequiresOwner(t *testing.T) {
	ctx := context.Background()
	f := newEditLockFixture(model.CourseStatusDraft)
	stranger := uuid.New()
	if err := f.sectionSvc().ReorderSections(ctx, f.courseID(), stranger, false, dto.ReorderDTO{}); !errors.Is(err, ErrNotSectionCourseOwner) {
		t.Fatalf("ReorderSections người lạ: err=%v, muốn ErrNotSectionCourseOwner", err)
	}
	if err := f.lessonSvc().ReorderLessons(ctx, f.sectionID(), stranger, false, dto.ReorderDTO{}); !errors.Is(err, ErrNotLessonCourseOwner) {
		t.Fatalf("ReorderLessons người lạ: err=%v, muốn ErrNotLessonCourseOwner", err)
	}
	if f.sections.writes != 0 || f.lessons.writes != 0 {
		t.Fatal("người lạ bị chặn nhưng vẫn ghi thứ tự")
	}
	if err := f.sectionSvc().ReorderSections(ctx, f.courseID(), f.owner, false, dto.ReorderDTO{}); err != nil {
		t.Fatalf("chủ khoá ReorderSections: %v", err)
	}
	if err := f.lessonSvc().ReorderLessons(ctx, f.sectionID(), f.owner, false, dto.ReorderDTO{}); err != nil {
		t.Fatalf("chủ khoá ReorderLessons: %v", err)
	}
}

// Bảng đầy đủ: MỌI trạng thái khoá phải được xếp loại công khai/riêng tư ở đây. Thêm trạng thái
// mới vào model.CourseStatuses mà quên quyết định ai được xem -> test đỏ.
func TestCanViewCourse_EveryStatusClassified(t *testing.T) {
	private := map[string]bool{
		model.CourseStatusDraft:         true,
		model.CourseStatusPendingReview: true,
		model.CourseStatusRejected:      true,
		model.CourseStatusPublished:     false,
		model.CourseStatusArchived:      false,
	}
	owner, stranger := uuid.New(), uuid.New()
	for _, s := range model.CourseStatuses {
		want, ok := private[s]
		if !ok {
			t.Fatalf("trạng thái %q chưa được xếp loại công khai/riêng tư trong canViewCourse", s)
		}
		c := &model.Course{InstructorID: owner, Status: s}
		if got := canViewCourse(c, stranger, false, false); got == want {
			t.Errorf("%s: người lạ xem được = %v, muốn %v", s, got, !want)
		}
		if !canViewCourse(c, owner, false, false) || !canViewCourse(c, stranger, true, false) || !canViewCourse(c, stranger, false, true) {
			t.Errorf("%s: chủ khoá/admin/người đã ghi danh phải luôn xem được", s)
		}
	}
	if canViewCourse(&model.Course{InstructorID: uuid.Nil, Status: model.CourseStatusDraft}, uuid.Nil, false, false) {
		t.Error("viewer uuid.Nil không được khớp khoá có instructor_id rỗng")
	}
}

// D4: giảng viên khác GET khoá nháp / chương / danh sách bài / 1 bài -> ErrCourseHidden (404); chủ
// khoá, admin, người đã ghi danh vẫn đọc được.
func TestCourseVisibility_DraftHiddenFromOtherTeacher(t *testing.T) {
	ctx := context.Background()
	for _, status := range []string{model.CourseStatusDraft, model.CourseStatusPendingReview, model.CourseStatusRejected} {
		f := newEditLockFixture(status)
		// Bài preview: trước review PR #79, nhánh preview của GetContentsByLessonID bỏ qua mọi kiểm
		// tra nên lộ video_url bài preview của khoá nháp.
		f.lessons.lesson.IsPreview = true
		f.enrollment.lessonOrder = []repository.LessonOrderInfo{{ID: f.lessonID(), IsPreview: true}}
		student := uuid.New()
		f.enrollment.enrolledUser = student
		reads := func(viewer uuid.UUID, isAdmin bool) map[string]error {
			_, e1 := NewCourseService(f.course, nil, nil, f.enrollment).GetCourseByID(ctx, f.courseID(), viewer, isAdmin)
			_, e2 := f.sectionSvc().GetAllSections(ctx, f.courseID(), viewer, isAdmin)
			_, e3 := f.sectionSvc().GetSectionByID(ctx, f.sectionID(), viewer, isAdmin)
			_, e4 := f.lessonSvc().GetAllLessons(ctx, f.sectionID(), viewer, isAdmin)
			_, e5 := f.lessonSvc().GetLessonByID(ctx, f.lessonID(), viewer, isAdmin)
			_, e6 := NewLessonContentService(f.lessons, f.sections, f.course, f.enrollment, nil).GetContentsByLessonID(ctx, f.lessonID(), viewer, isAdmin)
			return map[string]error{"GetCourseByID": e1, "GetAllSections": e2, "GetSectionByID": e3, "GetAllLessons": e4,
				"GetLessonByID": e5, "GetContentsByLessonID(preview)": e6}
		}
		for name, err := range reads(uuid.New(), false) {
			if !errors.Is(err, ErrCourseHidden) {
				t.Errorf("%s: GV khác %s: err=%v, muốn ErrCourseHidden", status, name, err)
			}
		}
		for who, v := range map[string]struct {
			id    uuid.UUID
			admin bool
		}{"chủ khoá": {f.owner, false}, "admin": {uuid.New(), true}, "học viên đã ghi danh": {student, false}} {
			for name, err := range reads(v.id, v.admin) {
				if err != nil {
					t.Errorf("%s: %s %s: %v", status, who, name, err)
				}
			}
		}
	}
	// Khoá đã xuất bản: ai đăng nhập cũng đọc được như trước.
	f := newEditLockFixture(model.CourseStatusPublished)
	if _, err := f.sectionSvc().GetAllSections(ctx, f.courseID(), uuid.New(), false); err != nil {
		t.Fatalf("published: GV khác đọc giáo trình: %v", err)
	}
}
