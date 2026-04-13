package router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupQuizRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	quizHandler *handler.QuizHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	// ============================================================================
// 	// QUIZ CRUD
// 	// ============================================================================
// 	// POST   /api/quizzes                              - Tao quiz
// 	// GET    /api/quizzes                              - Lay danh sach quizzes
// 	// GET    /api/quizzes/:id                          - Chi tiet quiz
// 	// PUT    /api/quizzes/:id                          - Cap nhat quiz
// 	// DELETE /api/quizzes/:id                          - Xoa quiz
// 	// POST   /api/quizzes/:id/duplicate                - Nhan ban quiz

// 	quizzes := api.Group("/quizzes", auth)
// 	{
// 		quizzes.Post("/", quizHandler.CreateQuiz)
// 		quizzes.Get("/", quizHandler.GetAllQuizzes)
// 		quizzes.Get("/:id", quizHandler.GetQuizByID)
// 		quizzes.Put("/:id", quizHandler.UpdateQuiz)
// 		quizzes.Delete("/:id", quizHandler.DeleteQuiz)
// 		quizzes.Post("/:id/duplicate", quizHandler.DuplicateQuiz)
// 	}

// 	// ============================================================================
// 	// QUESTIONS
// 	// ============================================================================
// 	// POST   /api/quizzes/:quizId/questions            - Them cau hoi
// 	// GET    /api/quizzes/:quizId/questions            - Lay cau hoi cua quiz
// 	// PUT    /api/quizzes/:quizId/questions/:id        - Cap nhat cau hoi
// 	// DELETE /api/quizzes/:quizId/questions/:id        - Xoa cau hoi
// 	// PUT    /api/quizzes/:quizId/questions/reorder    - Sap xep lai cau hoi
// 	// POST   /api/quizzes/:quizId/questions/bulk       - Them nhieu cau hoi

// 	questions := api.Group("/quizzes/:quizId/questions", auth)
// 	{
// 		questions.Post("/", quizHandler.CreateQuestion)
// 		questions.Get("/", quizHandler.GetQuestionsByQuiz)
// 		questions.Put("/:id", quizHandler.UpdateQuestion)
// 		questions.Delete("/:id", quizHandler.DeleteQuestion)
// 		questions.Put("/reorder", quizHandler.ReorderQuestions)
// 		questions.Post("/bulk", quizHandler.BulkCreateQuestions)
// 	}

// 	// ============================================================================
// 	// QUIZ ATTEMPTS - Lam quiz
// 	// ============================================================================
// 	// POST   /api/quizzes/:id/start                    - Bat dau lam quiz
// 	// POST   /api/quizzes/:id/submit                   - Nop quiz
// 	// GET    /api/quizzes/:id/attempts                 - Lich su lam quiz (cua user hien tai)
// 	// GET    /api/quizzes/:id/attempts/:attemptId      - Chi tiet 1 lan lam
// 	// GET    /api/quizzes/:id/results                  - Ket qua quiz (cho teacher)
// 	// GET    /api/quizzes/:id/statistics               - Thong ke quiz

// 	api.Post("/quizzes/:id/start", auth, quizHandler.StartQuiz)
// 	api.Post("/quizzes/:id/submit", auth, quizHandler.SubmitQuiz)
// 	api.Get("/quizzes/:id/attempts", auth, quizHandler.GetMyAttempts)
// 	api.Get("/quizzes/:id/attempts/:attemptId", auth, quizHandler.GetAttemptByID)
// 	api.Get("/quizzes/:id/results", auth, quizHandler.GetQuizResults)
// 	api.Get("/quizzes/:id/statistics", auth, quizHandler.GetQuizStatistics)

// 	// ============================================================================
// 	// QUIZ IN PROGRESS - Luu trang thai lam bai
// 	// ============================================================================
// 	// POST   /api/attempts/:attemptId/save-answer      - Luu cau tra loi (auto-save)
// 	// GET    /api/attempts/:attemptId/progress         - Lay trang thai hien tai

// 	api.Post("/attempts/:attemptId/save-answer", auth, quizHandler.SaveAnswer)
// 	api.Get("/attempts/:attemptId/progress", auth, quizHandler.GetAttemptProgress)

// 	// ============================================================================
// 	// MY QUIZZES - Quiz cua toi
// 	// ============================================================================
// 	// GET    /api/me/quizzes                           - Lay quizzes do toi tao
// 	// GET    /api/me/quiz-history                      - Lich su lam quiz

// 	api.Get("/me/quizzes", auth, quizHandler.GetMyCreatedQuizzes)
// 	api.Get("/me/quiz-history", auth, quizHandler.GetMyQuizHistory)
// }
