package app

import "study.com/v1/internal/service"

type Services struct {
	Auth           *service.AuthService
	RolePermission *service.RolePermissionService
}

func InitServices(resources *Resources, repos *Repositories) *Services {

	return &Services{
		Auth:           service.NewAuthService(resources.Config, repos.User, resources.Redis),
		RolePermission: service.NewRolePermissionService(repos.RolePermission),
	}
}
