package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	User         *repository.UserRepository
	Role         *repository.RoleRepository
	SystemRole   *repository.SystemRoleRepository
	UserRole     *repository.UserRoleRepository
	Permission   *repository.PermissionRepository
	Organization *repository.OrganizationRepository
	Teacher      *repository.TeacherRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		User:         repository.NewUserRepository(db),
		Role:         repository.NewRoleRepository(db),
		SystemRole:   repository.NewSystemRoleRepository(db),
		UserRole:     repository.NewUserRoleRepository(db),
		Permission:   repository.NewPermissionRepository(db),
		Organization: repository.NewOrganizationRepository(db),
		Teacher:      repository.NewTeacherRepository(db),
	}
}
