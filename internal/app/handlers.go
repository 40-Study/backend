package app

import "study.com/v1/internal/handler"

// Handlers holds all handler instances
type Handlers struct {
	Auth       *handler.AuthHandler
	Role       *handler.RoleHandler
	Permission *handler.PermissionHandler
}

// InitHandlers initializes all handlers
func InitHandlers(services *Services) *Handlers {
	return &Handlers{
		Auth:       handler.NewAuthHandler(services.Auth),
		Role:       handler.NewRoleHandler(services.Role),
		Permission: handler.NewPermissionHandler(services.Permission),
	}
}
