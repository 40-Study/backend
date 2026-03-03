package app

import "study.com/v1/internal/handler"

// Handlers holds all handler instances
type Handlers struct {
	Auth                 *handler.AuthHandler
	Role                 *handler.RoleHandler
	SystemRole           *handler.SystemRoleHandler
	UserSystemRole       *handler.UserSystemRoleHandler
	UserOrganizationRole *handler.UserOrganizationRoleHandler
	Permission           *handler.PermissionHandler
	Organization         *handler.OrganizationHandler
	Profile              *handler.ProfileHandler
	Teacher              *handler.TeacherHandler
	TeacherProfile       *handler.TeacherProfileHandler
	Class                *handler.ClassHandler
	ClassSchedule        *handler.ClassScheduleHandler
	Attendance           *handler.AttendanceHandler
}

// InitHandlers initializes all handlers
func InitHandlers(services *Services) *Handlers {
	return &Handlers{
		Auth:                 handler.NewAuthHandler(services.Auth),
		Role:                 handler.NewRoleHandler(services.Role),
		SystemRole:           handler.NewSystemRoleHandler(services.SystemRole),
		UserSystemRole:       handler.NewUserSystemRoleHandler(services.UserSystemRole),
		UserOrganizationRole: handler.NewUserOrganizationRoleHandler(services.UserOrganizationRole),
		Permission:           handler.NewPermissionHandler(services.Permission),
		Organization:         handler.NewOrganizationHandler(services.Organization),
		Profile:              handler.NewProfileHandler(services.Profile, services.UserOrganizationRole),
		Teacher:              handler.NewTeacherHandler(services.Teacher),
		TeacherProfile:       handler.NewTeacherProfileHandler(services.TeacherProfile),
		Class:                handler.NewClassHandler(services.Class),
		ClassSchedule:        handler.NewClassScheduleHandler(services.ClassSchedule),
		Attendance:           handler.NewAttendanceHandler(services.Attendance),
	}
}
