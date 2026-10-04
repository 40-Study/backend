package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupClassRoutes(
	api fiber.Router,
	cfg *config.Config,
	classHandler *handler.ClassHandler,
	attendanceHandler *handler.AttendanceHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	// B-02: danh sách lớp của một tổ chức cho chủ/quản trị tổ chức. Cùng quyền ORG_MEMBERS_MANAGE với orgManagesClass
	// (class_access.go) và route thành viên; active_org_id phải khớp tổ chức trên URL.
	api.Get("/organizations/:organization_id/classes", middleware.AuthMiddleware(cfg, redis),
		permChecker.RequireOrgPermission("organization_id", "ORG_MEMBERS_MANAGE"), classHandler.GetOrganizationClasses)

	classes := api.Group("/classes", middleware.AuthMiddleware(cfg, redis))
	{
		classes.Post("/", classHandler.CreateClass)
		classes.Get("/", classHandler.GetAllClasses)
		classes.Get("/me", classHandler.GetMyClasses)
		classes.Get("/:id", classHandler.GetClassByID)
		classes.Put("/:id", classHandler.UpdateClass)
		classes.Delete("/:id", classHandler.DeleteClass)

		// Teacher-Class
		classes.Post("/:id/teachers", classHandler.AssignTeacherToClass)
		classes.Delete("/:id/teachers/:teacherId", classHandler.RemoveTeacherFromClass)
		classes.Get("/:id/teachers", classHandler.GetTeachersByClass)
		classes.Get("/:id/assignable-teachers", classHandler.SearchAssignableTeachers)

		// Student-Class
		classes.Post("/:id/students", classHandler.EnrollStudentToClass)
		classes.Delete("/:id/students/:studentId", classHandler.RemoveStudentFromClass)
		classes.Get("/:id/students", classHandler.GetStudentsByClass)
		classes.Get("/:id/enrollable-students", classHandler.SearchEnrollableStudents)

		// Attendances
		attendances := classes.Group("/:classId/attendances")
		{
			attendances.Post("/", attendanceHandler.MarkAttendance)
			attendances.Get("/", attendanceHandler.GetAllAttendances)
			attendances.Get("/:id", attendanceHandler.GetAttendanceByID)
			attendances.Put("/:id", attendanceHandler.UpdateAttendance)
			attendances.Delete("/:id", attendanceHandler.DeleteAttendance)
		}
	}
}
