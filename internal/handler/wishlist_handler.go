package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// F-10 wishlist - contract: plans/261013-qa2-role-findings/contract-be3.md §4.
type WishlistHandler struct {
	service service.WishlistServiceInterface
}

func NewWishlistHandler(service service.WishlistServiceInterface) *WishlistHandler {
	return &WishlistHandler{service: service}
}

func wishlistUserID(c *fiber.Ctx) (uuid.UUID, bool) {
	id, ok := c.Locals("user_id").(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func wishlistUnauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
}

func wishlistInvalid(c *fiber.Ctx, message string, errs any) error {
	body := fiber.Map{"message": message, "code": "VALIDATION_FAILED"}
	if errs != nil {
		body["errors"] = errs
	}
	return c.Status(fiber.StatusBadRequest).JSON(body)
}

// AddToWishlist POST /wishlist - 201 lần đầu, 200 (cùng item) khi đã có.
func (h *WishlistHandler) AddToWishlist(c *fiber.Ctx) error {
	userID, ok := wishlistUserID(c)
	if !ok {
		return wishlistUnauthorized(c)
	}
	var req dto.AddToWishlistDTO
	if err := c.BodyParser(&req); err != nil {
		return wishlistInvalid(c, "Invalid request body", nil)
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return wishlistInvalid(c, "Validation failed", errs)
	}
	item, created, err := h.service.Add(c.Context(), userID, req.CourseID)
	if err != nil {
		return RespondServiceError(c, err, "Failed to add to wishlist")
	}
	status, message := fiber.StatusOK, "Course already in wishlist"
	if created {
		status, message = fiber.StatusCreated, "Course added to wishlist successfully"
	}
	return c.Status(status).JSON(fiber.Map{"message": message, "data": item})
}

// GetWishlist GET /wishlist
func (h *WishlistHandler) GetWishlist(c *fiber.Ctx) error {
	userID, ok := wishlistUserID(c)
	if !ok {
		return wishlistUnauthorized(c)
	}
	list, err := h.service.List(c.Context(), userID)
	if err != nil {
		return RespondServiceError(c, err, "Failed to get wishlist")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Wishlist retrieved successfully", "data": list})
}

// GetWishlistIDs GET /wishlist/ids
func (h *WishlistHandler) GetWishlistIDs(c *fiber.Ctx) error {
	userID, ok := wishlistUserID(c)
	if !ok {
		return wishlistUnauthorized(c)
	}
	ids, err := h.service.CourseIDs(c.Context(), userID)
	if err != nil {
		return RespondServiceError(c, err, "Failed to get wishlist")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Wishlist retrieved successfully", "data": fiber.Map{"course_ids": ids}})
}

// RemoveFromWishlist DELETE /wishlist/:courseID
func (h *WishlistHandler) RemoveFromWishlist(c *fiber.Ctx) error {
	userID, ok := wishlistUserID(c)
	if !ok {
		return wishlistUnauthorized(c)
	}
	courseID, err := uuid.Parse(c.Params("courseID"))
	if err != nil {
		return wishlistInvalid(c, "Invalid course ID", nil)
	}
	if err := h.service.Remove(c.Context(), userID, courseID); err != nil {
		return RespondServiceError(c, err, "Failed to remove from wishlist")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Course removed from wishlist successfully"})
}

// CheckCourseInWishlist GET /wishlist/check/:courseID
func (h *WishlistHandler) CheckCourseInWishlist(c *fiber.Ctx) error {
	userID, ok := wishlistUserID(c)
	if !ok {
		return wishlistUnauthorized(c)
	}
	courseID, err := uuid.Parse(c.Params("courseID"))
	if err != nil {
		return wishlistInvalid(c, "Invalid course ID", nil)
	}
	in, err := h.service.Check(c.Context(), userID, courseID)
	if err != nil {
		return RespondServiceError(c, err, "Failed to check wishlist")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Check completed", "data": fiber.Map{"in_wishlist": in}})
}
