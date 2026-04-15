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
	CreateSchedule(ctx context.Context, classID uuid.UUID, req dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error)
	GetSchedulesByClass(ctx context.Context, classID uuid.UUID) ([]dto.ClassScheduleResponseDTO, error)
	GetScheduleByID(ctx context.Context, id uuid.UUID) (*dto.ClassScheduleResponseDTO, error)
	UpdateSchedule(ctx context.Context, id uuid.UUID, req dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error)
	DeleteSchedule(ctx context.Context, id uuid.UUID) error

	// ClassSession
	CreateSession(ctx context.Context, classID uuid.UUID, req dto.CreateClassSessionDTO) (*dto.ClassSessionResponseDTO, error)
	GetSessionsByClass(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.ClassSessionListDTO, error)
	GetSessionByID(ctx context.Context, id uuid.UUID) (*dto.ClassSessionResponseDTO, error)
	UpdateSession(ctx context.Context, id uuid.UUID, req dto.UpdateClassSessionDTO) (*dto.ClassSessionResponseDTO, error)
	CancelSession(ctx context.Context, id uuid.UUID, reason string) error
	GenerateSessions(ctx context.Context, classID uuid.UUID, req dto.GenerateSessionsDTO) ([]dto.ClassSessionResponseDTO, error)

	// SessionAttendance
	GetSessionAttendances(ctx context.Context, sessionID uuid.UUID) ([]dto.SessionAttendanceResponseDTO, error)
	MarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.MarkAttendanceDTO, verifiedBy uuid.UUID) (*dto.SessionAttendanceResponseDTO, error)
	BulkMarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.BulkMarkAttendanceDTO, verifiedBy uuid.UUID) ([]dto.SessionAttendanceResponseDTO, error)
	UpdateAttendance(ctx context.Context, id uuid.UUID, req dto.UpdateSessionAttendanceDTO) (*dto.SessionAttendanceResponseDTO, error)
	StudentCheckIn(ctx context.Context, sessionID, studentID uuid.UUID) (*dto.SessionAttendanceResponseDTO, error)
	StudentCheckOut(ctx context.Context, sessionID, studentID uuid.UUID) (*dto.SessionAttendanceResponseDTO, error)
	GetMyAttendances(ctx context.Context, studentID uuid.UUID, page, pageSize int) ([]dto.SessionAttendanceResponseDTO, int64, error)

	// Timetable
	GetClassTimetable(ctx context.Context, classID uuid.UUID) (*dto.TimetableResponseDTO, error)
	GetMyTimetable(ctx context.Context, userID uuid.UUID, role string) (*dto.TimetableResponseDTO, error)

	// Reminder
	GetReminderSettings(ctx context.Context, userID uuid.UUID) ([]dto.ReminderSettingResponseDTO, error)
	UpdateReminderSetting(ctx context.Context, userID uuid.UUID, req dto.UpdateReminderSettingDTO) (*dto.ReminderSettingResponseDTO, error)
}

type ScheduleService struct {
	repo  repository.ScheduleRepositoryInterface
	redis *redis.Client
	queue *asynq_queue.Queue
}

func NewScheduleService(
	repo repository.ScheduleRepositoryInterface,
	redis *redis.Client,
	queue *asynq_queue.Queue,
) *ScheduleService {
	return &ScheduleService{repo: repo, redis: redis, queue: queue}
}

// ============================================================================
// CACHE HELPERS
// ============================================================================

const (
	schedulesCachePrefix  = "schedules:class:"
	sessionsCachePrefix   = "sessions:class:"
	timetableCachePrefix  = "timetable:user:"
	scheduleCacheTTL      = 15 * time.Minute
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

func (s *ScheduleService) CreateSchedule(ctx context.Context, classID uuid.UUID, req dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	effectiveFrom, err := time.Parse("2006-01-02", req.EffectiveFrom)
	if err != nil {
		return nil, errors.New("invalid effective_from date format, use YYYY-MM-DD")
	}

	schedule := &model.ClassSchedule{
		ClassID:       classID,
		DayOfWeek:     req.DayOfWeek,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
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

func (s *ScheduleService) GetSchedulesByClass(ctx context.Context, classID uuid.UUID) ([]dto.ClassScheduleResponseDTO, error) {
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

func (s *ScheduleService) GetScheduleByID(ctx context.Context, id uuid.UUID) (*dto.ClassScheduleResponseDTO, error) {
	schedule, err := s.repo.GetScheduleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		return nil, errors.New("schedule not found")
	}
	return s.mapScheduleToDTO(schedule), nil
}

func (s *ScheduleService) UpdateSchedule(ctx context.Context, id uuid.UUID, req dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	schedule, err := s.repo.GetScheduleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		return nil, errors.New("schedule not found")
	}

	if req.DayOfWeek != nil {
		schedule.DayOfWeek = *req.DayOfWeek
	}
	if req.StartTime != nil {
		schedule.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		schedule.EndTime = *req.EndTime
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

func (s *ScheduleService) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	schedule, err := s.repo.GetScheduleByID(ctx, id)
	if err != nil {
		return err
	}
	if schedule == nil {
		return errors.New("schedule not found")
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

func (s *ScheduleService) CreateSession(ctx context.Context, classID uuid.UUID, req dto.CreateClassSessionDTO) (*dto.ClassSessionResponseDTO, error) {
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}

	nextNum, err := s.repo.GetNextSessionNumber(ctx, classID)
	if err != nil {
		return nil, err
	}

	session := &model.ClassSession{
		ClassID:       classID,
		SessionNumber: nextNum,
		Date:          date,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
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

func (s *ScheduleService) GetSessionsByClass(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.ClassSessionListDTO, error) {
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

func (s *ScheduleService) GetSessionByID(ctx context.Context, id uuid.UUID) (*dto.ClassSessionResponseDTO, error) {
	session, err := s.repo.GetSessionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}
	return s.mapSessionToDTO(session), nil
}

func (s *ScheduleService) UpdateSession(ctx context.Context, id uuid.UUID, req dto.UpdateClassSessionDTO) (*dto.ClassSessionResponseDTO, error) {
	session, err := s.repo.GetSessionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	if req.Date != nil {
		date, err := time.Parse("2006-01-02", *req.Date)
		if err != nil {
			return nil, errors.New("invalid date format")
		}
		session.Date = date
	}
	if req.StartTime != nil {
		session.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		session.EndTime = *req.EndTime
	}
	if req.Status != nil {
		session.Status = model.ClassSessionStatus(*req.Status)
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

func (s *ScheduleService) CancelSession(ctx context.Context, id uuid.UUID, reason string) error {
	session, err := s.repo.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
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

func (s *ScheduleService) GenerateSessions(ctx context.Context, classID uuid.UUID, req dto.GenerateSessionsDTO) ([]dto.ClassSessionResponseDTO, error) {
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return nil, errors.New("invalid start_date format")
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return nil, errors.New("invalid end_date format")
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

func (s *ScheduleService) GetSessionAttendances(ctx context.Context, sessionID uuid.UUID) ([]dto.SessionAttendanceResponseDTO, error) {
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

func (s *ScheduleService) MarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.MarkAttendanceDTO, verifiedBy uuid.UUID) (*dto.SessionAttendanceResponseDTO, error) {
	studentID, err := uuid.Parse(req.StudentID)
	if err != nil {
		return nil, errors.New("invalid student_id")
	}

	existing, _ := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
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

func (s *ScheduleService) BulkMarkAttendance(ctx context.Context, sessionID uuid.UUID, req dto.BulkMarkAttendanceDTO, verifiedBy uuid.UUID) ([]dto.SessionAttendanceResponseDTO, error) {
	var results []dto.SessionAttendanceResponseDTO
	for _, item := range req.Attendances {
		result, err := s.MarkAttendance(ctx, sessionID, item, verifiedBy)
		if err != nil {
			log.Printf("BulkMarkAttendance: skip student %s: %v", item.StudentID, err)
			continue
		}
		results = append(results, *result)
	}
	return results, nil
}

func (s *ScheduleService) UpdateAttendance(ctx context.Context, id uuid.UUID, req dto.UpdateSessionAttendanceDTO) (*dto.SessionAttendanceResponseDTO, error) {
	att, err := s.repo.GetAttendanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if att == nil {
		return nil, errors.New("attendance not found")
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
	now := time.Now()
	existing, _ := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
	if existing != nil {
		existing.CheckInTime = &now
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
	att, _ := s.repo.GetAttendanceBySessionAndStudent(ctx, sessionID, studentID)
	if att == nil {
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

func (s *ScheduleService) GetClassTimetable(ctx context.Context, classID uuid.UUID) (*dto.TimetableResponseDTO, error) {
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
			StartTime:  sch.StartTime,
			EndTime:    sch.EndTime,
			Room:       sch.Room,
			Status:     "active",
		}
		if sch.Class.Name != "" {
			entry.ClassName = sch.Class.Name
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
			StartTime:  sch.StartTime,
			EndTime:    sch.EndTime,
			Room:       sch.Room,
			Status:     "active",
		}
		if sch.Class.Name != "" {
			entry.ClassName = sch.Class.Name
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
	sessionDate := session.Date.Format("2006-01-02")
	sessionDateTime, err := time.ParseInLocation("2006-01-02 15:04:05", sessionDate+" "+session.StartTime, time.FixedZone("ICT", 7*3600))
	if err != nil {
		log.Printf("[schedule] Failed to parse session datetime: %v", err)
		return
	}

	payload := asynq_queue.ClassReminderPayload{
		SessionID: session.ID,
		ClassID:   classID,
		StartTime: session.StartTime,
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

// ============================================================================
// MAPPERS
// ============================================================================

func (s *ScheduleService) mapScheduleToDTO(sch *model.ClassSchedule) *dto.ClassScheduleResponseDTO {
	return &dto.ClassScheduleResponseDTO{
		ID:             sch.ID,
		ClassID:        sch.ClassID,
		DayOfWeek:      sch.DayOfWeek,
		StartTime:      sch.StartTime,
		EndTime:        sch.EndTime,
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
		StartTime:           sess.StartTime,
		EndTime:             sess.EndTime,
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
