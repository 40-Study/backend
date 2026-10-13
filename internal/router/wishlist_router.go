package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupWishlistRoutes (F-10) - contract: plans/261013-qa2-role-findings/contract-be3.md §4.
// Đăng ký riêng ở app.go (như withdrawal/friends) để không thêm tham số vào SetupAllRoutes dùng chung.
func SetupWishlistRoutes(
	api fiber.Router,
	cfg *config.Config,
	wishlistHandler *handler.WishlistHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	wishlist := api.Group("/wishlist", auth)
	{
		wishlist.Get("/", wishlistHandler.GetWishlist)
		wishlist.Post("/", wishlistHandler.AddToWishlist)
		// Route tĩnh phải đứng trước "/:courseID".
		wishlist.Get("/ids", wishlistHandler.GetWishlistIDs)
		wishlist.Get("/check/:courseID", wishlistHandler.CheckCourseInWishlist)
		wishlist.Delete("/:courseID", wishlistHandler.RemoveFromWishlist)
	}
}
