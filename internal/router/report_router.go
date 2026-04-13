package router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupReportRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	reportHandler *handler.ReportHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	reports := api.Group("/reports", auth)

// 	// ============================================================================
// 	// CLASS REPORTS - Bao cao lop hoc
// 	// ============================================================================
// 	// GET    /api/reports/class/:classId/attendance    - Bao cao diem danh
// 	// GET    /api/reports/class/:classId/grades        - Bao cao diem
// 	// GET    /api/reports/class/:classId/engagement    - Bao cao tuong tac
// 	// GET    /api/reports/class/:classId/progress      - Tien do hoc tap
// 	// GET    /api/reports/class/:classId/summary       - Tong hop lop

// 	classReports := reports.Group("/class/:classId")
// 	{
// 		classReports.Get("/attendance", reportHandler.GetClassAttendanceReport)
// 		classReports.Get("/grades", reportHandler.GetClassGradeReport)
// 		classReports.Get("/engagement", reportHandler.GetClassEngagementReport)
// 		classReports.Get("/progress", reportHandler.GetClassProgressReport)
// 		classReports.Get("/summary", reportHandler.GetClassSummary)
// 	}

// 	// ============================================================================
// 	// STUDENT REPORTS - Bao cao hoc sinh
// 	// ============================================================================
// 	// GET    /api/reports/student/:studentId           - Bao cao tong hop hoc sinh
// 	// GET    /api/reports/student/:studentId/attendance - Diem danh hoc sinh
// 	// GET    /api/reports/student/:studentId/grades    - Diem so hoc sinh
// 	// GET    /api/reports/student/:studentId/progress  - Tien do hoc sinh

// 	studentReports := reports.Group("/student/:studentId")
// 	{
// 		studentReports.Get("/", reportHandler.GetStudentReport)
// 		studentReports.Get("/attendance", reportHandler.GetStudentAttendanceReport)
// 		studentReports.Get("/grades", reportHandler.GetStudentGradeReport)
// 		studentReports.Get("/progress", reportHandler.GetStudentProgressReport)
// 	}

// 	// ============================================================================
// 	// COURSE REPORTS - Bao cao khoa hoc
// 	// ============================================================================
// 	// GET    /api/reports/course/:courseId/revenue     - Doanh thu khoa hoc
// 	// GET    /api/reports/course/:courseId/enrollment  - Thong ke ghi danh
// 	// GET    /api/reports/course/:courseId/completion  - Ty le hoan thanh
// 	// GET    /api/reports/course/:courseId/reviews     - Thong ke danh gia

// 	courseReports := reports.Group("/course/:courseId")
// 	{
// 		courseReports.Get("/revenue", reportHandler.GetCourseRevenueReport)
// 		courseReports.Get("/enrollment", reportHandler.GetCourseEnrollmentReport)
// 		courseReports.Get("/completion", reportHandler.GetCourseCompletionReport)
// 		courseReports.Get("/reviews", reportHandler.GetCourseReviewReport)
// 	}

// 	// ============================================================================
// 	// ORGANIZATION REPORTS - Bao cao to chuc
// 	// ============================================================================
// 	// GET    /api/reports/organization/overview        - Tong quan organization
// 	// GET    /api/reports/organization/users           - Thong ke users
// 	// GET    /api/reports/organization/revenue         - Doanh thu
// 	// GET    /api/reports/organization/activity        - Hoat dong

// 	orgReports := reports.Group("/organization")
// 	{
// 		orgReports.Get("/overview", reportHandler.GetOrganizationOverview)
// 		orgReports.Get("/users", reportHandler.GetOrganizationUserReport)
// 		orgReports.Get("/revenue", reportHandler.GetOrganizationRevenueReport)
// 		orgReports.Get("/activity", reportHandler.GetOrganizationActivityReport)
// 	}

// 	// ============================================================================
// 	// TEACHER REPORTS - Bao cao giao vien
// 	// ============================================================================
// 	// GET    /api/reports/teacher/:teacherId           - Bao cao giao vien
// 	// GET    /api/reports/teacher/:teacherId/classes   - Cac lop cua giao vien
// 	// GET    /api/reports/teacher/:teacherId/students  - Hoc sinh cua giao vien

// 	teacherReports := reports.Group("/teacher/:teacherId")
// 	{
// 		teacherReports.Get("/", reportHandler.GetTeacherReport)
// 		teacherReports.Get("/classes", reportHandler.GetTeacherClassesReport)
// 		teacherReports.Get("/students", reportHandler.GetTeacherStudentsReport)
// 	}

// 	// ============================================================================
// 	// MY REPORTS - Bao cao ca nhan
// 	// ============================================================================
// 	// GET    /api/me/learning-stats                    - Thong ke hoc tap cua toi
// 	// GET    /api/me/learning-stats/weekly             - Thong ke tuan
// 	// GET    /api/me/learning-stats/monthly            - Thong ke thang

// 	api.Get("/me/learning-stats", auth, reportHandler.GetMyLearningStats)
// 	api.Get("/me/learning-stats/weekly", auth, reportHandler.GetMyWeeklyStats)
// 	api.Get("/me/learning-stats/monthly", auth, reportHandler.GetMyMonthlyStats)

// 	// ============================================================================
// 	// EXPORT - Xuat bao cao
// 	// ============================================================================
// 	// POST   /api/reports/export                       - Xuat bao cao (CSV/PDF/Excel)
// 	//        Body: { type, format, filters, ... }

// 	reports.Post("/export", reportHandler.ExportReport)
// }
