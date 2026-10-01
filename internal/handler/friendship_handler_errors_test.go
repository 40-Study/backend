package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

// Bảng lỗi bạn bè của contract-api.md §1, chép NGUYÊN VĂN (HTTP, code). Test này khoá việc ánh xạ
// sentinel error -> (status, code): đổi một dòng trong friendErrors mà không sửa contract thì đỏ, và
// thiếu một code của contract cũng đỏ (kiểm cả chiều "không có sentinel nào ra code này").
var friendContractCodes = []struct {
	err    error
	status int
	code   string
}{
	{service.ErrFriendSelfRequest, 400, "FRIEND_SELF_REQUEST"},
	{service.ErrFriendRoleNotAllowed, 403, "FRIEND_ROLE_NOT_ALLOWED"},
	{service.ErrFriendRequestNotAllowed, 403, "FRIEND_REQUEST_NOT_ALLOWED"},
	{service.ErrFriendUserNotFound, 404, "FRIEND_USER_NOT_FOUND"},
	{service.ErrFriendRequestNotFound, 404, "FRIEND_REQUEST_NOT_FOUND"},
	{service.ErrFriendNotFound, 404, "FRIEND_NOT_FOUND"},
	{service.ErrFriendAlreadyFriends, 409, "FRIEND_ALREADY_FRIENDS"},
	{service.ErrFriendRequestExists, 409, "FRIEND_REQUEST_EXISTS"},
	{service.ErrFriendRequestNotPending, 409, "FRIEND_REQUEST_NOT_PENDING"},
	{service.ErrFriendRequestCooldown, 409, "FRIEND_REQUEST_COOLDOWN"},
	{service.ErrFriendLimitReached, 409, "FRIEND_LIMIT_REACHED"},
	{service.ErrFriendDailyLimit, 429, "FRIEND_DAILY_LIMIT_REACHED"},
	{service.ErrFriendPendingLimit, 429, "FRIEND_PENDING_LIMIT_REACHED"},
}

func TestFriendshipHandler_ErrorMapping_KhopContract(t *testing.T) {
	h := &FriendshipHandler{}
	app := fiber.New()
	var current error
	app.Get("/x", func(c *fiber.Ctx) error { return h.fail(c, current) })

	for _, tc := range friendContractCodes {
		current = fmt.Errorf("bọc thêm ngữ cảnh: %w", tc.err) // errors.Is phải xuyên qua wrap
		res, err := app.Test(httptest.NewRequest("GET", "/x", nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		want := fmt.Sprintf(`"code":"%s"`, tc.code)
		if res.StatusCode != tc.status || !strings.Contains(string(body), want) {
			t.Errorf("%v: muốn %d %s, nhận %d %s", tc.err, tc.status, tc.code, res.StatusCode, body)
		}
	}

	// Lỗi lạ: 500 chung, KHÔNG lộ nội dung lỗi nội bộ.
	current = errors.New("pq: relation friendships does not exist")
	res, _ := app.Test(httptest.NewRequest("GET", "/x", nil), -1)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 500 || strings.Contains(string(body), "relation") || !strings.Contains(string(body), "ERR_INTERNAL") {
		t.Errorf("lỗi hạ tầng phải là 500 ERR_INTERNAL không lộ chi tiết, nhận %d %s", res.StatusCode, body)
	}
}
