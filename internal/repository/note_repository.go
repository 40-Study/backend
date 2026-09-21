package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// NoteRepositoryInterface — CRUD cho ghi chú theo mốc thời gian (Phase 1 §3).
type NoteRepositoryInterface interface {
	Create(ctx context.Context, note *model.UserNote) error
	// GetByID luôn Preload("Lesson.Section") — mọi nơi đọc một ghi chú (kiểm quyền trước khi
	// sửa/xoá, hoặc trả về sau khi tạo/sửa) đều cần lesson_title/section_title cho response.
	GetByID(ctx context.Context, id uuid.UUID) (*model.UserNote, error)
	// ListByLesson trả ghi chú CỦA userID trong MỘT bài học — sort "oldest" (created_at ASC)
	// hoặc mặc định "newest" (created_at DESC).
	ListByLesson(ctx context.Context, userID, lessonID uuid.UUID, sort string) ([]model.UserNote, error)
	// ListByCourse trả ghi chú CỦA userID trong TOÀN BỘ khoá (mọi chương) — sectionID khác nil
	// lọc về đúng một chương ("trong chương hiện tại" ở UI); sectionID nil trả tất cả.
	ListByCourse(ctx context.Context, userID, courseID uuid.UUID, sectionID *uuid.UUID, sort string) ([]model.UserNote, error)
	Update(ctx context.Context, note *model.UserNote) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type NoteRepository struct {
	db *gorm.DB
}

func NewNoteRepository(db *gorm.DB) *NoteRepository {
	return &NoteRepository{db: db}
}

func (r *NoteRepository) Create(ctx context.Context, note *model.UserNote) error {
	return r.db.WithContext(ctx).Create(note).Error
}

func (r *NoteRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.UserNote, error) {
	var note model.UserNote
	err := r.db.WithContext(ctx).
		Preload("Lesson.Section").
		First(&note, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &note, nil
}

// orderBySort dịch "newest"/"oldest" (contract §3) sang mệnh đề ORDER BY — mặc định "newest"
// cho bất kỳ giá trị nào khác "oldest" (kể cả rỗng), đúng ý contract "mặc định newest".
//
// R5 (code-reviewer-260919-1557): cột PHẢI có tiền tố bảng "user_notes." — ListByCourse khi có
// sectionID sẽ JOIN "lessons" (cũng có cột created_at), và Postgres từ chối cả câu SELECT với
// lỗi "column reference \"created_at\" is ambiguous" nếu ORDER BY không nói rõ created_at của
// bảng nào (500 cho toàn bộ request, không phải sắp xếp sai). Kèm tiebreaker "id" (R13, cùng báo
// cáo review) để hai ghi chú tạo cùng một mili-giây không đổi chỗ giữa các lần gọi.
func orderBySort(sort string) string {
	if sort == "oldest" {
		return "user_notes.created_at ASC, user_notes.id ASC"
	}
	return "user_notes.created_at DESC, user_notes.id DESC"
}

func (r *NoteRepository) ListByLesson(ctx context.Context, userID, lessonID uuid.UUID, sort string) ([]model.UserNote, error) {
	var notes []model.UserNote
	err := r.db.WithContext(ctx).
		Preload("Lesson.Section").
		Where("user_id = ? AND lesson_id = ?", userID, lessonID).
		Order(orderBySort(sort)).
		Find(&notes).Error
	return notes, err
}

func (r *NoteRepository) ListByCourse(ctx context.Context, userID, courseID uuid.UUID, sectionID *uuid.UUID, sort string) ([]model.UserNote, error) {
	query := r.db.WithContext(ctx).
		Preload("Lesson.Section").
		Where("user_notes.user_id = ? AND user_notes.course_id = ?", userID, courseID)

	if sectionID != nil {
		// section_id không nằm trực tiếp trên user_notes — lọc qua JOIN lessons.
		query = query.
			Joins("JOIN lessons ON lessons.id = user_notes.lesson_id").
			Where("lessons.section_id = ?", *sectionID)
	}

	var notes []model.UserNote
	err := query.Order(orderBySort(sort)).Find(&notes).Error
	return notes, err
}

func (r *NoteRepository) Update(ctx context.Context, note *model.UserNote) error {
	return r.db.WithContext(ctx).Save(note).Error
}

func (r *NoteRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.UserNote{}, "id = ?", id).Error
}
