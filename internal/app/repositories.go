package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	User *repository.UserRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		User: repository.NewUserRepository(db),
	}
}
