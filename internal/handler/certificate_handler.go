package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type CertificateHandler struct {
	service service.CertificateServiceInterface
}

func NewCertificateHandler(service service.CertificateServiceInterface) *CertificateHandler {
	return &CertificateHandler{service: service}
}

func (h *CertificateHandler) IssueCertificate(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var body struct {
		CourseID     string `json:"course_id"`
		EnrollmentID string `json:"enrollment_id"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	courseID, err := uuid.Parse(body.CourseID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	enrollmentID, err := uuid.Parse(body.EnrollmentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid enrollment ID",
			"error":   err.Error(),
		})
	}

	certificate, err := h.service.IssueCertificate(c.Context(), userID, courseID, enrollmentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to issue certificate",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Certificate issued successfully",
		"data":    certificate,
	})
}

func (h *CertificateHandler) GetMyCertificates(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	certificates, err := h.service.GetMyCertificates(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve certificates",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Certificates retrieved successfully",
		"data":    certificates,
	})
}

func (h *CertificateHandler) GetCertificateByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid certificate ID",
			"error":   err.Error(),
		})
	}

	certificate, err := h.service.GetCertificateByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Certificate not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Certificate retrieved successfully",
		"data":    certificate,
	})
}

func (h *CertificateHandler) VerifyCertificate(c *fiber.Ctx) error {
	number := c.Params("number")
	if number == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Certificate number is required",
		})
	}

	result, err := h.service.VerifyCertificate(c.Context(), number)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Certificate not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Certificate verified",
		"data":    result,
	})
}
