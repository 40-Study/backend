package app

import "study.com/v1/internal/service"

type Services struct {
	Auth                 *service.AuthService
	Role                 *service.RoleService
	SystemRole           *service.SystemRoleService
	UserSystemRole       *service.UserSystemRoleService
	UserOrganizationRole *service.UserOrganizationRoleService
	UserRole             *service.UserRoleService
	Permission           *service.PermissionService
	Organization         *service.OrganizationService
	Profile              *service.ProfileService
	Teacher              *service.TeacherService
	TeacherProfile       *service.TeacherProfileService
	Class                *service.ClassService
	ClassSchedule        *service.ClassScheduleService
	Attendance           *service.AttendanceService
}

func InitServices(resources *Resources, repos *Repositories) *Services {
	return &Services{
		Auth: service.NewAuthService(
			resources.Config,
			repos.User,
			repos.Role,
			repos.UserOrganizationRole,
			repos.UserSystemRole,
			repos.SystemRole,
			resources.Redis,
		),
		Role:       service.NewRoleService(repos.Role, repos.Permission),
		SystemRole: service.NewSystemRoleService(repos.SystemRole, repos.Permission),
		UserSystemRole: service.NewUserSystemRoleService(
			repos.UserSystemRole,
			repos.User,
			repos.SystemRole,
		),
		UserOrganizationRole: service.NewUserOrganizationRoleService(
			repos.UserOrganizationRole,
			repos.User,
			repos.Role,
			repos.Organization,
		),
		UserRole: service.NewUserRoleService(
			repos.User,
			repos.UserSystemRole,
			repos.UserOrganizationRole,
		),
		Permission:   service.NewPermissionService(repos.Permission),
		Organization: service.NewOrganizationService(repos.Organization),
		Profile: service.NewProfileService(
			repos.ParentStudent,
			repos.UserOrganizationRole,
		),
		Teacher:        service.NewTeacherService(repos.Teacher),
		TeacherProfile: service.NewTeacherProfileService(repos.TeacherProfile),
		Class:          service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student),
		ClassSchedule:  service.NewClassScheduleService(repos.ClassSchedule, repos.Class),
		Attendance:     service.NewAttendanceService(repos.Attendance),
	}
}
