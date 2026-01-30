package app

import "study.com/v1/internal/service"

type Services struct {
	Auth *service.AuthService
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
	}
}
