package app

import "study.com/v1/internal/service"

type Services struct {
	Auth       *service.AuthService
	Role       *service.RoleService
	Permission *service.PermissionService
}

func InitServices(resources *Resources, repos *Repositories) *Services {

	return &Services{
		Auth:       service.NewAuthService(resources.Config, repos.User, resources.Redis),
		Role:       service.NewRoleService(repos.Role),
		Permission: service.NewPermissionService(repos.Permission),
	}
}
