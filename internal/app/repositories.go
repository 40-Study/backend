package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	// ===== User & Role =====
	User                 *repository.UserRepository
	Role                 *repository.RoleRepository
	SystemRole           *repository.SystemRoleRepository
	UserSystemRole       *repository.UserSystemRoleRepository
	UserOrganizationRole *repository.UserOrganizationRoleRepository
	Permission           *repository.PermissionRepository

	// ===== Organization =====
	Organization  *repository.OrganizationRepository
	ParentStudent *repository.ParentStudentRepository

	// ===== Teacher =====
	Teacher        *repository.TeacherRepository
	TeacherProfile *repository.TeacherProfileRepository

	// ===== Class =====
	Class         *repository.ClassRepository
	ClassSchedule *repository.ClassScheduleRepository
	Attendance    *repository.AttendanceRepository

	// ===== Course & Student =====
	Course  *repository.CourseRepository
	Student *repository.StudentRepository

	// ===== Course Management =====
	Category      *repository.CategoryRepository
	Tag           *repository.TagRepository
	Section       *repository.SectionRepository
	Lesson        *repository.LessonRepository
	LessonContent *repository.LessonContentRepository
	Enrollment    *repository.EnrollmentRepository

	// ===== Video =====
	VideoUpload *repository.VideoUploadRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		// ===== User & Role =====
		User:                 repository.NewUserRepository(db),
		Role:                 repository.NewRoleRepository(db),
		SystemRole:           repository.NewSystemRoleRepository(db),
		UserSystemRole:       repository.NewUserSystemRoleRepository(db),
		UserOrganizationRole: repository.NewUserOrganizationRoleRepository(db),
		Permission:           repository.NewPermissionRepository(db),

		// ===== Organization =====
		Organization:  repository.NewOrganizationRepository(db),
		ParentStudent: repository.NewParentStudentRepository(db),

		// ===== Teacher =====
		Teacher:        repository.NewTeacherRepository(db),
		TeacherProfile: repository.NewTeacherProfileRepository(db),

		// ===== Class =====
		Class:         repository.NewClassRepository(db),
		ClassSchedule: repository.NewClassScheduleRepository(db),
		Attendance:    repository.NewAttendanceRepository(db),

		// ===== Course & Student =====
		Course:  repository.NewCourseRepository(db),
		Student: repository.NewStudentRepository(db),

		// ===== Course Management =====
		Category:      repository.NewCategoryRepository(db),
		Tag:           repository.NewTagRepository(db),
		Section:       repository.NewSectionRepository(db),
		Lesson:        repository.NewLessonRepository(db),
		LessonContent: repository.NewLessonContentRepository(db),
		Enrollment:    repository.NewEnrollmentRepository(db),

		// ===== Video =====
		VideoUpload: repository.NewVideoUploadRepository(db),
	}
}