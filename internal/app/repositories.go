package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	// ===== User & Role =====
	User                 *repository.UserRepository
	Role                 *repository.RoleRepository
	SystemRole           *repository.SystemRoleRepository
	UserSystemRole       *repository.UserSystemRoleRepository
	UserOrganizationRole *repository.UserOrganizationRoleRepository
	Permission           *repository.PermissionRepository

	// ===== Organization =====
	Organization  *repository.OrganizationRepository
	ParentStudent *repository.ParentStudentRepository

	// ===== Teacher =====
	Teacher        *repository.TeacherRepository
	TeacherProfile *repository.TeacherProfileRepository

	// ===== Class =====
	Class         *repository.ClassRepository
	ClassSchedule *repository.ClassScheduleRepository
	Attendance    *repository.AttendanceRepository

	// ===== Course & Student =====
	Course  *repository.CourseRepository
	Student *repository.StudentRepository

	// ===== Video =====
	VideoUpload *repository.VideoUploadRepository

	// ===== Livestream Learning Platform =====
	Livestream  *repository.LivestreamRepository
	Participant *repository.ParticipantRepository
	Assignment  *repository.AssignmentRepository
	TestCase    *repository.TestCaseRepository
	Submission  *repository.SubmissionRepository
	ChatMessage *repository.ChatMessageRepository
	Whiteboard  *repository.WhiteboardRepository
	Analytics   *repository.AnalyticsRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		// ===== User & Role =====
		User:                 repository.NewUserRepository(db),
		Role:                 repository.NewRoleRepository(db),
		SystemRole:           repository.NewSystemRoleRepository(db),
		UserSystemRole:       repository.NewUserSystemRoleRepository(db),
		UserOrganizationRole: repository.NewUserOrganizationRoleRepository(db),
		Permission:           repository.NewPermissionRepository(db),

		// ===== Organization =====
		Organization:  repository.NewOrganizationRepository(db),
		ParentStudent: repository.NewParentStudentRepository(db),

		// ===== Teacher =====
		Teacher:        repository.NewTeacherRepository(db),
		TeacherProfile: repository.NewTeacherProfileRepository(db),

		// ===== Class =====
		Class:         repository.NewClassRepository(db),
		ClassSchedule: repository.NewClassScheduleRepository(db),
		Attendance:    repository.NewAttendanceRepository(db),

		// ===== Course & Student =====
		Course:  repository.NewCourseRepository(db),
		Student: repository.NewStudentRepository(db),

		// ===== Livestream Learning Platform =====
		Livestream: repository.NewLivestreamRepository(db),
		Participant: repository.NewParticipantRepository(db),
		Assignment: repository.NewAssignmentRepository(db),
		TestCase: repository.NewTestCaseRepository(db),
		Submission: repository.NewSubmissionRepository(db),
		ChatMessage: repository.NewChatMessageRepository(db),
		Whiteboard: repository.NewWhiteboardRepository(db),
		Analytics: repository.NewAnalyticsRepository(db),
	}

}
