package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type LivestreamHandlerInterface interface {
	Create(c *fiber.Ctx) error
	GetByID(c *fiber.Ctx) error
	GetAll(c *fiber.Ctx) error
	Update(c *fiber.Ctx) error
	Delete(c *fiber.Ctx) error
	Start(c *fiber.Ctx) error
	End(c *fiber.Ctx) error
	Join(c *fiber.Ctx) error
	Leave(c *fiber.Ctx) error
	GetParticipants(c *fiber.Ctx) error
	MuteParticipant(c *fiber.Ctx) error
	KickParticipant(c *fiber.Ctx) error
	LockWhiteboard(c *fiber.Ctx) error
	UnlockWhiteboard(c *fiber.Ctx) error
	StartScreenShare(c *fiber.Ctx) error
	StopScreenShare(c *fiber.Ctx) error
}

type LivestreamHandler struct {
	svc service.LivestreamServiceInterface
	// permChecker (D2, issue #58 review vong 2): xac dinh nguoi goi co phai SYSTEM_ADMIN khong,
	// de admin quan tri duoc phien live (End/Kick/Mute/khoa bang) du khong phai host/GV lop —
	// cung pattern voi ClassLessonContentHandler. Co the nil (test khong truyen) —
	// isAdminActor fail-closed.
	permChecker *middleware.PermissionChecker
}

func NewLivestreamHandler(svc service.LivestreamServiceInterface, permChecker *middleware.PermissionChecker) *LivestreamHandler {
	return &LivestreamHandler{svc: svc, permChecker: permChecker}
}

// respondForbiddenOrError (V3-6/V3-7, issue #58) gom MOT cho duy nhat viec phan loai loi cua moi
// handler livestream: sentinel UY QUYEN -> 403, con lai -> 500. Truoc day moi handler tu viet
// `if err != nil { 500 }`, nen khi them kiem quyen ma quen nhanh phan loai thi nguoi dung nhan 500
// thay vi 403 (va test "user la -> 403" khong bao gio bat duoc loi hoi quy).
func respondForbiddenOrError(c *fiber.Ctx, err error) error {
	if service.IsForbiddenErr(err) {
		// D4 (issue #58 review vong 2): "message" mang MA LOI CO DINH (NOT_SESSION_MEMBER,
		// WHITEBOARD_LOCKED, KICKED, NOT_SESSION_HOST, ...) de web ghim vao thay vi chuoi "Forbidden"
		// khong phan biet duoc ly do — xem service.ForbiddenCode.
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": service.ForbiddenCode(err), "error": err.Error(),
		})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}

// callerOrUnauthorized lay danh tinh nguoi goi tu access token cho moi handler can biet AI dang
// goi. Tra ve (uuid.Nil, true) khi da ghi response 401 — caller chi can `return nil`.
func callerOrUnauthorized(c *fiber.Ctx) (uuid.UUID, bool) {
	userID, err := extractUserID(c)
	if err != nil {
		_ = c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
		return uuid.Nil, true
	}
	return userID, false
}

func (h *LivestreamHandler) Create(c *fiber.Ctx) error {
	// Finding review 260915 (tu PR web #16): host cua phien PHAI la nguoi dang goi API, lay tu
	// access token — truoc day handler nhan thang host_id tu body va dung nguyen, nen bat ky user
	// dang nhap nao cung tao duoc livestream mang ten mot user khac bang cach tu khai host_id.
	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	var req dto.CreateLivestreamDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed", "errors": errs,
		})
	}

	session, err := h.svc.Create(c.Context(), userID, req)
	if err != nil {
		// N1 (review vong 2, 260915): ErrNotClassTeacher la loi UY QUYEN (403), khong phai loi
		// ha tang — phan loai bang errors.Is (dung pattern sentinel LOW-7 da co cho select-org),
		// khong so chuoi.
		return respondForbiddenOrError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Livestream session created",
		"data":    session,
	})
}

func (h *LivestreamHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// F-1 (issue #58 review vong 2): truoc day handler nay chi co AuthMiddleware — bat ky user
	// dang nhap nao cung xem duoc chi tiet cua phien bat ky.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	detail, err := h.svc.GetByID(c.Context(), userID, isAdmin, id)
	if err != nil {
		return respondForbiddenOrError(c, err)
	}
	if detail == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})
	}

	return c.JSON(fiber.Map{"data": detail})
}

func (h *LivestreamHandler) GetAll(c *fiber.Ctx) error {
	// F-1 (issue #58 review vong 2): truoc day GetAll khong loc gi — bat ky user dang nhap nao
	// cung liet ke duoc TOAN BO phien cua he thong.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	status := c.Query("status")

	var hostID *uuid.UUID
	if hid := c.Query("host_id"); hid != "" {
		id, err := uuid.Parse(hid)
		if err == nil {
			hostID = &id
		}
	}

	// N10 (review vòng 2): filter theo lesson_content_id — bỏ qua giá trị không parse được thay
	// vì trả 400, giữ nguyên hành vi khoan dung sẵn có của host_id ở trên.
	var lessonContentID *uuid.UUID
	if lcid := c.Query("lesson_content_id"); lcid != "" {
		id, err := uuid.Parse(lcid)
		if err == nil {
			lessonContentID = &id
		}
	}

	result, err := h.svc.GetAll(c.Context(), userID, isAdmin, page, pageSize, status, hostID, lessonContentID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *LivestreamHandler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	var req dto.UpdateLivestreamDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	session, err := h.svc.Update(c.Context(), userID, isAdmin, id, req)
	if err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Session updated", "data": session})
}

func (h *LivestreamHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	if err := h.svc.Delete(c.Context(), userID, isAdmin, id); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Session deleted"})
}

func (h *LivestreamHandler) Start(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	session, err := h.svc.Start(c.Context(), userID, isAdmin, id)
	if err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Session started", "data": session})
}

func (h *LivestreamHandler) End(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	session, err := h.svc.End(c.Context(), userID, isAdmin, id)
	if err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Session ended", "data": session})
}

func (h *LivestreamHandler) Join(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// V3-6 (issue #58): danh tinh nguoi tham gia lay tu access token. `user_id` cu trong body bi
	// Fiber bo qua lang le (field da bi xoa khoi DTO) — khong doi contract response.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	var req dto.JoinLivestreamDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed", "errors": errs,
		})
	}

	participant, err := h.svc.Join(c.Context(), id, userID, isAdmin, req)
	if err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Joined session", "data": participant})
}

func (h *LivestreamHandler) Leave(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// V3-6 (issue #58): nguoi roi phong la nguoi dang goi API — truoc day lay `user_id` tu body
	// nen bat ky ai cung da duoc NGUOI KHAC ra khoi phong.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	if err := h.svc.Leave(c.Context(), sessionID, userID); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Left session"})
}

func (h *LivestreamHandler) GetParticipants(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// F-1 (issue #58 review vong 2): truoc day handler nay chi co AuthMiddleware — bat ky user
	// dang nhap nao cung doc duoc roster (user_id, role, joined_at) cua phien bat ky.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 50)

	participants, total, err := h.svc.GetParticipants(c.Context(), userID, isAdmin, id, page, pageSize)
	if err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{
		"data":      participants,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// moderationTarget (V3-6, issue #58) parse doi tuong bi tac dong tu body. `user_id` o day LA doi
// tuong (khong phai nguoi goi) — nguoi goi lay tu access token o callerOrUnauthorized.
func moderationTarget(c *fiber.Ctx) (uuid.UUID, *dto.ModerationActionDTO, error) {
	var req dto.ModerationActionDTO
	if err := c.BodyParser(&req); err != nil {
		return uuid.Nil, nil, err
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return uuid.Nil, nil, fiber.NewError(fiber.StatusBadRequest, "validation failed")
	}
	targetID, err := uuid.Parse(req.UserID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	return targetID, &req, nil
}

func (h *LivestreamHandler) MuteParticipant(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, actorID)

	targetID, _, err := moderationTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}

	if err := h.svc.MuteParticipant(c.Context(), actorID, isAdmin, id, targetID); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Participant muted"})
}

func (h *LivestreamHandler) KickParticipant(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, actorID)

	targetID, _, err := moderationTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}

	if err := h.svc.KickParticipant(c.Context(), actorID, isAdmin, id, targetID); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Participant kicked"})
}

func (h *LivestreamHandler) LockWhiteboard(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, actorID)

	if err := h.svc.LockWhiteboard(c.Context(), actorID, isAdmin, id, true); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Whiteboard locked"})
}

func (h *LivestreamHandler) UnlockWhiteboard(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, actorID)

	if err := h.svc.LockWhiteboard(c.Context(), actorID, isAdmin, id, false); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Whiteboard unlocked"})
}

// screenShareAction (V3-6, issue #58; D3 vong 2) parse body chia se man hinh. `user_id` o day LA
// DOI TUONG duoc host/GV DUYET chia se (rong = actor tu chia se, chinh minh) — khong phai danh
// tinh nguoi goi, actor luon lay tu access token o callerOrUnauthorized.
func screenShareAction(c *fiber.Ctx, actorID uuid.UUID) (uuid.UUID, error) {
	var req dto.ScreenShareDTO
	if err := c.BodyParser(&req); err != nil {
		return uuid.Nil, err
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "validation failed")
	}
	if req.UserID == "" {
		return actorID, nil
	}
	targetID, err := uuid.Parse(req.UserID)
	if err != nil {
		return uuid.Nil, err
	}
	return targetID, nil
}

func (h *LivestreamHandler) StartScreenShare(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, actorID)

	targetID, err := screenShareAction(c, actorID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.svc.StartScreenShare(c.Context(), actorID, isAdmin, id, targetID); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Screen share started"})
}

func (h *LivestreamHandler) StopScreenShare(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}
	isAdmin := isAdminActor(c, h.permChecker, actorID)

	targetID, err := screenShareAction(c, actorID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.svc.StopScreenShare(c.Context(), actorID, isAdmin, id, targetID); err != nil {
		return respondForbiddenOrError(c, err)
	}

	return c.JSON(fiber.Map{"message": "Screen share stopped"})
}
