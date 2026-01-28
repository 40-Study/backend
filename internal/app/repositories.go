package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	User           *repository.UserRepository
	RolePermission *repository.RolePermissionRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		User:           repository.NewUserRepository(db),
		RolePermission: repository.NewRolePermissionRepository(db),
	}
}
