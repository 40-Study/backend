package app

import (
	"study.com/v1/internal/handler"
	"study.com/v1/internal/storage"
)

// Handlers holds all handler instances
type Handlers struct {
	// ===== Auth & Role =====
	Auth                 *handler.AuthHandler
	Role                 *handler.RoleHandler
	SystemRole           *handler.SystemRoleHandler
	UserSystemRole       *handler.UserSystemRoleHandler
	UserOrganizationRole *handler.UserOrganizationRoleHandler
	Permission           *handler.PermissionHandler

	// ===== Organization & Profile =====
	Organization *handler.OrganizationHandler
	Profile      *handler.ProfileHandler

	// ===== Teacher =====
	Teacher        *handler.TeacherHandler
	TeacherProfile *handler.TeacherProfileHandler

	// ===== Class =====
	Class         *handler.ClassHandler
	ClassSchedule *handler.ClassScheduleHandler
	Attendance    *handler.AttendanceHandler

	// ===== Course Management =====
	Category      *handler.CategoryHandler
	Tag           *handler.TagHandler
	CourseHandler *handler.CourseHandler
	Section       *handler.SectionHandler
	Lesson        *handler.LessonHandler
	LessonContent *handler.LessonContentHandler
	Enrollment    *handler.EnrollmentHandler

	// ===== Video =====
	VideoUpload *handler.VideoUploadHandler
	HLS         *handler.HLSHandler

	// ===== LiveKit =====
	Livekit *handler.LivekitHandler
}

// InitHandlers initializes all handlers
func InitHandlers(services *Services, minioClient *storage.MinioClient) *Handlers {
	return &Handlers{
		// ===== Auth & Role =====
		Auth:                 handler.NewAuthHandler(services.Auth),
		Role:                 handler.NewRoleHandler(services.Role),
		SystemRole:           handler.NewSystemRoleHandler(services.SystemRole),
		UserSystemRole:       handler.NewUserSystemRoleHandler(services.UserSystemRole),
		UserOrganizationRole: handler.NewUserOrganizationRoleHandler(services.UserOrganizationRole),
		Permission:           handler.NewPermissionHandler(services.Permission),

		// ===== Organization & Profile =====
		Organization: handler.NewOrganizationHandler(services.Organization),
		Profile:      handler.NewProfileHandler(services.Profile, services.UserOrganizationRole),

		// ===== Teacher =====
		Teacher:        handler.NewTeacherHandler(services.Teacher),
		TeacherProfile: handler.NewTeacherProfileHandler(services.TeacherProfile),

		// ===== Class =====
		Class:         handler.NewClassHandler(services.Class),
		ClassSchedule: handler.NewClassScheduleHandler(services.ClassSchedule),
		Attendance:    handler.NewAttendanceHandler(services.Attendance),

		// ===== Course Management =====
		Category:      handler.NewCategoryHandler(services.Category),
		Tag:           handler.NewTagHandler(services.Tag),
		CourseHandler: handler.NewCourseHandler(services.CourseService),
		Section:       handler.NewSectionHandler(services.Section),
		Lesson:        handler.NewLessonHandler(services.Lesson),
		LessonContent: handler.NewLessonContentHandler(services.LessonContent),
		Enrollment:    handler.NewEnrollmentHandler(services.Enrollment),

		// ===== Video =====
		VideoUpload: handler.NewVideoUploadHandler(services.VideoUpload),
		HLS:         handler.NewHLSHandler(minioClient),

		// ===== LiveKit =====
		Livekit: handler.NewLivekitHandler(services.Livekit),
	}
}
