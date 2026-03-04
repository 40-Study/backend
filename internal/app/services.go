package app

import (
	"log"

	"study.com/v1/internal/queue"
	"study.com/v1/internal/service"
)

type Services struct {
	Auth            *service.AuthService
	Role            *service.RoleService
	SystemRole      *service.SystemRoleService
	Permission      *service.PermissionService
	Organization    *service.OrganizationService
	Teacher         *service.TeacherService
	TeacherProfile  *service.TeacherProfileService
	Class           *service.ClassService
	ClassSchedule   *service.ClassScheduleService
	Attendance      *service.AttendanceService
	VideoUpload     *service.VideoUploadService
	VideoProcessing *service.VideoProcessingService
}

func InitServices(resources *Resources, repos *Repositories) *Services {
	// Setup video queue if RabbitMQ is available
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

	return &Services{
		Auth: service.NewAuthService(
			resources.Config,
			repos.User,
			repos.Role,
			repos.UserRole,
			resources.Redis,
		),
		Role:            service.NewRoleService(repos.Role, repos.Permission),
		SystemRole:      service.NewSystemRoleService(repos.SystemRole, repos.Permission),
		Permission:      service.NewPermissionService(repos.Permission),
		Organization:    service.NewOrganizationService(repos.Organization),
		Teacher:         service.NewTeacherService(repos.Teacher),
		TeacherProfile:  service.NewTeacherProfileService(repos.TeacherProfile),
		Class:           service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student),
		ClassSchedule:   service.NewClassScheduleService(repos.ClassSchedule, repos.Class),
		Attendance:      service.NewAttendanceService(repos.Attendance),
		VideoUpload:     uploadSvc,
		VideoProcessing: videoProcessingSvc,
	}
}
