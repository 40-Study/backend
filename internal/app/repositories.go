package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	User                 *repository.UserRepository
	Role                 *repository.RoleRepository
	SystemRole           *repository.SystemRoleRepository
	UserSystemRole       *repository.UserSystemRoleRepository
	UserOrganizationRole *repository.UserOrganizationRoleRepository
	Permission           *repository.PermissionRepository
	Organization         *repository.OrganizationRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		User:                 repository.NewUserRepository(db),
		Role:                 repository.NewRoleRepository(db),
		SystemRole:           repository.NewSystemRoleRepository(db),
		UserSystemRole:       repository.NewUserSystemRoleRepository(db),
		UserOrganizationRole: repository.NewUserOrganizationRoleRepository(db),
		Permission:           repository.NewPermissionRepository(db),
		Organization:         repository.NewOrganizationRepository(db),
	}
}
