package handler

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type LivekitHandlerInterface interface {
	CreateRoom(c *fiber.Ctx) error
	ListRooms(c *fiber.Ctx) error
	GetRoom(c *fiber.Ctx) error
	DeleteRoom(c *fiber.Ctx) error
	UpdateRoomMetadata(c *fiber.Ctx) error
	JoinToken(c *fiber.Ctx) error
	ListParticipants(c *fiber.Ctx) error
	GetParticipant(c *fiber.Ctx) error
	RemoveParticipant(c *fiber.Ctx) error
	UpdateParticipant(c *fiber.Ctx) error
	SendData(c *fiber.Ctx) error
}

type LivekitHandler struct {
	liveService service.LivekitServiceInterface
}

func NewLivekitHandler(liveService service.LivekitServiceInterface) *LivekitHandler {
	return &LivekitHandler{liveService: liveService}
}

// CreateRoom godoc
// POST /api/live/rooms
func (h *LivekitHandler) CreateRoom(c *fiber.Ctx) error {
	var req dto.CreateRoomDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	room, err := h.liveService.CreateRoom(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Room created",
		"data":    room,
	})
}

// ListRooms godoc
// GET /api/live/rooms
func (h *LivekitHandler) ListRooms(c *fiber.Ctx) error {
	rooms, err := h.liveService.ListRooms(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"data": rooms,
	})
}

// GetRoom godoc
// GET /api/live/rooms/:roomName
func (h *LivekitHandler) GetRoom(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	room, err := h.liveService.GetRoom(c.Context(), roomName)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": room})
}

// DeleteRoom godoc
// DELETE /api/live/rooms/:roomName
func (h *LivekitHandler) DeleteRoom(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	if err := h.liveService.DeleteRoom(c.Context(), roomName); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Room deleted"})
}

// UpdateRoomMetadata godoc
// PUT /api/live/rooms/:roomName/metadata
func (h *LivekitHandler) UpdateRoomMetadata(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	var req dto.UpdateRoomMetadataDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	room, err := h.liveService.UpdateRoomMetadata(c.Context(), roomName, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Metadata updated", "data": room})
}

// JoinToken godoc
// POST /api/live/rooms/:roomName/token
func (h *LivekitHandler) JoinToken(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	var req dto.JoinTokenDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	token, err := h.liveService.CreateJoinToken(c.Context(), roomName, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"message": "Token generated",
		"token":   token,
	})
}

// ListParticipants godoc
// GET /api/live/rooms/:roomName/participants
func (h *LivekitHandler) ListParticipants(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	participants, err := h.liveService.ListParticipants(c.Context(), roomName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": participants})
}

// GetParticipant godoc
// GET /api/live/rooms/:roomName/participants/:identity
func (h *LivekitHandler) GetParticipant(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	identity := c.Params("identity")
	p, err := h.liveService.GetParticipant(c.Context(), roomName, identity)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": p})
}

// RemoveParticipant godoc
// DELETE /api/live/rooms/:roomName/participants/:identity
func (h *LivekitHandler) RemoveParticipant(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	identity := c.Params("identity")
	if err := h.liveService.RemoveParticipant(c.Context(), roomName, identity); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Participant removed"})
}

// UpdateParticipant godoc
// PUT /api/live/rooms/:roomName/participants/:identity
func (h *LivekitHandler) UpdateParticipant(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	identity := c.Params("identity")
	var req dto.UpdateParticipantDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	p, err := h.liveService.UpdateParticipant(c.Context(), roomName, identity, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Participant updated", "data": p})
}

// SendData godoc
// POST /api/live/rooms/:roomName/data
func (h *LivekitHandler) SendData(c *fiber.Ctx) error {
	roomName := c.Params("roomName")
	var req dto.SendDataDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.liveService.SendData(c.Context(), roomName, req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Data sent"})
}
