package app

import "study.com/v1/internal/service"

type Services struct {
	Auth                 *service.AuthService
	Role                 *service.RoleService
	SystemRole           *service.SystemRoleService
	UserSystemRole       *service.UserSystemRoleService
	UserOrganizationRole *service.UserOrganizationRoleService
	Permission           *service.PermissionService
	Organization         *service.OrganizationService
	Profile              *service.ProfileService
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
		Permission:   service.NewPermissionService(repos.Permission),
		Organization: service.NewOrganizationService(repos.Organization),
		Profile: service.NewProfileService(
			repos.ParentStudent,
			repos.UserOrganizationRole,
		),
	}
}
