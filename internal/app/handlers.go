package app

import "study.com/v1/internal/handler"

// Handlers holds all handler instances
type Handlers struct {
	Auth *handler.AuthHandler
}

// InitHandlers initializes all handlers
func InitHandlers(services *Services) *Handlers {
	return &Handlers{
		Auth: handler.NewAuthHandler(services.Auth),
	}
}
