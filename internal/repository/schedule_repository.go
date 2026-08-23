package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// ============================================================================
// SCHEDULE REPOSITORY
// ============================================================================

type ScheduleRepositoryInterface interface {
	// ClassSchedule
	CreateSchedule(ctx context.Context, schedule *model.ClassSchedule) error
	GetScheduleByID(ctx context.Context, id uuid.UUID) (*model.ClassSchedule, error)
	GetSchedulesByClassID(ctx context.Context, classID uuid.UUID) ([]model.ClassSchedule, error)
	UpdateSchedule(ctx context.Context, schedule *model.ClassSchedule) error
	DeleteSchedule(ctx context.Context, id uuid.UUID) error

	// ClassSession
	CreateSession(ctx context.Context, session *model.ClassSession) error
	GetSessionByID(ctx context.Context, id uuid.UUID) (*model.ClassSession, error)
	GetSessionsByClassID(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.ClassSession, int64, error)
	GetSessionsByDateRange(ctx context.Context, classID uuid.UUID, startDate, endDate string) ([]model.ClassSession, error)
	UpdateSession(ctx context.Context, session *model.ClassSession) error
	DeleteSession(ctx context.Context, id uuid.UUID) error
	GetNextSessionNumber(ctx context.Context, classID uuid.UUID) (int, error)
	GetUpcomingSessions(ctx context.Context, classID uuid.UUID, limit int) ([]model.ClassSession, error)

	// SessionAttendance
	CreateAttendance(ctx context.Context, attendance *model.SessionAttendance) error
	BulkCreateAttendance(ctx context.Context, attendances []model.SessionAttendance) error
	GetAttendanceByID(ctx context.Context, id uuid.UUID) (*model.SessionAttendance, error)
	GetAttendancesBySessionID(ctx context.Context, sessionID uuid.UUID) ([]model.SessionAttendance, error)
	UpdateAttendance(ctx context.Context, attendance *model.SessionAttendance) error
	GetStudentAttendanceHistory(ctx context.Context, studentID uuid.UUID, page, pageSize int) ([]model.SessionAttendance, int64, error)
	GetAttendanceBySessionAndStudent(ctx context.Context, sessionID, studentID uuid.UUID) (*model.SessionAttendance, error)
	TeacherCanManageSession(ctx context.Context, sessionID, teacherID uuid.UUID) (bool, error)
	StudentCanAttendSession(ctx context.Context, sessionID, studentID uuid.UUID) (bool, error)

	// ReminderSetting
	GetReminderSettings(ctx context.Context, userID uuid.UUID) ([]model.ReminderSetting, error)
	UpsertReminderSetting(ctx context.Context, setting *model.ReminderSetting) error

	// Timetable
	GetStudentClassIDs(ctx context.Context, studentID uuid.UUID) ([]uuid.UUID, error)
	GetTeacherClassIDs(ctx context.Context, teacherID uuid.UUID) ([]uuid.UUID, error)
	GetSchedulesByClassIDs(ctx context.Context, classIDs []uuid.UUID) ([]model.ClassSchedule, error)
}

type ScheduleRepository struct {
	db *gorm.DB
}

func NewScheduleRepository(db *gorm.DB) *ScheduleRepository {
	return &ScheduleRepository{db: db}
}

// ============================================================================
// CLASS SCHEDULE
// ============================================================================

func (r *ScheduleRepository) CreateSchedule(ctx context.Context, schedule *model.ClassSchedule) error {
	return r.db.WithContext(ctx).Create(schedule).Error
}

func (r *ScheduleRepository) GetScheduleByID(ctx context.Context, id uuid.UUID) (*model.ClassSchedule, error) {
	var schedule model.ClassSchedule
	err := r.db.WithContext(ctx).Preload("Class").First(&schedule, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &schedule, nil
}

func (r *ScheduleRepository) GetSchedulesByClassID(ctx context.Context, classID uuid.UUID) ([]model.ClassSchedule, error) {
	var schedules []model.ClassSchedule
	err := r.db.WithContext(ctx).
		Where("class_id = ?", classID).
		Order("day_of_week ASC, start_time ASC").
		Find(&schedules).Error
	return schedules, err
}

func (r *ScheduleRepository) UpdateSchedule(ctx context.Context, schedule *model.ClassSchedule) error {
	return r.db.WithContext(ctx).Save(schedule).Error
}

func (r *ScheduleRepository) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ClassSchedule{}, "id = ?", id).Error
}

// ============================================================================
// CLASS SESSION
// ============================================================================

func (r *ScheduleRepository) CreateSession(ctx context.Context, session *model.ClassSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *ScheduleRepository) GetSessionByID(ctx context.Context, id uuid.UUID) (*model.ClassSession, error) {
	var session model.ClassSession
	err := r.db.WithContext(ctx).
		Preload("Class").
		Preload("Schedule").
		First(&session, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *ScheduleRepository) GetSessionsByClassID(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.ClassSession, int64, error) {
	var sessions []model.ClassSession
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ClassSession{}).Where("class_id = ?", classID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Order("date ASC, start_time ASC").
		Offset(offset).
		Limit(pageSize).
		Find(&sessions).Error
	return sessions, total, err
}

func (r *ScheduleRepository) GetSessionsByDateRange(ctx context.Context, classID uuid.UUID, startDate, endDate string) ([]model.ClassSession, error) {
	var sessions []model.ClassSession
	err := r.db.WithContext(ctx).
		Where("class_id = ? AND date >= ? AND date <= ?", classID, startDate, endDate).
		Order("date ASC, start_time ASC").
		Find(&sessions).Error
	return sessions, err
}

func (r *ScheduleRepository) UpdateSession(ctx context.Context, session *model.ClassSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *ScheduleRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ClassSession{}, "id = ?", id).Error
}

func (r *ScheduleRepository) GetNextSessionNumber(ctx context.Context, classID uuid.UUID) (int, error) {
	var maxNum *int
	err := r.db.WithContext(ctx).
		Model(&model.ClassSession{}).
		Where("class_id = ?", classID).
		Select("COALESCE(MAX(session_number), 0)").
		Scan(&maxNum).Error
	if err != nil {
		return 1, err
	}
	if maxNum == nil {
		return 1, nil
	}
	return *maxNum + 1, nil
}

func (r *ScheduleRepository) GetUpcomingSessions(ctx context.Context, classID uuid.UUID, limit int) ([]model.ClassSession, error) {
	var sessions []model.ClassSession
	err := r.db.WithContext(ctx).
		Where("class_id = ? AND status = 'scheduled' AND date >= CURRENT_DATE", classID).
		Order("date ASC, start_time ASC").
		Limit(limit).
		Find(&sessions).Error
	return sessions, err
}

// ============================================================================
// SESSION ATTENDANCE
// ============================================================================

func (r *ScheduleRepository) CreateAttendance(ctx context.Context, attendance *model.SessionAttendance) error {
	return r.db.WithContext(ctx).Create(attendance).Error
}

func (r *ScheduleRepository) BulkCreateAttendance(ctx context.Context, attendances []model.SessionAttendance) error {
	return r.db.WithContext(ctx).Create(&attendances).Error
}

func (r *ScheduleRepository) GetAttendanceByID(ctx context.Context, id uuid.UUID) (*model.SessionAttendance, error) {
	var att model.SessionAttendance
	err := r.db.WithContext(ctx).Preload("Student").First(&att, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &att, nil
}

func (r *ScheduleRepository) GetAttendancesBySessionID(ctx context.Context, sessionID uuid.UUID) ([]model.SessionAttendance, error) {
	var atts []model.SessionAttendance
	err := r.db.WithContext(ctx).
		Preload("Student").
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&atts).Error
	return atts, err
}

func (r *ScheduleRepository) UpdateAttendance(ctx context.Context, attendance *model.SessionAttendance) error {
	return r.db.WithContext(ctx).Save(attendance).Error
}

func (r *ScheduleRepository) GetStudentAttendanceHistory(ctx context.Context, studentID uuid.UUID, page, pageSize int) ([]model.SessionAttendance, int64, error) {
	var atts []model.SessionAttendance
	var total int64

	query := r.db.WithContext(ctx).Model(&model.SessionAttendance{}).Where("student_id = ?", studentID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Preload("Session").
		Preload("Session.Class").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&atts).Error
	return atts, total, err
}

func (r *ScheduleRepository) GetAttendanceBySessionAndStudent(ctx context.Context, sessionID, studentID uuid.UUID) (*model.SessionAttendance, error) {
	var att model.SessionAttendance
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND student_id = ?", sessionID, studentID).
		First(&att).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &att, nil
}

func (r *ScheduleRepository) TeacherCanManageSession(ctx context.Context, sessionID, teacherID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("teacher_classes AS tc").
		Joins("JOIN class_sessions AS cs ON cs.class_id = tc.class_id").
		Where("cs.id = ? AND tc.teacher_id = ?", sessionID, teacherID).
		Count(&count).Error
	return count > 0, err
}

func (r *ScheduleRepository) StudentCanAttendSession(ctx context.Context, sessionID, studentID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("student_classes AS sc").
		Joins("JOIN class_sessions AS cs ON cs.class_id = sc.class_id").
		Where("cs.id = ? AND sc.student_id = ? AND sc.status = ?", sessionID, studentID, "active").
		Count(&count).Error
	return count > 0, err
}

// ============================================================================
// REMINDER SETTING
// ============================================================================

func (r *ScheduleRepository) GetReminderSettings(ctx context.Context, userID uuid.UUID) ([]model.ReminderSetting, error) {
	var settings []model.ReminderSetting
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&settings).Error
	return settings, err
}

func (r *ScheduleRepository) UpsertReminderSetting(ctx context.Context, setting *model.ReminderSetting) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND event_type = ?", setting.UserID, setting.EventType).
		Assign(model.ReminderSetting{
			RemindBeforeMinutes: setting.RemindBeforeMinutes,
			Channels:            setting.Channels,
			IsEnabled:           setting.IsEnabled,
		}).
		FirstOrCreate(setting).Error
}

// ============================================================================
// TIMETABLE HELPERS
// ============================================================================

func (r *ScheduleRepository) GetStudentClassIDs(ctx context.Context, studentID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).
		Table("student_classes").
		Where("student_id = ? AND deleted_at IS NULL", studentID).
		Pluck("class_id", &ids).Error
	return ids, err
}

func (r *ScheduleRepository) GetTeacherClassIDs(ctx context.Context, teacherID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).
		Table("teacher_classes").
		Where("teacher_id = ? AND deleted_at IS NULL", teacherID).
		Pluck("class_id", &ids).Error
	return ids, err
}

func (r *ScheduleRepository) GetSchedulesByClassIDs(ctx context.Context, classIDs []uuid.UUID) ([]model.ClassSchedule, error) {
	var schedules []model.ClassSchedule
	err := r.db.WithContext(ctx).
		Preload("Class").
		Where("class_id IN ? AND is_active = true", classIDs).
		Order("day_of_week ASC, start_time ASC").
		Find(&schedules).Error
	return schedules, err
}
