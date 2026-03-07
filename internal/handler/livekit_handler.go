package handler

type LivekitHandlerInterface interface {
	CreateToken(c *fiber.Ctx) error
}

type LivekitHandler struct {
	liveService LivekitServiceInterface
}

func NewLivekitHandler(liveService LivekitServiceInterface) *LivekitHandler {
	return &LivekitHandler{liveService: liveService}
}

func (h *LivekitHandler) CreateToken(c *fiber.Ctx) error {
	var req dto.CreateLiveTokenDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	token, err := h.liveService.CreateToken(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to create token",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Token created successfully",
		"data":    token,
	})
}
