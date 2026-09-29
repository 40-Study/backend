package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// ErrAttendanceNotFound (S4): bản ghi điểm danh không tồn tại HOẶC không thuộc lớp trong URL. Handler trả 404.
var ErrAttendanceNotFound = errors.New("attendance not found")

// AttendanceServiceInterface (S4): điểm danh của LỚP (bảng attendances). Ghi (tạo, sửa, xoá) chỉ người quản
// lý lớp (ensureClassManage: giảng viên lớp, chủ khoá, người tạo, admin), người khác ErrNotClassTeacher (403).
// Đọc (danh sách, một bản ghi) cũng chỉ người quản lý lớp, người khác ErrClassNotFound (404): học viên và
// phụ huynh xem điểm danh qua endpoint riêng (/me/attendances, /parent/.../attendance, bảng session_attendances),
// không qua route quản lý này. Quyền luôn xét trên lớp trong URL, rồi bản ghi phải thuộc đúng lớp đó.
type AttendanceServiceInterface interface {
	MarkAttendance(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.BulkCreateAttendanceDTO) ([]dto.AttendanceResponseDTO, error)
	GetAllAttendances(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, dateStr string, page, pageSize int) (*dto.AttendanceListResponseDTO, error)
	GetAttendanceByID(ctx context.Context, classID, id, actorUserID uuid.UUID, isAdmin bool) (*dto.AttendanceResponseDTO, error)
	UpdateAttendance(ctx context.Context, classID, id, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateAttendanceDTO) (*dto.AttendanceResponseDTO, error)
	DeleteAttendance(ctx context.Context, classID, id, actorUserID uuid.UUID, isAdmin bool) error
}

type AttendanceService struct {
	repo       repository.AttendanceRepositoryInterface
	classRepo  repository.ClassRepositoryInterface
	courseRepo repository.CourseRepositoryInterface
}

func NewAttendanceService(repo repository.AttendanceRepositoryInterface, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface) *AttendanceService {
	return &AttendanceService{repo: repo, classRepo: classRepo, courseRepo: courseRepo}
}

// attendanceOfClass tải bản ghi điểm danh và bắt buộc nó thuộc lớp trong URL (nếu không, quyền quản lý lớp A
// sẽ cho sửa/xoá điểm danh của lớp B chỉ bằng cách đặt id của B sau classId của A).
func (s *AttendanceService) attendanceOfClass(ctx context.Context, classID, id uuid.UUID) (*model.Attendance, error) {
	attendance, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if attendance == nil || attendance.ClassID != classID {
		return nil, ErrAttendanceNotFound
	}
	return attendance, nil
}

// MarkAttendance records attendance for multiple students in a class for a specific date.
// This is used by teachers to mark student presence/absence.
func (s *AttendanceService) MarkAttendance(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.BulkCreateAttendanceDTO) ([]dto.AttendanceResponseDTO, error) {
	if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, actorUserID, classID, isAdmin); err != nil {
		return nil, err
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, errors.New("invalid date format, expected YYYY-MM-DD")
	}

	result := make([]dto.AttendanceResponseDTO, 0, len(req.Attendances))
	for _, entry := range req.Attendances {
		attendance := &model.Attendance{
			ID:        uuid.New(),
			ClassID:   classID,
			StudentID: entry.StudentID,
			Date:      date,
			Status:    entry.Status,
			Note:      entry.Note,
		}

		if err := s.repo.Create(ctx, attendance); err != nil {
			return nil, err
		}

		result = append(result, *toAttendanceResponseDTO(attendance))
	}

	return result, nil
}

func (s *AttendanceService) GetAllAttendances(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, dateStr string, page, pageSize int) (*dto.AttendanceListResponseDTO, error) {
	if err := ensureClassManageOrNotFound(ctx, s.classRepo, s.courseRepo, actorUserID, classID, isAdmin); err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var dateFilter *time.Time
	if dateStr != "" {
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, errors.New("invalid date format, expected YYYY-MM-DD")
		}
		dateFilter = &d
	}

	attendances, total, err := s.repo.GetAll(ctx, classID, dateFilter, page, pageSize)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.AttendanceResponseDTO, len(attendances))
	for i, a := range attendances {
		dtos[i] = *toAttendanceResponseDTO(&a)
	}

	return &dto.AttendanceListResponseDTO{
		Attendances: dtos,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *AttendanceService) GetAttendanceByID(ctx context.Context, classID, id, actorUserID uuid.UUID, isAdmin bool) (*dto.AttendanceResponseDTO, error) {
	if err := ensureClassManageOrNotFound(ctx, s.classRepo, s.courseRepo, actorUserID, classID, isAdmin); err != nil {
		return nil, err
	}
	attendance, err := s.attendanceOfClass(ctx, classID, id)
	if err != nil {
		return nil, err
	}
	return toAttendanceResponseDTO(attendance), nil
}

func (s *AttendanceService) UpdateAttendance(ctx context.Context, classID, id, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateAttendanceDTO) (*dto.AttendanceResponseDTO, error) {
	if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, actorUserID, classID, isAdmin); err != nil {
		return nil, err
	}
	attendance, err := s.attendanceOfClass(ctx, classID, id)
	if err != nil {
		return nil, err
	}

	if req.Status != nil {
		attendance.Status = *req.Status
	}
	if req.Note != nil {
		attendance.Note = req.Note
	}

	if err := s.repo.Update(ctx, attendance); err != nil {
		return nil, err
	}

	return toAttendanceResponseDTO(attendance), nil
}

func (s *AttendanceService) DeleteAttendance(ctx context.Context, classID, id, actorUserID uuid.UUID, isAdmin bool) error {
	if err := ensureClassManage(ctx, s.classRepo, s.courseRepo, actorUserID, classID, isAdmin); err != nil {
		return err
	}
	if _, err := s.attendanceOfClass(ctx, classID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func toAttendanceResponseDTO(a *model.Attendance) *dto.AttendanceResponseDTO {
	return &dto.AttendanceResponseDTO{
		ID:        a.ID,
		ClassID:   a.ClassID,
		StudentID: a.StudentID,
		Date:      a.Date.Format("2006-01-02"),
		Status:    a.Status,
		Note:      a.Note,
		CreatedAt: utils.FormatTimestamp(a.CreatedAt),
	}
}
