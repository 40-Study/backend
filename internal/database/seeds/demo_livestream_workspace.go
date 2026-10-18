package seeds

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type LivestreamDemo struct {
	Email      string    `json:"email"`
	CourseID   uuid.UUID `json:"course_id"`
	CourseSlug string    `json:"course_slug"`
	ClassID    uuid.UUID `json:"class_id"`
	LessonID   uuid.UUID `json:"lesson_id"`
	SessionID  uuid.UUID `json:"session_id"`
}

// SeedLivestreamWorkspace creates only this demo workspace and preserves prior session state.
func (s *Seeder) SeedLivestreamWorkspace() (*LivestreamDemo, error) {
	out := &LivestreamDemo{Email: "teacher.live@demo.com", CourseSlug: "demo-lop-hoc-truc-tiep"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		seed := NewSeeder(tx)
		spec := demoUserSpec{Email: out.Email, UserName: "teacher.live", FullName: "Giáo viên Livestream Demo", SystemRole: "TEACHER", Specialization: "Dạy lập trình trực tiếp", Education: "Hồ sơ dùng thử", ExperienceYears: 5}
		var teacher model.User
		if err := tx.Where("email = ?", spec.Email).First(&teacher).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			hash, err := utils.HashPassword(DemoPassword)
			if err != nil {
				return err
			}
			teacher = model.User{Email: spec.Email, UserName: spec.UserName, FullName: ptr(spec.FullName), IsVerified: true, IsActive: true, PasswordHash: hash}
			if err := tx.Create(&teacher).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if !isDemoOwnedUser(teacher, spec) {
			return fmt.Errorf("existing account is not owned by this demo")
		}
		if err := seed.assignSystemRole(teacher.ID, "TEACHER"); err != nil {
			return err
		}
		if err := seed.upsertTeacherProfile(teacher.ID, spec); err != nil {
			return err
		}
		now := time.Now()
		course := model.Course{InstructorID: teacher.ID, Title: "Lớp học lập trình trực tiếp · Demo", Slug: out.CourseSlug, Description: ptr("Không gian thử livestream, bài học và bài tập cho giáo viên/học sinh demo."), ShortDescription: ptr("Lớp demo để kiểm tra buổi học trực tiếp."), Level: "beginner", Language: "vi", Status: "published", PublishedAt: &now, IsFree: true, TotalLessons: 1, TotalStudents: 2}
		if err := tx.Where("slug = ?", course.Slug).Attrs(course).FirstOrCreate(&course).Error; err != nil {
			return err
		}
		if course.InstructorID != teacher.ID {
			return fmt.Errorf("demo course belongs to another teacher")
		}
		out.CourseID = course.ID
		class := model.Class{Name: "DEMO-LIVE · Lớp lập trình trực tiếp", CourseID: &course.ID, CreatedBy: &teacher.ID, Status: "active"}
		if err := tx.Where("name = ?", class.Name).Attrs(class).FirstOrCreate(&class).Error; err != nil {
			return err
		}
		out.ClassID = class.ID
		tc := model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID, Role: "primary"}
		if err := tx.Where("teacher_id = ? AND class_id = ?", teacher.ID, class.ID).FirstOrCreate(&tc).Error; err != nil {
			return err
		}
		for _, email := range []string{"student1@demo.com", "student2@demo.com"} {
			var student model.User
			if err := tx.Where("email = ?", email).First(&student).Error; err != nil {
				return err
			}
			sc := model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active", EnrolledAt: now}
			if err := tx.Where("student_id = ? AND class_id = ?", student.ID, class.ID).Attrs(sc).FirstOrCreate(&sc).Error; err != nil {
				return err
			}
			enrollment := model.Enrollment{UserID: student.ID, CourseID: course.ID, TotalLessons: 1}
			if err := tx.Where("user_id = ? AND course_id = ?", student.ID, course.ID).FirstOrCreate(&enrollment).Error; err != nil {
				return err
			}
		}
		section := model.Section{CourseID: course.ID, Title: "Buổi học trực tiếp", DisplayOrder: 0}
		if err := tx.Where("course_id = ? AND title = ?", course.ID, section.Title).Attrs(section).FirstOrCreate(&section).Error; err != nil {
			return err
		}
		lesson := model.Lesson{SectionID: section.ID, Title: "Live · Chữa bài lập trình và hỏi đáp", DisplayOrder: 0}
		if err := tx.Where("section_id = ? AND title = ?", section.ID, lesson.Title).Attrs(lesson).FirstOrCreate(&lesson).Error; err != nil {
			return err
		}
		out.LessonID = lesson.ID
		scheduled := now.Add(30 * time.Minute)
		ends := scheduled.Add(time.Hour)
		live := model.LivestreamSession{HostID: teacher.ID, ClassID: class.ID, CourseID: &course.ID, RoomName: "demo-teaching-livestream", Title: "Demo · Chữa bài lập trình và hỏi đáp", Status: model.LivestreamStatusScheduled, ScheduledAt: &scheduled, ScheduledEndAt: &ends, MaxViewers: 30, IsRecorded: false, Settings: model.LivestreamSettings{IsChatEnabled: true, IsQAEnabled: true, IsWhiteboardEnabled: true, IsScreenShareEnabled: true, IsPollsEnabled: true}}
		if err := tx.Where("room_name = ?", live.RoomName).Attrs(live).FirstOrCreate(&live).Error; err != nil {
			return err
		}
		out.SessionID = live.ID
		content := model.LessonContent{LessonID: lesson.ID, Type: "livestream", Title: ptr(live.Title), LivestreamSessionID: &live.ID, IsMandatory: true}
		if err := tx.Where("lesson_id = ? AND type = ?", lesson.ID, "livestream").Attrs(content).FirstOrCreate(&content).Error; err != nil {
			return err
		}
		if live.LessonContentID == nil {
			if err := tx.Model(&live).Update("lesson_content_id", content.ID).Error; err != nil {
				return err
			}
		}
		analytics := model.LivestreamAnalytics{SessionID: live.ID}
		return tx.Where("session_id = ?", live.ID).FirstOrCreate(&analytics).Error
	})
	return out, err
}
