package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrNotNoteOwner (Phase 1 §3): "chỉ chủ sở hữu đọc/sửa/xoá" — dùng chung cho Update/Delete/
// GetByID khi note.UserID khác actorUserID. Handler ánh xạ sang 403.
var ErrNotNoteOwner = errors.New("forbidden: not the note owner")

// ErrNoteNotEnrolled (Phase 1 §3): "yêu cầu user đã enroll khoá" — chặn tạo ghi chú cho một
// bài học thuộc khoá mà người dùng chưa từng ghi danh.
var ErrNoteNotEnrolled = errors.New("not enrolled in the course containing this lesson")

type NoteServiceInterface interface {
	CreateNote(ctx context.Context, userID, lessonID uuid.UUID, req dto.CreateNoteDTO) (*dto.NoteResponseDTO, error)
	ListByLesson(ctx context.Context, userID, lessonID uuid.UUID, sort string) ([]dto.NoteResponseDTO, error)
	ListByCourse(ctx context.Context, userID, courseID uuid.UUID, sectionID *uuid.UUID, sort string) ([]dto.NoteResponseDTO, error)
	UpdateNote(ctx context.Context, userID, noteID uuid.UUID, req dto.UpdateNoteDTO) (*dto.NoteResponseDTO, error)
	DeleteNote(ctx context.Context, userID, noteID uuid.UUID) error
}

type NoteService struct {
	noteRepo       repository.NoteRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
}

func NewNoteService(
	noteRepo repository.NoteRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
) *NoteService {
	return &NoteService{noteRepo: noteRepo, enrollmentRepo: enrollmentRepo}
}

func (s *NoteService) CreateNote(ctx context.Context, userID, lessonID uuid.UUID, req dto.CreateNoteDTO) (*dto.NoteResponseDTO, error) {
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, ErrNoteNotEnrolled
	}

	note := &model.UserNote{
		UserID:        userID,
		LessonID:      lessonID,
		CourseID:      courseID,
		TimestampSecs: req.TimestampSecs,
		Content:       req.Content,
	}
	if err := s.noteRepo.Create(ctx, note); err != nil {
		return nil, err
	}

	// Doc lai de co Lesson.Section (lesson_title/section_title) cho response — Create() chi
	// tra ve ban ghi vua ghi, chua preload quan he.
	created, err := s.noteRepo.GetByID(ctx, note.ID)
	if err != nil || created == nil {
		return nil, errors.New("failed to load created note")
	}
	return toNoteResponseDTO(created), nil
}

func (s *NoteService) ListByLesson(ctx context.Context, userID, lessonID uuid.UUID, sort string) ([]dto.NoteResponseDTO, error) {
	notes, err := s.noteRepo.ListByLesson(ctx, userID, lessonID, sort)
	if err != nil {
		return nil, err
	}
	return toNoteResponseDTOs(notes), nil
}

func (s *NoteService) ListByCourse(ctx context.Context, userID, courseID uuid.UUID, sectionID *uuid.UUID, sort string) ([]dto.NoteResponseDTO, error) {
	notes, err := s.noteRepo.ListByCourse(ctx, userID, courseID, sectionID, sort)
	if err != nil {
		return nil, err
	}
	return toNoteResponseDTOs(notes), nil
}

func (s *NoteService) UpdateNote(ctx context.Context, userID, noteID uuid.UUID, req dto.UpdateNoteDTO) (*dto.NoteResponseDTO, error) {
	note, err := s.noteRepo.GetByID(ctx, noteID)
	if err != nil {
		return nil, err
	}
	if note == nil {
		return nil, errors.New("note not found")
	}
	if note.UserID != userID {
		return nil, ErrNotNoteOwner
	}

	if req.TimestampSecs != nil {
		note.TimestampSecs = *req.TimestampSecs
	}
	if req.Content != nil {
		note.Content = *req.Content
	}
	if err := s.noteRepo.Update(ctx, note); err != nil {
		return nil, err
	}
	return toNoteResponseDTO(note), nil
}

func (s *NoteService) DeleteNote(ctx context.Context, userID, noteID uuid.UUID) error {
	note, err := s.noteRepo.GetByID(ctx, noteID)
	if err != nil {
		return err
	}
	if note == nil {
		return errors.New("note not found")
	}
	if note.UserID != userID {
		return ErrNotNoteOwner
	}
	return s.noteRepo.Delete(ctx, noteID)
}

func toNoteResponseDTOs(notes []model.UserNote) []dto.NoteResponseDTO {
	result := make([]dto.NoteResponseDTO, len(notes))
	for i := range notes {
		result[i] = *toNoteResponseDTO(&notes[i])
	}
	return result
}

func toNoteResponseDTO(n *model.UserNote) *dto.NoteResponseDTO {
	return &dto.NoteResponseDTO{
		ID:            n.ID,
		LessonID:      n.LessonID,
		LessonTitle:   n.Lesson.Title,
		SectionID:     n.Lesson.SectionID,
		SectionTitle:  n.Lesson.Section.Title,
		CourseID:      n.CourseID,
		TimestampSecs: n.TimestampSecs,
		Content:       n.Content,
		CreatedAt:     n.CreatedAt,
		UpdatedAt:     n.UpdatedAt,
	}
}
