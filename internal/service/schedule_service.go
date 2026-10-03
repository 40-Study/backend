package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	asynq_queue "study.com/v1/internal/queue/asynq"
	"study.com/v1/internal/repository"
)

type ScheduleServiceInterface interface {
	// ClassSchedule
	CreateSchedule(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, req dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error)
	GetSchedulesByClass(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool) ([]dto.ClassScheduleResponseDTO, error)
	GetScheduleByID(ctx context.Context, id, actorID uuid.UUID, isAdmin bool) (*dto.ClassScheduleResponseDTO, error)
	UpdateSchedule(ctx context.Context, id, actorID uuid.UUID, isAdmin bool, req dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error)
	DeleteSchedule(ctx context.Context, id, actorID uuid.UUID, isAdmin bool) error

	// ClassSession
	CreateSession(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, req dto.CreateClassSessionDTO) (*dto.ClassSessionResponseDTO, error)
	GetSessionsByClass(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassSessionListDTO, error)
	GetSessionByID(ctx context.Context, id, actorID uuid.UUID, isAdmin bool) (*dto.ClassSessionResponseDTO, error)
	UpdateSession(ctx context.Context, id, actorID uuid.UUID, isAdmin bool, req dto.UpdateClassSessionDTO) (*dto.ClassSessionResponseDTO, error)
	CancelSession(ctx context.Context, id, actorID uuid.UUID, isAdmin bool, reason string) error
	GenerateSessions(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, req dto.GenerateSessionsDTO) ([]dto.ClassSessionResponseDTO, error)

	// SessionAttendance
	GetSessionAttendances(ctx context.Context, sessionID, requesterID uuid.UUID, isAdmin bool) ([]dto.SessionAttendanceResponseDTO, error)
	MarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.MarkAttendanceDTO, verifiedBy uuid.UUID, isAdmin bool) (*dto.SessionAttendanceResponseDTO, error)
	BulkMarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.BulkMarkAttendanceDTO, verifiedBy uuid.UUID, isAdmin bool) ([]dto.SessionAttendanceResponseDTO, error)
	UpdateAttendance(ctx context.Context, sessionID, id uuid.UUID, req dto.UpdateSessionAttendanceDTO, verifiedBy uuid.UUID, isAdmin bool) (*dto.SessionAttendanceResponseDTO, error)
	StudentCheckIn(ctx context.Context, sessionID, studentID uuid.UUID) (*dto.SessionAttendanceResponseDTO, error)
	StudentCheckOut(ctx context.Context, sessionID, studentID uuid.UUID) (*dto.SessionAttendanceResponseDTO, error)
	GetMyAttendances(ctx context.Context, studentID uuid.UUID, page, pageSize int) ([]dto.SessionAttendanceResponseDTO, int64, error)

	// Timetable
	GetClassTimetable(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool) (*dto.TimetableResponseDTO, error)
	GetMyTimetable(ctx context.Context, userID uuid.UUID, role string) (*dto.TimetableResponseDTO, error)
	GetMyTimetableWithSessions(ctx context.Context, userID uuid.UUID, role string, from, to time.Time) (*dto.TimetableResponseDTO, error)

	// Reminder
	GetReminderSettings(ctx context.Context, userID uuid.UUID) ([]dto.ReminderSettingResponseDTO, error)
	UpdateReminderSetting(ctx context.Context, userID uuid.UUID, req dto.UpdateReminderSettingDTO) (*dto.ReminderSettingResponseDTO, error)
}

type ScheduleService struct {
	repo       repository.ScheduleRepositoryInterface
	classRepo  repository.ClassRepositoryInterface
	courseRepo repository.CourseRepositoryInterface
	redis      *redis.Client
	queue      *asynq_queue.Queue
	authz      ClassAuthorizer
}

func NewScheduleService(
	repo repository.ScheduleRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	redis *redis.Client,
	queue *asynq_queue.Queue,
) *ScheduleService {
	return &ScheduleService{repo: repo, classRepo: classRepo, courseRepo: courseRepo, redis: redis, queue: queue}
}

// WithAuthorizer gắn PermissionChecker để chủ/quản trị tổ chức quản lý lịch, buổi học của lớp tổ chức mình (B-05).
func (s *ScheduleService) WithAuthorizer(authz ClassAuthorizer) *ScheduleService {
	s.authz = authz
	return s
}

// ============================================================================
// AUTHZ (S5): lịch học và buổi học của lớp. Trước đây không kiểm gì: học viên bất kỳ tạo, sửa, xoá,
// huỷ buổi và đọc lịch của lớp bất kỳ. Dùng lại helper của lớp (class_access.go), không định nghĩa quyền mới:
//   - ghi (lịch, buổi, điểm danh buổi): người quản lý lớp (giảng viên lớp, chủ khoá, người tạo) và admin;
//     người xem được lớp nhưng không quản lý -> 403; người không xem được -> 404.
//   - đọc lịch, buổi, thời khoá biểu: thành viên lớp, người quản lý lớp và admin; người khác -> 404.
// ============================================================================

// ErrScheduleNotFound / ErrClassSessionNotFound: bản ghi không tồn tại (404). Không dùng ErrSessionNotFound
// vì đó là buổi livestream, tài nguyên khác.
var (
	ErrScheduleNotFound     = errors.New("schedule not found")
	ErrClassSessionNotFound = errors.New("class session not found")
)

// Giới hạn tự điểm danh (lane P, câu hỏi mở 5 của rà soát phân quyền): trước đây học viên check-in buổi
// bất kỳ lúc nào, kể cả buổi năm ngoái hay buổi đã huỷ, nên "có mặt" không nói lên điều gì.
var (
	// ErrCheckInOutsideSessionDay: hôm nay (giờ Việt Nam) không phải ngày của buổi học.
	ErrCheckInOutsideSessionDay = errors.New("check-in is only open on the day of the session")
	// ErrSessionClosedForCheckIn: buổi đã huỷ hoặc đã kết thúc.
	ErrSessionClosedForCheckIn = errors.New("session is cancelled or already completed")
	// ErrGenerateRangeTooLong: khoảng ngày sinh buổi vượt maxGenerateSessionDays.
	ErrGenerateRangeTooLong = errors.New("date range for generating sessions is too long")
	// ErrSessionTimeOrder (B-10): giờ kết thúc không sau giờ bắt đầu. 400.
	ErrSessionTimeOrder = errors.New("giờ kết thúc phải sau giờ bắt đầu")
	// ErrSessionOverlap (B-10): buổi trùng giờ với buổi khác của cùng lớp hoặc của một giảng viên
	// của lớp. 409 (đúng người, xung đột lịch).
	ErrSessionOverlap = errors.New("buổi học trùng giờ với một buổi khác của lớp này hoặc của giảng viên lớp")
)

// ensureSessionSlotFree kiểm giờ của một buổi: kết thúc phải sau bắt đầu và không trùng buổi nào
// khác (cùng lớp, hoặc lớp khác có chung giảng viên). Buổi đã huỷ không tính là chiếm giờ.
// excludeID != nil khi sửa: không tự trùng với chính nó.
func (s *ScheduleService) ensureSessionSlotFree(ctx context.Context, classID uuid.UUID, date time.Time, start, end model.TimeOfDay, excludeID *uuid.UUID) error {
	// TimeOfDay luôn dạng "HH:MM" đủ 2 chữ số nên so chuỗi cũng là so thời gian (xem model.TimeOfDay).
	if end <= start {
		return ErrSessionTimeOrder
	}
	overlap, err := s.repo.HasOverlappingSession(ctx, classID, date, start, end, excludeID)
	if err != nil {
		return err
	}
	if overlap {
		return ErrSessionOverlap
	}
	return nil
}

const (
	// sessionDayUTCOffsetHours: múi giờ của "ngày buổi học". Cột class_sessions.date là kiểu date
	// (không múi giờ), còn giờ bắt đầu/kết thúc thì chưa thống nhất kiểu cột (rà soát S5, câu hỏi 9)
	// nên khung check-in chỉ dựa vào NGÀY. Việt Nam không có giờ mùa hè nên dùng độ lệch cố định.
	sessionDayUTCOffsetHours = 7
	// maxGenerateSessionDays: trần khoảng ngày của POST /classes/:classId/sessions/generate (một năm
	// có nhuận); trước đây một yêu cầu có thể sinh hàng chục nghìn buổi.
	maxGenerateSessionDays = 366
)

var sessionDayZone = time.FixedZone("ICT", sessionDayUTCOffsetHours*60*60)

// requireCheckInOpen: chỉ cho tự điểm danh khi buổi chưa huỷ/kết thúc và hôm nay đúng là ngày của buổi.
// Ngày của buổi lấy nguyên các trường lịch của cột date (đã là ngày, không có múi giờ).
func requireCheckInOpen(session *model.ClassSession, now time.Time) error {
	if session.Status == model.SessionCancelled || session.Status == model.SessionCompleted {
		return ErrSessionClosedForCheckIn
	}
	sy, sm, sd := session.Date.Date()
	ny, nm, nd := now.In(sessionDayZone).Date()
	if sy != ny || sm != nm || sd != nd {
		return ErrCheckInOutsideSessionDay
	}
	return nil
}

// requireClassWrite: người quản lý lớp và admin qua; người xem được lớp mà không quản lý nhận
// ErrNotClassTeacher (403); người không xem được nhận ErrClassNotFound (404).
func (s *ScheduleService) requireClassWrite(ctx context.Context, actorID uuid.UUID, isAdmin bool, classID uuid.UUID) error {
	elevated, err := classAccessAsAdmin(ctx, s.classRepo, s.authz, actorID, classID, isAdmin)
	if err != nil {
		return err
	}
	err = ensureClassManage(ctx, s.classRepo, s.courseRepo, actorID, classID, elevated)
	if !errors.Is(err, ErrNotClassTeacher) {
		return err
	}
	if visibleErr := ensureClassVisible(ctx, s.classRepo, s.courseRepo, actorID, classID, elevated); visibleErr != nil {
		return visibleErr
	}
	return ErrNotClassTeacher
}

// requireClassRead: thành viên lớp, người quản lý lớp, admin; người khác 404.
func (s *ScheduleService) requireClassRead(ctx context.Context, actorID uuid.UUID, isAdmin bool, classID uuid.UUID) error {
	elevated, err := classAccessAsAdmin(ctx, s.classRepo, s.authz, actorID, classID, isAdmin)
	if err != nil {
		return err
	}
	return ensureClassVisible(ctx, s.classRepo, s.courseRepo, actorID, classID, elevated)
}

// scheduleClass / sessionClass: lớp của bản ghi (quyền xét trên lớp THẬT của bản ghi, không tin :classId
// trên URL, nên đặt id lịch/buổi của lớp khác vào URL lớp mình không đi qua được).
func (s *ScheduleService) scheduleClass(ctx context.Context, id uuid.UUID) (*model.ClassSchedule, error) {
	schedule, err := s.repo.GetScheduleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		return nil, ErrScheduleNotFound
	}
	return schedule, nil
}

func (s *ScheduleService) sessionClass(ctx context.Context, id uuid.UUID) (*model.ClassSession, error) {
	session, err := s.repo.GetSessionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrClassSessionNotFound
	}
	return session, nil
}

// ============================================================================
// CACHE HELPERS
// ============================================================================

const (
	schedulesCachePrefix = "schedules:class:"
	sessionsCachePrefix  = "sessions:class:"
	timetableCachePrefix = "timetable:user:"
	scheduleCacheTTL     = 15 * time.Minute
)

func (s *ScheduleService) invalidateScheduleCache(ctx context.Context, classID uuid.UUID) {
	if s.redis != nil {
		s.redis.Del(ctx, schedulesCachePrefix+classID.String())
		// Also invalidate timetable caches (they'll be rebuilt on next access)
	}
}

func (s *ScheduleService) invalidateSessionCache(ctx context.Context, classID uuid.UUID) {
	if s.redis != nil {
		s.redis.Del(ctx, sessionsCachePrefix+classID.String())
	}
}

// ============================================================================
// CLASS SCHEDULE
// ============================================================================

func (s *ScheduleService) CreateSchedule(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, req dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	if err := s.requireClassWrite(ctx, actorID, isAdmin, classID); err != nil {
		return nil, err
	}
	effectiveFrom, err := time.Parse("2006-01-02", req.EffectiveFrom)
	if err != nil {
		return nil, errors.New("invalid effective_from date format, use YYYY-MM-DD")
	}
	startTime, err := parseClockField("start_time", req.StartTime)
	if err != nil {
		return nil, err
	}
	endTime, err := parseClockField("end_time", req.EndTime)
	if err != nil {
		return nil, err
	}

	schedule := &model.ClassSchedule{
		ClassID:       classID,
		DayOfWeek:     req.DayOfWeek,
		StartTime:     startTime,
		EndTime:       endTime,
		IsActive:      true,
		EffectiveFrom: effectiveFrom,
	}

	if req.Room != "" {
		schedule.Room = &req.Room
	}
	if req.EffectiveUntil != "" {
		t, err := time.Parse("2006-01-02", req.EffectiveUntil)
		if err != nil {
			return nil, errors.New("invalid effective_until date format, use YYYY-MM-DD")
		}
		schedule.EffectiveUntil = &t
	}

	if err := s.repo.CreateSchedule(ctx, schedule); err != nil {
		return nil, err
	}

	s.invalidateScheduleCache(ctx, classID)
	return s.mapScheduleToDTO(schedule), nil
}

func (s *ScheduleService) GetSchedulesByClass(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool) ([]dto.ClassScheduleResponseDTO, error) {
	if err := s.requireClassRead(ctx, actorID, isAdmin, classID); err != nil {
		return nil, err
	}
	// Check cache
	if s.redis != nil {
		cacheKey := schedulesCachePrefix + classID.String()
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result []dto.ClassScheduleResponseDTO
			if json.Unmarshal([]byte(cached), &result) == nil {
				return result, nil
			}
		}
	}

	schedules, err := s.repo.GetSchedulesByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.ClassScheduleResponseDTO, len(schedules))
	for i, sch := range schedules {
		result[i] = *s.mapScheduleToDTO(&sch)
	}

	// Cache result
	if s.redis != nil {
		if data, err := json.Marshal(result); err == nil {
			s.redis.Set(ctx, schedulesCachePrefix+classID.String(), data, scheduleCacheTTL)
		}
	}

	return result, nil
}

func (s *ScheduleService) GetScheduleByID(ctx context.Context, id, actorID uuid.UUID, isAdmin bool) (*dto.ClassScheduleResponseDTO, error) {
	schedule, err := s.scheduleClass(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireClassRead(ctx, actorID, isAdmin, schedule.ClassID); err != nil {
		return nil, err
	}
	return s.mapScheduleToDTO(schedule), nil
}

func (s *ScheduleService) UpdateSchedule(ctx context.Context, id, actorID uuid.UUID, isAdmin bool, req dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	schedule, err := s.scheduleClass(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireClassWrite(ctx, actorID, isAdmin, schedule.ClassID); err != nil {
		return nil, err
	}

	if req.DayOfWeek != nil {
		schedule.DayOfWeek = *req.DayOfWeek
	}
	if req.StartTime != nil {
		if schedule.StartTime, err = parseClockField("start_time", *req.StartTime); err != nil {
			return nil, err
		}
	}
	if req.EndTime != nil {
		if schedule.EndTime, err = parseClockField("end_time", *req.EndTime); err != nil {
			return nil, err
		}
	}
	if req.Room != nil {
		schedule.Room = req.Room
	}
	if req.IsActive != nil {
		schedule.IsActive = *req.IsActive
	}
	if req.EffectiveFrom != nil {
		t, err := time.Parse("2006-01-02", *req.EffectiveFrom)
		if err != nil {
			return nil, errors.New("invalid effective_from date format")
		}
		schedule.EffectiveFrom = t
	}
	if req.EffectiveUntil != nil {
		t, err := time.Parse("2006-01-02", *req.EffectiveUntil)
		if err != nil {
			return nil, errors.New("invalid effective_until date format")
		}
		schedule.EffectiveUntil = &t
	}

	if err := s.repo.UpdateSchedule(ctx, schedule); err != nil {
		return nil, err
	}

	s.invalidateScheduleCache(ctx, schedule.ClassID)
	return s.mapScheduleToDTO(schedule), nil
}

func (s *ScheduleService) DeleteSchedule(ctx context.Context, id, actorID uuid.UUID, isAdmin bool) error {
	schedule, err := s.scheduleClass(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireClassWrite(ctx, actorID, isAdmin, schedule.ClassID); err != nil {
		return err
	}

	if err := s.repo.DeleteSchedule(ctx, id); err != nil {
		return err
	}

	s.invalidateScheduleCache(ctx, schedule.ClassID)
	return nil
}

// ============================================================================
// CLASS SESSION
// ============================================================================

func (s *ScheduleService) CreateSession(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, req dto.CreateClassSessionDTO) (*dto.ClassSessionResponseDTO, error) {
	if err := s.requireClassWrite(ctx, actorID, isAdmin, classID); err != nil {
		return nil, err
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}
	startTime, err := parseClockField("start_time", req.StartTime)
	if err != nil {
		return nil, err
	}
	endTime, err := parseClockField("end_time", req.EndTime)
	if err != nil {
		return nil, err
	}

	if err := s.ensureSessionSlotFree(ctx, classID, date, startTime, endTime, nil); err != nil {
		return nil, err
	}

	nextNum, err := s.repo.GetNextSessionNumber(ctx, classID)
	if err != nil {
		return nil, err
	}

	session := &model.ClassSession{
		ClassID:       classID,
		SessionNumber: nextNum,
		Date:          date,
		StartTime:     startTime,
		EndTime:       endTime,
		Status:        model.SessionScheduled,
	}

	if req.Topic != "" {
		session.Topic = &req.Topic
	}
	if req.Notes != "" {
		session.Notes = &req.Notes
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	// Schedule reminder via Asynq
	s.scheduleSessionReminder(ctx, session, classID)

	s.invalidateSessionCache(ctx, classID)
	return s.mapSessionToDTO(session), nil
}

func (s *ScheduleService) GetSessionsByClass(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.ClassSessionListDTO, error) {
	if err := s.requireClassRead(ctx, actorID, isAdmin, classID); err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	sessions, total, err := s.repo.GetSessionsByClassID(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.ClassSessionResponseDTO, len(sessions))
	for i, sess := range sessions {
		data[i] = *s.mapSessionToDTO(&sess)
	}

	return &dto.ClassSessionListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ScheduleService) GetSessionByID(ctx context.Context, id, actorID uuid.UUID, isAdmin bool) (*dto.ClassSessionResponseDTO, error) {
	session, err := s.sessionClass(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireClassRead(ctx, actorID, isAdmin, session.ClassID); err != nil {
		return nil, err
	}
	return s.mapSessionToDTO(session), nil
}

func (s *ScheduleService) UpdateSession(ctx context.Context, id, actorID uuid.UUID, isAdmin bool, req dto.UpdateClassSessionDTO) (*dto.ClassSessionResponseDTO, error) {
	session, err := s.sessionClass(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireClassWrite(ctx, actorID, isAdmin, session.ClassID); err != nil {
		return nil, err
	}

	if req.Date != nil {
		date, err := time.Parse("2006-01-02", *req.Date)
		if err != nil {
			return nil, errors.New("invalid date format")
		}
		session.Date = date
	}
	if req.StartTime != nil {
		if session.StartTime, err = parseClockField("start_time", *req.StartTime); err != nil {
			return nil, err
		}
	}
	if req.EndTime != nil {
		if session.EndTime, err = parseClockField("end_time", *req.EndTime); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		session.Status = model.ClassSessionStatus(*req.Status)
	}
	// B-10: chỉ kiểm giờ khi sửa ngày/giờ của buổi còn hiệu lực — đổi ghi chú hay huỷ buổi không bị
	// chặn vì một buổi cũ đã lỡ trùng giờ từ trước.
	if (req.Date != nil || req.StartTime != nil || req.EndTime != nil) && session.Status != model.SessionCancelled {
		if err := s.ensureSessionSlotFree(ctx, session.ClassID, session.Date, session.StartTime, session.EndTime, &session.ID); err != nil {
			return nil, err
		}
	}
	if req.Topic != nil {
		session.Topic = req.Topic
	}
	if req.Notes != nil {
		session.Notes = req.Notes
	}

	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}

	s.invalidateSessionCache(ctx, session.ClassID)
	return s.mapSessionToDTO(session), nil
}

func (s *ScheduleService) CancelSession(ctx context.Context, id, actorID uuid.UUID, isAdmin bool, reason string) error {
	session, err := s.sessionClass(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireClassWrite(ctx, actorID, isAdmin, session.ClassID); err != nil {
		return err
	}

	now := time.Now()
	session.Status = model.SessionCancelled
	session.CancelledAt = &now
	if reason != "" {
		session.CancelReason = &reason
	}

	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return err
	}

	s.invalidateSessionCache(ctx, session.ClassID)
	return nil
}

func (s *ScheduleService) GenerateSessions(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool, req dto.GenerateSessionsDTO) ([]dto.ClassSessionResponseDTO, error) {
	if err := s.requireClassWrite(ctx, actorID, isAdmin, classID); err != nil {
		return nil, err
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return nil, errors.New("invalid start_date format")
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return nil, errors.New("invalid end_date format")
	}
	if endDate.Sub(startDate) > maxGenerateSessionDays*24*time.Hour {
		return nil, ErrGenerateRangeTooLong
	}

	schedules, err := s.repo.GetSchedulesByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	if len(schedules) == 0 {
		return nil, errors.New("no recurring schedules found for this class")
	}

	nextNum, err := s.repo.GetNextSessionNumber(ctx, classID)
	if err != nil {
		return nil, err
	}

	var sessions []model.ClassSession
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		weekday := int(d.Weekday())
		for _, sch := range schedules {
			if !sch.IsActive || sch.DayOfWeek != weekday {
				continue
			}
			if d.Before(sch.EffectiveFrom) {
				continue
			}
			if sch.EffectiveUntil != nil && d.After(*sch.EffectiveUntil) {
				continue
			}

			session := model.ClassSession{
				ClassID:       classID,
				ScheduleID:    &sch.ID,
				SessionNumber: nextNum,
				Date:          d,
				StartTime:     sch.StartTime,
				EndTime:       sch.EndTime,
				Status:        model.SessionScheduled,
			}
			if sch.Room != nil {
				// Room info available from schedule
			}
			sessions = append(sessions, session)
			nextNum++
		}
	}

	if len(sessions) == 0 {
		return nil, errors.New("no sessions generated for the given date range")
	}

	for i := range sessions {
		if err := s.repo.CreateSession(ctx, &sessions[i]); err != nil {
			return nil, fmt.Errorf("failed to create session #%d: %w", sessions[i].SessionNumber, err)
		}
		// Schedule Asynq reminders for each generated session
		s.scheduleSessionReminder(ctx, &sessions[i], classID)
	}

	s.invalidateSessionCache(ctx, classID)

	result := make([]dto.ClassSessionResponseDTO, len(sessions))
	for i, sess := range sessions {
		result[i] = *s.mapSessionToDTO(&sess)
	}
	return result, nil
}

// ============================================================================
// SESSION ATTENDANCE
// ============================================================================

// requireSessionManage (S5): quyền quản lý buổi = quyền quản lý LỚP của buổi (giảng viên lớp, chủ khoá,
// người tạo, admin), dùng lại ensureClassManage thay cho TeacherCanManageSession (chỉ tra teacher_classes,
// nên chủ khoá và người tạo lớp bị từ chối oan). viewerNotFound=true (đọc danh sách điểm danh): người
// không quản lý nhận 404 kể cả thành viên lớp; false (ghi): thành viên lớp nhận 403, người ngoài 404.
func (s *ScheduleService) requireSessionManage(ctx context.Context, sessionID, actorID uuid.UUID, isAdmin, viewerNotFound bool) error {
	session, err := s.sessionClass(ctx, sessionID)
	if err != nil {
		return err
	}
	if viewerNotFound {
		elevated, err := classAccessAsAdmin(ctx, s.classRepo, s.authz, actorID, session.ClassID, isAdmin)
		if err != nil {
			return err
		}
		return ensureClassManageOrNotFound(ctx, s.classRepo, s.courseRepo, actorID, session.ClassID, elevated)
	}
	return s.requireClassWrite(ctx, actorID, isAdmin, session.ClassID)
}

// ErrStudentNotInSession: học viên trong danh sách điểm danh không đang học lớp của buổi (400).
var ErrStudentNotInSession = errors.New("student is not an active member of this session's class")

// requireStudentInSession (S5, cùng lỗi M-4 của điểm danh lớp): chỉ ghi điểm danh cho học viên đang học lớp của buổi.
func (s *ScheduleService) requireStudentInSession(ctx context.Context, sessionID, studentID uuid.UUID) error {
	allowed, err := s.repo.StudentCanAttendSession(ctx, sessionID, studentID)
	if err != nil {
		return fmt.Errorf("check student session membership: %w", err)
	}
	if !allowed {
		return ErrStudentNotInSession
	}
	return nil
}

func (s *ScheduleService) requireStudentSessionAccess(ctx context.Context, sessionID, studentID uuid.UUID) error {
	allowed, err := s.repo.StudentCanAttendSession(ctx, sessionID, studentID)
	if err != nil {
		return fmt.Errorf("check student session access: %w", err)
	}
	if !allowed {
		return ErrClassSessionNotFound
	}
	return nil
}

func (s *ScheduleService) GetSessionAttendances(ctx context.Context, sessionID, requesterID uuid.UUID, isAdmin bool) ([]dto.SessionAttendanceResponseDTO, error) {
	if err := s.requireSessionManage(ctx, sessionID, requesterID, isAdmin, true); err != nil {
		return nil, err
	}
	atts, err := s.repo.GetAttendancesBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.SessionAttendanceResponseDTO, len(atts))
	for i, att := range atts {
		result[i] = *s.mapAttendanceToDTO(&att)
	}
	return result, nil
}

func (s *ScheduleService) MarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.MarkAttendanceDTO, verifiedBy uuid.UUID, isAdmin bool) (*dto.SessionAttendanceResponseDTO, error) {
	if err := s.requireSessionManage(ctx, sessionID, verifiedBy, isAdmin, false); err != nil {
		return nil, err
	}
	studentID, err := uuid.Parse(req.StudentID)
	if err != nil {
		return nil, errors.New("invalid student_id")
	}
	if err := s.requireStudentInSession(ctx, sessionID, studentID); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
	if err != nil {
		return nil, fmt.Errorf("check existing attendance: %w", err)
	}
	if existing != nil {
		return nil, errors.New("attendance already recorded for this student")
	}

	att := &model.SessionAttendance{
		SessionID:  sessionID,
		StudentID:  studentID,
		Status:     model.AttendanceStatus(req.Status),
		VerifiedBy: &verifiedBy,
	}
	if req.Note != "" {
		att.Note = &req.Note
	}

	if err := s.repo.CreateAttendance(ctx, att); err != nil {
		return nil, err
	}

	created, _ := s.repo.GetAttendanceByID(ctx, att.ID)
	if created != nil {
		return s.mapAttendanceToDTO(created), nil
	}
	return s.mapAttendanceToDTO(att), nil
}

func (s *ScheduleService) BulkMarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.BulkMarkAttendanceDTO, verifiedBy uuid.UUID, isAdmin bool) ([]dto.SessionAttendanceResponseDTO, error) {
	if err := s.requireSessionManage(ctx, sessionID, verifiedBy, isAdmin, false); err != nil {
		return nil, err
	}
	attendances := make([]model.SessionAttendance, 0, len(req.Attendances))
	seen := make(map[uuid.UUID]struct{}, len(req.Attendances))
	for _, item := range req.Attendances {
		studentID, err := uuid.Parse(item.StudentID)
		if err != nil {
			return nil, fmt.Errorf("invalid student_id %q", item.StudentID)
		}
		if _, duplicate := seen[studentID]; duplicate {
			return nil, fmt.Errorf("student %s appears more than once", studentID)
		}
		seen[studentID] = struct{}{}
		if err := s.requireStudentInSession(ctx, sessionID, studentID); err != nil {
			return nil, err
		}

		existing, err := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
		if err != nil {
			return nil, fmt.Errorf("check attendance for student %s: %w", studentID, err)
		}
		if existing != nil {
			return nil, fmt.Errorf("attendance already recorded for student %s", studentID)
		}

		attendance := model.SessionAttendance{
			SessionID:  sessionID,
			StudentID:  studentID,
			Status:     model.AttendanceStatus(item.Status),
			VerifiedBy: &verifiedBy,
		}
		if item.Note != "" {
			attendance.Note = &item.Note
		}
		attendances = append(attendances, attendance)
	}

	if err := s.repo.BulkCreateAttendance(ctx, attendances); err != nil {
		return nil, err
	}

	results := make([]dto.SessionAttendanceResponseDTO, len(attendances))
	for i := range attendances {
		results[i] = *s.mapAttendanceToDTO(&attendances[i])
	}
	return results, nil
}

func (s *ScheduleService) UpdateAttendance(ctx context.Context, sessionID, id uuid.UUID, req dto.UpdateSessionAttendanceDTO, verifiedBy uuid.UUID, isAdmin bool) (*dto.SessionAttendanceResponseDTO, error) {
	if err := s.requireSessionManage(ctx, sessionID, verifiedBy, isAdmin, false); err != nil {
		return nil, err
	}
	att, err := s.repo.GetAttendanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if att == nil {
		return nil, errors.New("attendance not found")
	}
	if att.SessionID != sessionID {
		return nil, errors.New("attendance does not belong to this session")
	}

	if req.Status != nil {
		att.Status = model.AttendanceStatus(*req.Status)
	}
	if req.Note != nil {
		att.Note = req.Note
	}
	if req.LateMinutes != nil {
		att.LateMinutes = *req.LateMinutes
	}

	if err := s.repo.UpdateAttendance(ctx, att); err != nil {
		return nil, err
	}
	return s.mapAttendanceToDTO(att), nil
}

func (s *ScheduleService) StudentCheckIn(ctx context.Context, sessionID, studentID uuid.UUID) (*dto.SessionAttendanceResponseDTO, error) {
	if err := s.requireStudentSessionAccess(ctx, sessionID, studentID); err != nil {
		return nil, err
	}
	now := time.Now()
	// Sau bước quyền (người ngoài lớp vẫn 404): học viên trong lớp mới biết buổi đó có/đóng hay không.
	session, err := s.sessionClass(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := requireCheckInOpen(session, now); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
	if err != nil {
		return nil, fmt.Errorf("get attendance for check-in: %w", err)
	}
	if existing != nil {
		existing.CheckInTime = &now
		existing.Status = model.AttendancePresent
		if err := s.repo.UpdateAttendance(ctx, existing); err != nil {
			return nil, err
		}
		return s.mapAttendanceToDTO(existing), nil
	}

	att := &model.SessionAttendance{
		SessionID:   sessionID,
		StudentID:   studentID,
		Status:      model.AttendancePresent,
		CheckInTime: &now,
	}
	if err := s.repo.CreateAttendance(ctx, att); err != nil {
		return nil, err
	}
	return s.mapAttendanceToDTO(att), nil
}

func (s *ScheduleService) StudentCheckOut(ctx context.Context, sessionID, studentID uuid.UUID) (*dto.SessionAttendanceResponseDTO, error) {
	if err := s.requireStudentSessionAccess(ctx, sessionID, studentID); err != nil {
		return nil, err
	}
	att, err := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
	if err != nil {
		return nil, fmt.Errorf("get attendance for check-out: %w", err)
	}
	if att == nil || att.CheckInTime == nil {
		return nil, errors.New("no check-in found for this session")
	}

	now := time.Now()
	att.CheckOutTime = &now
	if err := s.repo.UpdateAttendance(ctx, att); err != nil {
		return nil, err
	}
	return s.mapAttendanceToDTO(att), nil
}

func (s *ScheduleService) GetMyAttendances(ctx context.Context, studentID uuid.UUID, page, pageSize int) ([]dto.SessionAttendanceResponseDTO, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	atts, total, err := s.repo.GetStudentAttendanceHistory(ctx, studentID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	result := make([]dto.SessionAttendanceResponseDTO, len(atts))
	for i, att := range atts {
		result[i] = *s.mapAttendanceToDTO(&att)
	}
	return result, total, nil
}

// ============================================================================
// TIMETABLE
// ============================================================================

func (s *ScheduleService) GetClassTimetable(ctx context.Context, classID, actorID uuid.UUID, isAdmin bool) (*dto.TimetableResponseDTO, error) {
	if err := s.requireClassRead(ctx, actorID, isAdmin, classID); err != nil {
		return nil, err
	}
	schedules, err := s.repo.GetSchedulesByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	entries := make([]dto.TimetableEntryDTO, 0, len(schedules))
	for _, sch := range schedules {
		if !sch.IsActive {
			continue
		}
		entry := dto.TimetableEntryDTO{
			ScheduleID: &sch.ID,
			ClassID:    sch.ClassID,
			DayOfWeek:  sch.DayOfWeek,
			StartTime:  sch.StartTime.String(),
			EndTime:    sch.EndTime.String(),
			Room:       sch.Room,
			Status:     "active",
		}
		if sch.Class.Name != "" {
			entry.ClassName = sch.Class.Name
		}
		from := sch.EffectiveFrom.Format("2006-01-02")
		entry.EffectiveFrom = &from
		if sch.EffectiveUntil != nil {
			until := sch.EffectiveUntil.Format("2006-01-02")
			entry.EffectiveUntil = &until
		}
		entries = append(entries, entry)
	}

	return &dto.TimetableResponseDTO{Entries: entries}, nil
}

func (s *ScheduleService) GetMyTimetable(ctx context.Context, userID uuid.UUID, role string) (*dto.TimetableResponseDTO, error) {
	// Check cache
	cacheKey := timetableCachePrefix + userID.String()
	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result dto.TimetableResponseDTO
			if json.Unmarshal([]byte(cached), &result) == nil {
				return &result, nil
			}
		}
	}

	var classIDs []uuid.UUID
	var err error

	if role == "TEACHER" {
		classIDs, err = s.repo.GetTeacherClassIDs(ctx, userID)
	} else {
		classIDs, err = s.repo.GetStudentClassIDs(ctx, userID)
	}
	if err != nil {
		return nil, err
	}

	if len(classIDs) == 0 {
		return &dto.TimetableResponseDTO{Entries: []dto.TimetableEntryDTO{}}, nil
	}

	schedules, err := s.repo.GetSchedulesByClassIDs(ctx, classIDs)
	if err != nil {
		return nil, err
	}

	entries := make([]dto.TimetableEntryDTO, 0, len(schedules))
	for _, sch := range schedules {
		entry := dto.TimetableEntryDTO{
			ScheduleID: &sch.ID,
			ClassID:    sch.ClassID,
			DayOfWeek:  sch.DayOfWeek,
			StartTime:  sch.StartTime.String(),
			EndTime:    sch.EndTime.String(),
			Room:       sch.Room,
			Status:     "active",
		}
		if sch.Class.Name != "" {
			entry.ClassName = sch.Class.Name
		}
		from := sch.EffectiveFrom.Format("2006-01-02")
		entry.EffectiveFrom = &from
		if sch.EffectiveUntil != nil {
			until := sch.EffectiveUntil.Format("2006-01-02")
			entry.EffectiveUntil = &until
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DayOfWeek != entries[j].DayOfWeek {
			return entries[i].DayOfWeek < entries[j].DayOfWeek
		}
		return entries[i].StartTime < entries[j].StartTime
	})

	result := &dto.TimetableResponseDTO{Entries: entries}

	// Cache for 15 minutes
	if s.redis != nil {
		if data, err := json.Marshal(result); err == nil {
			s.redis.Set(ctx, cacheKey, data, scheduleCacheTTL)
		}
	}

	return result, nil
}

// maxTimetableSessionDays: trần khoảng ngày của GET /me/timetable?sessions_from&sessions_to.
const maxTimetableSessionDays = 120

// ErrTimetableRangeInvalid: khoảng ngày xin buổi học cụ thể sai thứ tự hoặc quá dài (400).
var ErrTimetableRangeInvalid = errors.New("invalid sessions range: sessions_to must not be before sessions_from and span at most 120 days")

// GetMyTimetableWithSessions (B-08): lịch lặp tuần như GetMyTimetable, cộng thêm các BUỔI HỌC CỤ THỂ chưa huỷ
// của lớp mình dạy/học trong [from, to] (mục có session_id + date). Trước đây lịch giảng viên chỉ vẽ
// livestream nên buổi tạo qua POST /classes/:id/sessions và lịch lặp của lớp không hiện ở đâu. Không cache
// phần buổi cụ thể (thay đổi theo từng thao tác tạo/sửa/huỷ buổi); lịch lặp vẫn đi qua cache cũ.
func (s *ScheduleService) GetMyTimetableWithSessions(ctx context.Context, userID uuid.UUID, role string, from, to time.Time) (*dto.TimetableResponseDTO, error) {
	if to.Before(from) || to.Sub(from) > maxTimetableSessionDays*24*time.Hour {
		return nil, ErrTimetableRangeInvalid
	}
	base, err := s.GetMyTimetable(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	var classIDs []uuid.UUID
	if role == "TEACHER" {
		classIDs, err = s.repo.GetTeacherClassIDs(ctx, userID)
	} else {
		classIDs, err = s.repo.GetStudentClassIDs(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	if len(classIDs) == 0 {
		return base, nil
	}
	sessions, err := s.repo.GetSessionsByClassIDsAndDateRange(ctx, classIDs, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}

	entries := append([]dto.TimetableEntryDTO(nil), base.Entries...)
	for _, sess := range sessions {
		id := sess.ID
		date := sess.Date.Format("2006-01-02")
		entries = append(entries, dto.TimetableEntryDTO{
			SessionID:  &id,
			ScheduleID: sess.ScheduleID,
			ClassName:  sess.Class.Name,
			ClassID:    sess.ClassID,
			DayOfWeek:  int(sess.Date.Weekday()),
			Date:       &date,
			StartTime:  sess.StartTime.String(),
			EndTime:    sess.EndTime.String(),
			Topic:      sess.Topic,
			Status:     string(sess.Status),
		})
	}
	return &dto.TimetableResponseDTO{Entries: entries, Week: base.Week}, nil
}

// ============================================================================
// REMINDER
// ============================================================================

func (s *ScheduleService) GetReminderSettings(ctx context.Context, userID uuid.UUID) ([]dto.ReminderSettingResponseDTO, error) {
	settings, err := s.repo.GetReminderSettings(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.ReminderSettingResponseDTO, len(settings))
	for i, rs := range settings {
		result[i] = dto.ReminderSettingResponseDTO{
			ID:                  rs.ID,
			EventType:           rs.EventType,
			RemindBeforeMinutes: rs.RemindBeforeMinutes,
			Channels:            rs.Channels,
			IsEnabled:           rs.IsEnabled,
		}
	}
	return result, nil
}

func (s *ScheduleService) UpdateReminderSetting(ctx context.Context, userID uuid.UUID, req dto.UpdateReminderSettingDTO) (*dto.ReminderSettingResponseDTO, error) {
	setting := &model.ReminderSetting{
		UserID:              userID,
		EventType:           req.EventType,
		RemindBeforeMinutes: pq.Int32Array(req.RemindBeforeMinutes),
		Channels:            pq.StringArray(req.Channels),
		IsEnabled:           true,
	}
	if req.IsEnabled != nil {
		setting.IsEnabled = *req.IsEnabled
	}

	if err := s.repo.UpsertReminderSetting(ctx, setting); err != nil {
		return nil, err
	}

	return &dto.ReminderSettingResponseDTO{
		ID:                  setting.ID,
		EventType:           setting.EventType,
		RemindBeforeMinutes: setting.RemindBeforeMinutes,
		Channels:            setting.Channels,
		IsEnabled:           setting.IsEnabled,
	}, nil
}

// ============================================================================
// ASYNQ REMINDERS
// ============================================================================

func (s *ScheduleService) scheduleSessionReminder(ctx context.Context, session *model.ClassSession, classID uuid.UUID) {
	if s.queue == nil {
		return
	}

	// Parse session datetime
	sessionDateTime, err := session.StartTime.On(session.Date, time.FixedZone("ICT", 7*3600))
	if err != nil {
		log.Printf("[schedule] Failed to parse session datetime: %v", err)
		return
	}

	payload := asynq_queue.ClassReminderPayload{
		SessionID:  session.ID,
		ClassID:    classID,
		StartTime:  session.StartTime.String(),
		MinsBefore: 30,
	}

	// Schedule reminders at 30, 15, 5, 1 minutes before
	for _, mins := range []int{30, 15, 5, 1} {
		payload.MinsBefore = mins
		reminderAt := sessionDateTime.Add(-time.Duration(mins) * time.Minute)
		if reminderAt.After(time.Now()) {
			s.queue.ScheduleAt(asynq_queue.TaskLessonReminder, payload, reminderAt, "notifications")
		}
	}
}

// parseClockField đọc giờ trong ngày từ request ("HH:MM" hoặc "HH:MM:SS"). Lỗi là lỗi đầu vào:
// handler trả 400 qua scheduleFail.
func parseClockField(field, value string) (model.TimeOfDay, error) {
	t, err := model.ParseTimeOfDay(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", field, err)
	}
	return t, nil
}

// ============================================================================
// MAPPERS
// ============================================================================

func (s *ScheduleService) mapScheduleToDTO(sch *model.ClassSchedule) *dto.ClassScheduleResponseDTO {
	return &dto.ClassScheduleResponseDTO{
		ID:             sch.ID,
		ClassID:        sch.ClassID,
		DayOfWeek:      sch.DayOfWeek,
		StartTime:      sch.StartTime.String(),
		EndTime:        sch.EndTime.String(),
		Room:           sch.Room,
		IsActive:       sch.IsActive,
		EffectiveFrom:  sch.EffectiveFrom,
		EffectiveUntil: sch.EffectiveUntil,
		CreatedAt:      sch.CreatedAt,
	}
}

func (s *ScheduleService) mapSessionToDTO(sess *model.ClassSession) *dto.ClassSessionResponseDTO {
	return &dto.ClassSessionResponseDTO{
		ID:                  sess.ID,
		ClassID:             sess.ClassID,
		ScheduleID:          sess.ScheduleID,
		SessionNumber:       sess.SessionNumber,
		Date:                sess.Date.Format("2006-01-02"),
		StartTime:           sess.StartTime.String(),
		EndTime:             sess.EndTime.String(),
		Status:              string(sess.Status),
		Topic:               sess.Topic,
		Notes:               sess.Notes,
		LivestreamSessionID: sess.LivestreamSessionID,
		CancelledAt:         sess.CancelledAt,
		CancelReason:        sess.CancelReason,
		CreatedAt:           sess.CreatedAt,
	}
}

func (s *ScheduleService) mapAttendanceToDTO(att *model.SessionAttendance) *dto.SessionAttendanceResponseDTO {
	studentName := ""
	if att.Student.UserName != "" {
		studentName = att.Student.UserName
	}
	if att.Student.FullName != nil {
		studentName = *att.Student.FullName
	}

	return &dto.SessionAttendanceResponseDTO{
		ID:                att.ID,
		SessionID:         att.SessionID,
		StudentID:         att.StudentID,
		StudentName:       studentName,
		Status:            string(att.Status),
		CheckInTime:       att.CheckInTime,
		CheckOutTime:      att.CheckOutTime,
		LateMinutes:       att.LateMinutes,
		EarlyLeaveMinutes: att.EarlyLeaveMinutes,
		Note:              att.Note,
		CreatedAt:         att.CreatedAt,
	}
}
