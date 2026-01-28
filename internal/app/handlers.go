package app

import "study.com/v1/internal/handler"

// Handlers holds all handler instances
type Handlers struct {
	Auth           *handler.AuthHandler
	RolePermission *handler.RolePermissionHandler
}

// InitHandlers initializes all handlers
func InitHandlers(services *Services) *Handlers {
	return &Handlers{
		Auth:           handler.NewAuthHandler(services.Auth),
		RolePermission: handler.NewRolePermissionHandler(services.RolePermission),
	}
}
