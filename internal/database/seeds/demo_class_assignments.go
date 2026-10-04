package seeds

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// upsertDemoLivestreams tạo phiên livestream chữa bài của lớp (khoá: room_name, vốn unique).
// Trang teacher/schedule và teacher/assignments liệt kê theo /livestream?host_id=, trang
// my-assignments của học sinh liệt kê /livestream rồi lấy bài tập theo từng phiên — nên bài tập
// phải gắn vào phiên có class_id là lớp học sinh đang học.
func (s *Seeder) upsertDemoLivestreams(spec classSpec, class model.Class, teacher model.User, course model.Course) ([]model.LivestreamSession, error) {
	today := demoToday()
	lives := make([]model.LivestreamSession, 0, len(spec.Lives))
	for i, ls := range spec.Lives {
		scheduled := demoWallClock(today, ls.OffsetDays, ls.Hour, ls.Min)
		live := model.LivestreamSession{
			Title:       ls.Title,
			Description: ptr(ls.Description),
			HostID:      teacher.ID,
			ClassID:     class.ID,
			CourseID:    &course.ID,
			RoomName:    fmt.Sprintf("demo-%s-live-%d", spec.RoomKey, i+1),
			Status:      model.LivestreamStatusScheduled,
			ScheduledAt: &scheduled,
			MaxViewers:  100,
			IsRecorded:  true,
			Settings:    model.LivestreamSettings{IsChatEnabled: true, IsQAEnabled: true, IsWhiteboardEnabled: true, IsScreenShareEnabled: true},
		}
		if ls.OffsetDays < 0 {
			ended := scheduled.Add(90 * time.Minute)
			live.Status = model.LivestreamStatusEnded
			live.StartedAt = &scheduled
			live.EndedAt = &ended
		}
		if err := s.db.Where("room_name = ?", live.RoomName).Attrs(live).FirstOrCreate(&live).Error; err != nil {
			return nil, fmt.Errorf("failed to seed livestream %s: %w", live.RoomName, err)
		}
		lives = append(lives, live)
	}
	return lives, nil
}

// seedDemoAssignments tạo bài tập (khoá: lớp + tiêu đề), test case, bài nộp và điểm.
// Bài đã publish mới hiện với học sinh (GetBySession lọc publishedOnly cho người không quản lý).
func (s *Seeder) seedDemoAssignments(spec classSpec, class model.Class, teacher model.User, users map[string]model.User, lives []model.LivestreamSession) error {
	today := demoToday()
	for _, as := range spec.Assignments {
		if as.LiveIndex < 0 || as.LiveIndex >= len(lives) {
			return fmt.Errorf("assignment %q: live index %d out of range", as.Title, as.LiveIndex)
		}
		live := lives[as.LiveIndex]
		// 23:59 GIỜ VIỆT NAM tường minh (không phải giờ máy chủ): xem seedZone.
		due := demoWallClock(today, as.DueOffsetDays, 23, 59)

		assignment := model.Assignment{
			SessionID:           &live.ID,
			ClassID:             &class.ID,
			Title:               as.Title,
			Description:         as.Description,
			Difficulty:          as.Difficulty,
			Language:            pq.StringArray{spec.Language},
			StarterCode:         as.StarterCode,
			IsPublished:         as.Published,
			EndTime:             &due,
			AllowLateSubmission: as.Type == "homework",
			Type:                as.Type,
		}
		if as.Published {
			assignment.PublishedAt = live.ScheduledAt
		} else {
			assignment.StartTime = live.ScheduledAt // hẹn giờ publish vào buổi live
		}
		if err := s.db.Where("class_id = ? AND title = ?", class.ID, as.Title).
			Attrs(assignment).FirstOrCreate(&assignment).Error; err != nil {
			return fmt.Errorf("failed to seed assignment %q: %w", as.Title, err)
		}

		if err := s.upsertDemoTestCases(assignment, as.TestCases); err != nil {
			return err
		}
		for _, sub := range as.Results {
			student, ok := users[sub.StudentEmail]
			if !ok {
				return fmt.Errorf("student %s not found", sub.StudentEmail)
			}
			if err := s.upsertDemoSubmission(spec, as, assignment, due, student, sub); err != nil {
				return err
			}
			if sub.Score > 0 {
				gradeType := model.GradeAssignment
				if as.Type == "project" {
					gradeType = model.GradeProject
				}
				g := demoGrade{Student: student, Class: class, Teacher: teacher, AssignmentID: &assignment.ID, Type: gradeType,
					Title: "Bài tập: " + as.Title, Score: sub.Score, Weight: 0.2, GradedAt: due.AddDate(0, 0, 1), Feedback: sub.Feedback}
				if err := s.upsertDemoGrade(g); err != nil {
					return err
				}
			}
		}
	}

	for _, eg := range spec.ExtraGrades {
		student, ok := users[eg.StudentEmail]
		if !ok {
			return fmt.Errorf("student %s not found", eg.StudentEmail)
		}
		g := demoGrade{Student: student, Class: class, Teacher: teacher, Type: eg.Type, Title: eg.Title,
			Score: eg.Score, Weight: eg.Weight, GradedAt: daysAgo(eg.DaysAgo), Feedback: eg.Feedback}
		if err := s.upsertDemoGrade(g); err != nil {
			return err
		}
	}
	return nil
}

// upsertDemoTestCases: khoá assignment + display_order; test cuối cùng là test ẩn.
func (s *Seeder) upsertDemoTestCases(assignment model.Assignment, cases [][2]string) error {
	for i, tc := range cases {
		row := model.TestCase{AssignmentID: assignment.ID, Input: tc[0], ExpectedOutput: tc[1], IsHidden: i == len(cases)-1, DisplayOrder: i + 1}
		if err := s.db.Where("assignment_id = ? AND display_order = ?", assignment.ID, i+1).
			Attrs(row).FirstOrCreate(&row).Error; err != nil {
			return fmt.Errorf("failed to seed test case: %w", err)
		}
	}
	return nil
}

// upsertDemoSubmission: một bài nộp mỗi (bài tập, học sinh). Điểm trên trang giáo viên suy từ
// test_cases_passed/total (submission_service.go), trạng thái "đã chấm" suy từ verdict accepted.
func (s *Seeder) upsertDemoSubmission(spec classSpec, as assignmentSpec, assignment model.Assignment, due time.Time, student model.User, sub submissionSpec) error {
	row := model.Submission{
		AssignmentID:    assignment.ID,
		UserID:          student.ID,
		Language:        spec.Language,
		Code:            as.Solution,
		Verdict:         sub.Verdict,
		TestCasesPassed: sub.Passed,
		TotalTestCases:  len(as.TestCases),
	}
	if sub.Verdict != model.VerdictPending {
		row.ExecutionTime = 40 + 15*sub.Passed
		row.MemoryUsed = 2048
	}
	row.CreatedAt = due.AddDate(0, 0, -sub.DaysBeforeDue).Add(-3 * time.Hour)
	if err := s.db.Where("assignment_id = ? AND user_id = ?", assignment.ID, student.ID).
		Attrs(row).FirstOrCreate(&row).Error; err != nil {
		return fmt.Errorf("failed to seed submission: %w", err)
	}
	return nil
}

// demoGrade gom tham số của một cột điểm để hàm upsert không có danh sách tham số dài.
type demoGrade struct {
	Student      model.User
	Class        model.Class
	Teacher      model.User
	AssignmentID *uuid.UUID
	Type         model.GradeType
	Title        string
	Score        float64
	Weight       float64
	GradedAt     time.Time
	Feedback     string
}

// upsertDemoGrade ghi điểm thang 10 kèm nhận xét (khoá: học sinh + lớp + tên cột điểm). Nguồn của
// /me/grades và /parent/children/:id/grades.
func (s *Seeder) upsertDemoGrade(g demoGrade) error {
	row := model.Grade{
		StudentID:    g.Student.ID,
		ClassID:      g.Class.ID,
		AssignmentID: g.AssignmentID,
		GradeType:    g.Type,
		Title:        g.Title,
		Score:        decimal.NewFromFloat(g.Score),
		MaxScore:     decimal.NewFromInt(10),
		Weight:       decimal.NewFromFloat(g.Weight),
		GradedBy:     g.Teacher.ID,
		GradedAt:     g.GradedAt,
		Feedback:     ptr(g.Feedback),
	}
	if err := s.db.Where("student_id = ? AND class_id = ? AND title = ?", g.Student.ID, g.Class.ID, g.Title).
		Attrs(row).FirstOrCreate(&row).Error; err != nil {
		return fmt.Errorf("failed to seed grade %q: %w", g.Title, err)
	}
	return nil
}
