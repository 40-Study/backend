package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

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
	enrollmentIDs := make([]uuid.UUID, 0, len(enrollments))
	for _, e := range enrollments {
		enrollmentIDs = append(enrollmentIDs, e.ID)
		if e.CompletedAt != nil {
			completedCount++
		}
	}

	// TotalStudyMinutes: thoi gian hoc THAT cua child, lay tu lesson_progress.video_watched_seconds.
	// Tai dung SumWatchedSecondsByEnrollmentIDs (cung ham ma GET /my-enrollments dung) de toan he
	// thong chi co MOT dinh nghia "da hoc bao nhieu" — tu viet SUM rieng o day se la ban sao thu
	// hai, va hai ban sao se lech nhau.
	totalStudyMinutes := 0
	if len(enrollmentIDs) > 0 {
		watchedByEnrollment, err := s.enrollmentRepo.SumWatchedSecondsByEnrollmentIDs(ctx, enrollmentIDs)
		if err != nil {
			// MED-4 (review 260915): quyet dinh co chu y la GIU throw o day (khong fallback ve 0)
			// — 0 se lam phu huynh tuong con khong hoc gi, sai hon ca viec sap trang bao loi. Chi
			// them log co ngu canh (childID/parentID/so enrollment) de phan biet duoc voi loi XP/
			// streak ben duoi (nhanh do CO CHU Y nuot loi — xem comment tai do, khong sua trong PR
			// nay) khi doc log san xuat.
			log.Printf("[ERROR] GetChildOverview: failed to sum watched seconds for child=%s parent=%s enrollments=%d: %v",
				childID, parentID, len(enrollmentIDs), err)
			return nil, fmt.Errorf("failed to sum watched seconds: %w", err)
		}
		totalWatchedSeconds := 0
		for _, seconds := range watchedByEnrollment {
			totalWatchedSeconds += seconds
		}
		totalStudyMinutes = totalWatchedSeconds / 60
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
		ID:        childID.String(),
		Username:  child.UserName,
		FullName:  child.FullName,
		AvatarURL: child.AvatarURL,

		Relationship:      relation.Relationship,
		TotalXP:           totalXP,
		CurrentStreak:     currentStreak,
		EnrolledCourses:   int(totalEnrolled),
		CompletedCourses:  completedCount,
		TotalStudyMinutes: totalStudyMinutes,
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
		instructorName, instructorID := "", ""
		if e.Course.InstructorID != uuid.Nil && e.Course.Instructor.ID != uuid.Nil {
			instructorID = e.Course.InstructorID.String()
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
			InstructorID:    instructorID,
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
			ID:           g.ID.String(),
			ClassID:      g.ClassID.String(),
			ClassName:    className,
			GradeType:    string(g.GradeType),
			Title:        g.Title,
			Score:        score,
			MaxScore:     maxScore,
			Percentage:   percentage,
			Weight:       weight,
			GradedBy:     g.GradedBy.String(),
			GradedByName: graderDisplayName(&g.Grader),
			GradedAt:     g.CreatedAt,
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
				StartTime: sch.StartTime.String(),
				EndTime:   sch.EndTime.String(),
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
					StartTime:     sess.StartTime.String(),
					EndTime:       sess.EndTime.String(),
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
	relation, err := s.verifyParentChildRelation(ctx, parentID, childID)
	if err != nil {
		return nil, err
	}

	if !relation.CanViewAttendance {
		return nil, errors.New("không có quyền xem lịch học của học sinh này")
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
			StartTime:  sch.StartTime.String(),
			EndTime:    sch.EndTime.String(),
			Room:       sch.Room,
			Status:     "active",
		}
		from := sch.EffectiveFrom.Format("2006-01-02")
		entry.EffectiveFrom = &from
		if sch.EffectiveUntil != nil {
			until := sch.EffectiveUntil.Format("2006-01-02")
			entry.EffectiveUntil = &until
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

	liveSessions, err := s.scheduleRepo.GetCalendarLivestreamsByClassIDs(ctx, classIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get class livestreams: %w", err)
	}
	for _, live := range liveSessions {
		if live.ScheduledAt == nil {
			continue
		}
		loc := time.FixedZone("ICT", 7*60*60)
		start := live.ScheduledAt.In(loc)
		end := start.Add(time.Hour)
		if live.ScheduledEndAt != nil {
			end = live.ScheduledEndAt.In(loc)
		}
		day := start.Format("2006-01-02")
		status := "scheduled"
		if live.EffectiveStatus(time.Now()) == model.LivestreamStatusLive {
			status = "in_progress"
		}
		if live.EffectiveStatus(time.Now()) == model.LivestreamStatusEnded {
			status = "completed"
		}
		entry := dto.TimetableEntryDTO{SessionID: &live.ID, ClassID: live.ClassID, DayOfWeek: int(start.Weekday()), Date: &day, StartTime: start.Format("15:04"), EndTime: end.Format("15:04"), Topic: &live.Title, Room: live.Location, Status: status}
		if live.Class != nil {
			entry.ClassName = live.Class.Name
		}
		entries = append(entries, entry)
	}
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

	// Danh sách dựng từ bài tập của các lớp học sinh đang theo học (LEFT JOIN bài nộp của em), không từ bài đã nộp:
	// bài quá hạn mà chưa nộp lần nào phải hiện ra với phụ huynh (QA3 P-01). Phân trang theo bài tập; thống kê
	// đếm trên toàn bộ bài tập chứ không chỉ trang này.
	assignments, total, err := s.submissionRepo.ListStudentClassAssignments(ctx, childID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get assignments: %w", err)
	}

	ids := make([]uuid.UUID, len(assignments))
	for i, a := range assignments {
		ids[i] = a.ID
	}
	submissions, err := s.submissionRepo.ListUserSubmissionsForAssignments(ctx, childID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get submissions: %w", err)
	}
	subsByAssignment := make(map[uuid.UUID][]model.Submission, len(assignments))
	for _, sub := range submissions {
		subsByAssignment[sub.AssignmentID] = append(subsByAssignment[sub.AssignmentID], sub)
	}

	now := time.Now()
	items := make([]dto.ChildAssignmentDto, 0, len(assignments))
	for _, a := range assignments {
		items = append(items, buildChildAssignmentDto(a, subsByAssignment[a.ID], now))
	}

	counts, err := s.submissionRepo.StudentClassAssignmentStats(ctx, childID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to get assignment stats: %w", err)
	}

	return &dto.ChildAssignmentsResponseDto{
		Assignments: items,
		Stats: dto.AssignmentStatsDto{
			TotalAssignments: int(counts.Total),
			Completed:        int(counts.Completed),
			InProgress:       int(counts.InProgress),
			NotStarted:       int(counts.NotStarted),
			Overdue:          int(counts.Overdue),
		},
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// buildChildAssignmentDto dựng một dòng bài tập từ bài tập + các bài nộp của học sinh (mới nhất trước, có thể rỗng).
// Trạng thái: completed = có bài accepted; in_progress = đã nộp nhưng chưa accepted; chưa nộp thì overdue khi quá
// hạn, còn lại not_started. PHẢI khớp StudentClassAssignmentStats (cùng quy tắc, phía SQL).
func buildChildAssignmentDto(a model.Assignment, subs []model.Submission, now time.Time) dto.ChildAssignmentDto {
	className := ""
	if a.Class != nil {
		className = a.Class.Name
	}
	item := dto.ChildAssignmentDto{
		ID:              a.ID.String(),
		Title:           a.Title,
		Type:            a.Type,
		Difficulty:      string(a.Difficulty),
		ClassName:       className,
		DueDate:         a.EndTime,
		SubmissionCount: len(subs),
	}

	if len(subs) == 0 {
		item.Status = "not_started"
		if a.EndTime != nil && a.EndTime.Before(now) {
			item.Status = "overdue"
		}
		return item
	}

	item.Status = "in_progress"
	last := subs[0].CreatedAt
	item.LastSubmittedAt = &last
	best := subs[0]
	for _, sub := range subs {
		if sub.Verdict == model.VerdictAccepted {
			item.Status = "completed"
		}
		if sub.TestCasesPassed > best.TestCasesPassed {
			best = sub
		}
	}
	verdict := string(best.Verdict)
	item.BestVerdict = &verdict
	item.TestCasesPassed = best.TestCasesPassed
	item.TotalTestCases = best.TotalTestCases
	return item
}
