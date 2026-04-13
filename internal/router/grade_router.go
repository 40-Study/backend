package router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupGradeRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	gradeHandler *handler.GradeHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	// ============================================================================
// 	// GRADE COLUMNS - Cau truc cot diem cua lop
// 	// ============================================================================
// 	// POST   /api/classes/:classId/grade-columns       - Tao cot diem
// 	// GET    /api/classes/:classId/grade-columns       - Lay cau truc cot diem
// 	// PUT    /api/classes/:classId/grade-columns/:id   - Cap nhat cot diem
// 	// DELETE /api/classes/:classId/grade-columns/:id   - Xoa cot diem
// 	// PUT    /api/classes/:classId/grade-columns/reorder - Sap xep lai cot diem

// 	gradeColumns := api.Group("/classes/:classId/grade-columns", auth)
// 	{
// 		gradeColumns.Post("/", gradeHandler.CreateGradeColumn)
// 		gradeColumns.Get("/", gradeHandler.GetGradeColumns)
// 		gradeColumns.Put("/:id", gradeHandler.UpdateGradeColumn)
// 		gradeColumns.Delete("/:id", gradeHandler.DeleteGradeColumn)
// 		gradeColumns.Put("/reorder", gradeHandler.ReorderGradeColumns)
// 	}

// 	// ============================================================================
// 	// GRADES - Nhap va quan ly diem
// 	// ============================================================================
// 	// POST   /api/classes/:classId/grades              - Nhap diem
// 	// GET    /api/classes/:classId/grades              - Lay bang diem class
// 	// PUT    /api/grades/:id                           - Cap nhat diem
// 	// DELETE /api/grades/:id                           - Xoa diem
// 	// GET    /api/classes/:classId/grades/student/:studentId - Diem cua 1 student
// 	// POST   /api/classes/:classId/grades/bulk         - Nhap diem hang loat
// 	// POST   /api/classes/:classId/grades/import       - Import diem tu file

// 	classGrades := api.Group("/classes/:classId/grades", auth)
// 	{
// 		classGrades.Post("/", gradeHandler.CreateGrade)
// 		classGrades.Get("/", gradeHandler.GetGradesByClass)
// 		classGrades.Get("/student/:studentId", gradeHandler.GetStudentGrades)
// 		classGrades.Post("/bulk", gradeHandler.BulkCreateGrades)
// 		classGrades.Post("/import", gradeHandler.ImportGrades)
// 	}

// 	grades := api.Group("/grades", auth)
// 	{
// 		grades.Put("/:id", gradeHandler.UpdateGrade)
// 		grades.Delete("/:id", gradeHandler.DeleteGrade)
// 	}

// 	// ============================================================================
// 	// FINAL GRADES - Diem tong ket
// 	// ============================================================================
// 	// POST   /api/classes/:classId/final-grades/calculate  - Tinh diem tong ket
// 	// GET    /api/classes/:classId/final-grades            - Lay diem tong ket class
// 	// PUT    /api/classes/:classId/final-grades/:id        - Cap nhat diem tong ket
// 	// POST   /api/classes/:classId/final-grades/finalize   - Chot diem
// 	// POST   /api/classes/:classId/final-grades/export     - Xuat bang diem

// 	finalGrades := api.Group("/classes/:classId/final-grades", auth)
// 	{
// 		finalGrades.Post("/calculate", gradeHandler.CalculateFinalGrades)
// 		finalGrades.Get("/", gradeHandler.GetFinalGrades)
// 		finalGrades.Put("/:id", gradeHandler.UpdateFinalGrade)
// 		finalGrades.Post("/finalize", gradeHandler.FinalizeFinalGrades)
// 		finalGrades.Post("/export", gradeHandler.ExportGrades)
// 	}

// 	// ============================================================================
// 	// MY GRADES - Diem cua toi
// 	// ============================================================================
// 	// GET    /api/me/grades                            - Lay tat ca diem cua toi
// 	// GET    /api/me/grades/class/:classId             - Diem cua toi trong 1 lop

// 	api.Get("/me/grades", auth, gradeHandler.GetMyGrades)
// 	api.Get("/me/grades/class/:classId", auth, gradeHandler.GetMyGradesByClass)
// }
