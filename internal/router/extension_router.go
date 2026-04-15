package router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupExtensionRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	extensionHandler *handler.ExtensionHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	// ============================================================================
// 	// EXTENSION REQUESTS - Xin gia han deadline
// 	// ============================================================================
// 	// POST   /api/assignments/:assignmentId/extension-request        - Xin gia han
// 	// GET    /api/assignments/:assignmentId/extension-requests       - Lay cac yeu cau gia han
// 	// GET    /api/assignments/:assignmentId/extension-requests/:id   - Chi tiet yeu cau
// 	// POST   /api/extension-requests/:id/approve                     - Duyet gia han
// 	// POST   /api/extension-requests/:id/reject                      - Tu choi gia han

// 	api.Post("/assignments/:assignmentId/extension-request", auth, extensionHandler.CreateRequest)
// 	api.Get("/assignments/:assignmentId/extension-requests", auth, extensionHandler.GetRequestsByAssignment)
// 	api.Get("/assignments/:assignmentId/extension-requests/:id", auth, extensionHandler.GetRequestByID)

// 	extensionRequests := api.Group("/extension-requests", auth)
// 	{
// 		extensionRequests.Post("/:id/approve", extensionHandler.ApproveRequest)
// 		extensionRequests.Post("/:id/reject", extensionHandler.RejectRequest)
// 	}

// 	// ============================================================================
// 	// MY EXTENSION REQUESTS - Yeu cau gia han cua toi
// 	// ============================================================================
// 	// GET    /api/me/extension-requests                - Lay cac yeu cau cua toi

// 	api.Get("/me/extension-requests", auth, extensionHandler.GetMyRequests)

// 	// ============================================================================
// 	// BULK OPERATIONS
// 	// ============================================================================
// 	// POST   /api/assignments/bulk-extend              - Gia han hang loat

// 	api.Post("/assignments/bulk-extend", auth, extensionHandler.BulkExtend)
// }
