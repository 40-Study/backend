package app

import (
	"study.com/v1/internal/handler"
	"study.com/v1/internal/storage"
)

// Handlers holds all handler instances
type Handlers struct {
	Auth           *handler.AuthHandler
	Role           *handler.RoleHandler
	SystemRole     *handler.SystemRoleHandler
	Permission     *handler.PermissionHandler
	Organization   *handler.OrganizationHandler
	Teacher        *handler.TeacherHandler
	TeacherProfile *handler.TeacherProfileHandler
	Class          *handler.ClassHandler
	ClassSchedule  *handler.ClassScheduleHandler
	Attendance     *handler.AttendanceHandler
	VideoUpload    *handler.VideoUploadHandler
	HLS            *handler.HLSHandler
}

// InitHandlers initializes all handlers
func InitHandlers(services *Services, minioClient *storage.MinioClient) *Handlers {
	return &Handlers{
		Auth:           handler.NewAuthHandler(services.Auth),
		Role:           handler.NewRoleHandler(services.Role),
		SystemRole:     handler.NewSystemRoleHandler(services.SystemRole),
		Permission:     handler.NewPermissionHandler(services.Permission),
		Organization:   handler.NewOrganizationHandler(services.Organization),
		Teacher:        handler.NewTeacherHandler(services.Teacher),
		TeacherProfile: handler.NewTeacherProfileHandler(services.TeacherProfile),
		Class:          handler.NewClassHandler(services.Class),
		ClassSchedule:  handler.NewClassScheduleHandler(services.ClassSchedule),
		Attendance:     handler.NewAttendanceHandler(services.Attendance),
		VideoUpload:    handler.NewVideoUploadHandler(services.VideoUpload),
		HLS:            handler.NewHLSHandler(minioClient),
	}
}
