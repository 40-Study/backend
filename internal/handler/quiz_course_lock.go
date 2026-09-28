package handler

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

// QuizCourseEditLocker — Q5 cho quiz (review PR #79): quiz/câu hỏi của khoá đang chờ duyệt không
// tạo/sửa/xoá được. Tách thành middleware ở router thay vì chèn vào từng handler/service ghi quiz
// để không đổi QuizServiceInterface/constructor (PR #80 đang sửa chính các hàm đó).
type QuizCourseEditLocker interface {
	EnsureQuizCourseEditable(ctx context.Context, quizID uuid.UUID) error
	EnsureNewQuizCourseEditable(ctx context.Context, userID uuid.UUID, isAdmin bool, lessonID, courseID *uuid.UUID) error
}

// Service thật luôn phải có guard — nếu method bị đổi tên thì vỡ lúc biên dịch, không âm thầm
// mất khoá.
var _ QuizCourseEditLocker = (*service.QuizService)(nil)

// quizLocker — nil khi service là fake trong test handler/router cũ (không cài guard).
func (h *QuizHandler) quizLocker() QuizCourseEditLocker {
	l, _ := h.service.(QuizCourseEditLocker)
	return l
}

func respondQuizLockCheck(c *fiber.Ctx, err error) error {
	if err == nil {
		return c.Next()
	}
	if writeCourseLocked(c, err) {
		return nil
	}
	switch err {
	case service.ErrQuizCourseMismatch:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	case service.ErrQuizCourseNotOwner:
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": err.Error()})
	case service.ErrCourseHidden:
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Course not found"})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"message": "Failed to check course status", "error": err.Error(),
	})
}

// CourseEditLock — middleware cho route ghi quiz/câu hỏi đã có; param là tên tham số route chứa
// quiz id ("id" hoặc "quizId"). Id sai định dạng: để handler phía sau trả 400 như cũ.
func (h *QuizHandler) CourseEditLock(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		locker := h.quizLocker()
		quizID, err := uuid.Parse(c.Params(param))
		if locker == nil || err != nil {
			return c.Next()
		}
		return respondQuizLockCheck(c, locker.EnsureQuizCourseEditable(c.Context(), quizID))
	}
}

// NewQuizCourseEditLock — middleware cho POST /quizzes: đọc course_id/lesson_id từ body, kiểm
// khoá nhất quán + người tạo là chủ khoá/admin + khoá không chờ duyệt (re-review vòng 2).
// Body/uuid sai: để CreateQuiz tự validate như cũ.
func (h *QuizHandler) NewQuizCourseEditLock() fiber.Handler {
	return func(c *fiber.Ctx) error {
		locker := h.quizLocker()
		if locker == nil {
			return c.Next()
		}
		userID, ok := c.Locals("user_id").(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
		}
		var body struct {
			LessonID string `json:"lesson_id"`
			CourseID string `json:"course_id"`
		}
		if json.Unmarshal(c.Body(), &body) != nil {
			return c.Next()
		}
		var lessonID, courseID *uuid.UUID
		if id, err := uuid.Parse(body.LessonID); err == nil {
			lessonID = &id
		}
		if id, err := uuid.Parse(body.CourseID); err == nil {
			courseID = &id
		}
		isAdmin := isAdminActor(c, h.permChecker, userID)
		return respondQuizLockCheck(c, locker.EnsureNewQuizCourseEditable(c.Context(), userID, isAdmin, lessonID, courseID))
	}
}
