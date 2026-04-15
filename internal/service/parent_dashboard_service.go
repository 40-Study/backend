package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ParentDashboardServiceInterface interface {
	GetChildOverview(ctx context.Context, parentID, childID uuid.UUID) (*dto.ChildOverviewDto, error)
	GetChildCourses(ctx context.Context, parentID, childID uuid.UUID, page, pageSize int) (*dto.ChildCoursesResponseDto, error)
	GetChildGrades(ctx context.Context, parentID, childID uuid.UUID) (*dto.ChildGradesResponseDto, error)
	GetChildSchedule(ctx context.Context, parentID, childID uuid.UUID) (*dto.ChildScheduleResponseDto, error)
	GetChildTimetable(ctx context.Context, parentID, childID uuid.UUID) (*dto.TimetableResponseDTO, error)
	GetChildAttendance(ctx context.Context, parentID, childID uuid.UUID, page, pageSize int) (*dto.ChildAttendanceResponseDto, error)
	GetChildAssignments(ctx context.Context, parentID, childID uuid.UUID, page, pageSize int) (*dto.ChildAssignmentsResponseDto, error)
}

type ParentDashboardService struct {
	parentStudentRepo repository.ParentStudentRepositoryInterface
	userRepo          repository.UserRepositoryInterface
	enrollmentRepo    repository.EnrollmentRepositoryInterface
	gradeRepo         repository.GradeRepositoryInterface
	scheduleRepo      repository.ScheduleRepositoryInterface
	submissionRepo    repository.SubmissionRepositoryInterface
	userStatsRepo     *repository.UserStatsRepository
}

func NewParentDashboardService(
	parentStudentRepo repository.ParentStudentRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	gradeRepo repository.GradeRepositoryInterface,
	scheduleRepo repository.ScheduleRepositoryInterface,
	submissionRepo repository.SubmissionRepositoryInterface,
	userStatsRepo *repository.UserStatsRepository,
) *ParentDashboardService {
	return &ParentDashboardService{
		parentStudentRepo: parentStudentRepo,
		userRepo:          userRepo,
		enrollmentRepo:    enrollmentRepo,
		gradeRepo:         gradeRepo,
		scheduleRepo:      scheduleRepo,
		submissionRepo:    submissionRepo,
		userStatsRepo:     userStatsRepo,
	}
}

func (s *ParentDashboardService) verifyParentChildRelation(ctx context.Context, parentID, childID uuid.UUID) (*model.ParentStudentRelation, error) {
	relation, err := s.parentStudentRepo.FindByParentAndStudent(ctx, parentID, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify relation: %w", err)
	}
	if relation == nil {
		return nil, errors.New("không có quyền xem thông tin học sinh này")
	}
	if relation.Status != model.ParentStudentStatusActive {
		return nil, errors.New("liên kết chưa được kích hoạt")
	}
	return relation, nil
}

func (s *ParentDashboardService) GetChildOverview(ctx context.Context, parentID, childID uuid.UUID) (*dto.ChildOverviewDto, error) {
	relation, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}

	child, err := s.userRepo.FindUserByID(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child info: %w", err)
	}
	if child == nil {
		return nil, errors.New("không tìm thấy học sinh")
	}

	enrollments, totalEnrolled, err := s.enrollmentRepo.GetByUserID(ctx, childID, 1, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get enrollments: %w", err)
	}

	completedCount := 0
	for _, e := range enrollments {
		if e.CompletedAt != nil {
			completedCount++
		}
	}

	// Get XP and streak from UserStatsRepository
	var totalXP, currentStreak int
	if s.userStatsRepo != nil {
		profile, _ := s.userStatsRepo.GetPublicProfile(ctx, childID)
		if profile != nil {
			totalXP = profile.TotalPoints
			currentStreak = profile.CurrentStreak
		}
	}

	return &dto.ChildOverviewDto{
		ID:                childID.String(),
		Username:          child.UserName,
		FullName:          child.FullName,
		AvatarURL:         child.AvatarURL,
		Email:             child.Email,
		Relationship:      relation.Relationship,
		TotalXP:           totalXP,
		CurrentStreak:     currentStreak,
		EnrolledCourses:   int(totalEnrolled),
		CompletedCourses:  completedCount,
		TotalStudyMinutes: 0, // TODO: calculate from lesson progress
		CanViewProgress:   relation.CanViewProgress,
		CanViewGrades:     relation.CanViewGrades,
		CanViewAttendance: relation.CanViewAttendance,
	}, nil
}

func (s *ParentDashboardService) GetChildCourses(ctx context.Context, parentID, childID uuid.UUID, page, pageSize int) (*dto.ChildCoursesResponseDto, error) {
	relation, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}
	if !relation.CanViewProgress {
		return nil, errors.New("không có quyền xem tiến độ học tập")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	enrollments, total, err := s.enrollmentRepo.GetByUserID(ctx, childID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get enrollments: %w", err)
	}

	courses := make([]dto.ChildCourseDto, len(enrollments))
	for i, e := range enrollments {
		instructorName := ""
		if e.Course.InstructorID != uuid.Nil && e.Course.Instructor.ID != uuid.Nil {
			if e.Course.Instructor.FullName != nil {
				instructorName = *e.Course.Instructor.FullName
			} else {
				instructorName = e.Course.Instructor.UserName
			}
		}

		progressFloat, _ := e.ProgressPercent.Float64()

		courses[i] = dto.ChildCourseDto{
			ID:              e.ID.String(),
			CourseID:        e.CourseID.String(),
			CourseName:      e.Course.Title,
			CourseThumbnail: e.Course.ThumbnailURL,
			InstructorName:  instructorName,
			ProgressPercent: progressFloat,
			LastAccessedAt:  e.LastAccessedAt,
			CompletedAt:     e.CompletedAt,
			EnrolledAt:      e.CreatedAt,
		}
	}

	return &dto.ChildCoursesResponseDto{
		Courses:  courses,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ParentDashboardService) GetChildGrades(ctx context.Context, parentID, childID uuid.UUID) (*dto.ChildGradesResponseDto, error) {
	relation, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}
	if !relation.CanViewGrades {
		return nil, errors.New("không có quyền xem điểm số")
	}

	grades, err := s.gradeRepo.GetGradesByStudentID(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to get grades: %w", err)
	}

	gradeDtos := make([]dto.ChildGradeDto, len(grades))
	for i, g := range grades {
		className := g.Class.Name

		score, _ := g.Score.Float64()
		maxScore, _ := g.MaxScore.Float64()
		weight, _ := g.Weight.Float64()

		percentage := 0.0
		if maxScore > 0 {
			percentage = (score / maxScore) * 100
		}

		gradeDtos[i] = dto.ChildGradeDto{
			ID:         g.ID.String(),
			ClassID:    g.ClassID.String(),
			ClassName:  className,
			GradeType:  string(g.GradeType),
			Title:      g.Title,
			Score:      score,
			MaxScore:   maxScore,
			Percentage: percentage,
			Weight:     weight,
			GradedAt:   g.CreatedAt,
		}
	}

	return &dto.ChildGradesResponseDto{
		Grades:      gradeDtos,
		FinalGrades: []dto.ChildFinalGradeDto{},
	}, nil
}

func (s *ParentDashboardService) GetChildSchedule(ctx context.Context, parentID, childID uuid.UUID) (*dto.ChildScheduleResponseDto, error) {
	_, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}

	classIDs, err := s.scheduleRepo.GetStudentClassIDs(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to get student classes: %w", err)
	}

	weeklySchedule := []dto.ChildScheduleItemDto{}
	upcomingSessions := []dto.ChildUpcomingSessionDto{}

	if len(classIDs) > 0 {
		schedules, err := s.scheduleRepo.GetSchedulesByClassIDs(ctx, classIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get schedules: %w", err)
		}

		for _, sch := range schedules {
			if !sch.IsActive {
				continue
			}
			className := sch.Class.Name
			room := ""
			if sch.Room != nil {
				room = *sch.Room
			}

			weeklySchedule = append(weeklySchedule, dto.ChildScheduleItemDto{
				ClassID:   sch.ClassID.String(),
				ClassName: className,
				DayOfWeek: sch.DayOfWeek,
				StartTime: sch.StartTime,
				EndTime:   sch.EndTime,
				Room:      room,
			})
		}

		for _, classID := range classIDs {
			sessions, err := s.scheduleRepo.GetUpcomingSessions(ctx, classID, 5)
			if err != nil {
				continue
			}
			for _, sess := range sessions {
				className := sess.Class.Name
				topic := ""
				if sess.Topic != nil {
					topic = *sess.Topic
				}
				// Get room from schedule if available
				room := ""
				if sess.Schedule != nil && sess.Schedule.Room != nil {
					room = *sess.Schedule.Room
				}

				upcomingSessions = append(upcomingSessions, dto.ChildUpcomingSessionDto{
					ID:            sess.ID.String(),
					ClassID:       sess.ClassID.String(),
					ClassName:     className,
					SessionNumber: sess.SessionNumber,
					Topic:         topic,
					Date:          sess.Date,
					StartTime:     sess.StartTime,
					EndTime:       sess.EndTime,
					Room:          room,
				})
			}
		}
	}

	return &dto.ChildScheduleResponseDto{
		WeeklySchedule:   weeklySchedule,
		UpcomingSessions: upcomingSessions,
	}, nil
}

func (s *ParentDashboardService) GetChildTimetable(ctx context.Context, parentID, childID uuid.UUID) (*dto.TimetableResponseDTO, error) {
	_, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}

	classIDs, err := s.scheduleRepo.GetStudentClassIDs(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to get student classes: %w", err)
	}

	if len(classIDs) == 0 {
		return &dto.TimetableResponseDTO{Entries: []dto.TimetableEntryDTO{}}, nil
	}

	schedules, err := s.scheduleRepo.GetSchedulesByClassIDs(ctx, classIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedules: %w", err)
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

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DayOfWeek != entries[j].DayOfWeek {
			return entries[i].DayOfWeek < entries[j].DayOfWeek
		}
		return entries[i].StartTime < entries[j].StartTime
	})

	return &dto.TimetableResponseDTO{Entries: entries}, nil
}

func (s *ParentDashboardService) GetChildAttendance(ctx context.Context, parentID, childID uuid.UUID, page, pageSize int) (*dto.ChildAttendanceResponseDto, error) {
	relation, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}
	if !relation.CanViewAttendance {
		return nil, errors.New("không có quyền xem điểm danh")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	records, total, err := s.scheduleRepo.GetStudentAttendanceHistory(ctx, childID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get attendance: %w", err)
	}

	attendanceDtos := make([]dto.ChildAttendanceDto, len(records))
	stats := dto.ChildAttendanceStatsDto{}

	for i, r := range records {
		sessionNum := r.Session.SessionNumber
		date := r.Session.Date
		classID := r.Session.ClassID
		className := r.Session.Class.Name

		var checkInTime *string
		if r.CheckInTime != nil {
			t := r.CheckInTime.Format("15:04")
			checkInTime = &t
		}

		attendanceDtos[i] = dto.ChildAttendanceDto{
			ID:          r.ID.String(),
			ClassID:     classID.String(),
			ClassName:   className,
			SessionNum:  sessionNum,
			Date:        date,
			Status:      string(r.Status),
			CheckInTime: checkInTime,
			LateMinutes: r.LateMinutes,
		}

		stats.TotalSessions++
		switch r.Status {
		case model.AttendancePresent:
			stats.PresentCount++
		case model.AttendanceAbsent:
			stats.AbsentCount++
		case model.AttendanceLate:
			stats.LateCount++
		case model.AttendanceExcused:
			stats.ExcusedCount++
		}
	}

	if stats.TotalSessions > 0 {
		stats.AttendanceRate = float64(stats.PresentCount+stats.LateCount) / float64(stats.TotalSessions) * 100
	}

	return &dto.ChildAttendanceResponseDto{
		Records:  attendanceDtos,
		Stats:    stats,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ParentDashboardService) GetChildAssignments(ctx context.Context, parentID, childID uuid.UUID, page, pageSize int) (*dto.ChildAssignmentsResponseDto, error) {
	relation, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}
	if !relation.CanViewProgress {
		return nil, errors.New("không có quyền xem bài tập")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	submissions, total, err := s.submissionRepo.GetByUser(ctx, childID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get submissions: %w", err)
	}

	assignmentMap := make(map[uuid.UUID]*dto.ChildAssignmentDto)
	stats := dto.AssignmentStatsDto{}

	for _, sub := range submissions {
		if sub.Assignment == nil {
			continue
		}

		aID := sub.AssignmentID
		if existing, ok := assignmentMap[aID]; ok {
			existing.SubmissionCount++
			if sub.CreatedAt.After(*existing.LastSubmittedAt) {
				existing.LastSubmittedAt = &sub.CreatedAt
			}
			if sub.TestCasesPassed > existing.TestCasesPassed {
				existing.TestCasesPassed = sub.TestCasesPassed
				verdict := string(sub.Verdict)
				existing.BestVerdict = &verdict
			}
		} else {
			className := ""
			if sub.Assignment.Class != nil {
				className = sub.Assignment.Class.Name
			}

			status := "in_progress"
			if sub.Verdict == model.VerdictAccepted {
				status = "completed"
				stats.Completed++
			} else {
				stats.InProgress++
			}

			submittedAt := sub.CreatedAt
			verdict := string(sub.Verdict)
			assignmentMap[aID] = &dto.ChildAssignmentDto{
				ID:              sub.Assignment.ID.String(),
				Title:           sub.Assignment.Title,
				Type:            sub.Assignment.Type,
				Difficulty:      string(sub.Assignment.Difficulty),
				ClassName:       className,
				DueDate:         sub.Assignment.EndTime,
				Status:          status,
				BestVerdict:     &verdict,
				TestCasesPassed: sub.TestCasesPassed,
				TotalTestCases:  sub.TotalTestCases,
				SubmissionCount: 1,
				LastSubmittedAt: &submittedAt,
			}
		}
	}

	assignments := make([]dto.ChildAssignmentDto, 0, len(assignmentMap))
	for _, a := range assignmentMap {
		assignments = append(assignments, *a)
	}

	stats.TotalAssignments = len(assignments)

	return &dto.ChildAssignmentsResponseDto{
		Assignments: assignments,
		Stats:       stats,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}
