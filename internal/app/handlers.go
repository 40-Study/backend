package app

import (
	"study.com/v1/internal/handler"
	"study.com/v1/internal/storage"
)

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

	// ===== Video =====
	VideoUpload *handler.VideoUploadHandler
	HLS         *handler.HLSHandler

	// ===== LiveKit =====
	Livekit *handler.LivekitHandler

	// ===== Livestream Learning Platform =====
	Livestream *handler.LivestreamHandler
	Assignment *handler.AssignmentHandler
	Submission *handler.SubmissionHandler
	Chat       *handler.ChatHandler
	Whiteboard *handler.WhiteboardHandler
	Analytics  *handler.AnalyticsHandler
}

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

		// ===== Video =====
		VideoUpload: handler.NewVideoUploadHandler(services.VideoUpload),
		HLS:         handler.NewHLSHandler(minioClient),

		// ===== LiveKit =====
		Livekit: handler.NewLivekitHandler(services.Livekit),

		// ===== Livestream Learning Platform =====
		Livestream: handler.NewLivestreamHandler(services.Livestream),
		Assignment: handler.NewAssignmentHandler(services.Assignment, services.Livekit),
		Submission: handler.NewSubmissionHandler(services.Submission),
		Chat:       handler.NewChatHandler(services.Chat),
		Whiteboard: handler.NewWhiteboardHandler(services.Whiteboard, services.Livekit),
		Analytics:  handler.NewAnalyticsHandler(services.Analytics),
	}
}
