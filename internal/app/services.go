package app

import "study.com/v1/internal/service"

type Services struct {
	Auth         *service.AuthService
	Role         *service.RoleService
	SystemRole   *service.SystemRoleService
	Permission   *service.PermissionService
	Organization *service.OrganizationService
}

func InitServices(resources *Resources, repos *Repositories) *Services {

	return &Services{
		Auth:         service.NewAuthService(resources.Config, repos.User, resources.Redis),
		Role:         service.NewRoleService(repos.Role, repos.Permission),
		SystemRole:   service.NewSystemRoleService(repos.SystemRole, repos.Permission),
		Permission:   service.NewPermissionService(repos.Permission),
		Organization: service.NewOrganizationService(repos.Organization),
	}
}
