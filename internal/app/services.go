package app

import (
	"log"

	"study.com/v1/internal/queue"
	"study.com/v1/internal/service"
)

type Services struct {
	// ===== Auth & Role =====
	Auth                 *service.AuthService
	Role                 *service.RoleService
	SystemRole           *service.SystemRoleService
	UserSystemRole       *service.UserSystemRoleService
	UserOrganizationRole *service.UserOrganizationRoleService
	Permission           *service.PermissionService

	// ===== Organization & Profile =====
	Organization *service.OrganizationService
	Profile      *service.ProfileService

	// ===== Teacher =====
	Teacher        *service.TeacherService
	TeacherProfile *service.TeacherProfileService

	// ===== Class =====
	Class         *service.ClassService
	ClassSchedule *service.ClassScheduleService
	Attendance    *service.AttendanceService

	// ===== Course Management =====
	Category      *service.CategoryService
	Tag           *service.TagService
	CourseService *service.CourseService
	Section       *service.SectionService
	Lesson        *service.LessonService
	LessonContent *service.LessonContentService
	Enrollment    *service.EnrollmentService

	// ===== Video =====
	VideoUpload     *service.VideoUploadService
	VideoProcessing *service.VideoProcessingService
}

func InitServices(resources *Resources, repos *Repositories) *Services {

	// ================= Video Queue Setup =================
	var videoQueue *queue.VideoQueueSetup
	if resources.RabbitMQ != nil {
		videoQueue = queue.NewVideoQueueSetup(resources.RabbitMQ)
		if err := videoQueue.SetupVideoQueues(); err != nil {
			log.Printf("Warning: Failed to setup video queues: %v", err)
			videoQueue = nil
		}
	}

	uploadSvc := service.NewVideoUploadService(
		repos.VideoUpload,
		resources.MinioWrapper,
		resources.RabbitMQ,
		videoQueue,
		resources.Redis,
	)

	var videoProcessingSvc *service.VideoProcessingService
	if resources.RabbitMQ != nil && resources.MinioWrapper != nil {
		var err error
		videoProcessingSvc, err = service.NewVideoProcessingService(
			repos.VideoUpload,
			resources.MinioWrapper,
			uploadSvc,
			resources.RabbitMQ,
		)
		if err != nil {
			log.Printf("Warning: Failed to create video processing service: %v", err)
		}
	}

	// ================= Return Services =================
	return &Services{
		// ===== Auth =====
		Auth: service.NewAuthService(
			resources.Config,
			repos.User,
			repos.Role,
			repos.UserOrganizationRole,
			repos.UserSystemRole,
			repos.SystemRole,
			resources.Redis,
		),

		// ===== Role =====
		Role:       service.NewRoleService(repos.Role, repos.Permission),
		SystemRole: service.NewSystemRoleService(repos.SystemRole, repos.Permission),

		UserSystemRole: service.NewUserSystemRoleService(
			repos.UserSystemRole,
			repos.User,
			repos.SystemRole,
		),

		UserOrganizationRole: service.NewUserOrganizationRoleService(
			repos.UserOrganizationRole,
			repos.User,
			repos.Role,
			repos.Organization,
		),

		Permission: service.NewPermissionService(repos.Permission),

		// ===== Organization & Profile =====
		Organization: service.NewOrganizationService(repos.Organization),

		Profile: service.NewProfileService(
			repos.ParentStudent,
			repos.UserOrganizationRole,
		),

		// ===== Teacher =====
		Teacher:        service.NewTeacherService(repos.Teacher),
		TeacherProfile: service.NewTeacherProfileService(repos.TeacherProfile),

		// ===== Class =====
		Class:         service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student),
		ClassSchedule: service.NewClassScheduleService(repos.ClassSchedule, repos.Class),
		Attendance:    service.NewAttendanceService(repos.Attendance),

		// ===== Course Management =====
		Category:      service.NewCategoryService(repos.Category),
		Tag:           service.NewTagService(repos.Tag),
		CourseService: service.NewCourseService(repos.Course, repos.Category, repos.Tag),
		Section:       service.NewSectionService(repos.Section, repos.Course),
		Lesson:        service.NewLessonService(repos.Lesson, repos.Section, repos.Course),
		LessonContent: service.NewLessonContentService(repos.LessonContent, repos.Lesson),
		Enrollment:    service.NewEnrollmentService(repos.Enrollment, repos.Course, repos.Lesson),

		// ===== Video =====
		VideoUpload:     uploadSvc,
		VideoProcessing: videoProcessingSvc,
	}
}