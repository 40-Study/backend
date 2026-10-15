package handler

import (
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// codeEditorErrorSpec: một dòng của bảng lỗi contract sec. 1.
type codeEditorErrorSpec struct {
	sentinel error
	status   int
	code     string
	message  string
}

// codeEditorErrors: SSOT ánh xạ sentinel -> HTTP + code + câu tiếng Việt (contract sec. 1). Thứ tự không quan trọng
// vì mỗi lỗi chỉ bọc một sentinel.
var codeEditorErrors = []codeEditorErrorSpec{
	{service.ErrUnsupportedLanguage, fiber.StatusBadRequest, "UNSUPPORTED_LANGUAGE", "Ngôn ngữ lập trình này không được hỗ trợ."},
	{service.ErrNoTestCases, fiber.StatusBadRequest, "NO_TEST_CASES", "Bài tập chưa có test case nào để chấm."},
	{service.ErrInvalidScope, fiber.StatusBadRequest, "INVALID_SCOPE", "Đường dẫn bản nháp không hợp lệ."},
	{service.ErrDraftLimitReached, fiber.StatusConflict, "DRAFT_LIMIT_REACHED", "Bạn đã có quá nhiều bản nháp, hãy xoá bớt bản nháp cũ rồi thử lại."},
	{service.ErrCodeTooLarge, fiber.StatusRequestEntityTooLarge, "CODE_TOO_LARGE", "Mã nguồn vượt quá giới hạn 64 KB."},
	{service.ErrJudgeUnavailable, fiber.StatusServiceUnavailable, "JUDGE_UNAVAILABLE", "Máy chấm tạm thời không khả dụng, vui lòng thử lại sau."},
}

const (
	codeEditorInternalCode    = "INTERNAL_ERROR"
	codeEditorInternalMessage = "Đã có lỗi xảy ra, vui lòng thử lại sau."
)

// respondCodeEditorError trả lỗi của code editor theo contract sec. 1: {code, error, message}. Body không bao giờ
// chứa err.Error() (có thể mang URL/host/cổng Judge0 hay lỗi DB); chi tiết chỉ đi vào log. Lỗi lạ (kể cả Judge0 422)
// -> 500 INTERNAL_ERROR. BE-A và BE-C gọi hàm này cho mọi endpoint trong phạm vi; không lane nào sửa nó.
func respondCodeEditorError(c *fiber.Ctx, err error) error {
	for _, spec := range codeEditorErrors {
		if errors.Is(err, spec.sentinel) {
			if spec.status >= fiber.StatusInternalServerError {
				log.Printf("[code-editor] %s %s: %s: %v", c.Method(), c.Path(), spec.code, err)
			}
			return c.Status(spec.status).JSON(newCodeEditorErrorBody(spec.code, spec.message))
		}
	}
	log.Printf("[code-editor] %s %s: %s: %v", c.Method(), c.Path(), codeEditorInternalCode, err)
	return c.Status(fiber.StatusInternalServerError).JSON(newCodeEditorErrorBody(codeEditorInternalCode, codeEditorInternalMessage))
}

func newCodeEditorErrorBody(code, message string) dto.APIErrorDTO {
	return dto.APIErrorDTO{Code: code, Error: strings.ToLower(code), Message: message}
}
