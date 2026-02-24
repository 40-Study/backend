package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	User           *repository.UserRepository
	Role           *repository.RoleRepository
	SystemRole     *repository.SystemRoleRepository
	UserRole       *repository.UserRoleRepository
	Permission     *repository.PermissionRepository
	Organization   *repository.OrganizationRepository
	Teacher        *repository.TeacherRepository
	TeacherProfile *repository.TeacherProfileRepository
	Class          *repository.ClassRepository
	ClassSchedule  *repository.ClassScheduleRepository
	Attendance     *repository.AttendanceRepository
	Course         *repository.CourseRepository
	Student        *repository.StudentRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		User:           repository.NewUserRepository(db),
		Role:           repository.NewRoleRepository(db),
		SystemRole:     repository.NewSystemRoleRepository(db),
		UserRole:       repository.NewUserRoleRepository(db),
		Permission:     repository.NewPermissionRepository(db),
		Organization:   repository.NewOrganizationRepository(db),
		Teacher:        repository.NewTeacherRepository(db),
		TeacherProfile: repository.NewTeacherProfileRepository(db),
		Class:          repository.NewClassRepository(db),
		ClassSchedule:  repository.NewClassScheduleRepository(db),
		Attendance:     repository.NewAttendanceRepository(db),
		Course:         repository.NewCourseRepository(db),
		Student:        repository.NewStudentRepository(db),
	}
}
