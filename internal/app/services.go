package app

import "study.com/v1/internal/service"

type Services struct {
	Auth           *service.AuthService
	Role           *service.RoleService
	SystemRole     *service.SystemRoleService
	Permission     *service.PermissionService
	Organization   *service.OrganizationService
	Teacher        *service.TeacherService
	TeacherProfile *service.TeacherProfileService
	Class          *service.ClassService
	ClassSchedule  *service.ClassScheduleService
	Attendance     *service.AttendanceService
}

func InitServices(resources *Resources, repos *Repositories) *Services {
	return &Services{
		Auth: service.NewAuthService(
			resources.Config,
			repos.User,
			repos.Role,
			repos.UserRole,
			resources.Redis,
		),
		Role:           service.NewRoleService(repos.Role, repos.Permission),
		SystemRole:     service.NewSystemRoleService(repos.SystemRole, repos.Permission),
		Permission:     service.NewPermissionService(repos.Permission),
		Organization:   service.NewOrganizationService(repos.Organization),
		Teacher:        service.NewTeacherService(repos.Teacher),
		TeacherProfile: service.NewTeacherProfileService(repos.TeacherProfile),
		Class:          service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student),
		ClassSchedule:  service.NewClassScheduleService(repos.ClassSchedule, repos.Class),
		Attendance:     service.NewAttendanceService(repos.Attendance),
	}
}
